package ui

import (
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func TestHoverTooltipManagerHideIgnoresDifferentOwner(t *testing.T) {
	test.NewTempApp(t)

	layer := newTooltipOverlay()
	layer.Resize(fyne.NewSize(320, 240))
	manager := newHoverTooltipManager(layer)

	owner := widget.NewLabel("owner")
	other := widget.NewLabel("other")

	win := test.NewWindow(container.NewStack(owner, layer))
	defer win.Close()
	win.Resize(fyne.NewSize(320, 240))
	owner.Resize(owner.MinSize())
	owner.Move(fyne.NewPos(20, 20))

	fyne.DoAndWait(func() {
		manager.Show(owner, widget.NewLabel("tip"))
	})

	if len(layer.layer.Objects) != 1 {
		t.Fatalf("expected tooltip to be visible, got %d objects", len(layer.layer.Objects))
	}

	fyne.DoAndWait(func() {
		manager.Hide(other)
	})

	if len(layer.layer.Objects) != 1 {
		t.Fatalf("tooltip should remain visible on mismatched owner hide, got %d objects", len(layer.layer.Objects))
	}

	fyne.DoAndWait(func() {
		manager.Hide(owner)
	})

	if len(layer.layer.Objects) != 0 {
		t.Fatalf("expected tooltip to be hidden, got %d objects", len(layer.layer.Objects))
	}
}

func TestHoverTooltipOwnerScheduleHideHonorsReentry(t *testing.T) {
	test.NewTempApp(t)

	layer := newTooltipOverlay()
	layer.Resize(fyne.NewSize(320, 240))
	manager := newHoverTooltipManager(layer)

	owner := widget.NewLabel("owner")
	win := test.NewWindow(container.NewStack(owner, layer))
	defer win.Close()
	win.Resize(fyne.NewSize(320, 240))
	owner.Resize(owner.MinSize())
	owner.Move(fyne.NewPos(20, 20))

	// The fyne test driver runs fyne.Do callbacks inline on the timer
	// goroutine, so a live hide timer would mutate widget state concurrently
	// with the assertions. Disable the timer and drive finishHide directly.
	state := hoverTooltipOwner{manager: manager, hideDelay: time.Hour}
	tip := widget.NewLabel("tip")

	fyne.DoAndWait(func() {
		state.showTooltip(owner, tip)
	})
	if len(layer.layer.Objects) != 1 {
		t.Fatalf("expected tooltip to be visible, got %d objects", len(layer.layer.Objects))
	}

	fyne.DoAndWait(func() {
		state.scheduleHide(owner)
	})
	if len(layer.layer.Objects) != 1 {
		t.Fatalf("tooltip should stay visible while the hide delay is pending, got %d objects", len(layer.layer.Objects))
	}

	fyne.DoAndWait(func() {
		state.showTooltip(owner, tip)
	})
	fyne.DoAndWait(func() {
		state.finishHide(owner)
	})

	if len(layer.layer.Objects) != 1 {
		t.Fatalf("tooltip should stay visible after hover reentry, got %d objects", len(layer.layer.Objects))
	}

	fyne.DoAndWait(func() {
		state.scheduleHide(owner)
	})
	fyne.DoAndWait(func() {
		state.finishHide(owner)
	})

	if len(layer.layer.Objects) != 0 {
		t.Fatalf("expected tooltip to hide after delay, got %d objects", len(layer.layer.Objects))
	}
}

func TestTooltipOverlayMinSizeDoesNotGrowWithTooltip(t *testing.T) {
	test.NewTempApp(t)

	layer := newTooltipOverlay()
	layer.Resize(fyne.NewSize(320, 240))
	manager := newHoverTooltipManager(layer)

	owner := widget.NewLabel("owner")
	win := test.NewWindow(container.NewStack(owner, layer))
	defer win.Close()
	win.Resize(fyne.NewSize(320, 240))
	owner.Resize(owner.MinSize())
	owner.Move(fyne.NewPos(20, 20))

	fyne.DoAndWait(func() {
		manager.Show(owner, newTextTooltip("This tooltip should not affect the window minimum size.", 180))
	})

	if got := layer.MinSize(); got.Width != 0 || got.Height != 0 {
		t.Fatalf("expected zero overlay min size, got %v", got)
	}
}

func TestTooltipPopupPositionKeepsTooltipInsideBounds(t *testing.T) {
	pos := tooltipPopupPosition(
		fyne.NewPos(290, 8),
		fyne.NewSize(24, 24),
		fyne.NewSize(140, 48),
		fyne.NewSize(320, 240),
	)

	if pos.X < 0 || pos.Y < 0 {
		t.Fatalf("tooltip placed outside top/left bounds: pos=%v", pos)
	}
	if pos.X+140 > 320 {
		t.Fatalf("tooltip placed outside right bound: pos=%v", pos)
	}
	if pos.Y+48 > 240 {
		t.Fatalf("tooltip placed outside bottom bound: pos=%v", pos)
	}
}

func TestWrapTooltipTextWrapsLongParagraph(t *testing.T) {
	wrapped := wrapTooltipText("Connection status: Connected. Incoming connections are available.", 140)

	if strings.Count(wrapped, "\n") == 0 {
		t.Fatalf("expected wrapped tooltip text, got %q", wrapped)
	}
}

func TestNewTextTooltipUsesNaturalWidthWhenBelowMax(t *testing.T) {
	test.NewTempApp(t)

	win := test.NewWindow(widget.NewLabel("anchor"))
	defer win.Close()
	win.Resize(fyne.NewSize(800, 600))

	short := newTextTooltip("Connected", 800*tooltipMaxWidthRatio)
	shortSize := short.MinSize()
	if shortSize.Width >= 800*tooltipMaxWidthRatio {
		t.Fatalf("short tooltip should be narrower than max, got %v", shortSize)
	}
}

func TestNewTextTooltipCapsAtMaxWidth(t *testing.T) {
	test.NewTempApp(t)

	win := test.NewWindow(widget.NewLabel("anchor"))
	defer win.Close()
	win.Resize(fyne.NewSize(800, 600))

	max := float32(300)
	long := newTextTooltip(
		"[Judas] Tensei Shitara Slime Datta Ken (That Time I Got Reincarnated as a Slime) - S04E10 [1080p][HEVC x265 10bit][Multi-Subs][Weekly]",
		max,
	)

	size := long.MinSize()
	if size.Width > max {
		t.Fatalf("expected tooltip width <= %v, got %v", max, size)
	}
}

func TestNewTextTooltipSkipsCapWhenMaxIsZero(t *testing.T) {
	test.NewTempApp(t)

	win := test.NewWindow(widget.NewLabel("anchor"))
	defer win.Close()
	win.Resize(fyne.NewSize(100, 100))

	short := newTextTooltip("Connected", 0)
	size := short.MinSize()
	if size.Width > 100 {
		t.Fatalf("expected no cap when max=0, got width %v", size.Width)
	}
}

func TestTooltipMaxWidthForReturnsCanvasRatio(t *testing.T) {
	test.NewTempApp(t)

	win := test.NewWindow(widget.NewLabel("anchor"))
	defer win.Close()
	win.Resize(fyne.NewSize(800, 600))
	owner := widget.NewLabel("anchor")
	owner.Resize(fyne.NewSize(1, 1))

	got := tooltipMaxWidthFor(owner, tooltipMaxWidthRatio)
	want := float32(800) * tooltipMaxWidthRatio
	if got != want {
		t.Fatalf("expected max width %v, got %v", want, got)
	}
}

func TestTooltipMaxWidthForRejectsZeroRatio(t *testing.T) {
	test.NewTempApp(t)

	win := test.NewWindow(widget.NewLabel("anchor"))
	defer win.Close()
	win.Resize(fyne.NewSize(800, 600))
	owner := widget.NewLabel("anchor")
	owner.Resize(fyne.NewSize(1, 1))

	if got := tooltipMaxWidthFor(owner, 0); got != 0 {
		t.Fatalf("expected 0 for zero ratio, got %v", got)
	}
	if got := tooltipMaxWidthFor(owner, -0.5); got != 0 {
		t.Fatalf("expected 0 for negative ratio, got %v", got)
	}
}

func TestHoverTooltipOwnerScheduleShowFiresAfterDelay(t *testing.T) {
	test.NewTempApp(t)

	layer := newTooltipOverlay()
	layer.Resize(fyne.NewSize(320, 240))
	manager := newHoverTooltipManager(layer)

	owner := widget.NewLabel("owner")
	win := test.NewWindow(container.NewStack(owner, layer))
	defer win.Close()
	win.Resize(fyne.NewSize(320, 240))
	owner.Resize(owner.MinSize())
	owner.Move(fyne.NewPos(20, 20))

	state := hoverTooltipOwner{manager: manager}
	tip := widget.NewLabel("tip")

	// The fyne test driver runs fyne.Do callbacks inline on the timer
	// goroutine, so use a delay the real timer cannot reach and drive
	// finishShow directly (see hoverTooltipOwner.hideDelay).
	fyne.DoAndWait(func() {
		state.scheduleShow(owner, tip, time.Hour)
	})
	if len(layer.layer.Objects) != 0 {
		t.Fatalf("tooltip should not be visible immediately after schedule, got %d objects", len(layer.layer.Objects))
	}

	fyne.DoAndWait(func() {
		state.finishShow(owner, tip)
	})

	if len(layer.layer.Objects) != 1 {
		t.Fatalf("expected tooltip to be visible after delay, got %d objects", len(layer.layer.Objects))
	}
}

func TestHoverTooltipOwnerScheduleShowCanBeCancelled(t *testing.T) {
	test.NewTempApp(t)

	layer := newTooltipOverlay()
	layer.Resize(fyne.NewSize(320, 240))
	manager := newHoverTooltipManager(layer)

	owner := widget.NewLabel("owner")
	win := test.NewWindow(container.NewStack(owner, layer))
	defer win.Close()
	win.Resize(fyne.NewSize(320, 240))
	owner.Resize(owner.MinSize())
	owner.Move(fyne.NewPos(20, 20))

	state := hoverTooltipOwner{manager: manager}

	fyne.DoAndWait(func() {
		state.scheduleShow(owner, widget.NewLabel("tip"), tooltipShowDelay)
	})
	time.Sleep(tooltipShowDelay / 4)
	fyne.DoAndWait(func() {
		state.cancelShow()
	})
	time.Sleep(tooltipShowDelay + 50*time.Millisecond)
	fyne.DoAndWait(func() {})

	if len(layer.layer.Objects) != 0 {
		t.Fatalf("tooltip should not appear after cancel, got %d objects", len(layer.layer.Objects))
	}
}

func TestHoverTooltipOwnerScheduleHideCancelsPendingShow(t *testing.T) {
	test.NewTempApp(t)

	layer := newTooltipOverlay()
	layer.Resize(fyne.NewSize(320, 240))
	manager := newHoverTooltipManager(layer)

	owner := widget.NewLabel("owner")
	win := test.NewWindow(container.NewStack(owner, layer))
	defer win.Close()
	win.Resize(fyne.NewSize(320, 240))
	owner.Resize(owner.MinSize())
	owner.Move(fyne.NewPos(20, 20))

	// The fyne test driver runs fyne.Do callbacks inline on the timer
	// goroutine, so disable the real hide timer and drive finishHide directly
	// (see hoverTooltipOwner.hideDelay).
	state := hoverTooltipOwner{manager: manager, hideDelay: time.Hour}

	fyne.DoAndWait(func() {
		state.scheduleShow(owner, widget.NewLabel("tip"), tooltipShowDelay)
		state.scheduleHide(owner)
	})
	fyne.DoAndWait(func() {
		state.finishHide(owner)
	})

	if len(layer.layer.Objects) != 0 {
		t.Fatalf("tooltip should not appear after hide cancels pending show, got %d objects", len(layer.layer.Objects))
	}
}

func TestHoverTooltipLabelSetTextCancelsPendingShow(t *testing.T) {
	test.NewTempApp(t)

	layer := newTooltipOverlay()
	layer.Resize(fyne.NewSize(320, 240))
	manager := newHoverTooltipManager(layer)

	owner := newHoverLabel(manager, tooltipShowDelay, nil)
	owner.Resize(fyne.NewSize(200, 20))
	owner.Move(fyne.NewPos(20, 20))
	win := test.NewWindow(container.NewStack(owner, layer))
	defer win.Close()
	win.Resize(fyne.NewSize(320, 240))

	fyne.DoAndWait(func() {
		owner.SetText("a very long torrent name that will be truncated by the cell width", "a very long torrent name that will be truncated by the cell width")
		owner.MouseIn(nil)
	})
	if len(layer.layer.Objects) != 0 {
		t.Fatalf("tooltip should not be visible immediately after MouseIn, got %d objects", len(layer.layer.Objects))
	}

	fyne.DoAndWait(func() {
		owner.SetText("different long torrent name that should not show the old tooltip", "different long torrent name that should not show the old tooltip")
	})
	time.Sleep(tooltipShowDelay + 50*time.Millisecond)
	fyne.DoAndWait(func() {})

	if len(layer.layer.Objects) != 0 {
		t.Fatalf("SetText should cancel pending show, got %d objects", len(layer.layer.Objects))
	}
}

func TestHoverLabelMouseInShowsTooltipInstantly(t *testing.T) {
	test.NewTempApp(t)

	layer := newTooltipOverlay()
	layer.Resize(fyne.NewSize(320, 240))
	manager := newHoverTooltipManager(layer)

	owner := newHoverLabel(manager, 0, nil)
	owner.Resize(fyne.NewSize(80, 20))
	owner.Move(fyne.NewPos(20, 20))
	win := test.NewWindow(container.NewWithoutLayout(owner, layer))
	defer win.Close()
	win.Resize(fyne.NewSize(320, 240))

	fyne.DoAndWait(func() {
		owner.SetText("very long text that exceeds the cell width and will be truncated", "very long text that exceeds the cell width and will be truncated")
		owner.MouseIn(nil)
	})

	if len(layer.layer.Objects) != 1 {
		t.Fatalf("expected tooltip to appear immediately when showDelay=0, got %d objects", len(layer.layer.Objects))
	}
}

func TestHoverLabelMouseInDelayedShowAppearsAfterDelay(t *testing.T) {
	test.NewTempApp(t)

	layer := newTooltipOverlay()
	layer.Resize(fyne.NewSize(320, 240))
	manager := newHoverTooltipManager(layer)

	// The fyne test driver runs fyne.Do callbacks inline on the timer
	// goroutine, so use a show delay the real timer cannot reach and drive
	// finishShow directly (see hoverTooltipOwner.hideDelay).
	owner := newHoverLabel(manager, time.Hour, nil)
	owner.Resize(fyne.NewSize(80, 20))
	owner.Move(fyne.NewPos(20, 20))
	win := test.NewWindow(container.NewWithoutLayout(owner, layer))
	defer win.Close()
	win.Resize(fyne.NewSize(320, 240))

	fyne.DoAndWait(func() {
		owner.SetText("very long text that exceeds the cell width and will be truncated", "very long text that exceeds the cell width and will be truncated")
		owner.MouseIn(nil)
	})
	if len(layer.layer.Objects) != 0 {
		t.Fatalf("tooltip should not be visible immediately after MouseIn with delay, got %d objects", len(layer.layer.Objects))
	}

	fyne.DoAndWait(func() {
		owner.finishShow(owner, widget.NewLabel("tip"))
	})

	if len(layer.layer.Objects) != 1 {
		t.Fatalf("expected tooltip to appear after showDelay, got %d objects", len(layer.layer.Objects))
	}
}

func newTestApplication(t *testing.T) *application {
	t.Helper()
	layer := newTooltipOverlay()
	return &application{
		tooltipManager: newHoverTooltipManager(layer),
		tooltipLayer:   layer,
		columnWidths:   map[string]float32{},
	}
}

func TestRowHoverInShowsBackground(t *testing.T) {
	test.NewTempApp(t)

	app := newTestApplication(t)
	row := newTorrentListRow(app)
	win := test.NewWindow(row)
	defer win.Close()
	win.Resize(fyne.NewSize(row.MinSize().Width, row.MinSize().Height))

	if row.hoverBG.Visible() {
		t.Fatalf("hover background should start hidden")
	}

	fyne.DoAndWait(func() {
		row.hoverIn()
	})

	if !row.hoverBG.Visible() {
		t.Fatalf("hover background should be visible after hoverIn")
	}
}

func TestRowHoverOutHidesBackgroundAfterDelay(t *testing.T) {
	test.NewTempApp(t)

	app := newTestApplication(t)
	row := newTorrentListRow(app)
	// The fyne test driver runs fyne.Do callbacks inline on the timer
	// goroutine, so a live 50 ms timer would write widget state concurrently
	// with the assertions. Disable the timer and drive the delayed hide here.
	row.hoverOutDelay = time.Hour
	win := test.NewWindow(row)
	defer win.Close()
	win.Resize(fyne.NewSize(row.MinSize().Width, row.MinSize().Height))

	fyne.DoAndWait(func() {
		row.hoverIn()
	})
	if !row.hoverBG.Visible() {
		t.Fatalf("hover background should be visible after hoverIn")
	}

	fyne.DoAndWait(func() {
		row.hoverOut()
	})
	if !row.hoverBG.Visible() {
		t.Fatalf("hover background should remain visible immediately after hoverOut (delay pending)")
	}

	fyne.DoAndWait(row.finishHoverOut)

	if row.hoverBG.Visible() {
		t.Fatalf("hover background should be hidden after hoverOut delay elapsed")
	}
}

func TestRowHoverInCancelsPendingHoverOut(t *testing.T) {
	test.NewTempApp(t)

	app := newTestApplication(t)
	row := newTorrentListRow(app)
	win := test.NewWindow(row)
	defer win.Close()
	win.Resize(fyne.NewSize(row.MinSize().Width, row.MinSize().Height))

	fyne.DoAndWait(func() {
		row.hoverIn()
		row.hoverOut()
		row.hoverIn()
	})
	time.Sleep(100 * time.Millisecond)
	fyne.DoAndWait(func() {})

	if !row.hoverBG.Visible() {
		t.Fatalf("hover background should remain visible after re-entry cancels pending hoverOut")
	}
}
