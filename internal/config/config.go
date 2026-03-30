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

const (
	AppDirName     = "qbtremotego"
	ConfigFileName = "config.json"
)

type AppConfig struct {
	Connection  ConnectionConfig  `json:"connection"`
	UI          UIConfig          `json:"ui"`
	Integration IntegrationConfig `json:"integration"`
	Logging     LoggingConfig     `json:"logging"`
}

type ConnectionConfig struct {
	URL                  string `json:"url"`
	Username             string `json:"username"`
	Password             string `json:"password"` //nolint:gosec // User-managed qBittorrent credential persisted in the local app config.
	SkipCertificateCheck bool   `json:"skip_certificate_check"`
}

type UIConfig struct {
	RememberPathCount          int                `json:"remember_path_count"`
	PathAutocomplete           bool               `json:"path_autocomplete"`
	ActivePollSeconds          int                `json:"active_poll_seconds"`
	BackgroundPollSeconds      int                `json:"background_poll_seconds"`
	StartMinimizedToTray       bool               `json:"start_minimized_to_tray"`
	AddTorrentAdvancedExpanded bool               `json:"add_torrent_advanced_expanded"`
	FilterBy                   string             `json:"filter_by"`
	SortColumn                 string             `json:"sort_column"`
	SortDescending             bool               `json:"sort_descending"`
	ColumnWidths               map[string]float32 `json:"column_widths"`
	RecentSavePaths            []string           `json:"recent_save_paths"`
}

type IntegrationConfig struct {
	RegisterMagnetHandler  bool `json:"register_magnet_handler"`
	RegisterTorrentHandler bool `json:"register_torrent_handler"`
	StartWithSystem        bool `json:"start_with_system"`
}

type LoggingConfig struct {
	Level string `json:"level"`
}

func Default() AppConfig {
	return AppConfig{
		Connection: ConnectionConfig{},
		UI: UIConfig{
			RememberPathCount:          6,
			PathAutocomplete:           true,
			ActivePollSeconds:          5,
			BackgroundPollSeconds:      30,
			StartMinimizedToTray:       false,
			AddTorrentAdvancedExpanded: false,
			FilterBy:                   "name",
			SortColumn:                 "added",
			SortDescending:             true,
			ColumnWidths:               nil,
			RecentSavePaths:            nil,
		},
		Integration: IntegrationConfig{},
		Logging: LoggingConfig{
			Level: "info",
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

	if cfg.UI.RememberPathCount <= 0 {
		cfg.UI.RememberPathCount = def.UI.RememberPathCount
	}
	if cfg.UI.ActivePollSeconds <= 0 {
		cfg.UI.ActivePollSeconds = def.UI.ActivePollSeconds
	}
	if cfg.UI.BackgroundPollSeconds <= 0 {
		cfg.UI.BackgroundPollSeconds = def.UI.BackgroundPollSeconds
	}
	if cfg.UI.FilterBy != "name" && cfg.UI.FilterBy != "location" {
		cfg.UI.FilterBy = def.UI.FilterBy
	}
	if !isValidSortColumn(cfg.UI.SortColumn) {
		cfg.UI.SortColumn = def.UI.SortColumn
	}
	cfg.UI.ColumnWidths = normalizeColumnWidths(cfg.UI.ColumnWidths)
	cfg.UI.RecentSavePaths = normalizePaths(cfg.UI.RecentSavePaths, cfg.UI.RememberPathCount)

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

func Save(path string, cfg AppConfig) error {
	Normalize(&cfg)

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}

func DefaultConfigDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}

	return filepath.Join(base, AppDirName), nil
}

func DefaultConfigPath() (string, error) {
	dir, err := DefaultConfigDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, ConfigFileName), nil
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

func isValidSortColumn(column string) bool {
	return slices.Contains([]string{
		"name", "size", "progress", "status", "down", "up", "eta", "added",
	}, column)
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
