package ui

import (
	"testing"

	"fyne.io/fyne/v2"

	"github.com/skobkin/qbtremotego/internal/config"
)

func TestMainWindowSize(t *testing.T) {
	if got := mainWindowSize(config.UIConfig{}); got != fyne.NewSize(defaultMainWindowWidth, defaultMainWindowHeight) {
		t.Fatalf("default size %v", got)
	}
	if got := mainWindowSize(config.UIConfig{WindowWidth: 1400, WindowHeight: 900}); got != fyne.NewSize(1400, 900) {
		t.Fatalf("remembered size %v, want 1400x900", got)
	}
	if got := mainWindowSize(config.UIConfig{WindowWidth: 1400}); got != fyne.NewSize(defaultMainWindowWidth, defaultMainWindowHeight) {
		t.Fatalf("half-set size %v, want the default", got)
	}
}

func TestPersistWindowSizeSavesCanvasSize(t *testing.T) {
	app := newTorrentTableTestApp(t)
	app.window.Resize(fyne.NewSize(1300, 850))

	app.persistWindowSize()

	cfg := app.controller.Config()
	got := fyne.NewSize(cfg.UI.WindowWidth, cfg.UI.WindowHeight)
	if got != app.window.Canvas().Size() {
		t.Fatalf("saved %v, want the canvas size %v", got, app.window.Canvas().Size())
	}
}

func TestHidingMainWindowPersistsSize(t *testing.T) {
	app := newTorrentTableTestApp(t)
	app.window.Resize(fyne.NewSize(1250, 820))

	app.hideMainWindow()

	if app.controller.Config().UI.WindowWidth == 0 {
		t.Fatal("hiding to the tray did not remember the window size")
	}
}
