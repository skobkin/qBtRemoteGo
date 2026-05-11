package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
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

	result, err := controller.SaveSettings(context.Background(), controller.Config(), credentials.Credentials{
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
