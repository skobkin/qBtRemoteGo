package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type AppConfig struct {
	Connection  ConnectionConfig  `json:"connection"`
	UI          UIConfig          `json:"ui"`
	Integration IntegrationConfig `json:"integration"`
	Logging     LoggingConfig     `json:"logging"`
	Updates     UpdatesConfig     `json:"updates"`
}

type ConnectionConfig struct {
	URL               string                `json:"url"`
	AuthMethod        AuthMethod            `json:"auth_method,omitempty"`
	CredentialStorage CredentialStorageMode `json:"credential_storage,omitempty"`
	// KeychainHasCredentials records that a successful system keychain write
	// has stored the credentials, so a later "not found" from the keychain
	// means "stored but not loaded yet" (e.g. a wallet that had not finished
	// unlocking at boot) rather than "nothing stored". It is cleared whenever
	// a save persists non-keychain storage; the keychain delete path has no
	// production callers, so there is deliberately no delete-clearing case.
	KeychainHasCredentials bool   `json:"keychain_has_credentials,omitempty"`
	Username               string `json:"username,omitempty"`
	Password               string `json:"password,omitempty"` //nolint:gosec // User-managed qBittorrent credential persisted in the local app config.
	APIKey                 string `json:"api_key,omitempty"`  //nolint:gosec // User-managed qBittorrent API key persisted in the local app config.
	SkipCertificateCheck   bool   `json:"skip_certificate_check"`
}

type AuthMethod string

const (
	AuthMethodPassword AuthMethod = "password"
	AuthMethodAPIKey   AuthMethod = "api_key"
)

type CredentialStorageMode string

const (
	CredentialStorageNone      CredentialStorageMode = "none"
	CredentialStoragePlaintext CredentialStorageMode = "plaintext"
	CredentialStorageKeychain  CredentialStorageMode = "keychain"
)

type UIConfig struct {
	RememberPathCount          int                `json:"remember_path_count"`
	RememberLastSaveLocation   bool               `json:"remember_last_save_location"`
	PathAutocomplete           bool               `json:"path_autocomplete"`
	ActivePollSeconds          int                `json:"active_poll_seconds"`
	BackgroundPollSeconds      int                `json:"background_poll_seconds"`
	StartMinimizedToTray       bool               `json:"start_minimized_to_tray"`
	AddTorrentAdvancedExpanded bool               `json:"add_torrent_advanced_expanded"`
	DetailsPanelEnabled        bool               `json:"details_panel_enabled"`
	DetailsPanelMode           string             `json:"details_panel_mode"`
	FilterBy                   string             `json:"filter_by"`
	SortColumn                 string             `json:"sort_column"`
	SortDescending             bool               `json:"sort_descending"`
	CompactRows                bool               `json:"compact_rows"`
	ColumnWidths               map[string]float32 `json:"column_widths"`
	RecentSavePaths            []string           `json:"recent_save_paths"`
}

type IntegrationConfig struct {
	RegisterMagnetHandler  bool `json:"register_magnet_handler"`
	RegisterTorrentHandler bool `json:"register_torrent_handler"`
	StartWithSystem        bool `json:"start_with_system"`
}

type LoggingConfig struct {
	Level     string `json:"level"`
	LogToFile bool   `json:"log_to_file"`
}

// UpdatesConfig controls the application's release update checks. Manual
// checks from the tray menu are always available regardless of this section.
type UpdatesConfig struct {
	// CheckAutomatically enables the startup and periodic update checks.
	CheckAutomatically bool `json:"check_automatically"`
}

func Default() AppConfig {
	return AppConfig{
		Connection: ConnectionConfig{},
		UI: UIConfig{
			RememberPathCount:          6,
			RememberLastSaveLocation:   true,
			PathAutocomplete:           true,
			ActivePollSeconds:          5,
			BackgroundPollSeconds:      30,
			StartMinimizedToTray:       false,
			AddTorrentAdvancedExpanded: false,
			DetailsPanelEnabled:        false,
			DetailsPanelMode:           "off",
			FilterBy:                   "name",
			SortColumn:                 "added",
			SortDescending:             true,
			CompactRows:                false,
			ColumnWidths:               nil,
			RecentSavePaths:            nil,
		},
		Integration: IntegrationConfig{},
		Logging: LoggingConfig{
			Level:     "info",
			LogToFile: false,
		},
		Updates: UpdatesConfig{
			CheckAutomatically: true,
		},
	}
}

func Normalize(cfg *AppConfig) {
	if cfg == nil {
		return
	}

	def := Default()

	cfg.Connection.URL = strings.TrimSpace(cfg.Connection.URL)
	cfg.Connection.Username = strings.TrimSpace(cfg.Connection.Username)
	cfg.Connection.CredentialStorage = normalizeCredentialStorage(cfg.Connection.CredentialStorage)
	cfg.Connection.AuthMethod = normalizeAuthMethod(cfg.Connection.AuthMethod)
	cfg.Connection.APIKey = strings.TrimSpace(cfg.Connection.APIKey)

	if cfg.UI.RememberPathCount <= 0 {
		cfg.UI.RememberPathCount = def.UI.RememberPathCount
	}
	if cfg.UI.ActivePollSeconds <= 0 {
		cfg.UI.ActivePollSeconds = def.UI.ActivePollSeconds
	}
	if cfg.UI.BackgroundPollSeconds <= 0 {
		cfg.UI.BackgroundPollSeconds = def.UI.BackgroundPollSeconds
	}
	if !isValidDetailsPanelMode(cfg.UI.DetailsPanelMode) {
		cfg.UI.DetailsPanelMode = def.UI.DetailsPanelMode
	}
	if cfg.UI.FilterBy != "name" && cfg.UI.FilterBy != "location" {
		cfg.UI.FilterBy = def.UI.FilterBy
	}
	if !isValidSortColumn(cfg.UI.SortColumn) {
		cfg.UI.SortColumn = def.UI.SortColumn
	}
	cfg.UI.ColumnWidths = normalizeColumnWidths(cfg.UI.ColumnWidths)
	cfg.UI.RecentSavePaths = normalizePaths(cfg.UI.RecentSavePaths, cfg.UI.RememberPathCount)

	cfg.Logging.Level = normalizeLogLevel(cfg.Logging.Level)
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = def.Logging.Level
	}
}

func Load(path string) (AppConfig, error) {
	// #nosec G304 -- path comes from app config resolution and tests control their own temp paths.
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			cfg := Default()
			Normalize(&cfg)

			return cfg, nil
		}

		return Default(), fmt.Errorf("read config: %w", err)
	}

	cfg := Default()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Default(), fmt.Errorf("decode config: %w", err)
	}
	Normalize(&cfg)

	return cfg, nil
}

// Save persists the config atomically: the payload is written to a temporary
// file in the same directory (same filesystem) and renamed into place, so a
// crash or concurrent reader never observes a truncated or partially written
// config.json. Concurrent Save calls remain last-writer-wins by design.
func Save(path string, cfg AppConfig) error {
	Normalize(&cfg)

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(dir, ".qbtremotego-config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	tmpName := tmp.Name()

	if err := writeTempConfig(tmp, data); err != nil {
		_ = os.Remove(tmpName)

		return err
	}

	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)

		return fmt.Errorf("replace config: %w", err)
	}

	return nil
}

func writeTempConfig(tmp *os.File, data []byte) error {
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()

		return fmt.Errorf("write config: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()

		return fmt.Errorf("sync config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close config: %w", err)
	}

	return nil
}

func AddRecentPath(cfg *AppConfig, path string) {
	if cfg == nil {
		return
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}

	paths := []string{path}
	for _, existing := range cfg.UI.RecentSavePaths {
		if !strings.EqualFold(strings.TrimSpace(existing), path) {
			paths = append(paths, strings.TrimSpace(existing))
		}
	}
	cfg.UI.RecentSavePaths = normalizePaths(paths, cfg.UI.RememberPathCount)
}

func normalizePaths(paths []string, limit int) []string {
	if limit <= 0 {
		return nil
	}

	out := make([]string, 0, min(len(paths), limit))
	seen := map[string]struct{}{}
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		key := strings.ToLower(path)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, path)
		if len(out) == limit {
			break
		}
	}

	return out
}

// normalizeAuthMethod defaults unknown and empty values to API-key auth so
// consumers can always switch on a canonical, non-empty method. Configs saved
// before auth methods existed (password credentials, no auth_method field) are
// kept on password auth by the controller, which infers the method from the
// stored credentials.
func normalizeAuthMethod(method AuthMethod) AuthMethod {
	if strings.ToLower(strings.TrimSpace(string(method))) == string(AuthMethodPassword) {
		return AuthMethodPassword
	}

	return AuthMethodAPIKey
}

func normalizeCredentialStorage(mode CredentialStorageMode) CredentialStorageMode {
	switch strings.ToLower(strings.TrimSpace(string(mode))) {
	case "":
		return ""
	case string(CredentialStorageNone):
		return CredentialStorageNone
	case string(CredentialStoragePlaintext):
		return CredentialStoragePlaintext
	case string(CredentialStorageKeychain):
		return CredentialStorageKeychain
	default:
		return CredentialStorageNone
	}
}

func isValidSortColumn(column string) bool {
	return slices.Contains([]string{
		"name", "size", "progress", "status", "down", "up", "eta", "added",
	}, column)
}

func isValidDetailsPanelMode(mode string) bool {
	return slices.Contains([]string{"off", "overlay_right", "bottom_pane"}, strings.ToLower(strings.TrimSpace(mode)))
}

func normalizeLogLevel(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "", "info":
		return "info"
	case "debug":
		return "debug"
	case "warn", "warning":
		return "warn"
	case "error":
		return "error"
	default:
		return ""
	}
}

func normalizeColumnWidths(widths map[string]float32) map[string]float32 {
	if len(widths) == 0 {
		return nil
	}

	out := make(map[string]float32, len(widths))
	for key, width := range widths {
		if !isValidSortColumn(key) {
			continue
		}
		if width <= 0 {
			continue
		}
		out[key] = width
	}
	if len(out) == 0 {
		return nil
	}

	return out
}
