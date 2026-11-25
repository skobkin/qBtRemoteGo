package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

func TestSearchEntryEscapeClearsText(t *testing.T) {
	test.NewTempApp(t)
	cleared := 0
	entry := newSearchEntry(func() { cleared++ })
	entry.SetText("ubuntu")

	entry.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})

	if entry.Text != "" {
		t.Fatalf("text %q after Esc, want empty", entry.Text)
	}
	if cleared != 1 {
		t.Fatalf("clear callback ran %d times, want 1", cleared)
	}
}

func TestSearchClearButtonAppliesImmediately(t *testing.T) {
	app := newTorrentTableTestApp(t)
	if app.filterClear.Visible() {
		t.Fatal("clear button visible with an empty search")
	}

	app.filterEntry.SetText("A")
	app.applyFilterQueryNow("A")
	if !app.filterClear.Visible() {
		t.Fatal("clear button hidden with a non-empty search")
	}
	if len(app.visibleTorrents) != 1 {
		t.Fatalf("visible %d torrents for query A, want 1", len(app.visibleTorrents))
	}

	test.Tap(app.filterClear)

	if app.filterEntry.Text != "" || app.filterQuery != "" {
		t.Fatalf("search text %q / query %q after clear, want empty", app.filterEntry.Text, app.filterQuery)
	}
	if len(app.visibleTorrents) != 3 {
		t.Fatalf("visible %d torrents after clear, want 3", len(app.visibleTorrents))
	}
	if app.filterClear.Visible() {
		t.Fatal("clear button still visible after clearing")
	}
}

func TestSearchFieldPickerSwitchesField(t *testing.T) {
	app := newTorrentTableTestApp(t)
	if app.filterFieldButton.Text != "Name" {
		t.Fatalf("field button %q, want Name", app.filterFieldButton.Text)
	}

	app.setFilterField("location")

	if app.filterFieldButton.Text != "Location" {
		t.Fatalf("field button %q, want Location", app.filterFieldButton.Text)
	}
	if app.filterEntry.PlaceHolder != "Search locations…" {
		t.Fatalf("placeholder %q, want location placeholder", app.filterEntry.PlaceHolder)
	}
	if got := app.controller.Config().UI.FilterBy; got != "location" {
		t.Fatalf("persisted filter_by %q, want location", got)
	}
}
