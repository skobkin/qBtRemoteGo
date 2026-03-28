package config

import (
	"path/filepath"
	"testing"
)

func TestNormalize(t *testing.T) {
	cfg := AppConfig{
		UI: UIConfig{
			RememberPathCount:     0,
			ActivePollSeconds:     0,
			BackgroundPollSeconds: -1,
			FilterBy:              "bad",
			SortColumn:            "bad",
			RecentSavePaths:       []string{"", "/data", "/data", "/more"},
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
	if len(cfg.UI.RecentSavePaths) != 2 {
		t.Fatalf("unexpected recent paths: %#v", cfg.UI.RecentSavePaths)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := Default()
	cfg.Connection.URL = "https://example.invalid/qbt"
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
