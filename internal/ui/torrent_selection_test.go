package ui

import (
	"testing"

	"fyne.io/fyne/v2"

	"github.com/skobkin/qbtremotego/internal/qbt"
)

func TestApplyTorrentSelectionModifiers(t *testing.T) {
	app := &application{
		selection: map[string]bool{},
		visibleTorrents: []qbt.Torrent{
			{Hash: "a"},
			{Hash: "b"},
			{Hash: "c"},
			{Hash: "d"},
		},
	}

	app.applyTorrentSelection("b", 0)
	assertSelection(t, app, "b")
	if app.selectionAnchor != "b" {
		t.Fatalf("unexpected anchor after primary selection: %q", app.selectionAnchor)
	}

	app.applyTorrentSelection("d", fyne.KeyModifierControl)
	assertSelection(t, app, "b", "d")
	if app.selectionAnchor != "d" {
		t.Fatalf("unexpected anchor after toggle selection: %q", app.selectionAnchor)
	}

	app.applyTorrentSelection("c", fyne.KeyModifierShift)
	assertSelection(t, app, "c", "d")
	if app.selectionAnchor != "d" {
		t.Fatalf("unexpected anchor after range selection: %q", app.selectionAnchor)
	}
}

func TestPrepareTorrentContextSelection(t *testing.T) {
	app := &application{
		selection: map[string]bool{
			"a": true,
			"b": true,
		},
		selectionAnchor: "b",
	}

	app.prepareTorrentContextSelection("a")
	assertSelection(t, app, "a", "b")
	if app.selectionAnchor != "b" {
		t.Fatalf("unexpected anchor when context target was already selected: %q", app.selectionAnchor)
	}

	app.prepareTorrentContextSelection("c")
	assertSelection(t, app, "c")
	if app.selectionAnchor != "c" {
		t.Fatalf("unexpected anchor when context target replaced selection: %q", app.selectionAnchor)
	}
}

func TestSelectedTorrentNamesTextSingle(t *testing.T) {
	app := &application{
		selection: map[string]bool{"a": true},
		allTorrents: []qbt.Torrent{
			{Hash: "a", Name: "Alpha"},
		},
		visibleTorrents: []qbt.Torrent{
			{Hash: "a", Name: "Alpha"},
		},
	}

	got, ok := app.selectedTorrentNamesText()
	if !ok {
		t.Fatal("expected copyable torrent name")
	}
	if got != "Alpha" {
		t.Fatalf("unexpected names text: %q", got)
	}
}

func TestSelectedTorrentNamesTextPreservesSelectedHashesOrder(t *testing.T) {
	app := &application{
		selection: map[string]bool{
			"a": true,
			"b": true,
			"c": true,
		},
		allTorrents: []qbt.Torrent{
			{Hash: "a", Name: "Alpha"},
			{Hash: "b", Name: "Beta"},
			{Hash: "c", Name: "Gamma"},
		},
		visibleTorrents: []qbt.Torrent{
			{Hash: "b", Name: "Beta"},
			{Hash: "a", Name: "Alpha"},
		},
	}

	got, ok := app.selectedTorrentNamesText()
	if !ok {
		t.Fatal("expected copyable torrent names")
	}
	if got != "Beta\nAlpha\nGamma" {
		t.Fatalf("unexpected names text: %q", got)
	}
}

func TestSelectedTorrentMagnetLinksTextSkipsEmptyValues(t *testing.T) {
	app := &application{
		selection: map[string]bool{
			"a": true,
			"b": true,
			"c": true,
		},
		allTorrents: []qbt.Torrent{
			{Hash: "a", MagnetURI: "magnet:?xt=urn:btih:a"},
			{Hash: "b"},
			{Hash: "c", MagnetURI: "magnet:?xt=urn:btih:c"},
		},
		visibleTorrents: []qbt.Torrent{
			{Hash: "a"},
			{Hash: "b"},
			{Hash: "c"},
		},
	}

	got, ok := app.selectedTorrentMagnetLinksText()
	if !ok {
		t.Fatal("expected copyable magnet links")
	}
	if got != "magnet:?xt=urn:btih:a\nmagnet:?xt=urn:btih:c" {
		t.Fatalf("unexpected magnet links text: %q", got)
	}
}

func TestSelectedTorrentMagnetLinksTextAllEmpty(t *testing.T) {
	app := &application{
		selection: map[string]bool{
			"a": true,
			"b": true,
		},
		allTorrents: []qbt.Torrent{
			{Hash: "a"},
			{Hash: "b", MagnetURI: " "},
		},
		visibleTorrents: []qbt.Torrent{
			{Hash: "a"},
			{Hash: "b"},
		},
	}

	got, ok := app.selectedTorrentMagnetLinksText()
	if ok {
		t.Fatalf("expected no copyable magnet links, got %q", got)
	}
	if got != "" {
		t.Fatalf("unexpected magnet links text: %q", got)
	}
}

func TestCommonSelectedSavePath(t *testing.T) {
	tests := []struct {
		name     string
		hashes   []string
		torrents []qbt.Torrent
		want     string
	}{
		{
			name:   "single selected torrent",
			hashes: []string{"a"},
			torrents: []qbt.Torrent{
				{Hash: "a", SavePath: "/data/main"},
			},
			want: "/data/main",
		},
		{
			name:   "multiple selected torrents with same save path",
			hashes: []string{"a", "b"},
			torrents: []qbt.Torrent{
				{Hash: "a", SavePath: "/data/main"},
				{Hash: "b", SavePath: "/data/main"},
			},
			want: "/data/main",
		},
		{
			name:   "mixed save paths",
			hashes: []string{"a", "b"},
			torrents: []qbt.Torrent{
				{Hash: "a", SavePath: "/data/main"},
				{Hash: "b", SavePath: "/data/other"},
			},
		},
		{
			name:   "empty save path",
			hashes: []string{"a", "b"},
			torrents: []qbt.Torrent{
				{Hash: "a", SavePath: "/data/main"},
				{Hash: "b"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := &application{allTorrents: tc.torrents}
			if got := app.commonSelectedSavePath(tc.hashes); got != tc.want {
				t.Fatalf("unexpected save path: got %q want %q", got, tc.want)
			}
		})
	}
}

func TestSelectedRenameTarget(t *testing.T) {
	tests := []struct {
		name      string
		selection map[string]bool
		torrents  []qbt.Torrent
		wantHash  string
		wantOK    bool
	}{
		{
			name:      "single selected torrent",
			selection: map[string]bool{"a": true},
			torrents: []qbt.Torrent{
				{Hash: "a", Name: "Alpha"},
			},
			wantHash: "a",
			wantOK:   true,
		},
		{
			name: "zero selected torrents",
			torrents: []qbt.Torrent{
				{Hash: "a", Name: "Alpha"},
			},
		},
		{
			name: "multiple selected torrents",
			selection: map[string]bool{
				"a": true,
				"b": true,
			},
			torrents: []qbt.Torrent{
				{Hash: "a", Name: "Alpha"},
				{Hash: "b", Name: "Beta"},
			},
		},
		{
			name:      "selected hash missing from torrent data",
			selection: map[string]bool{"missing": true},
			torrents: []qbt.Torrent{
				{Hash: "a", Name: "Alpha"},
			},
		},
		{
			name: "multiple selected hashes with one missing from torrent data",
			selection: map[string]bool{
				"a":       true,
				"missing": true,
			},
			torrents: []qbt.Torrent{
				{Hash: "a", Name: "Alpha"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := &application{
				selection:       tc.selection,
				allTorrents:     tc.torrents,
				visibleTorrents: tc.torrents,
			}

			got, ok := app.selectedRenameTarget()
			if ok != tc.wantOK {
				t.Fatalf("unexpected ok: got %t want %t", ok, tc.wantOK)
			}
			if got.Hash != tc.wantHash {
				t.Fatalf("unexpected target hash: got %q want %q", got.Hash, tc.wantHash)
			}
		})
	}
}

func TestRelativeDialogSizeUsesParentWidthRatio(t *testing.T) {
	got := relativeDialogSize(fyne.NewSize(1000, 700), fyne.NewSize(240, 120), 0.85)
	if got.Width != 850 || got.Height != 120 {
		t.Fatalf("unexpected dialog size: %#v", got)
	}

	got = relativeDialogSize(fyne.NewSize(200, 700), fyne.NewSize(240, 120), 0.85)
	if got.Width != 240 || got.Height != 120 {
		t.Fatalf("expected minimum size to win, got %#v", got)
	}
}

func TestRelativeDialogSizeRejectsZeroParentWidth(t *testing.T) {
	got := relativeDialogSize(fyne.NewSize(0, 700), fyne.NewSize(240, 120), 0.85)
	if got.Width != 240 || got.Height != 120 {
		t.Fatalf("expected minimum size when parent width is 0, got %#v", got)
	}
}

func TestRelativeDialogSizeRejectsNegativeParentWidth(t *testing.T) {
	got := relativeDialogSize(fyne.NewSize(-100, 700), fyne.NewSize(240, 120), 0.85)
	if got.Width != 240 || got.Height != 120 {
		t.Fatalf("expected minimum size when parent width is negative, got %#v", got)
	}
}

func TestRelativeDialogSizeRejectsZeroWidthRatio(t *testing.T) {
	got := relativeDialogSize(fyne.NewSize(1000, 700), fyne.NewSize(240, 120), 0)
	if got.Width != 240 || got.Height != 120 {
		t.Fatalf("expected minimum size when width ratio is 0, got %#v", got)
	}
}

func TestRelativeDialogSizeRejectsNegativeWidthRatio(t *testing.T) {
	got := relativeDialogSize(fyne.NewSize(1000, 700), fyne.NewSize(240, 120), -0.5)
	if got.Width != 240 || got.Height != 120 {
		t.Fatalf("expected minimum size when width ratio is negative, got %#v", got)
	}
}

func TestPruneSelectionToVisible(t *testing.T) {
	app := &application{
		selection: map[string]bool{
			"b":    true,
			"gone": true,
		},
		selectionAnchor: "gone",
		visibleTorrents: []qbt.Torrent{
			{Hash: "a"},
			{Hash: "b"},
			{Hash: "c"},
		},
	}

	app.pruneSelectionToVisible()

	assertSelection(t, app, "b")
	if app.selectionAnchor != "b" {
		t.Fatalf("unexpected anchor after pruning: %q", app.selectionAnchor)
	}
}

func assertSelection(t *testing.T, app *application, hashes ...string) {
	t.Helper()

	expected := make(map[string]bool, len(hashes))
	for _, hash := range hashes {
		expected[hash] = true
	}

	if len(app.selection) != len(expected) {
		t.Fatalf("unexpected selection size: got %d want %d", len(app.selection), len(expected))
	}
	for hash := range expected {
		if !app.selection[hash] {
			t.Fatalf("expected %q to be selected", hash)
		}
	}
	for hash := range app.selection {
		if !expected[hash] {
			t.Fatalf("did not expect %q to be selected", hash)
		}
	}
}
