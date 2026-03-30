package config

import (
	"path/filepath"
	"testing"
)

func TestNormalize(t *testing.T) {
	cfg := AppConfig{
		UI: UIConfig{
			RememberPathCount:          0,
			ActivePollSeconds:          0,
			BackgroundPollSeconds:      -1,
			AddTorrentAdvancedExpanded: true,
			FilterBy:                   "bad",
			SortColumn:                 "bad",
			ColumnWidths: map[string]float32{
				"name": 320,
				"bad":  50,
				"eta":  0,
			},
			RecentSavePaths: []string{"", "/data", "/data", "/more"},
		},
	}

	Normalize(&cfg)

	if cfg.UI.RememberPathCount != Default().UI.RememberPathCount {
		t.Fatalf("unexpected remember count: %d", cfg.UI.RememberPathCount)
	}
	if cfg.UI.FilterBy != "name" {
		t.Fatalf("unexpected filter by: %q", cfg.UI.FilterBy)
	}
	if cfg.UI.SortColumn != "added" {
		t.Fatalf("unexpected sort column: %q", cfg.UI.SortColumn)
	}
	if len(cfg.UI.ColumnWidths) != 1 || cfg.UI.ColumnWidths["name"] != 320 {
		t.Fatalf("unexpected column widths: %#v", cfg.UI.ColumnWidths)
	}
	if len(cfg.UI.RecentSavePaths) != 2 {
		t.Fatalf("unexpected recent paths: %#v", cfg.UI.RecentSavePaths)
	}
	if !cfg.UI.AddTorrentAdvancedExpanded {
		t.Fatal("expected advanced add-torrent state to be preserved")
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := Default()
	cfg.Connection.URL = "https://example.invalid/qbt"
	cfg.UI.AddTorrentAdvancedExpanded = true
	cfg.UI.ColumnWidths = map[string]float32{"name": 480, "progress": 160}
	cfg.UI.RecentSavePaths = []string{"/data/one", "/data/two"}

	if err := Save(path, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if loaded.Connection.URL != cfg.Connection.URL {
		t.Fatalf("unexpected URL: %q", loaded.Connection.URL)
	}
	if !loaded.UI.AddTorrentAdvancedExpanded {
		t.Fatal("expected advanced add-torrent state to round-trip")
	}
	if loaded.UI.ColumnWidths["name"] != 480 {
		t.Fatalf("unexpected column widths: %#v", loaded.UI.ColumnWidths)
	}
	if len(loaded.UI.RecentSavePaths) != 2 {
		t.Fatalf("unexpected paths: %#v", loaded.UI.RecentSavePaths)
	}
}

func TestAddRecentPath(t *testing.T) {
	cfg := Default()
	cfg.UI.RememberPathCount = 2
	cfg.UI.RecentSavePaths = []string{"/old", "/older"}

	AddRecentPath(&cfg, "/new")
	AddRecentPath(&cfg, "/old")

	want := []string{"/old", "/new"}
	for i, item := range want {
		if cfg.UI.RecentSavePaths[i] != item {
			t.Fatalf("unexpected path at %d: %#v", i, cfg.UI.RecentSavePaths)
		}
	}
}

func TestDefaultAddTorrentAdvancedExpanded(t *testing.T) {
	if Default().UI.AddTorrentAdvancedExpanded {
		t.Fatal("expected advanced add-torrent section to default to collapsed")
	}
}

func TestNormalizeLoggingLevel(t *testing.T) {
	tests := []struct {
		name  string
		level string
		want  string
	}{
		{name: "empty defaults to info", level: "", want: "info"},
		{name: "warning alias normalizes", level: "warning", want: "warn"},
		{name: "invalid falls back to info", level: "trace", want: "info"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Default()
			cfg.Logging.Level = tc.level

			Normalize(&cfg)

			if cfg.Logging.Level != tc.want {
				t.Fatalf("Normalize().Logging.Level = %q, want %q", cfg.Logging.Level, tc.want)
			}
		})
	}
}
