package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// The tray left-click toggles the main window. windowVisible is both the
// toggle branch and the poll-interval input, so it must track every toggle.
func TestToggleMainWindowFlipsVisibility(t *testing.T) {
	test.NewTempApp(t)
	app := &application{
		fyApp:  fyne.CurrentApp(),
		window: fyne.CurrentApp().NewWindow("toggle"),
	}
	app.windowVisible.Store(true)

	app.toggleMainWindow()
	if app.windowVisible.Load() {
		t.Fatal("expected a shown window to hide on toggle")
	}

	app.toggleMainWindow()
	if !app.windowVisible.Load() {
		t.Fatal("expected a hidden window to show on toggle")
	}
}
