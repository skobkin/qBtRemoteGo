package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/skobkin/qbtremotego/internal/config"
	"github.com/skobkin/qbtremotego/internal/credentials"
	"github.com/skobkin/qbtremotego/internal/qbt"
	keyring "github.com/zalando/go-keyring"
)

func TestValidateAddDialogData(t *testing.T) {
	req, err := ValidateAddDialogData(AddDialogData{
		SourceType:        qbt.SourceMagnet,
		MagnetText:        "magnet:?xt=urn:btih:abc\n",
		ManagementMode:    "Auto",
		DownloadLimitText: "0",
		UploadLimitText:   "128",
		ContentLayout:     "Create subfolder",
		StopCondition:     "Files checked",
		StartTorrent:      true,
	})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if len(req.MagnetLinks) != 1 || req.ManagementMode != "auto" {
		t.Fatalf("unexpected request: %#v", req)
	}
	if req.ContentLayout != "Subfolder" || req.StopCondition != "FilesChecked" {
		t.Fatalf("unexpected enums: %#v", req)
	}
	if req.UploadLimitKiB == nil || *req.UploadLimitKiB != 128 {
		t.Fatalf("unexpected upload limit: %#v", req.UploadLimitKiB)
	}
}

func TestValidateAddDialogDataRejectsBadRate(t *testing.T) {
	if _, err := ValidateAddDialogData(AddDialogData{
		SourceType:        qbt.SourceMagnet,
		MagnetText:        "magnet:?xt=urn:btih:abc",
		DownloadLimitText: "-1",
	}); err == nil {
		t.Fatal("expected error")
	}
}

func TestFilterAndSortTorrents(t *testing.T) {
	now := time.Now()
	items := []qbt.Torrent{
		{Name: "Zulu", SavePath: "/data/other", AddedUnix: now.Add(-time.Hour).Unix(), AddedAt: now.Add(-time.Hour)},
		{Name: "Alpha", SavePath: "/data/main", AddedUnix: now.Unix(), AddedAt: now},
	}

	filtered := FilterAndSortTorrents(items, "main", "location", "name", false)
	if len(filtered) != 1 || filtered[0].Name != "Alpha" {
		t.Fatalf("unexpected filtered list: %#v", filtered)
	}

	sorted := FilterAndSortTorrents(items, "", "name", "added", true)
	if sorted[0].Name != "Alpha" {
		t.Fatalf("unexpected sorted order: %#v", sorted)
	}
}

func TestStatusLabel(t *testing.T) {
	if StatusLabel("uploading") != "Seeding" {
		t.Fatalf("unexpected label")
	}
	if StatusLabel("missingFiles") != "Missing files" {
		t.Fatalf("unexpected missing files label")
	}
}

func TestHumanSpeedLimit(t *testing.T) {
	if got := HumanSpeedLimit(0); got != "∞" {
		t.Fatalf("unexpected unlimited label: %q", got)
	}
	if got := HumanSpeedLimit(1536); got != "1.5 KiB/s" {
		t.Fatalf("unexpected limited label: %q", got)
	}
}

func TestConnectionStatusLabel(t *testing.T) {
	cases := map[string]string{
		"connected":    "Connected",
		"firewalled":   "Firewalled",
		"disconnected": "Disconnected",
		"":             "Unknown",
	}

	for raw, want := range cases {
		if got := ConnectionStatusLabel(raw); got != want {
			t.Fatalf("unexpected connection label for %q: got %q want %q", raw, got, want)
		}
	}
}

func TestHumanAdded(t *testing.T) {
	now := time.Date(2026, time.March, 29, 12, 0, 0, 0, time.UTC)

	cases := map[string]time.Time{
		"2y4mo":  now.Add(-(2*365*24 + 4*30*24) * time.Hour),
		"4mo10d": now.Add(-(4*30*24 + 10*24) * time.Hour),
		"15d10h": now.Add(-(15*24 + 10) * time.Hour),
		"10h20m": now.Add(-(10*time.Hour + 20*time.Minute)),
		"12m":    now.Add(-12 * time.Minute),
		"now":    now,
	}

	for want, addedAt := range cases {
		if got := humanElapsed(now, addedAt); got != want {
			t.Fatalf("unexpected human elapsed for %s: got %q", want, got)
		}
	}
}

func TestHumanETA(t *testing.T) {
	cases := map[int64]string{
		-1:      "Unknown",
		0:       "Done",
		59:      "59s",
		60:      "1m",
		3660:    "1h 1m",
		90000:   "1d 1h",
		8640000: "∞",
	}

	for seconds, want := range cases {
		if got := HumanETA(seconds); got != want {
			t.Fatalf("unexpected ETA for %d: got %q want %q", seconds, got, want)
		}
	}
}

func TestHumanDuration(t *testing.T) {
	cases := map[int64]string{
		-1:     "∞",
		0:      "0s",
		59:     "59s",
		60:     "1m",
		125:    "2m",
		3660:   "1h 1m",
		90000:  "1d 1h",
		172800: "2d 0h",
	}

	for seconds, want := range cases {
		if got := HumanDuration(seconds); got != want {
			t.Fatalf("unexpected duration for %d: got %q want %q", seconds, got, want)
		}
	}
}

func TestNewControllerInfersPasswordAuthFromLegacyConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	// Released builds write no auth_method field; password auth is the only
	// method they have.
	data := `{"connection":{"url":"https://example.invalid","username":"demo","password":"secret"}}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}

	controller, err := newController(path, slog.New(slog.NewTextHandler(io.Discard, nil)), credentials.NewStoreForTests(
		func(service, user string) (string, error) { return "", keyring.ErrNotFound },
		func(service, user, password string) error { return nil },
		func(service, user string) error { return nil },
	))
	if err != nil {
		t.Fatalf("new controller: %v", err)
	}
	controller.platform = nil

	if got := controller.Config().Connection.AuthMethod; got != config.AuthMethodPassword {
		t.Fatalf("expected inferred password auth, got %q", got)
	}
	if got := controller.SessionCredentials(); got != (credentials.Credentials{Username: "demo", Password: "secret"}) {
		t.Fatalf("unexpected session credentials: %#v", got)
	}
}

func TestNewControllerInfersPasswordAuthFromKeychainPayload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	data := `{"connection":{"url":"https://example.invalid","credential_storage":"keychain"}}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}

	controller, err := newController(path, slog.New(slog.NewTextHandler(io.Discard, nil)), credentials.NewStoreForTests(
		func(service, user string) (string, error) {
			return `{"username":"demo","password":"secret"}`, nil
		},
		func(service, user, password string) error { return nil },
		func(service, user string) error { return nil },
	))
	if err != nil {
		t.Fatalf("new controller: %v", err)
	}
	controller.platform = nil

	if got := controller.Config().Connection.AuthMethod; got != config.AuthMethodPassword {
		t.Fatalf("expected inferred password auth, got %q", got)
	}
	if got := controller.SessionCredentials(); got != (credentials.Credentials{Username: "demo", Password: "secret"}) {
		t.Fatalf("unexpected session credentials: %#v", got)
	}
}

func TestSetTorrentLocationRejectsBlankLocation(t *testing.T) {
	controller := newTestController(t, config.Default(), credentials.NewStoreForTests(
		func(service, user string) (string, error) { return "", nil },
		func(service, user, password string) error { return nil },
		func(service, user string) error { return nil },
	))

	err := controller.SetTorrentLocation(context.Background(), []string{"a"}, " \t ")
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); got != "save location is required" {
		t.Fatalf("unexpected error: %q", got)
	}
}

func TestRenameTorrentRejectsBlankName(t *testing.T) {
	controller := newTestController(t, config.Default(), credentials.NewStoreForTests(
		func(service, user string) (string, error) { return "", nil },
		func(service, user, password string) error { return nil },
		func(service, user string) error { return nil },
	))

	err := controller.RenameTorrent(context.Background(), "a", " \t ")
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); got != "torrent name is required" {
		t.Fatalf("unexpected error: %q", got)
	}
}

func TestNewControllerMigratesLegacyPlaintextCredentials(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := config.Default()
	cfg.Connection.Username = "demo"
	cfg.Connection.Password = "secret"
	if err := config.Save(path, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	var stored credentials.Credentials
	store := credentials.NewStoreForTests(
		func(service, user string) (string, error) {
			if stored == (credentials.Credentials{}) {
				return "", keyring.ErrNotFound
			}
			return `{"username":"` + stored.Username + `","password":"` + stored.Password + `"}`, nil
		},
		func(service, user, password string) error {
			stored = credentials.Credentials{Username: "demo", Password: "secret"}
			return nil
		},
		func(service, user string) error { return nil },
	)

	controller, err := newController(path, slog.Default(), store)
	if err != nil {
		t.Fatalf("new controller: %v", err)
	}
	controller.platform = nil

	if controller.Config().Connection.CredentialStorage != config.CredentialStorageKeychain {
		t.Fatalf("unexpected storage mode: %q", controller.Config().Connection.CredentialStorage)
	}
	if controller.Config().Connection.Username != "" || controller.Config().Connection.Password != "" {
		t.Fatalf("expected config credentials to be scrubbed: %#v", controller.Config().Connection)
	}
	if controller.SessionCredentials().Username != "demo" || controller.SessionCredentials().Password != "secret" {
		t.Fatalf("unexpected session credentials: %#v", controller.SessionCredentials())
	}
}

func TestSaveSettingsRequiresDecisionWhenKeychainUnavailableAndCredentialsChanged(t *testing.T) {
	controller := newTestController(t, config.Default(), credentials.NewStoreForTests(
		func(service, user string) (string, error) { return "", errors.New("keychain locked") },
		func(service, user, password string) error { return errors.New("keychain locked") },
		func(service, user string) error { return nil },
	))

	updated := controller.Config()
	updated.Connection.AuthMethod = config.AuthMethodPassword

	result, err := controller.SaveSettings(context.Background(), updated, credentials.Credentials{
		Username: "new-user",
		Password: "new-pass",
	}, CredentialFallbackUnspecified)
	if err != nil {
		t.Fatalf("save settings: %v", err)
	}
	if !result.DecisionRequired {
		t.Fatalf("expected decision to be required: %#v", result)
	}
}

func TestSaveSettingsPlaintextFallback(t *testing.T) {
	controller := newTestController(t, config.Default(), credentials.NewStoreForTests(
		func(service, user string) (string, error) { return "", errors.New("keychain unavailable") },
		func(service, user, password string) error { return errors.New("keychain unavailable") },
		func(service, user string) error { return nil },
	))

	updated := controller.Config()
	updated.Connection.AuthMethod = config.AuthMethodPassword
	updated.Logging.Level = "debug"

	result, err := controller.SaveSettings(context.Background(), updated, credentials.Credentials{
		Username: "demo",
		Password: "secret",
	}, CredentialFallbackPlaintext)
	if err != nil {
		t.Fatalf("save settings: %v", err)
	}
	if result.DecisionRequired {
		t.Fatalf("did not expect decision to be required: %#v", result)
	}
	if controller.Config().Connection.CredentialStorage != config.CredentialStoragePlaintext {
		t.Fatalf("unexpected storage mode: %q", controller.Config().Connection.CredentialStorage)
	}
	if controller.Config().Connection.Username != "demo" || controller.Config().Connection.Password != "secret" {
		t.Fatalf("unexpected persisted credentials: %#v", controller.Config().Connection)
	}
	if controller.SessionCredentials().Username != "demo" || controller.SessionCredentials().Password != "secret" {
		t.Fatalf("unexpected session credentials: %#v", controller.SessionCredentials())
	}
}

func TestSaveSettingsSessionOnlyKeepsKeychainModeDuringTemporaryOutage(t *testing.T) {
	cfg := config.Default()
	cfg.Connection.AuthMethod = config.AuthMethodPassword
	cfg.Connection.CredentialStorage = config.CredentialStorageKeychain

	controller := newTestController(t, cfg, credentials.NewStoreForTests(
		func(service, user string) (string, error) { return "", errors.New("keychain locked") },
		func(service, user, password string) error { return errors.New("keychain locked") },
		func(service, user string) error { return nil },
	))
	controller.config.Connection.CredentialStorage = config.CredentialStorageKeychain

	result, err := controller.SaveSettings(context.Background(), controller.Config(), credentials.Credentials{
		Username: "temp-user",
		Password: "temp-pass",
	}, CredentialFallbackSessionOnly)
	if err != nil {
		t.Fatalf("save settings: %v", err)
	}
	if result.DecisionRequired {
		t.Fatalf("did not expect decision to be required: %#v", result)
	}
	if controller.Config().Connection.CredentialStorage != config.CredentialStorageKeychain {
		t.Fatalf("expected keychain mode to remain configured: %q", controller.Config().Connection.CredentialStorage)
	}
	if controller.Config().Connection.Username != "" || controller.Config().Connection.Password != "" {
		t.Fatalf("expected config credentials to be scrubbed: %#v", controller.Config().Connection)
	}
	if controller.SessionCredentials().Username != "temp-user" || controller.SessionCredentials().Password != "temp-pass" {
		t.Fatalf("unexpected session credentials: %#v", controller.SessionCredentials())
	}
}

var testAPIKey = "qbt_" + strings.Repeat("a", 28)

func newCapturingKeychainStore() (credentials.Store, *string) {
	var raw string
	store := credentials.NewStoreForTests(
		func(service, user string) (string, error) {
			if raw == "" {
				return "", keyring.ErrNotFound
			}
			return raw, nil
		},
		func(service, user, password string) error {
			raw = password
			return nil
		},
		func(service, user string) error { return nil },
	)

	return store, &raw
}

func decodeStoredPayload(t *testing.T, raw string) credentials.Credentials {
	t.Helper()

	var stored struct {
		Username string `json:"username"`
		Password string `json:"password"`
		APIKey   string `json:"api_key"`
	}
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		t.Fatalf("decode stored payload %q: %v", raw, err)
	}

	return credentials.Credentials{Username: stored.Username, Password: stored.Password, APIKey: stored.APIKey}
}

func TestSaveSettingsAPIKeyDropsPasswordCredentials(t *testing.T) {
	store, raw := newCapturingKeychainStore()
	controller := newTestController(t, config.Default(), store)

	updated := controller.Config()
	updated.Connection.AuthMethod = config.AuthMethodAPIKey

	if _, err := controller.SaveSettings(context.Background(), updated, credentials.Credentials{
		Username: "demo",
		Password: "secret",
		APIKey:   testAPIKey,
	}, CredentialFallbackUnspecified); err != nil {
		t.Fatalf("save settings: %v", err)
	}

	stored := decodeStoredPayload(t, *raw)
	if stored.Username != "" || stored.Password != "" {
		t.Fatalf("expected keychain payload to drop password credentials, got %#v", stored)
	}
	if stored.APIKey != testAPIKey {
		t.Fatalf("unexpected stored API key: %q", stored.APIKey)
	}
	if got := controller.SessionCredentials(); got != (credentials.Credentials{APIKey: testAPIKey}) {
		t.Fatalf("unexpected session credentials: %#v", got)
	}
	conn := controller.Config().Connection
	if conn.Username != "" || conn.Password != "" || conn.APIKey != "" {
		t.Fatalf("expected config to be scrubbed, got %#v", conn)
	}
}

func TestSaveSettingsPasswordDropsAPIKey(t *testing.T) {
	store, raw := newCapturingKeychainStore()
	controller := newTestController(t, config.Default(), store)

	updated := controller.Config()
	updated.Connection.AuthMethod = config.AuthMethodPassword

	if _, err := controller.SaveSettings(context.Background(), updated, credentials.Credentials{
		Username: "demo",
		Password: "secret",
		APIKey:   testAPIKey,
	}, CredentialFallbackUnspecified); err != nil {
		t.Fatalf("save settings: %v", err)
	}

	stored := decodeStoredPayload(t, *raw)
	if stored.APIKey != "" {
		t.Fatalf("expected keychain payload to drop the API key, got %#v", stored)
	}
	if stored.Username != "demo" || stored.Password != "secret" {
		t.Fatalf("unexpected stored credentials: %#v", stored)
	}
	if got := controller.SessionCredentials(); got != (credentials.Credentials{Username: "demo", Password: "secret"}) {
		t.Fatalf("unexpected session credentials: %#v", got)
	}
}

func TestSaveSettingsAPIKeyPlaintextFallbackDropsPasswordCredentials(t *testing.T) {
	controller := newTestController(t, config.Default(), credentials.NewStoreForTests(
		func(service, user string) (string, error) { return "", errors.New("keychain unavailable") },
		func(service, user, password string) error { return errors.New("keychain unavailable") },
		func(service, user string) error { return nil },
	))

	updated := controller.Config()
	updated.Connection.AuthMethod = config.AuthMethodAPIKey

	if _, err := controller.SaveSettings(context.Background(), updated, credentials.Credentials{
		Username: "demo",
		Password: "secret",
		APIKey:   testAPIKey,
	}, CredentialFallbackPlaintext); err != nil {
		t.Fatalf("save settings: %v", err)
	}

	conn := controller.Config().Connection
	if conn.CredentialStorage != config.CredentialStoragePlaintext {
		t.Fatalf("unexpected storage mode: %q", conn.CredentialStorage)
	}
	if conn.Username != "" || conn.Password != "" {
		t.Fatalf("expected plaintext config to drop password credentials, got %#v", conn)
	}
	if conn.APIKey != testAPIKey {
		t.Fatalf("unexpected persisted API key: %q", conn.APIKey)
	}
	if got := controller.SessionCredentials(); got != (credentials.Credentials{APIKey: testAPIKey}) {
		t.Fatalf("unexpected session credentials: %#v", got)
	}
}

func TestControllerUsesAPIKeyAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/auth/login" {
			t.Fatalf("API key auth must never call auth/login")
		}
		switch r.URL.Path {
		case "/api/v2/torrents/info":
			if got := r.Header.Get("Authorization"); got != "Bearer "+testAPIKey {
				t.Fatalf("unexpected Authorization header: %q", got)
			}
			_, _ = io.WriteString(w, `[]`)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	cfg := config.Default()
	cfg.Connection.URL = server.URL
	cfg.Connection.CredentialStorage = config.CredentialStoragePlaintext
	cfg.Connection.AuthMethod = config.AuthMethodAPIKey
	cfg.Connection.APIKey = testAPIKey

	controller := newTestController(t, cfg, credentials.NewStoreForTests(
		func(service, user string) (string, error) { return "", keyring.ErrNotFound },
		func(service, user, password string) error { return nil },
		func(service, user string) error { return nil },
	))

	if got := controller.SessionCredentials(); got != (credentials.Credentials{APIKey: testAPIKey}) {
		t.Fatalf("unexpected session credentials: %#v", got)
	}

	torrents, err := controller.FetchTorrents(context.Background())
	if err != nil {
		t.Fatalf("fetch torrents: %v", err)
	}
	if len(torrents) != 0 {
		t.Fatalf("unexpected torrents: %#v", torrents)
	}
}

func TestControllerRejectsMissingAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected request to %s", r.URL.Path)
	}))
	defer server.Close()

	cfg := config.Default()
	cfg.Connection.URL = server.URL
	cfg.Connection.CredentialStorage = config.CredentialStoragePlaintext
	cfg.Connection.AuthMethod = config.AuthMethodAPIKey

	controller := newTestController(t, cfg, credentials.NewStoreForTests(
		func(service, user string) (string, error) { return "", keyring.ErrNotFound },
		func(service, user, password string) error { return nil },
		func(service, user string) error { return nil },
	))

	_, err := controller.FetchTorrents(context.Background())
	if err == nil {
		t.Fatal("expected missing API key error")
	}
	if !strings.Contains(err.Error(), "no API key is stored") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestControllerIgnoresAPIKeyInPasswordMode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = io.WriteString(w, "Ok.")
		case "/api/v2/torrents/info":
			if got := r.Header.Get("Authorization"); got != "" {
				t.Fatalf("did not expect Authorization header in password mode, got %q", got)
			}
			_, _ = io.WriteString(w, `[]`)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	cfg := config.Default()
	cfg.Connection.URL = server.URL
	cfg.Connection.AuthMethod = config.AuthMethodPassword
	cfg.Connection.CredentialStorage = config.CredentialStoragePlaintext
	cfg.Connection.Username = "demo"
	cfg.Connection.Password = "secret"
	cfg.Connection.APIKey = testAPIKey

	controller := newTestController(t, cfg, credentials.NewStoreForTests(
		func(service, user string) (string, error) { return "", keyring.ErrNotFound },
		func(service, user, password string) error { return nil },
		func(service, user string) error { return nil },
	))

	torrents, err := controller.FetchTorrents(context.Background())
	if err != nil {
		t.Fatalf("fetch torrents: %v", err)
	}
	if len(torrents) != 0 {
		t.Fatalf("unexpected torrents: %#v", torrents)
	}
}

func TestNewControllerCanonicalizesKeychainCredentials(t *testing.T) {
	cfg := config.Default()
	cfg.Connection.AuthMethod = config.AuthMethodPassword
	cfg.Connection.CredentialStorage = config.CredentialStorageKeychain

	store := credentials.NewStoreForTests(
		func(service, user string) (string, error) {
			// Simulates a store written while another auth method was active.
			return `{"username":"demo","password":"secret","api_key":"` + testAPIKey + `"}`, nil
		},
		func(service, user, password string) error { return nil },
		func(service, user string) error { return nil },
	)

	controller := newTestController(t, cfg, store)

	want := credentials.Credentials{Username: "demo", Password: "secret"}
	if got := controller.SessionCredentials(); got != want {
		t.Fatalf("expected read-side canonicalization to drop the API key, got %#v", got)
	}
}

func TestNewControllerMigratesLegacyPlaintextAPIKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := config.Default()
	cfg.Connection.AuthMethod = config.AuthMethodAPIKey
	cfg.Connection.APIKey = testAPIKey
	if err := config.Save(path, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	store, raw := newCapturingKeychainStore()

	controller, err := newController(path, slog.New(slog.NewTextHandler(io.Discard, nil)), store)
	if err != nil {
		t.Fatalf("new controller: %v", err)
	}
	controller.platform = nil

	if controller.Config().Connection.CredentialStorage != config.CredentialStorageKeychain {
		t.Fatalf("unexpected storage mode: %q", controller.Config().Connection.CredentialStorage)
	}
	if controller.Config().Connection.APIKey != "" {
		t.Fatalf("expected config API key to be scrubbed: %#v", controller.Config().Connection)
	}
	stored := decodeStoredPayload(t, *raw)
	if stored.APIKey != testAPIKey || stored.Username != "" || stored.Password != "" {
		t.Fatalf("unexpected migrated payload: %#v", stored)
	}
	if got := controller.SessionCredentials(); got != (credentials.Credentials{APIKey: testAPIKey}) {
		t.Fatalf("unexpected session credentials: %#v", got)
	}
}

func TestSetTorrentLocationHappyPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = io.WriteString(w, "Ok.")
		case "/api/v2/torrents/setLocation":
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method: %s", r.Method)
			}
			data, _ := io.ReadAll(r.Body)
			values, _ := url.ParseQuery(string(data))
			if got := values.Get("location"); got != "/data/new" {
				t.Fatalf("unexpected location: got %q want %q", got, "/data/new")
			}
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	cfg := config.Default()
	cfg.Connection.URL = server.URL
	cfg.Connection.AuthMethod = config.AuthMethodPassword

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := config.Save(path, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	store := credentials.NewStoreForTests(
		func(service, user string) (string, error) { return "", nil },
		func(service, user, password string) error { return nil },
		func(service, user string) error { return nil },
	)

	controller, err := newController(path, slog.New(slog.NewTextHandler(io.Discard, nil)), store)
	if err != nil {
		t.Fatalf("new controller: %v", err)
	}
	controller.platform = nil
	controller.logger = slog.New(slog.NewTextHandler(io.Discard, nil))

	err = controller.SetTorrentLocation(context.Background(), []string{"abc123"}, "/data/new")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(controller.config.UI.RecentSavePaths) != 1 {
		t.Fatalf("expected 1 recent path, got %d: %#v", len(controller.config.UI.RecentSavePaths), controller.config.UI.RecentSavePaths)
	}
	if controller.config.UI.RecentSavePaths[0] != "/data/new" {
		t.Fatalf("unexpected recent path: %q", controller.config.UI.RecentSavePaths[0])
	}
}

func TestFetchServerVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = io.WriteString(w, "Ok.")
		case "/api/v2/app/version":
			_, _ = io.WriteString(w, "5.1.2")
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	cfg := config.Default()
	cfg.Connection.URL = server.URL
	cfg.Connection.AuthMethod = config.AuthMethodPassword

	controller := newTestController(t, cfg, credentials.NewStoreForTests(
		func(service, user string) (string, error) { return "", keyring.ErrNotFound },
		func(service, user, password string) error { return nil },
		func(service, user string) error { return nil },
	))

	version, err := controller.FetchServerVersion(context.Background())
	if err != nil {
		t.Fatalf("fetch server version: %v", err)
	}
	if version != "5.1.2" {
		t.Fatalf("unexpected version: %q", version)
	}
}

func TestClientCacheReusesSingleLogin(t *testing.T) {
	var logins atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			logins.Add(1)
			_, _ = io.WriteString(w, "Ok.")
		case "/api/v2/torrents/info":
			_, _ = io.WriteString(w, "[]")
		case "/api/v2/torrents/properties":
			_, _ = io.WriteString(w, "{}")
		case "/api/v2/sync/torrentPeers":
			_, _ = io.WriteString(w, "{}")
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	cfg := config.Default()
	cfg.Connection.URL = server.URL
	cfg.Connection.AuthMethod = config.AuthMethodPassword

	controller := newTestController(t, cfg, credentials.NewStoreForTests(
		func(service, user string) (string, error) { return "", keyring.ErrNotFound },
		func(service, user, password string) error { return nil },
		func(service, user string) error { return nil },
	))

	ctx := context.Background()
	if _, err := controller.FetchTorrents(ctx); err != nil {
		t.Fatalf("fetch torrents: %v", err)
	}
	if _, err := controller.FetchTorrentProperties(ctx, "abc"); err != nil {
		t.Fatalf("fetch properties: %v", err)
	}
	if _, err := controller.FetchTorrentPeers(ctx, "abc", 0); err != nil {
		t.Fatalf("fetch peers: %v", err)
	}

	if got := logins.Load(); got != 1 {
		t.Fatalf("expected exactly one auth/login across requests, got %d", got)
	}
}

func TestClientCacheInvalidatedOnConnectionChange(t *testing.T) {
	var logins atomic.Int32
	newServer := func() *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api/v2/auth/login":
				logins.Add(1)
				_, _ = io.WriteString(w, "Ok.")
			case "/api/v2/torrents/info":
				_, _ = io.WriteString(w, "[]")
			default:
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
		}))
	}
	server1 := newServer()
	defer server1.Close()
	server2 := newServer()
	defer server2.Close()

	cfg := config.Default()
	cfg.Connection.URL = server1.URL
	cfg.Connection.AuthMethod = config.AuthMethodPassword

	controller := newTestController(t, cfg, credentials.NewStoreForTests(
		func(service, user string) (string, error) { return "", keyring.ErrNotFound },
		func(service, user, password string) error { return nil },
		func(service, user string) error { return nil },
	))

	ctx := context.Background()
	if _, err := controller.FetchTorrents(ctx); err != nil {
		t.Fatalf("fetch torrents from first server: %v", err)
	}

	updated := controller.Config()
	updated.Connection.URL = server2.URL
	if _, err := controller.SaveSettings(ctx, updated, credentials.Credentials{
		Username: "demo",
		Password: "secret",
	}, CredentialFallbackUnspecified); err != nil {
		t.Fatalf("save settings: %v", err)
	}

	if _, err := controller.FetchTorrents(ctx); err != nil {
		t.Fatalf("fetch torrents from second server: %v", err)
	}

	if got := logins.Load(); got != 2 {
		t.Fatalf("expected one auth/login per server, got %d", got)
	}
}

// Regression for the config lost-update race: rememberSavePath used to persist
// a full config snapshot taken before a concurrent SaveSettings, reverting the
// saved connection URL. Recent-path writes must always start from the latest
// config, so the settings save survives any interleaving. The settings form
// owns the UI section, so recent-path survival depends on write order and is
// deliberately not asserted.
func TestRememberSavePathDoesNotRevertConcurrentSettingsSave(t *testing.T) {
	// Plaintext credentials plus an unavailable keychain keep SaveSettings on
	// the plain persist path: no keychain writes, no fallback decision.
	cfg := config.Default()
	cfg.Connection.URL = "https://old.example.invalid"
	cfg.Connection.CredentialStorage = config.CredentialStoragePlaintext
	cfg.Connection.Username = "demo"
	cfg.Connection.Password = "secret"
	controller := newTestController(t, cfg, credentials.NewStoreForTests(
		func(service, user string) (string, error) { return "", keyring.ErrNotFound },
		func(service, user, password string) error { return nil },
		func(service, user string) error { return nil },
	))

	settingsDone := make(chan error, 1)
	go func() {
		updated := controller.Config()
		updated.Connection.URL = "https://new.example.invalid"
		_, err := controller.SaveSettings(context.Background(), updated, credentials.Credentials{
			Username: "demo",
			Password: "secret",
		}, CredentialFallbackUnspecified)
		settingsDone <- err
	}()

	// Hammer rememberSavePath for the whole duration of the settings save so
	// recent-path writes land both before and after it. The iteration cap keeps
	// the test bounded even if the settings save never needs the lock.
	var wg sync.WaitGroup
	for worker := range 8 {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := range 200 {
				select {
				case <-settingsDone:
					return
				default:
				}
				controller.rememberSavePath(fmt.Sprintf("/data/worker-%d-%d", worker, i), "test")
			}
		}(worker)
	}

	err := <-settingsDone
	// The buffered send was consumed above; close the channel so every worker
	// observes the completion instead of racing main for the single value.
	close(settingsDone)
	if err != nil {
		t.Fatalf("save settings: %v", err)
	}
	wg.Wait()

	final := controller.Config()
	if final.Connection.URL != "https://new.example.invalid" {
		t.Fatalf("concurrent recent-path updates reverted the saved URL: %q", final.Connection.URL)
	}
	disk, err := config.Load(controller.configPath)
	if err != nil {
		t.Fatalf("load persisted config: %v", err)
	}
	if disk.Connection.URL != "https://new.example.invalid" {
		t.Fatalf("persisted URL was reverted by concurrent recent-path updates: %q", disk.Connection.URL)
	}
}

func newTestController(t *testing.T, cfg config.AppConfig, store credentials.Store) *Controller {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := config.Save(path, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	controller, err := newController(path, slog.Default(), store)
	if err != nil {
		t.Fatalf("new controller: %v", err)
	}
	controller.platform = nil
	controller.logger = slog.New(slog.NewTextHandler(io.Discard, nil))

	return controller
}
