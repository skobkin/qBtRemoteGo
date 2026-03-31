package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"github.com/skobkin/qbtremotego/internal/config"
	"github.com/skobkin/qbtremotego/internal/credentials"
	"github.com/skobkin/qbtremotego/internal/qbt"
)

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
	if got := connectionCredentialStorageText(config.CredentialStorageKeychain, credentials.Status{
		Backend: "Secret Service",
		State:   credentials.StateAvailable,
	}, credentials.Credentials{}); got != "System keychain (Secret Service)" {
		t.Fatalf("unexpected keychain text: %q", got)
	}

	if got := connectionCredentialStorageText(config.CredentialStoragePlaintext, credentials.Status{}, credentials.Credentials{}); got != "Plain text config file" {
		t.Fatalf("unexpected plaintext text: %q", got)
	}

	if got := connectionCredentialStorageText(config.CredentialStorageNone, credentials.Status{}, credentials.Credentials{
		Username: "demo",
	}); got != "Session only" {
		t.Fatalf("unexpected session-only text: %q", got)
	}
}

func TestConnectionCredentialWarningText(t *testing.T) {
	if got := connectionCredentialWarningText(config.CredentialStorageKeychain, credentials.Status{
		Backend: "Secret Service",
		State:   credentials.StateLocked,
		Message: "keychain locked",
	}, credentials.Credentials{}); got == "" {
		t.Fatal("expected keychain warning")
	}

	if got := connectionCredentialWarningText(config.CredentialStoragePlaintext, credentials.Status{}, credentials.Credentials{}); got == "" {
		t.Fatal("expected plaintext warning")
	}

	if got := connectionCredentialWarningText(config.CredentialStorageNone, credentials.Status{}, credentials.Credentials{
		Username: "demo",
	}); got == "" {
		t.Fatal("expected session-only message")
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
