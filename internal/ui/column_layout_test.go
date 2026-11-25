package ui

import (
	"testing"

	"fyne.io/fyne/v2"
)

func nameColumnSpec(t *testing.T) torrentColumnSpec {
	t.Helper()
	return torrentColumnSpecs[torrentColumnIndex("name")]
}

// Name is the flexible column: it absorbs whatever width the other columns
// leave, but never shrinks below its minimum.
func TestNameColumnFillsRemainingTableWidth(t *testing.T) {
	app := newTorrentTableTestApp(t)
	others := float32(0)
	for _, spec := range torrentColumnSpecs {
		if spec.key != "name" {
			others += app.columnWidth(spec)
		}
	}
	name := nameColumnSpec(t)

	layout := &torrentTableLayout{app: app}
	objects := []fyne.CanvasObject{app.tableHeader, app.list, app.tablePreview}

	layout.Layout(objects, fyne.NewSize(others+300, 400))
	if got := app.columnWidth(name); got != 300 {
		t.Fatalf("name width %.0f with 300px spare, want 300", got)
	}

	layout.Layout(objects, fyne.NewSize(others+50, 400))
	if got := app.columnWidth(name); got != name.minWidth {
		t.Fatalf("name width %.0f in a narrow table, want minimum %.0f", got, name.minWidth)
	}
}

// The table's minimum width uses Name's minimum, so the horizontal scrollbar
// only appears once the window is narrower than every column at its floor.
func TestTableMinWidthUsesNameMinimum(t *testing.T) {
	app := newTorrentTableTestApp(t)
	want := nameColumnSpec(t).minWidth
	for _, spec := range torrentColumnSpecs {
		if spec.key != "name" {
			want += app.columnWidth(spec)
		}
	}
	layout := &torrentTableLayout{app: app}
	layout.Layout([]fyne.CanvasObject{app.tableHeader, app.list, app.tablePreview}, fyne.NewSize(want+500, 400))

	objects := []fyne.CanvasObject{app.tableHeader, app.list, app.tablePreview}
	if got := layout.MinSize(objects).Width; got != want {
		t.Fatalf("table min width %.0f, want %.0f", got, want)
	}
	if got := app.rowForTest(t, "hash-a").MinSize().Width; got != want {
		t.Fatalf("row min width %.0f, want %.0f", got, want)
	}
}

func TestNameColumnHasNoResizeHandle(t *testing.T) {
	app := newTorrentTableTestApp(t)
	handle := app.tableHeader.handles[torrentColumnIndex("name")]
	if handle.Visible() {
		t.Fatal("expected the flexible name column to have no resize handle")
	}
}

// Resizing a fixed column in a wide table leaves the total width unchanged
// (Name absorbs the difference), so rows keep their size and the list never
// re-lays them out. The resize must re-lay out the visible rows itself.
func TestColumnResizeRelaysOutVisibleRows(t *testing.T) {
	app := newTorrentTableTestApp(t)
	app.tableScroll.Resize(fyne.NewSize(1600, 400))
	row := app.rowForTest(t, "hash-a")
	sizeX := row.size.Position().X
	statusX := row.statusCt.Position().X

	size := torrentColumnSpecs[torrentColumnIndex("size")]
	app.setColumnWidth(size, app.columnWidth(size)+50, false)

	// Name shrinks by 50, so the Size cell starts 50px earlier and the
	// columns to its right stay where they were.
	if got := row.size.Position().X; got != sizeX-50 {
		t.Fatalf("size cell at x=%.0f after widening it by 50, want %.0f", got, sizeX-50)
	}
	if got := row.statusCt.Position().X; got != statusX {
		t.Fatalf("status cell moved to x=%.0f, want %.0f", got, statusX)
	}
}
