package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"
)

func TestNextSortOrder(t *testing.T) {
	tests := []struct {
		name           string
		column         string
		descending     bool
		tapped         string
		wantColumn     string
		wantDescending bool
	}{
		{"same column flips to descending", "name", false, "name", "name", true},
		{"same column flips to ascending", "size", true, "size", "size", false},
		{"new text column starts ascending", "size", true, "name", "name", false},
		{"new status column starts ascending", "name", true, "status", "status", false},
		{"new size column starts descending", "name", false, "size", "size", true},
		{"new speed column starts descending", "name", false, "down", "down", true},
		{"new added column starts descending", "name", false, "added", "added", true},
		{"new ETA column starts ascending", "name", true, "eta", "eta", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			column, descending := nextSortOrder(tt.column, tt.descending, tt.tapped)
			if column != tt.wantColumn || descending != tt.wantDescending {
				t.Fatalf("nextSortOrder(%q, %v, %q) = (%q, %v), want (%q, %v)",
					tt.column, tt.descending, tt.tapped, column, descending, tt.wantColumn, tt.wantDescending)
			}
		})
	}
}

// Tapping a header sorts by that column, persists it and marks the header.
func TestTappingHeaderSortsAndMarksColumn(t *testing.T) {
	app := newTorrentTableTestApp(t)
	nameIndex := torrentColumnIndex("name")
	sizeIndex := torrentColumnIndex("size")

	test.Tap(app.tableHeader.cells[nameIndex])

	cfg := app.controller.Config()
	if cfg.UI.SortColumn != "name" || cfg.UI.SortDescending {
		t.Fatalf("sort = (%q, desc=%v), want (name, asc)", cfg.UI.SortColumn, cfg.UI.SortDescending)
	}
	if got := app.tableHeader.cells[nameIndex].label.Text; got != "Name ▲" {
		t.Fatalf("name header %q, want %q", got, "Name ▲")
	}
	if got := app.tableHeader.cells[sizeIndex].label.Text; got != "Size" {
		t.Fatalf("size header %q, want plain label", got)
	}
	if got := app.visibleTorrents[0].Name; got != "A" {
		t.Fatalf("first row %q after ascending name sort, want A", got)
	}

	test.Tap(app.tableHeader.cells[nameIndex])

	if got := app.tableHeader.cells[nameIndex].label.Text; got != "Name ▼" {
		t.Fatalf("name header %q after second tap, want %q", got, "Name ▼")
	}
	if got := app.visibleTorrents[0].Name; got != "C" {
		t.Fatalf("first row %q after descending name sort, want C", got)
	}
}
