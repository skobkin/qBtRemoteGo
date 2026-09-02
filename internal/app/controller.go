package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/skobkin/qbtremotego/internal/config"
	"github.com/skobkin/qbtremotego/internal/credentials"
	"github.com/skobkin/qbtremotego/internal/platform"
	"github.com/skobkin/qbtremotego/internal/qbt"
)

type Controller struct {
	configPath      string
	credentialStore credentials.Store

	// stateMu guards the state shared between the UI thread (which mutates
	// settings) and the poll / details fetch goroutines (which read it): the
	// config, the logger, the platform manager, session credentials, the
	// credential status, and the cached client below.
	stateMu            sync.RWMutex
	config             config.AppConfig
	logger             *slog.Logger
	platform           *platform.Manager
	sessionCredentials credentials.Credentials
	credentialStatus   credentials.Status

	// Cached qBittorrent client: caching keeps the login session (cookie jar)
	// alive across requests instead of performing a fresh auth/login per call.
	// Guarded by stateMu like everything above; qbt.NewClient does no I/O, so
	// building a client under the lock is cheap.
	cachedClient       *qbt.Client
	cachedClientConfig qbt.ClientConfig

	// Background keychain retry state (see credential_retry.go).
	// credentialRetryActive is guarded by stateMu; the channels are closed at
	// most once, guarded by credentialRetryOnce.
	credentialRetryActive bool
	credentialRetryStop   chan struct{}
	credentialRetryDone   chan struct{}
	credentialRetryOnce   sync.Once
}

type AddDialogData struct {
	SourceType         qbt.SourceType
	TorrentFilePath    string
	MagnetText         string
	ManagementMode     string
	SavePath           string
	Rename             string
	Category           string
	Tags               string
	StartTorrent       bool
	TopOfQueue         bool
	StopCondition      string
	SkipHashCheck      bool
	ContentLayout      string
	SequentialDownload bool
	FirstLastPieces    bool
	DownloadLimitText  string
	UploadLimitText    string
}

type CredentialFallbackChoice string

const (
	CredentialFallbackUnspecified CredentialFallbackChoice = ""
	CredentialFallbackPlaintext   CredentialFallbackChoice = "plaintext"
	CredentialFallbackSessionOnly CredentialFallbackChoice = "session_only"
)

type SaveSettingsResult struct {
	CredentialStatus credentials.Status
	DecisionRequired bool
}

func NewController(configPath string, logger *slog.Logger) (*Controller, error) {
	return newController(configPath, logger, credentials.NewStore())
}

func newController(configPath string, logger *slog.Logger, store credentials.Store) (*Controller, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}

	controller := &Controller{
		configPath:          configPath,
		config:              cfg,
		logger:              logger,
		platform:            platform.NewManager(logger),
		credentialStore:     store,
		credentialRetryStop: make(chan struct{}),
		credentialRetryDone: make(chan struct{}),
	}
	if err := controller.loadSessionCredentials(context.Background()); err != nil {
		return nil, err
	}
	// Stored credentials that were not loaded yet (e.g. the wallet was still
	// unlocking at boot) are re-read in the background instead of failing the
	// whole session.
	if controller.keychainLoadPending() {
		controller.startCredentialRetryLoop()
	}

	return controller, nil
}

func (c *Controller) Config() config.AppConfig {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()

	return c.config
}

// currentLogger returns the active logger; the UI thread swaps it when log
// settings change while fetch goroutines keep using it.
func (c *Controller) currentLogger() *slog.Logger {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()

	return c.logger
}

// SetLogger swaps the active logger, rebuilds the platform manager with it,
// and drops the cached client (which captured the previous logger) as one
// locked step, so an in-flight client lookup cannot finish with the stale
// logger.
func (c *Controller) SetLogger(logger *slog.Logger) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()

	c.logger = logger
	c.platform = platform.NewManager(logger)
	c.cachedClient = nil
	c.cachedClientConfig = qbt.ClientConfig{}
}

func (c *Controller) SessionCredentials() credentials.Credentials {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()

	return c.sessionCredentials
}

func (c *Controller) CredentialStatus() credentials.Status {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()

	return c.credentialStatus
}

func (c *Controller) setSessionCredentials(creds credentials.Credentials) {
	c.stateMu.Lock()
	c.sessionCredentials = creds
	c.stateMu.Unlock()
}

func (c *Controller) setCredentialStatus(status credentials.Status) {
	c.stateMu.Lock()
	c.credentialStatus = status
	c.stateMu.Unlock()
}

func (c *Controller) SaveSettings(
	ctx context.Context,
	cfg config.AppConfig,
	creds credentials.Credentials,
	fallback CredentialFallbackChoice,
) (SaveSettingsResult, error) {
	config.Normalize(&cfg)

	// Keep only credentials for the active auth method; inactive ones must not
	// reach the keychain, the config file, or the session.
	creds = canonicalCredentials(cfg.Connection.AuthMethod, creds)

	// Edits are applied to the latest in-memory config (see persistConfig), not
	// to a snapshot taken when the settings window opened, so they can never
	// revert changes persisted in the meantime. Only the fields this form owns
	// are written; credential fields are managed by the branches below.
	applyEdits := func(dst *config.AppConfig) {
		dst.Connection.URL = cfg.Connection.URL
		dst.Connection.AuthMethod = cfg.Connection.AuthMethod
		dst.Connection.SkipCertificateCheck = cfg.Connection.SkipCertificateCheck
		dst.UI = cfg.UI
		dst.Integration = cfg.Integration
		dst.Logging = cfg.Logging
	}

	trimmedCreds := credentials.Credentials{
		Username: strings.TrimSpace(creds.Username),
		Password: creds.Password,
		APIKey:   strings.TrimSpace(creds.APIKey),
	}
	credsChanged := trimmedCreds != c.SessionCredentials()
	status := c.credentialStore.Status(ctx)
	c.setCredentialStatus(status)

	persistedMode := c.Config().Connection.CredentialStorage

	// An empty credentials form must never blank out a stored keychain payload:
	// while a stored load is pending the form cannot show the values, and a
	// blank write would destroy them (also once the retry has given up and the
	// wallet is reachable again). Treat such a save as a pure settings change;
	// clearing credentials is still possible by switching the storage mode
	// away from the keychain.
	if trimmedCreds == (credentials.Credentials{}) &&
		persistedMode == config.CredentialStorageKeychain &&
		c.Config().Connection.KeychainHasCredentials {
		_, err := c.persistConfig(true, applyEdits)
		if err != nil {
			return SaveSettingsResult{CredentialStatus: status}, err
		}
		c.setCredentialStatus(status)

		return SaveSettingsResult{CredentialStatus: status}, nil
	}

	if status.State == credentials.StateAvailable {
		if err := c.credentialStore.Set(ctx, trimmedCreds); err != nil {
			status = c.statusFromError(err)
			c.setCredentialStatus(status)
			if credsChanged && fallback == CredentialFallbackUnspecified {
				return SaveSettingsResult{
					CredentialStatus: status,
					DecisionRequired: true,
				}, nil
			}

			return c.saveWithFallback(applyEdits, trimmedCreds, persistedMode, fallback, status)
		}

		saved, err := c.persistConfig(true, func(dst *config.AppConfig) {
			applyEdits(dst)
			dst.Connection.CredentialStorage = config.CredentialStorageKeychain
			dst.Connection.KeychainHasCredentials = true
			dst.Connection.Username = ""
			dst.Connection.Password = ""
			dst.Connection.APIKey = ""
		})
		if err != nil && !saved {
			return SaveSettingsResult{CredentialStatus: status}, err
		}
		c.setSessionCredentials(trimmedCreds)
		c.setCredentialStatus(status)

		return SaveSettingsResult{CredentialStatus: status}, err
	}

	if !credsChanged {
		_, err := c.persistConfig(true, applyEdits)
		if err != nil {
			return SaveSettingsResult{CredentialStatus: status}, err
		}
		c.setCredentialStatus(status)

		return SaveSettingsResult{CredentialStatus: status}, nil
	}

	if fallback == CredentialFallbackUnspecified {
		return SaveSettingsResult{
			CredentialStatus: status,
			DecisionRequired: true,
		}, nil
	}

	return c.saveWithFallback(applyEdits, trimmedCreds, persistedMode, fallback, status)
}

// SaveLocalUI persists local UI preferences without touching connection
// settings or desktop integrations. Edits are applied to the latest in-memory
// config (see persistConfig).
func (c *Controller) SaveLocalUI(mutate func(cfg *config.AppConfig)) error {
	_, err := c.persistConfig(false, mutate)

	return err
}

func (c *Controller) SyncIntegrations() []error {
	c.stateMu.RLock()
	platformManager := c.platform
	integration := c.config.Integration
	c.stateMu.RUnlock()

	return platformManager.Sync(integration)
}

func (c *Controller) TestConnection(ctx context.Context, cfg config.ConnectionConfig, creds credentials.Credentials) error {
	qcfg, err := connectionClientConfig(cfg, creds)
	if err != nil {
		return err
	}
	client, err := qbt.NewClient(qcfg, c.currentLogger().With("remote", strings.TrimSpace(cfg.URL)))
	if err != nil {
		return err
	}

	return client.TestConnection(ctx)
}

func (c *Controller) FetchTorrents(ctx context.Context) ([]qbt.Torrent, error) {
	client, err := c.client()
	if err != nil {
		return nil, err
	}

	return client.Torrents(ctx)
}

func (c *Controller) FetchTransferInfo(ctx context.Context) (qbt.TransferInfo, error) {
	client, err := c.client()
	if err != nil {
		return qbt.TransferInfo{}, err
	}

	return client.TransferInfo(ctx)
}

func (c *Controller) FetchServerState(ctx context.Context) (qbt.ServerState, error) {
	client, err := c.client()
	if err != nil {
		return qbt.ServerState{}, err
	}

	return client.ServerState(ctx)
}

func (c *Controller) FetchServerVersion(ctx context.Context) (string, error) {
	client, err := c.client()
	if err != nil {
		return "", err
	}

	return client.Version(ctx)
}

func (c *Controller) FetchCategoriesAndTags(ctx context.Context) ([]string, []string, error) {
	client, err := c.client()
	if err != nil {
		return nil, nil, err
	}

	categories, catErr := client.Categories(ctx)
	tags, tagErr := client.Tags(ctx)
	if catErr != nil && tagErr != nil {
		return nil, nil, fmt.Errorf("load categories and tags: %w", errors.Join(catErr, tagErr))
	}

	return categories, tags, errors.Join(catErr, tagErr)
}

func (c *Controller) FetchDefaultSavePath(ctx context.Context) (string, error) {
	client, err := c.client()
	if err != nil {
		return "", err
	}

	return client.DefaultSavePath(ctx)
}

func (c *Controller) FetchTorrentProperties(ctx context.Context, hash string) (qbt.TorrentProperties, error) {
	client, err := c.client()
	if err != nil {
		return qbt.TorrentProperties{}, err
	}

	return client.TorrentProperties(ctx, hash)
}

func (c *Controller) FetchTorrentFiles(ctx context.Context, hash string) ([]qbt.TorrentFile, error) {
	client, err := c.client()
	if err != nil {
		return nil, err
	}

	return client.TorrentFiles(ctx, hash)
}

func (c *Controller) FetchTorrentTrackers(ctx context.Context, hash string) ([]qbt.TorrentTracker, error) {
	client, err := c.client()
	if err != nil {
		return nil, err
	}

	return client.TorrentTrackers(ctx, hash)
}

func (c *Controller) FetchTorrentWebSeeds(ctx context.Context, hash string) ([]qbt.TorrentWebSeed, error) {
	client, err := c.client()
	if err != nil {
		return nil, err
	}

	return client.TorrentWebSeeds(ctx, hash)
}

func (c *Controller) FetchTorrentPeers(ctx context.Context, hash string, rid int) (qbt.TorrentPeersSync, error) {
	client, err := c.client()
	if err != nil {
		return qbt.TorrentPeersSync{}, err
	}

	return client.TorrentPeers(ctx, hash, rid)
}

func (c *Controller) AddTorrent(ctx context.Context, data AddDialogData) error {
	req, err := ValidateAddDialogData(data)
	if err != nil {
		return err
	}

	client, err := c.client()
	if err != nil {
		return err
	}
	if err := client.AddTorrent(ctx, req); err != nil {
		return err
	}

	c.rememberSavePath(req.SavePath, "add")

	return nil
}

// rememberSavePath runs inside bulk-action goroutines, so it must go through
// the locked accessors instead of touching c.config directly. The unchanged
// pre-check works on a cheap Config() snapshot; the real read-modify-write
// happens under persistConfig's lock, so a settings save in flight cannot be
// reverted by a stale recent-paths list.
func (c *Controller) rememberSavePath(path string, trigger string) {
	probe := c.Config()
	config.AddRecentPath(&probe, path)
	if slices.Equal(probe.UI.RecentSavePaths, c.Config().UI.RecentSavePaths) {
		return
	}
	if _, err := c.persistConfig(false, func(cfg *config.AppConfig) {
		config.AddRecentPath(cfg, path)
	}); err != nil {
		c.currentLogger().Warn("save config after recent path update", "trigger", trigger, "error", err)
	}
}

func (c *Controller) StartTorrents(ctx context.Context, hashes []string) error {
	client, err := c.client()
	if err != nil {
		return err
	}

	return client.Start(ctx, hashes)
}

func (c *Controller) StopTorrents(ctx context.Context, hashes []string) error {
	client, err := c.client()
	if err != nil {
		return err
	}

	return client.Stop(ctx, hashes)
}

func (c *Controller) ForceRecheckTorrents(ctx context.Context, hashes []string) error {
	client, err := c.client()
	if err != nil {
		return err
	}

	return client.ForceRecheck(ctx, hashes)
}

func (c *Controller) ForceReannounceTorrents(ctx context.Context, hashes []string) error {
	client, err := c.client()
	if err != nil {
		return err
	}

	return client.ForceReannounce(ctx, hashes)
}

func (c *Controller) SetTorrentLocation(ctx context.Context, hashes []string, location string) error {
	location = strings.TrimSpace(location)
	if location == "" {
		return errors.New("save location is required")
	}

	client, err := c.client()
	if err != nil {
		return err
	}
	if err := client.SetLocation(ctx, hashes, location); err != nil {
		return err
	}

	c.rememberSavePath(location, "set_location")

	return nil
}

func (c *Controller) RenameTorrent(ctx context.Context, hash string, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("torrent name is required")
	}

	client, err := c.client()
	if err != nil {
		return err
	}

	return client.Rename(ctx, hash, name)
}

func (c *Controller) DeleteTorrents(ctx context.Context, hashes []string, deleteFiles bool) error {
	client, err := c.client()
	if err != nil {
		return err
	}

	return client.Delete(ctx, hashes, deleteFiles)
}

func (c *Controller) SuggestDirectories(ctx context.Context, path string) ([]string, error) {
	if !c.Config().UI.PathAutocomplete {
		return nil, nil
	}
	client, err := c.client()
	if err != nil {
		return nil, err
	}

	return client.DirectorySuggestions(ctx, path)
}

func FilterAndSortTorrents(torrents []qbt.Torrent, query string, filterBy string, sortColumn string, descending bool) []qbt.Torrent {
	filtered := make([]qbt.Torrent, 0, len(torrents))
	query = strings.ToLower(strings.TrimSpace(query))
	for _, torrent := range torrents {
		if query != "" {
			candidate := torrent.Name
			if filterBy == "location" {
				candidate = torrent.SavePath
			}
			if !strings.Contains(strings.ToLower(candidate), query) {
				continue
			}
		}
		filtered = append(filtered, torrent)
	}

	slices.SortStableFunc(filtered, func(a qbt.Torrent, b qbt.Torrent) int {
		less := compareTorrent(a, b, sortColumn)
		if descending {
			return -less
		}

		return less
	})

	return filtered
}

func compareTorrent(a qbt.Torrent, b qbt.Torrent, sortColumn string) int {
	switch sortColumn {
	case "name":
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	case "size":
		return cmpInt64(a.Size, b.Size)
	case "progress":
		return cmpFloat(a.Progress, b.Progress)
	case "status":
		return strings.Compare(strings.ToLower(StatusLabel(a.State)), strings.ToLower(StatusLabel(b.State)))
	case "down":
		return cmpInt64(a.DLSpeed, b.DLSpeed)
	case "up":
		return cmpInt64(a.UPSpeed, b.UPSpeed)
	case "eta":
		return cmpInt64(a.ETASeconds, b.ETASeconds)
	case "added":
		return cmpInt64(a.AddedUnix, b.AddedUnix)
	default:
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	}
}

func ValidateAddDialogData(data AddDialogData) (qbt.AddRequest, error) {
	req := qbt.AddRequest{
		SourceType:          data.SourceType,
		TorrentFilePath:     strings.TrimSpace(data.TorrentFilePath),
		ManagementMode:      firstNonEmpty(strings.ToLower(strings.TrimSpace(data.ManagementMode)), "manual"),
		SavePath:            strings.TrimSpace(data.SavePath),
		Rename:              strings.TrimSpace(data.Rename),
		Category:            strings.TrimSpace(data.Category),
		Tags:                splitCSV(data.Tags),
		StartTorrent:        data.StartTorrent,
		TopOfQueue:          data.TopOfQueue,
		StopCondition:       normalizeStopCondition(data.StopCondition),
		SkipHashCheck:       data.SkipHashCheck,
		ContentLayout:       normalizeContentLayout(data.ContentLayout),
		SequentialDownload:  data.SequentialDownload,
		FirstLastPieceFirst: data.FirstLastPieces,
	}

	switch req.SourceType {
	case qbt.SourceTorrentFile:
		if req.TorrentFilePath == "" {
			return qbt.AddRequest{}, errors.New("torrent file path is required")
		}
	case qbt.SourceMagnet:
		req.MagnetLinks = splitLines(data.MagnetText)
		if len(req.MagnetLinks) == 0 {
			return qbt.AddRequest{}, errors.New("at least one magnet link is required")
		}
	default:
		return qbt.AddRequest{}, errors.New("source type is required")
	}

	if data.DownloadLimitText != "" {
		value, err := parseRateLimit(data.DownloadLimitText)
		if err != nil {
			return qbt.AddRequest{}, fmt.Errorf("download rate: %w", err)
		}
		req.DownloadLimitKiB = &value
	}
	if data.UploadLimitText != "" {
		value, err := parseRateLimit(data.UploadLimitText)
		if err != nil {
			return qbt.AddRequest{}, fmt.Errorf("upload rate: %w", err)
		}
		req.UploadLimitKiB = &value
	}

	return req, nil
}

func parseRateLimit(raw string) (int, error) {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, errors.New("must be a whole number")
	}
	if value < 0 {
		return 0, errors.New("must be zero or greater")
	}
	return value, nil
}

func StatusLabel(state string) string {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "forceddl", "downloading":
		return "Downloading"
	case "forcedmetaDL", "metadl", "downloadingmetadata":
		return "Metadata"
	case "stalleddl":
		return "Stalled"
	case "forcedup", "uploading":
		return "Seeding"
	case "stalledup":
		return "Completed"
	case "queueddl", "queuedup":
		return "Queued"
	case "checkingdl", "checkingup", "checkingresume":
		return "Checking"
	case "pauseddl", "pausedup", "stoppeddl", "stoppedup":
		return "Paused"
	case "moving":
		return "Moving"
	case "missingfiles":
		return "Missing files"
	case "error":
		return "Error"
	default:
		return "Unknown"
	}
}

func HumanBytes(value int64) string {
	const unit = 1024
	if value < unit {
		return fmt.Sprintf("%d B", value)
	}
	div, exp := int64(unit), 0
	for n := value / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(value)/float64(div), "KMGTPE"[exp])
}

func HumanSpeed(value int64) string {
	return HumanBytes(value) + "/s"
}

func HumanSpeedLimit(value int64) string {
	if value <= 0 {
		return "∞"
	}
	return HumanSpeed(value)
}

func ConnectionStatusLabel(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "connected":
		return "Connected"
	case "firewalled":
		return "Firewalled"
	case "disconnected":
		return "Disconnected"
	default:
		return "Unknown"
	}
}

func HumanETA(seconds int64) string {
	const infiniteETASeconds = 100 * 24 * 60 * 60

	switch {
	case seconds < 0:
		return "Unknown"
	case seconds >= infiniteETASeconds:
		return "∞"
	case seconds == 0:
		return "Done"
	default:
		return humanCountdown(seconds)
	}
}

// HumanDuration formats a plain elapsed duration in seconds; negative values
// mean unlimited (e.g. seeding without a time limit).
func HumanDuration(seconds int64) string {
	if seconds < 0 {
		return "∞"
	}
	return humanCountdown(seconds)
}

// humanCountdown renders a non-negative second count as a compact duration,
// rounding down to the coarsest two units.
func humanCountdown(seconds int64) string {
	switch {
	case seconds < 60:
		return fmt.Sprintf("%ds", seconds)
	case seconds < 3600:
		return fmt.Sprintf("%dm", seconds/60)
	case seconds < 86400:
		return fmt.Sprintf("%dh %dm", seconds/3600, (seconds%3600)/60)
	default:
		return fmt.Sprintf("%dd %dh", seconds/86400, (seconds%86400)/3600)
	}
}

func HumanAdded(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return humanElapsed(time.Now(), t)
}

func humanElapsed(now time.Time, then time.Time) string {
	if then.IsZero() {
		return ""
	}
	if now.Before(then) {
		now = then
	}

	totalMinutes := int64(now.Sub(then).Minutes())
	if totalMinutes <= 0 {
		return "now"
	}

	const (
		minutesPerHour  = int64(60)
		minutesPerDay   = 24 * minutesPerHour
		minutesPerMonth = 30 * minutesPerDay
		minutesPerYear  = 365 * minutesPerDay
	)

	switch {
	case totalMinutes >= minutesPerYear:
		years := totalMinutes / minutesPerYear
		months := (totalMinutes % minutesPerYear) / minutesPerMonth
		if months > 0 {
			return fmt.Sprintf("%dy%dmo", years, months)
		}
		return fmt.Sprintf("%dy", years)
	case totalMinutes >= minutesPerMonth:
		months := totalMinutes / minutesPerMonth
		days := (totalMinutes % minutesPerMonth) / minutesPerDay
		if days > 0 {
			return fmt.Sprintf("%dmo%dd", months, days)
		}
		return fmt.Sprintf("%dmo", months)
	case totalMinutes >= minutesPerDay:
		days := totalMinutes / minutesPerDay
		hours := (totalMinutes % minutesPerDay) / minutesPerHour
		if hours > 0 {
			return fmt.Sprintf("%dd%dh", days, hours)
		}
		return fmt.Sprintf("%dd", days)
	case totalMinutes >= minutesPerHour:
		hours := totalMinutes / minutesPerHour
		minutes := totalMinutes % minutesPerHour
		if minutes > 0 {
			return fmt.Sprintf("%dh%dm", hours, minutes)
		}
		return fmt.Sprintf("%dh", hours)
	default:
		return fmt.Sprintf("%dm", max(totalMinutes, 1))
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, item := range parts {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func splitLines(raw string) []string {
	lines := strings.Split(raw, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func normalizeStopCondition(raw string) string {
	switch strings.TrimSpace(raw) {
	case "", "None":
		return "None"
	case "Metadata received":
		return "MetadataReceived"
	case "Files checked":
		return "FilesChecked"
	default:
		return raw
	}
}

func normalizeContentLayout(raw string) string {
	switch strings.TrimSpace(raw) {
	case "", "Original":
		return "Original"
	case "Create subfolder":
		return "Subfolder"
	case "Don't create subfolder":
		return "NoSubfolder"
	default:
		return raw
	}
}

func cmpInt64(a int64, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func cmpFloat(a float64, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// persistConfig applies mutate to the latest in-memory config and writes the
// result to disk and back into memory as one locked read-modify-write, so
// concurrent savers (settings saves, recent-path updates on bulk-action
// goroutines, keychain migration) cannot revert each other's changes. mutate
// must replace — never edit in place — any slice or map it touches (see
// config.AddRecentPath). The returned bool reports whether the config was
// persisted even when err is non-nil (integration sync warnings).
func (c *Controller) persistConfig(syncIntegrations bool, mutate func(cfg *config.AppConfig)) (bool, error) {
	c.stateMu.Lock()
	cfg := c.config
	if mutate != nil {
		mutate(&cfg)
	}
	if err := config.Save(c.configPath, cfg); err != nil {
		c.stateMu.Unlock()

		return false, err
	}
	c.config = cfg
	c.stateMu.Unlock()

	if !syncIntegrations {
		return true, nil
	}
	c.stateMu.RLock()
	platformManager := c.platform
	c.stateMu.RUnlock()
	if platformManager == nil {
		return true, nil
	}
	if errs := platformManager.Sync(cfg.Integration); len(errs) > 0 {
		return true, errors.New(platform.JoinErrors(errs))
	}

	return true, nil
}

// loadSessionCredentials runs once at construction, before any other
// goroutine touches the controller, so its direct field writes are safe. Every
// keychain call is bounded: an unreachable or slow wallet no longer blocks
// startup indefinitely — the controller starts with empty credentials and the
// retry loop takes over (see credential_retry.go).
func (c *Controller) loadSessionCredentials(ctx context.Context) error {
	method := c.config.Connection.AuthMethod
	switch c.config.Connection.CredentialStorage {
	case config.CredentialStorageKeychain:
		creds, err := callWithTimeout(startupCredentialTimeout, func() (credentials.Credentials, error) {
			return c.credentialStore.Get(ctx)
		})
		if err != nil {
			status := keychainLoadStatus(c.config.Connection.KeychainHasCredentials, err)
			c.credentialStatus = status
			c.sessionCredentials = credentials.Credentials{}
			c.logger.Warn("system keychain credentials unavailable at startup", "backend", status.Backend, "state", status.State, "error", err)

			return nil
		}
		method = c.reconcileAuthMethod(method, creds)
		c.sessionCredentials = canonicalCredentials(method, creds)
		c.credentialStatus = credentials.Status{
			Backend: credentials.BackendName(),
			State:   credentials.StateAvailable,
			Message: "System keychain is available.",
		}

		return nil
	case config.CredentialStoragePlaintext:
		creds := credentials.Credentials{
			Username: c.config.Connection.Username,
			Password: c.config.Connection.Password,
			APIKey:   c.config.Connection.APIKey,
		}
		method = c.reconcileAuthMethod(method, creds)
		c.sessionCredentials = canonicalCredentials(method, creds)
		c.credentialStatus = credentials.Status{
			Backend: credentials.BackendName(),
			State:   credentials.StateAvailable,
			Message: "Credentials are stored in the local config file.",
		}

		return nil
	case config.CredentialStorageNone:
		c.sessionCredentials = credentials.Credentials{}
		c.credentialStatus = credentials.Status{
			Backend: credentials.BackendName(),
			State:   credentials.StateAvailable,
			Message: "Credential storage is disabled.",
		}

		return nil
	default:
		if c.config.Connection.Username == "" && c.config.Connection.Password == "" && c.config.Connection.APIKey == "" {
			c.sessionCredentials = credentials.Credentials{}
			c.credentialStatus = credentials.Status{
				Backend: credentials.BackendName(),
				State:   credentials.StateAvailable,
				Message: "No credentials are stored yet.",
			}

			return nil
		}

		legacy := credentials.Credentials{
			Username: c.config.Connection.Username,
			Password: c.config.Connection.Password,
			APIKey:   c.config.Connection.APIKey,
		}
		method = c.reconcileAuthMethod(method, legacy)
		legacy = canonicalCredentials(method, legacy)
		c.sessionCredentials = legacy

		status := boundedKeychainStatus(startupCredentialTimeout, c.credentialStore)
		c.credentialStatus = status
		if status.State != credentials.StateAvailable {
			c.logger.Warn("legacy plaintext credentials remain because system keychain is unavailable", "backend", status.Backend, "state", status.State)

			return nil
		}
		if _, err := callWithTimeout(startupCredentialTimeout, func() (struct{}, error) {
			return struct{}{}, c.credentialStore.Set(ctx, legacy)
		}); err != nil {
			c.credentialStatus = c.statusFromError(err)
			c.logger.Warn("migrate plaintext credentials to system keychain", "error", err, "backend", c.credentialStatus.Backend, "state", c.credentialStatus.State)

			return nil
		}

		if _, err := c.persistConfig(false, func(cfg *config.AppConfig) {
			cfg.Connection.CredentialStorage = config.CredentialStorageKeychain
			cfg.Connection.KeychainHasCredentials = true
			cfg.Connection.Username = ""
			cfg.Connection.Password = ""
			cfg.Connection.APIKey = ""
		}); err != nil {
			return err
		}
		c.logger.Info("migrated plaintext credentials to system keychain", "backend", status.Backend)

		return nil
	}
}

func (c *Controller) saveWithFallback(
	applyEdits func(cfg *config.AppConfig),
	creds credentials.Credentials,
	persistedMode config.CredentialStorageMode,
	fallback CredentialFallbackChoice,
	status credentials.Status,
) (SaveSettingsResult, error) {
	switch fallback {
	case CredentialFallbackPlaintext:
		saved, err := c.persistConfig(true, func(dst *config.AppConfig) {
			applyEdits(dst)
			dst.Connection.CredentialStorage = config.CredentialStoragePlaintext
			dst.Connection.KeychainHasCredentials = false
			dst.Connection.Username = creds.Username
			dst.Connection.Password = creds.Password
			dst.Connection.APIKey = creds.APIKey
		})
		if err != nil && !saved {
			return SaveSettingsResult{CredentialStatus: status}, err
		}
		if err != nil {
			c.setSessionCredentials(creds)
			c.setCredentialStatus(status)

			return SaveSettingsResult{CredentialStatus: status}, err
		}
	case CredentialFallbackSessionOnly:
		saved, err := c.persistConfig(true, func(dst *config.AppConfig) {
			applyEdits(dst)
			switch persistedMode {
			case config.CredentialStorageKeychain:
				dst.Connection.CredentialStorage = config.CredentialStorageKeychain
			default:
				dst.Connection.CredentialStorage = config.CredentialStorageNone
			}
			dst.Connection.Username = ""
			dst.Connection.Password = ""
			dst.Connection.APIKey = ""
		})
		if err != nil && !saved {
			return SaveSettingsResult{CredentialStatus: status}, err
		}
		if err != nil {
			c.setSessionCredentials(creds)
			c.setCredentialStatus(status)

			return SaveSettingsResult{CredentialStatus: status}, err
		}
	default:
		return SaveSettingsResult{CredentialStatus: status}, fmt.Errorf("unsupported credential fallback: %q", fallback)
	}

	c.setSessionCredentials(creds)
	c.setCredentialStatus(status)

	return SaveSettingsResult{CredentialStatus: status}, nil
}

// statusFromError classifies a keychain error for the UI. The stored status is
// read under the lock, but the backend lookup may probe the keyring, so it
// runs outside the lock.
func (c *Controller) statusFromError(err error) credentials.Status {
	c.stateMu.RLock()
	previous := c.credentialStatus
	c.stateMu.RUnlock()

	backend := currentCredentialBackend(previous, c.credentialStore)
	state := credentials.StateUnavailable
	var credErr *credentials.Error
	if errors.As(err, &credErr) {
		state = credErr.State()
	}

	return credentials.Status{
		Backend: backend,
		State:   state,
		Message: err.Error(),
	}
}

func currentCredentialBackend(status credentials.Status, store credentials.Store) string {
	if status.Backend != "" {
		return status.Backend
	}

	return store.Status(context.Background()).Backend
}

// errKeychainTimeout reports that a keychain call outlived its timeout; go-keyring
// has no context support, so timed-out calls are abandoned in a goroutine.
var errKeychainTimeout = errors.New("system keychain did not respond in time")

// keychainLoadStatus classifies the outcome of a keychain credential load for
// the UI. The persisted marker set on every successful keychain write is what
// lets "not found" mean "stored but not loaded yet" (the boot race: the wallet
// had not finished unlocking) instead of "nothing stored".
func keychainLoadStatus(marker bool, err error) credentials.Status {
	backend := credentials.BackendName()
	switch {
	case err == nil:
		return credentials.Status{
			Backend: backend,
			State:   credentials.StateAvailable,
			Message: "System keychain is available.",
		}
	case errors.Is(err, errKeychainTimeout):
		return credentials.Status{
			Backend: backend,
			State:   credentials.StateUnavailable,
			Message: err.Error(),
		}
	case credentials.IsNotStored(err):
		if marker {
			return credentials.Status{
				Backend: backend,
				State:   credentials.StateUnavailable,
				Message: "The system keychain has not loaded the stored credentials yet.",
			}
		}

		return credentials.Status{
			Backend: backend,
			State:   credentials.StateAvailable,
			Message: "System keychain is available.",
		}
	default:
		var credErr *credentials.Error
		if errors.As(err, &credErr) {
			return credentials.Status{
				Backend: backend,
				State:   credErr.State(),
				Message: err.Error(),
			}
		}

		return credentials.Status{
			Backend: backend,
			State:   credentials.StateUnavailable,
			Message: err.Error(),
		}
	}
}

// canonicalCredentials drops credentials that do not belong to the active auth
// method so only the method in use is ever stored or applied.
func canonicalCredentials(method config.AuthMethod, creds credentials.Credentials) credentials.Credentials {
	if method == config.AuthMethodAPIKey {
		return credentials.Credentials{APIKey: creds.APIKey}
	}

	return credentials.Credentials{Username: creds.Username, Password: creds.Password}
}

// inferAuthMethod keeps configs saved before auth methods existed on password
// auth: released builds stored only password credentials and no auth_method
// field, so stored password credentials without an API key identify a legacy
// password setup rather than the new api_key default. Saves never keep
// credentials for an inactive method (see canonicalCredentials), so an
// explicitly chosen api_key method cannot be overridden this way.
func inferAuthMethod(method config.AuthMethod, creds credentials.Credentials) config.AuthMethod {
	if method == config.AuthMethodAPIKey && creds.APIKey == "" && (creds.Username != "" || creds.Password != "") {
		return config.AuthMethodPassword
	}

	return method
}

// reconcileAuthMethod applies legacy inference and records the effective method
// on the in-memory config, so the rest of the controller acts on the method
// that matches the credentials actually being loaded.
func (c *Controller) reconcileAuthMethod(method config.AuthMethod, creds credentials.Credentials) config.AuthMethod {
	inferred := inferAuthMethod(method, creds)
	if inferred == method {
		return method
	}

	c.config.Connection.AuthMethod = inferred
	c.logger.Info("kept password auth for credentials stored before auth methods existed")

	return inferred
}

func connectionClientConfig(cfg config.ConnectionConfig, creds credentials.Credentials) (qbt.ClientConfig, error) {
	qcfg := qbt.ClientConfig{
		URL:                  cfg.URL,
		Username:             strings.TrimSpace(creds.Username),
		Password:             creds.Password,
		SkipCertificateCheck: cfg.SkipCertificateCheck,
	}
	if cfg.AuthMethod == config.AuthMethodAPIKey {
		key := strings.TrimSpace(creds.APIKey)
		if key == "" {
			return qbt.ClientConfig{}, errors.New("API key authentication is selected but no API key is stored; open Settings > Connection and paste the API key")
		}
		qcfg.APIKey = key
	}

	return qcfg, nil
}

// client returns a cached qBittorrent client for the current connection
// configuration, rebuilding it whenever the connection settings or session
// credentials change (the config struct doubles as the cache key). The whole
// snapshot → cache check → build → store sequence runs under one exclusive
// stateMu hold: the key cannot be observed half-applied by a concurrent
// settings save, and SetLogger cannot leave the cache holding a client built
// with the old logger. The lock is exclusive because it also writes the cache;
// qbt.NewClient does no I/O, so the critical section is cheap.
func (c *Controller) client() (*qbt.Client, error) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()

	// While stored keychain credentials have not been loaded into the session,
	// fetches must fail with a typed error instead of connecting with empty
	// credentials (in password mode empty credentials are otherwise legal and
	// would just fail at login with a confusing message). This covers the
	// running background retry as well as the terminal states it can end in —
	// an unreadable payload, an unsupported keychain, or an exhausted retry
	// cap — whose statuses carry the actionable message.
	if c.config.Connection.CredentialStorage == config.CredentialStorageKeychain &&
		c.sessionCredentials == (credentials.Credentials{}) &&
		c.credentialStatus.State != credentials.StateAvailable {
		return nil, &CredentialUnavailableError{
			Status:  c.credentialStatus,
			Waiting: c.credentialRetryActive,
		}
	}

	qcfg, err := connectionClientConfig(c.config.Connection, c.sessionCredentials)
	if err != nil {
		return nil, err
	}

	if c.cachedClient != nil && c.cachedClientConfig == qcfg {
		return c.cachedClient, nil
	}

	client, err := qbt.NewClient(qcfg, c.logger)
	if err != nil {
		return nil, err
	}
	c.cachedClient = client
	c.cachedClientConfig = qcfg

	return client, nil
}
