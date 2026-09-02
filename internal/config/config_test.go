package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	cfg.Connection.CredentialStorage = CredentialStoragePlaintext
	cfg.Connection.AuthMethod = AuthMethodAPIKey
	cfg.Connection.Username = "demo"
	cfg.Connection.Password = "secret"
	cfg.Connection.APIKey = "qbt_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	cfg.UI.AddTorrentAdvancedExpanded = true
	cfg.UI.ColumnWidths = map[string]float32{"name": 480, "progress": 160}
	cfg.UI.RecentSavePaths = []string{"/data/one", "/data/two"}
	cfg.UI.RememberLastSaveLocation = false
	cfg.UI.CompactRows = true

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
	if loaded.Connection.CredentialStorage != CredentialStoragePlaintext {
		t.Fatalf("unexpected credential storage: %q", loaded.Connection.CredentialStorage)
	}
	if loaded.Connection.Username != "demo" || loaded.Connection.Password != "secret" {
		t.Fatalf("unexpected credentials: %#v", loaded.Connection)
	}
	if loaded.Connection.AuthMethod != AuthMethodAPIKey {
		t.Fatalf("unexpected auth method: %q", loaded.Connection.AuthMethod)
	}
	if loaded.Connection.APIKey != "qbt_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("unexpected API key: %q", loaded.Connection.APIKey)
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
	if loaded.UI.RememberLastSaveLocation {
		t.Fatal("expected remember-last-save-location override to round-trip")
	}
	if !loaded.UI.CompactRows {
		t.Fatal("expected compact rows to round-trip")
	}
}

func TestDefaultCompactRowsDisabled(t *testing.T) {
	if Default().UI.CompactRows {
		t.Fatal("expected compact rows to default to disabled")
	}
}

func TestNormalizeCredentialStorage(t *testing.T) {
	cfg := Default()
	cfg.Connection.CredentialStorage = " KEYCHAIN "

	Normalize(&cfg)

	if cfg.Connection.CredentialStorage != CredentialStorageKeychain {
		t.Fatalf("unexpected credential storage: %q", cfg.Connection.CredentialStorage)
	}
}

func TestNormalizeAuthMethod(t *testing.T) {
	tests := []struct {
		name   string
		method AuthMethod
		want   AuthMethod
	}{
		{name: "empty defaults to api key", method: "", want: AuthMethodAPIKey},
		{name: "unknown falls back to api key", method: "bogus", want: AuthMethodAPIKey},
		{name: "case and space insensitive api key", method: " Api_Key ", want: AuthMethodAPIKey},
		{name: "api key preserved", method: AuthMethodAPIKey, want: AuthMethodAPIKey},
		{name: "password preserved", method: AuthMethodPassword, want: AuthMethodPassword},
		{name: "case and space insensitive password", method: " Password ", want: AuthMethodPassword},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Default()
			cfg.Connection.AuthMethod = tc.method

			Normalize(&cfg)

			if cfg.Connection.AuthMethod != tc.want {
				t.Fatalf("unexpected auth method: got %q, want %q", cfg.Connection.AuthMethod, tc.want)
			}
		})
	}
}

func TestNormalizeTrimsAPIKey(t *testing.T) {
	cfg := Default()
	cfg.Connection.APIKey = "  qbt_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n"

	Normalize(&cfg)

	if cfg.Connection.APIKey != "qbt_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("unexpected trimmed API key: %q", cfg.Connection.APIKey)
	}
}

func TestLoadLegacyConfigDefaultsToAPIKeyAuth(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	data := []byte("{\n  \"connection\": {\n    \"url\": \"https://example.invalid/qbt\",\n    \"username\": \"demo\"\n  }\n}\n")

	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	// At the config layer a missing auth_method defaults to API-key auth; the
	// controller keeps configs carrying password credentials on password auth
	// (see the controller inference tests).
	if loaded.Connection.AuthMethod != AuthMethodAPIKey {
		t.Fatalf("expected legacy config to default to api key auth, got %q", loaded.Connection.AuthMethod)
	}
}

func TestNormalizeInvalidCredentialStorageFallsBackToNone(t *testing.T) {
	cfg := Default()
	cfg.Connection.CredentialStorage = "bad"

	Normalize(&cfg)

	if cfg.Connection.CredentialStorage != CredentialStorageNone {
		t.Fatalf("unexpected credential storage: %q", cfg.Connection.CredentialStorage)
	}
}

func TestSaveOmitsScrubbedKeychainCredentials(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := Default()
	cfg.Connection.CredentialStorage = CredentialStorageKeychain
	cfg.Connection.Username = ""
	cfg.Connection.Password = ""

	if err := Save(path, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	// #nosec G304 -- test reads back a temp config file written in this test.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	text := string(data)
	if containsAny(text, `"username":`, `"password":`, `"api_key":`) {
		t.Fatalf("expected scrubbed config to omit credentials:\n%s", text)
	}
}

func TestSaveOmitsKeychainMarkerWhenUnset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	if err := Save(path, Default()); err != nil {
		t.Fatalf("save config: %v", err)
	}

	// #nosec G304 -- test reads back a temp config file written in this test.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if strings.Contains(string(data), "keychain_has_credentials") {
		t.Fatalf("expected the unset keychain marker to be omitted:\n%s", data)
	}
}

func TestSaveLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	if err := Save(path, Default()); err != nil {
		t.Fatalf("save config: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read config dir: %v", err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Fatalf("expected no temp file leftovers, found %q", entry.Name())
		}
	}
}

func TestSaveCleansUpTempFileWhenTargetUnwritable(t *testing.T) {
	dir := t.TempDir()
	// A directory at the target path makes the final rename fail.
	path := filepath.Join(dir, "config.json")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("create dir at target path: %v", err)
	}

	if err := Save(path, Default()); err == nil {
		t.Fatal("expected save into a directory path to fail")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read config dir: %v", err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Fatalf("expected failed save to remove its temp file, found %q", entry.Name())
		}
	}
}

func TestConcurrentSavesKeepConfigLoadable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	const writers = 8
	const savesPerWriter = 25

	var wg sync.WaitGroup
	for writer := range writers {
		wg.Add(1)
		go func(writer int) {
			defer wg.Done()

			for i := range savesPerWriter {
				cfg := Default()
				cfg.Connection.URL = fmt.Sprintf("https://writer-%d.example.invalid/%d", writer, i)
				if err := Save(path, cfg); err != nil {
					t.Errorf("save config: %v", err)

					return
				}
			}
		}(writer)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)

		for range writers * savesPerWriter {
			if _, err := Load(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("load config while saving: %v", err)

				return
			}
		}
	}()

	wg.Wait()
	<-done

	if _, err := Load(path); err != nil {
		t.Fatalf("load final config: %v", err)
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

func TestDefaultRememberLastSaveLocation(t *testing.T) {
	if !Default().UI.RememberLastSaveLocation {
		t.Fatal("expected remember-last-save-location to default to enabled")
	}
}

func TestDefaultDetailsPanelDisabled(t *testing.T) {
	if Default().UI.DetailsPanelEnabled {
		t.Fatal("expected details panel to default to disabled")
	}
	if Default().UI.DetailsPanelMode != "off" {
		t.Fatalf("unexpected details panel mode: %q", Default().UI.DetailsPanelMode)
	}
}

func TestNormalizeInvalidDetailsPanelModeFallsBackToOff(t *testing.T) {
	cfg := Default()
	cfg.UI.DetailsPanelEnabled = true
	cfg.UI.DetailsPanelMode = "drawer-left"

	Normalize(&cfg)

	if cfg.UI.DetailsPanelMode != "off" {
		t.Fatalf("unexpected details mode: %q", cfg.UI.DetailsPanelMode)
	}
}

func TestLoadMissingRememberLastSaveLocationDefaultsToTrue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	data := []byte("{\n  \"ui\": {\n    \"remember_path_count\": 3,\n    \"recent_save_paths\": [\"/data\"]\n  }\n}\n")

	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !loaded.UI.RememberLastSaveLocation {
		t.Fatal("expected missing remember-last-save-location to default to enabled")
	}
}

func TestLoadMissingLogToFileDefaultsToFalse(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	// Existing config.json from before #13 landed: no log_to_file key.
	data := []byte("{\n  \"logging\": {\n    \"level\": \"info\"\n  },\n  \"ui\": {}\n}\n")

	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if loaded.Logging.LogToFile {
		t.Fatal("expected missing log_to_file to default to false")
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

func containsAny(text string, patterns ...string) bool {
	for _, pattern := range patterns {
		if strings.Contains(text, pattern) {
			return true
		}
	}

	return false
}
