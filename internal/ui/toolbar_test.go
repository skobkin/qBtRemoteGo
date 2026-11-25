package ui

import "testing"

func TestSelectionActionsFollowSelection(t *testing.T) {
	app := newTorrentTableTestApp(t)
	buttons := map[string]interface{ Disabled() bool }{
		"remove": app.removeButton,
		"start":  app.startButton,
		"stop":   app.stopButton,
	}
	for name, button := range buttons {
		if !button.Disabled() {
			t.Fatalf("%s button enabled with no selection", name)
		}
	}

	app.applyTorrentSelection("hash-a", 0)
	for name, button := range buttons {
		if button.Disabled() {
			t.Fatalf("%s button disabled with a selection", name)
		}
	}

	// A filter that hides the selected torrent prunes the selection.
	app.applyFilterQueryNow("B")
	for name, button := range buttons {
		if !button.Disabled() {
			t.Fatalf("%s button enabled after the selection was filtered out", name)
		}
	}
}

func TestStatusBarSelectionFollowsSelection(t *testing.T) {
	app := newTorrentTableTestApp(t)
	if app.statusBar.selectionGroup.Visible() {
		t.Fatal("selection segment visible with no selection")
	}

	app.applyTorrentSelection("hash-a", 0)

	if !app.statusBar.selectionGroup.Visible() {
		t.Fatal("selection segment hidden after selecting a torrent")
	}
	if got := app.statusBar.selection.Text; got != "1 selected · 0 B" {
		t.Fatalf("selection text %q, want 1 selected · 0 B", got)
	}
}
