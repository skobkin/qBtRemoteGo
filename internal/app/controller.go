package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/skobkin/qbtremotego/internal/config"
	"github.com/skobkin/qbtremotego/internal/platform"
	"github.com/skobkin/qbtremotego/internal/qbt"
)

type Controller struct {
	configPath string
	config     config.AppConfig
	logger     *slog.Logger
	platform   *platform.Manager
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

func NewController(configPath string, logger *slog.Logger) (*Controller, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}

	return &Controller{
		configPath: configPath,
		config:     cfg,
		logger:     logger,
		platform:   platform.NewManager(logger),
	}, nil
}

func (c *Controller) Config() config.AppConfig {
	return c.config
}

func (c *Controller) SetLogger(logger *slog.Logger) {
	c.logger = logger
	c.platform = platform.NewManager(logger)
}

func (c *Controller) SaveConfig(cfg config.AppConfig) error {
	config.Normalize(&cfg)
	if err := config.Save(c.configPath, cfg); err != nil {
		return err
	}

	c.config = cfg
	if errs := c.platform.Sync(cfg.Integration); len(errs) > 0 {
		return errors.New(platform.JoinErrors(errs))
	}

	return nil
}

func (c *Controller) SaveLocalUI(cfg config.AppConfig) error {
	config.Normalize(&cfg)
	if err := config.Save(c.configPath, cfg); err != nil {
		return err
	}
	c.config = cfg

	return nil
}

func (c *Controller) SyncIntegrations() []error {
	return c.platform.Sync(c.config.Integration)
}

func (c *Controller) TestConnection(ctx context.Context, cfg config.ConnectionConfig) error {
	client, err := qbt.NewClient(cfg, c.logger.With("remote", strings.TrimSpace(cfg.URL)))
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

	cfg := c.config
	config.AddRecentPath(&cfg, req.SavePath)
	if err := config.Save(c.configPath, cfg); err != nil {
		c.logger.Warn("save config after add", "error", err)
	} else {
		c.config = cfg
	}

	return nil
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

func (c *Controller) DeleteTorrents(ctx context.Context, hashes []string, deleteFiles bool) error {
	client, err := c.client()
	if err != nil {
		return err
	}

	return client.Delete(ctx, hashes, deleteFiles)
}

func (c *Controller) SuggestDirectories(ctx context.Context, path string) ([]string, error) {
	if !c.config.UI.PathAutocomplete {
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

func (c *Controller) client() (*qbt.Client, error) {
	return qbt.NewClient(c.config.Connection, c.logger)
}
