package ui

import (
	"slices"
	"testing"

	"github.com/skobkin/qbtremotego/internal/config"
	"github.com/skobkin/qbtremotego/internal/qbt"
)

func TestColumnSpecKeysMatchConfig(t *testing.T) {
	keys := make([]string, 0, len(torrentColumnSpecs))
	for _, spec := range torrentColumnSpecs {
		keys = append(keys, spec.key)
	}
	if !slices.Equal(keys, config.ColumnKeys) {
		t.Fatalf("column spec keys %v differ from config.ColumnKeys %v", keys, config.ColumnKeys)
	}
}

func TestOptionalColumnsHiddenByDefault(t *testing.T) {
	app := newTorrentTableTestApp(t)
	app.tableHeader.Refresh()

	ratio := torrentColumnIndex("ratio")
	if app.tableHeader.cells[ratio].Visible() {
		t.Fatal("optional Ratio column visible by default")
	}
	if !app.tableHeader.cells[torrentColumnIndex("size")].Visible() {
		t.Fatal("default Size column hidden")
	}
}

func TestSetColumnVisibleShowsAndPersists(t *testing.T) {
	app := newTorrentTableTestApp(t)
	before := app.minTotalColumnWidth()

	app.setColumnVisible("ratio", true)

	if !slices.Contains(app.controller.Config().UI.VisibleColumns, "ratio") {
		t.Fatalf("visible columns %v, want ratio saved", app.controller.Config().UI.VisibleColumns)
	}
	if !app.tableHeader.cells[torrentColumnIndex("ratio")].Visible() {
		t.Fatal("Ratio header hidden after enabling it")
	}
	if got, want := app.minTotalColumnWidth(), before+app.columnWidth(torrentColumnSpecs[torrentColumnIndex("ratio")]); got != want {
		t.Fatalf("min table width %.0f, want %.0f with Ratio added", got, want)
	}

	row := app.rowForTest(t, "hash-a")
	row.setTorrent(qbt.Torrent{Hash: "hash-a", Ratio: 1.25, NumSeeds: 5, NumComplete: 120, NumLeechs: 2, NumIncomplete: 30, Uploaded: 2048})
	if got := row.ratio.label.Text; got != "1.25" {
		t.Fatalf("ratio cell %q, want 1.25", got)
	}
	if got := row.seeds.label.Text; got != "5 (120)" {
		t.Fatalf("seeds cell %q, want 5 (120)", got)
	}
	if got := row.peers.label.Text; got != "2 (30)" {
		t.Fatalf("peers cell %q, want 2 (30)", got)
	}
	if got := row.uploaded.label.Text; got != "2.0 KiB" {
		t.Fatalf("uploaded cell %q, want 2.0 KiB", got)
	}

	app.setColumnVisible("ratio", false)
	if app.tableHeader.cells[torrentColumnIndex("ratio")].Visible() {
		t.Fatal("Ratio header visible after hiding it")
	}
}

func TestNameColumnCannotBeHidden(t *testing.T) {
	app := newTorrentTableTestApp(t)
	app.setColumnVisible("name", false)
	if !app.columnVisible(torrentColumnSpecs[torrentColumnIndex("name")]) {
		t.Fatal("Name column was hidden")
	}
}

func TestColumnMenuListsHideableColumns(t *testing.T) {
	app := newTorrentTableTestApp(t)
	menu := app.columnMenu()

	labels := make([]string, 0, len(menu.Items))
	for _, item := range menu.Items {
		labels = append(labels, item.Label)
		if item.Label == "Size" && !item.Checked {
			t.Fatal("visible Size column should be checked")
		}
		if item.Label == "Ratio" && item.Checked {
			t.Fatal("hidden Ratio column should be unchecked")
		}
	}
	if slices.Contains(labels, "Name") {
		t.Fatal("Name must not be offered for hiding")
	}
	if !slices.Contains(labels, "Save path") {
		t.Fatalf("menu %v lacks Save path", labels)
	}
}
