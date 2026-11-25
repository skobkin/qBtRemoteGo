package ui

import (
	"fyne.io/fyne/v2"

	"github.com/skobkin/qbtremotego/internal/config"
)

const (
	defaultMainWindowWidth  = float32(1120)
	defaultMainWindowHeight = float32(680)
)

// mainWindowSize is the remembered main window size, or the default when
// none (or only half of one) was saved. Fyne has no portable window
// position, so only the size is restored.
func mainWindowSize(cfg config.UIConfig) fyne.Size {
	if cfg.WindowWidth <= 0 || cfg.WindowHeight <= 0 {
		return fyne.NewSize(defaultMainWindowWidth, defaultMainWindowHeight)
	}
	return fyne.NewSize(cfg.WindowWidth, cfg.WindowHeight)
}

// persistWindowSize remembers the main window's current size.
func (a *application) persistWindowSize() {
	if a.controller == nil || a.window == nil {
		return
	}
	size := a.window.Canvas().Size()
	if size.Width <= 0 || size.Height <= 0 {
		return
	}
	cfg := a.controller.Config()
	if cfg.UI.WindowWidth == size.Width && cfg.UI.WindowHeight == size.Height {
		return
	}
	if err := a.controller.SaveLocalUI(func(cfg *config.AppConfig) {
		cfg.UI.WindowWidth = size.Width
		cfg.UI.WindowHeight = size.Height
	}); err != nil {
		a.logger.Warn("persist window size", "error", err)
	}
}
