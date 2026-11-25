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
