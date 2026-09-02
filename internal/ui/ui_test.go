package ui

import (
	"errors"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	appcore "github.com/skobkin/qbtremotego/internal/app"
	"github.com/skobkin/qbtremotego/internal/config"
	"github.com/skobkin/qbtremotego/internal/credentials"
	"github.com/skobkin/qbtremotego/internal/qbt"
)

func TestMainWindowTitle(t *testing.T) {
	original := appcore.Version
	t.Cleanup(func() {
		appcore.Version = original
	})

	appcore.Version = "0.5.0"

	tests := []struct {
		name          string
		state         connectionState
		serverVersion string
		want          string
	}{
		{
			name:  "unknown state before first refresh",
			state: connectionStateUnknown,
			want:  "qBtRemoteGo 0.5.0",
		},
		{
			name:          "connected with server version",
			state:         connectionStateConnected,
			serverVersion: "5.2.5",
			want:          "qBtRemoteGo 0.5.0 -> qBittorrent 5.2.5",
		},
		{
			name:          "connected trims server version",
			state:         connectionStateConnected,
			serverVersion: " 5.2.5\n",
			want:          "qBtRemoteGo 0.5.0 -> qBittorrent 5.2.5",
		},
		{
			name:  "connected without server version",
			state: connectionStateConnected,
			want:  "qBtRemoteGo 0.5.0",
		},
		{
			name:  "disconnected",
			state: connectionStateDisconnected,
			want:  "qBtRemoteGo 0.5.0 x disconnected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mainWindowTitle(tt.state, tt.serverVersion); got != tt.want {
				t.Fatalf("mainWindowTitle(%v, %q) = %q, want %q", tt.state, tt.serverVersion, got, tt.want)
			}
		})
	}
}

func TestStatusTextUsesEmojiMarkers(t *testing.T) {
	app := &application{
		allTorrents:     []qbt.Torrent{{Hash: "a"}, {Hash: "b"}},
		visibleTorrents: []qbt.Torrent{{Hash: "a"}},
		transfer: qbt.TransferInfo{
			DownloadSpeed: 2048,
			UploadSpeed:   1024,
			DownloadLimit: 0,
			UploadLimit:   1536,
		},
		serverState: qbt.ServerState{
			FreeSpaceOnDisk: 2048,
		},
		serverStateKnown: true,
		lastError:        "boom",
	}

	want := "📦 2 | 🔎 1 | ⬇️ 2.0 KiB/s | ⬆️ 1.0 KiB/s | Lim ⬇️:∞ ⬆️:1.5 KiB/s | Free 2.0 KiB | Last error: boom"
	if got := app.statusText(); got != want {
		t.Fatalf("unexpected status text:\n got: %q\nwant: %q", got, want)
	}
}

func TestStatusTextShowsKeychainWait(t *testing.T) {
	wait := "Waiting for system keychain (Secret Service)…"
	app := &application{
		allTorrents:     []qbt.Torrent{{Hash: "a"}},
		visibleTorrents: []qbt.Torrent{{Hash: "a"}},
		credentialWait:  wait,
		lastError:       "boom",
	}

	got := app.statusText()
	want := "📦 1 | 🔎 1 | ⬇️ 0 B/s | ⬆️ 0 B/s | Lim ⬇️:∞ ⬆️:∞ | Waiting for system keychain (Secret Service)… | Last error: boom"
	if got != want {
		t.Fatalf("unexpected status text:\n got: %q\nwant: %q", got, want)
	}

	// The waiting hint renders without a trailing "Last error" when no
	// connection failure was recorded alongside it.
	app.lastError = ""
	if strings.Contains(app.statusText(), "Last error") {
		t.Fatalf("expected the waiting hint to replace the error, got %q", app.statusText())
	}
}

func TestKeychainWaitingText(t *testing.T) {
	if got, want := keychainWaitingText(credentials.Status{Backend: "Secret Service"}), "Waiting for system keychain (Secret Service)…"; got != want {
		t.Fatalf("unexpected waiting text: got %q, want %q", got, want)
	}
	if got, want := keychainWaitingText(credentials.Status{Backend: "  "}), "Waiting for system keychain…"; got != want {
		t.Fatalf("unexpected fallback waiting text: got %q, want %q", got, want)
	}
	if got, want := keychainWaitingText(credentials.Status{}), "Waiting for system keychain…"; got != want {
		t.Fatalf("unexpected empty-status waiting text: got %q, want %q", got, want)
	}
}

func TestNoteFetchErrorClassification(t *testing.T) {
	t.Run("typed waiting error becomes the hint and clears the error", func(t *testing.T) {
		app := &application{lastError: "connection refused"}

		app.noteFetchError(&appcore.CredentialUnavailableError{Status: credentials.Status{
			Backend: "Secret Service",
			State:   credentials.StateUnavailable,
		}})

		if app.credentialWait != "Waiting for system keychain (Secret Service)…" {
			t.Fatalf("unexpected waiting hint: %q", app.credentialWait)
		}
		if app.lastError != "" {
			t.Fatalf("expected the stale error to be cleared, got %q", app.lastError)
		}
	})

	t.Run("plain error keeps the first failure", func(t *testing.T) {
		app := &application{}

		app.noteFetchError(errors.New("first failure"))
		app.noteFetchError(errors.New("second failure"))

		if app.lastError != "first failure" {
			t.Fatalf("expected first-error-wins, got %q", app.lastError)
		}
		if app.credentialWait != "" {
			t.Fatalf("unexpected waiting hint: %q", app.credentialWait)
		}
	})

	t.Run("plain error replaces a stale waiting hint", func(t *testing.T) {
		stale := "Waiting for system keychain…"
		app := &application{credentialWait: stale}

		app.noteFetchError(errors.New("no API key is stored"))

		if app.lastError != "no API key is stored" {
			t.Fatalf("expected the stale hint to be superseded, got lastError %q", app.lastError)
		}
		if app.credentialWait != "" {
			t.Fatalf("expected the stale hint to be cleared, got %q", app.credentialWait)
		}
	})

	t.Run("nil error is ignored", func(t *testing.T) {
		app := &application{lastError: "kept"}

		app.noteFetchError(nil)

		if app.lastError != "kept" {
			t.Fatalf("nil error cleared the recorded failure: %q", app.lastError)
		}
	})
}

func TestBuildAddTorrentFormSections(t *testing.T) {
	basic, advanced := buildAddTorrentFormSections(addTorrentFormControls{
		sourceSelect:       widget.NewLabel("source-select"),
		sourceContainer:    widget.NewLabel("source-container"),
		savePathEntry:      widget.NewLabel("save-path"),
		categoryEntry:      widget.NewLabel("category"),
		startCheck:         widget.NewLabel("start"),
		managementSelect:   widget.NewLabel("management"),
		renameEntry:        widget.NewLabel("rename"),
		tagsEntry:          widget.NewLabel("tags"),
		topOfQueue:         widget.NewLabel("top"),
		stopSelect:         widget.NewLabel("stop"),
		skipHashCheck:      widget.NewLabel("skip"),
		contentLayout:      widget.NewLabel("layout"),
		sequential:         widget.NewLabel("sequential"),
		firstLastPieces:    widget.NewLabel("pieces"),
		downloadLimitEntry: widget.NewLabel("download"),
		uploadLimitEntry:   widget.NewLabel("upload"),
	})

	if got, want := formItemTexts(basic), []string{
		"Source type",
		"Source",
		"Save location",
		"Category",
		"Start torrent",
	}; !equalStrings(got, want) {
		t.Fatalf("unexpected basic fields:\n got: %#v\nwant: %#v", got, want)
	}

	if got, want := formItemTexts(advanced), []string{
		"Torrent management mode",
		"Name override",
		"Tags",
		"Top of queue",
		"Stop condition",
		"Skip hash check",
		"Content layout",
		"Download sequentially",
		"Download first and last pieces first",
		"Limit download rate (KiB/s)",
		"Limit upload rate (KiB/s)",
	}; !equalStrings(got, want) {
		t.Fatalf("unexpected advanced fields:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestNewAddTorrentAdvancedAccordion(t *testing.T) {
	accordion, item := newAddTorrentAdvancedAccordion(widget.NewLabel("advanced"), true)

	if item.Title != "Advanced" {
		t.Fatalf("unexpected title: %q", item.Title)
	}
	if !item.Open {
		t.Fatal("expected advanced section to start opened")
	}
	if len(accordion.Items) != 1 || accordion.Items[0] != item {
		t.Fatalf("unexpected accordion items: %#v", accordion.Items)
	}
}

func TestAddTorrentWindowSize(t *testing.T) {
	if got := addTorrentWindowSize(false); got != fyne.NewSize(720, 480) {
		t.Fatalf("unexpected collapsed size: %#v", got)
	}
	if got := addTorrentWindowSize(true); got != fyne.NewSize(720, 680) {
		t.Fatalf("unexpected expanded size: %#v", got)
	}
}

func TestNeedsConnectionSetup(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{
			name: "blank URL requires setup",
			url:  "",
			want: true,
		},
		{
			name: "whitespace URL requires setup",
			url:  " \t\n ",
			want: true,
		},
		{
			name: "non-empty URL does not require setup",
			url:  "https://example.invalid/qbt",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.Default()
			cfg.Connection.URL = tt.url

			if got := needsConnectionSetup(cfg); got != tt.want {
				t.Fatalf("unexpected setup requirement: got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNeedsConnectionSetupIgnoresMissingAPIKey(t *testing.T) {
	cfg := config.Default()
	cfg.Connection.URL = "https://example.invalid/qbt"
	cfg.Connection.AuthMethod = config.AuthMethodAPIKey

	// Deliberate: setup is driven by the URL only. A missing API key in
	// session-only mode must not force the settings window on every launch;
	// the failure surfaces at request time instead.
	if needsConnectionSetup(cfg) {
		t.Fatal("expected a configured URL with API key auth to not require connection setup")
	}
}

func TestMergeInvocationBatches(t *testing.T) {
	pending := appcore.InvocationBatch{
		MagnetLinks:  []string{"magnet:?xt=urn:btih:first"},
		TorrentFiles: []string{"/tmp/first.torrent"},
	}
	next := appcore.InvocationBatch{
		MagnetLinks:  []string{"magnet:?xt=urn:btih:second"},
		TorrentFiles: []string{"/tmp/second.torrent"},
	}

	got := mergeInvocationBatches(pending, next)

	if !equalStrings(got.MagnetLinks, []string{
		"magnet:?xt=urn:btih:first",
		"magnet:?xt=urn:btih:second",
	}) {
		t.Fatalf("unexpected magnet links: %#v", got.MagnetLinks)
	}
	if !equalStrings(got.TorrentFiles, []string{
		"/tmp/first.torrent",
		"/tmp/second.torrent",
	}) {
		t.Fatalf("unexpected torrent files: %#v", got.TorrentFiles)
	}
}

func TestInitialAddDialogSavePath(t *testing.T) {
	t.Run("uses remembered path when enabled", func(t *testing.T) {
		cfg := config.UIConfig{
			RememberLastSaveLocation: true,
			RecentSavePaths:          []string{"/data/recent", "/data/older"},
		}

		path, shouldFetchDefault := initialAddDialogSavePath(cfg)

		if path != "/data/recent" {
			t.Fatalf("unexpected initial path: %q", path)
		}
		if shouldFetchDefault {
			t.Fatal("expected remembered path to skip default fetch")
		}
	})

	t.Run("falls back to lazy default fetch when history is empty", func(t *testing.T) {
		path, shouldFetchDefault := initialAddDialogSavePath(config.UIConfig{
			RememberLastSaveLocation: true,
		})

		if path != "" {
			t.Fatalf("unexpected initial path: %q", path)
		}
		if !shouldFetchDefault {
			t.Fatal("expected empty history to trigger default fetch")
		}
	})

	t.Run("ignores remembered history when disabled", func(t *testing.T) {
		path, shouldFetchDefault := initialAddDialogSavePath(config.UIConfig{
			RememberLastSaveLocation: false,
			RecentSavePaths:          []string{"/data/recent"},
		})

		if path != "" {
			t.Fatalf("unexpected initial path: %q", path)
		}
		if !shouldFetchDefault {
			t.Fatal("expected disabled remember-last setting to trigger default fetch")
		}
	})
}

func TestShouldApplyLazySavePath(t *testing.T) {
	if !shouldApplyLazySavePath("", "/data/default") {
		t.Fatal("expected blank entry to accept fetched save path")
	}
	if shouldApplyLazySavePath("/data/current", "/data/default") {
		t.Fatal("expected existing entry text to block fetched save path")
	}
	if shouldApplyLazySavePath("", "   ") {
		t.Fatal("expected blank fetched save path to be ignored")
	}
}

func TestConnectionCredentialStorageText(t *testing.T) {
	if got := connectionCredentialStorageText(config.AuthMethodPassword, config.CredentialStorageKeychain, credentials.Status{
		Backend: "Secret Service",
		State:   credentials.StateAvailable,
	}, credentials.Credentials{}); got != "System keychain (Secret Service)" {
		t.Fatalf("unexpected keychain text: %q", got)
	}

	if got := connectionCredentialStorageText(config.AuthMethodPassword, config.CredentialStoragePlaintext, credentials.Status{}, credentials.Credentials{}); got != "Plain text config file" {
		t.Fatalf("unexpected plaintext text: %q", got)
	}

	if got := connectionCredentialStorageText(config.AuthMethodPassword, config.CredentialStorageNone, credentials.Status{}, credentials.Credentials{
		Username: "demo",
	}); got != "Session only" {
		t.Fatalf("unexpected session-only text: %q", got)
	}

	if got := connectionCredentialStorageText(config.AuthMethodAPIKey, config.CredentialStorageNone, credentials.Status{}, credentials.Credentials{
		APIKey: "qbt_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}); got != "Session only" {
		t.Fatalf("unexpected API key session-only text: %q", got)
	}

	if got := connectionCredentialStorageText(config.AuthMethodAPIKey, config.CredentialStorageNone, credentials.Status{}, credentials.Credentials{
		Username: "demo",
		Password: "secret",
	}); got != "Not saved" {
		t.Fatalf("expected API key mode to ignore password credentials: %q", got)
	}
}

func TestConnectionCredentialWarningText(t *testing.T) {
	const deprecation = authMethodPasswordDeprecationNotice

	if got := connectionCredentialWarningText(config.AuthMethodPassword, config.CredentialStorageKeychain, credentials.Status{
		Backend: "Secret Service",
		State:   credentials.StateLocked,
		Message: "keychain locked",
	}, credentials.Credentials{}); got == "" || !strings.Contains(got, deprecation) || !strings.Contains(got, "keychain locked") {
		t.Fatalf("expected deprecation notice and keychain warning, got %q", got)
	}

	if got := connectionCredentialWarningText(config.AuthMethodPassword, config.CredentialStoragePlaintext, credentials.Status{}, credentials.Credentials{}); got == "" || !strings.Contains(got, deprecation) || !strings.Contains(got, "plain text") {
		t.Fatalf("expected deprecation notice and plaintext warning, got %q", got)
	}

	if got := connectionCredentialWarningText(config.AuthMethodPassword, config.CredentialStorageNone, credentials.Status{}, credentials.Credentials{
		Username: "demo",
	}); got == "" || !strings.Contains(got, deprecation) || !strings.Contains(got, "in memory") {
		t.Fatalf("expected deprecation notice and session-only message, got %q", got)
	}

	if got := connectionCredentialWarningText(config.AuthMethodPassword, config.CredentialStorageKeychain, credentials.Status{
		State: credentials.StateAvailable,
	}, credentials.Credentials{}); got != deprecation {
		t.Fatalf("expected only the deprecation notice, got %q", got)
	}

	if got := connectionCredentialWarningText(config.AuthMethodAPIKey, config.CredentialStorageKeychain, credentials.Status{
		State: credentials.StateAvailable,
	}, credentials.Credentials{}); got != "" {
		t.Fatalf("expected no warning for API key auth, got %q", got)
	}

	if got := connectionCredentialWarningText(config.AuthMethodAPIKey, config.CredentialStoragePlaintext, credentials.Status{}, credentials.Credentials{}); got != "Warning: The API key is stored in plain text in the local config file." {
		t.Fatalf("unexpected API key plaintext warning: %q", got)
	}

	if got := connectionCredentialWarningText(config.AuthMethodAPIKey, config.CredentialStorageNone, credentials.Status{}, credentials.Credentials{
		APIKey: "qbt_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}); got != "The API key is stored only in memory for this run." {
		t.Fatalf("unexpected API key session-only message: %q", got)
	}
}

func TestAuthMethodLabelRoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		method config.AuthMethod
		label  string
	}{
		{name: "password", method: config.AuthMethodPassword, label: authMethodLabelPassword},
		{name: "empty falls back to password", method: "", label: authMethodLabelPassword},
		{name: "api key", method: config.AuthMethodAPIKey, label: authMethodLabelAPIKey},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := authMethodLabel(tt.method); got != tt.label {
				t.Fatalf("unexpected label: got %q, want %q", got, tt.label)
			}
			wantMethod := tt.method
			if wantMethod == "" {
				wantMethod = config.AuthMethodPassword
			}
			if got := authMethodKey(tt.label); got != wantMethod {
				t.Fatalf("unexpected round-tripped method: got %q, want %q", got, wantMethod)
			}
		})
	}

	if got := authMethodKey("some unknown entry"); got != config.AuthMethodPassword {
		t.Fatalf("expected unknown label to fall back to password auth: %q", got)
	}
}

func formItemTexts(items []*widget.FormItem) []string {
	texts := make([]string, 0, len(items))
	for _, item := range items {
		texts = append(texts, item.Text)
	}
	return texts
}

func equalStrings(got []string, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
