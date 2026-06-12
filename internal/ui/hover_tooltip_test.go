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

	state := hoverTooltipOwner{manager: manager}

	fyne.DoAndWait(func() {
		state.showTooltip(owner, widget.NewLabel("tip"))
	})
	if len(layer.layer.Objects) != 1 {
		t.Fatalf("expected tooltip to be visible, got %d objects", len(layer.layer.Objects))
	}

	fyne.DoAndWait(func() {
		state.scheduleHide(owner)
	})
	time.Sleep(tooltipHideDelay / 2)
	fyne.DoAndWait(func() {
		state.showTooltip(owner, widget.NewLabel("tip"))
	})
	time.Sleep(tooltipHideDelay + 50*time.Millisecond)
	fyne.DoAndWait(func() {})

	if len(layer.layer.Objects) != 1 {
		t.Fatalf("tooltip should stay visible after hover reentry, got %d objects", len(layer.layer.Objects))
	}

	fyne.DoAndWait(func() {
		state.scheduleHide(owner)
	})
	time.Sleep(tooltipHideDelay + 50*time.Millisecond)
	fyne.DoAndWait(func() {})

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

	fyne.DoAndWait(func() {
		state.scheduleShow(owner, widget.NewLabel("tip"), tooltipShowDelay)
	})
	if len(layer.layer.Objects) != 0 {
		t.Fatalf("tooltip should not be visible immediately after schedule, got %d objects", len(layer.layer.Objects))
	}

	time.Sleep(tooltipShowDelay + 50*time.Millisecond)
	fyne.DoAndWait(func() {})

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

	state := hoverTooltipOwner{manager: manager}

	fyne.DoAndWait(func() {
		state.scheduleShow(owner, widget.NewLabel("tip"), tooltipShowDelay)
		state.scheduleHide(owner)
	})
	time.Sleep(tooltipShowDelay + tooltipHideDelay + 50*time.Millisecond)
	fyne.DoAndWait(func() {})

	if len(layer.layer.Objects) != 0 {
		t.Fatalf("tooltip should not appear after hide cancels pending show, got %d objects", len(layer.layer.Objects))
	}
}

func TestHoverTooltipLabelSetTextCancelsPendingShow(t *testing.T) {
	test.NewTempApp(t)

	layer := newTooltipOverlay()
	layer.Resize(fyne.NewSize(320, 240))
	manager := newHoverTooltipManager(layer)

	owner := newHoverLabel(manager, tooltipShowDelay)
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
