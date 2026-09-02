package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

// The overlay drawer's renderer is destroyed by Fyne's renderer cache after a
// minute hidden and re-created on the next open. A renderer re-creation never
// receives a Layout call, so the drawer must own its inner container: sizes
// survive renderer destruction, a fresh container does not (it re-lays out at
// the zero minimum and paints the panel over the torrent list).

func newDetailsDrawerForTest(t *testing.T) *detailsDrawer {
	t.Helper()
	test.NewTempApp(t)

	return newDetailsDrawer(
		&application{detailsState: newTorrentDetailsState()},
		widget.NewLabel("content"),
	)
}

func TestDetailsDrawerLayoutPositionsPanel(t *testing.T) {
	drawer := newDetailsDrawerForTest(t)

	drawer.Resize(fyne.NewSize(800, 600))

	// 800 * 0.4 = 320 is below the 360 minimum, so the panel takes 360 of the
	// 800 available and sits at the right edge.
	if got := drawer.panel.Position(); got != (fyne.NewPos(440, 0)) {
		t.Fatalf("panel position %v, want (440,0)", got)
	}
	if got := drawer.panel.Size(); got != (fyne.NewSize(360, 600)) {
		t.Fatalf("panel size %v, want (360,600)", got)
	}
	if got := drawer.backdrop.Position(); got != (fyne.NewPos(0, 0)) {
		t.Fatalf("backdrop position %v, want (0,0)", got)
	}
	if got := drawer.backdrop.Size(); got != (fyne.NewSize(800, 600)) {
		t.Fatalf("backdrop size %v, want (800,600)", got)
	}
}

func TestDetailsDrawerCreateRendererReusesInnerContainer(t *testing.T) {
	drawer := newDetailsDrawerForTest(t)

	first := drawer.CreateRenderer()
	second := drawer.CreateRenderer()

	if first.Objects()[0] != second.Objects()[0] {
		t.Fatal("re-created renderers must wrap the same cached container")
	}
}

func TestDetailsDrawerRendererRecreationKeepsPanelGeometry(t *testing.T) {
	drawer := newDetailsDrawerForTest(t)
	drawer.Resize(fyne.NewSize(800, 600))
	position := drawer.panel.Position()
	size := drawer.panel.Size()

	// Simulates the renderer cache expiring while the drawer is hidden and the
	// next open re-creating it: a plain Refresh, no Layout call.
	drawer.CreateRenderer().Refresh()

	if got := drawer.panel.Position(); got != position {
		t.Fatalf("panel position %v after renderer re-creation, want %v", got, position)
	}
	if got := drawer.panel.Size(); got != size {
		t.Fatalf("panel size %v after renderer re-creation, want %v", got, size)
	}
}
