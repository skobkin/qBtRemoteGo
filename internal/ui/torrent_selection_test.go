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
