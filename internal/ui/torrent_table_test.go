package ui

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/skobkin/qbtremotego/internal/qbt"
)

// newTorrentTableTestApp builds the main window with a handful of torrents so
// the row-selection machinery can be exercised without a server.
func newTorrentTableTestApp(t *testing.T) *application {
	t.Helper()
	test.NewTempApp(t)
	app := &application{
		fyApp:        fyne.CurrentApp(),
		window:       fyne.CurrentApp().NewWindow("torrent table"),
		controller:   newPollTestController(t),
		selection:    map[string]bool{},
		detailsState: newTorrentDetailsState(),
		rowTaps:      rowTapSequencer{interval: torrentDoubleTapInterval},
		statusLabel:  widget.NewLabel(""),
	}
	app.buildMainWindow()
	// The test canvas defaults to a size that only fits a couple of rows; size
	// the list so every seeded torrent gets a bound row widget.
	app.list.Resize(fyne.NewSize(800, 400))
	app.allTorrents = []qbt.Torrent{
		{Hash: "hash-a", Name: "A"},
		{Hash: "hash-b", Name: "B"},
		{Hash: "hash-c", Name: "C"},
	}
	app.refreshVisibleTorrents()

	for _, torrent := range app.allTorrents {
		app.rowForTest(t, torrent.Hash)
	}

	return app
}

func (a *application) rowForTest(t *testing.T, hash string) *torrentListRow {
	t.Helper()
	for _, row := range a.listRows {
		if row.hash == hash {
			return row
		}
	}
	t.Fatalf("no list row is bound to %s", hash)

	return nil
}

// The selection refresh must toggle exactly the rows whose selection state
// changed: the scan runs inside the tap handler, where a full list refresh
// re-renders every visible row and makes selection feel laggy.
func TestRefreshTorrentSelectionTogglesChangedRowsOnly(t *testing.T) {
	app := newTorrentTableTestApp(t)

	app.selectOnlyTorrent("hash-a")
	app.refreshTorrentSelection()
	if !app.rowForTest(t, "hash-a").selectedShown {
		t.Fatal("expected the selected row to show its selection background")
	}

	app.selectOnlyTorrent("hash-b")
	app.refreshTorrentSelection()
	if app.rowForTest(t, "hash-a").selectedShown {
		t.Fatal("expected the deselected row to hide its selection background")
	}
	if !app.rowForTest(t, "hash-b").selectedShown {
		t.Fatal("expected the newly selected row to show its selection background")
	}
	if app.rowForTest(t, "hash-c").selectedShown {
		t.Fatal("expected an untouched row to keep its background hidden")
	}

	app.toggleTorrentSelection("hash-b")
	app.refreshTorrentSelection()
	if app.rowForTest(t, "hash-b").selectedShown {
		t.Fatal("expected the toggled-off row to hide its selection background")
	}
}

// Rows bound after the selection was made (first render, scrolling, poll
// refresh) must pick up the current selection state.
func TestSetTorrentAppliesSelectionState(t *testing.T) {
	app := newTorrentTableTestApp(t)

	app.selectOnlyTorrent("hash-b")
	app.refreshVisibleTorrents()

	if !app.rowForTest(t, "hash-b").selectedShown {
		t.Fatal("expected a row bound to the selected torrent to show its selection background")
	}
	if app.rowForTest(t, "hash-a").selectedShown {
		t.Fatal("expected a row bound to an unselected torrent to hide its selection background")
	}
}

// The compact-rows setting must flow into both min-size sources the list
// measures (the row widget and its content layout) and re-layout the visible
// rows on a plain list refresh.
func TestTorrentRowMinSizeFollowsCompactRows(t *testing.T) {
	app := newTorrentTableTestApp(t)

	if got := app.torrentRowHeightValue(); got != torrentRowHeight {
		t.Fatalf("default row height %.0f, want %.0f", got, torrentRowHeight)
	}
	if got := app.rowForTest(t, "hash-a").MinSize().Height; got != torrentRowHeight {
		t.Fatalf("default row min height %.0f, want %.0f", got, torrentRowHeight)
	}

	app.compactRows = true

	if got := app.torrentRowHeightValue(); got != compactRowHeight {
		t.Fatalf("compact row height %.0f, want %.0f", got, compactRowHeight)
	}
	if got := app.rowForTest(t, "hash-a").MinSize().Height; got != compactRowHeight {
		t.Fatalf("compact row min height %.0f, want %.0f", got, compactRowHeight)
	}
	layout := &torrentRowLayout{app: app}
	if got := layout.MinSize(nil).Height; got != compactRowHeight {
		t.Fatalf("compact layout min height %.0f, want %.0f", got, compactRowHeight)
	}

	app.list.Refresh()

	if got := app.rowForTest(t, "hash-a").Size().Height; got != compactRowHeight {
		t.Fatalf("row height %.0f after list refresh, want %.0f", got, compactRowHeight)
	}
}

func TestRowTapSequencer(t *testing.T) {
	start := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	t.Run("single tap does not trigger", func(t *testing.T) {
		var s rowTapSequencer
		s.interval = torrentDoubleTapInterval
		if s.record("a", start, 0) {
			t.Fatal("first tap must not complete a double-click")
		}
	})

	t.Run("same hash within interval triggers", func(t *testing.T) {
		var s rowTapSequencer
		s.interval = torrentDoubleTapInterval
		s.record("a", start, 0)
		if !s.record("a", start.Add(100*time.Millisecond), 0) {
			t.Fatal("second tap on the same row within the interval must trigger")
		}
	})

	t.Run("different hash re-anchors instead of triggering", func(t *testing.T) {
		var s rowTapSequencer
		s.interval = torrentDoubleTapInterval
		s.record("a", start, 0)
		if s.record("b", start.Add(100*time.Millisecond), 0) {
			t.Fatal("a tap on a different row must not complete the previous row's double-click")
		}
		if !s.record("b", start.Add(200*time.Millisecond), 0) {
			t.Fatal("second tap on the new row must trigger")
		}
	})

	t.Run("tap beyond the interval re-anchors", func(t *testing.T) {
		var s rowTapSequencer
		s.interval = torrentDoubleTapInterval
		s.record("a", start, 0)
		if s.record("a", start.Add(torrentDoubleTapInterval+time.Millisecond), 0) {
			t.Fatal("a tap after the interval must not trigger")
		}
		if !s.record("a", start.Add(2*torrentDoubleTapInterval+time.Millisecond), 0) {
			t.Fatal("next tap within the interval must trigger")
		}
	})

	t.Run("triple tap opens details only once", func(t *testing.T) {
		var s rowTapSequencer
		s.interval = torrentDoubleTapInterval
		s.record("a", start, 0)
		if !s.record("a", start.Add(100*time.Millisecond), 0) {
			t.Fatal("second tap must trigger")
		}
		if s.record("a", start.Add(200*time.Millisecond), 0) {
			t.Fatal("third tap must start a new sequence, not trigger again")
		}
	})

	t.Run("modified tap never triggers", func(t *testing.T) {
		var s rowTapSequencer
		s.interval = torrentDoubleTapInterval
		s.record("a", start, 0)
		if s.record("a", start.Add(100*time.Millisecond), fyne.KeyModifierControl) {
			t.Fatal("a ctrl tap on the same row must not complete a double-click; it toggles the selection")
		}
	})

	t.Run("modified tap breaks a pending sequence", func(t *testing.T) {
		var s rowTapSequencer
		s.interval = torrentDoubleTapInterval
		s.record("a", start, 0)
		s.record("a", start.Add(100*time.Millisecond), fyne.KeyModifierControl)
		if s.record("a", start.Add(200*time.Millisecond), 0) {
			t.Fatal("a plain tap after a modifier tap must re-anchor, not fuse with the earlier plain tap")
		}
		if !s.record("a", start.Add(300*time.Millisecond), 0) {
			t.Fatal("second plain tap after the re-anchor must trigger")
		}
	})

	t.Run("shift tap never triggers", func(t *testing.T) {
		var s rowTapSequencer
		s.interval = torrentDoubleTapInterval
		s.record("a", start, 0)
		if s.record("a", start.Add(100*time.Millisecond), fyne.KeyModifierShift) {
			t.Fatal("a shift tap must not complete a double-click; it extends the selection")
		}
	})
}
