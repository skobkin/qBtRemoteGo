package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
)

// syncSelectionActions enables the toolbar actions that act on the selection
// only while something is selected.
func (a *application) syncSelectionActions() {
	hasSelection := len(a.selection) > 0
	for _, button := range []*buttonWithTooltip{a.removeButton, a.startButton, a.stopButton} {
		if button == nil {
			continue
		}
		if hasSelection {
			button.Enable()
		} else {
			button.Disable()
		}
	}
}

// newToolbarSeparator is a short vertical rule between toolbar groups.
func newToolbarSeparator() fyne.CanvasObject {
	line := canvas.NewRectangle(theme.Color(theme.ColorNameSeparator))
	line.SetMinSize(fyne.NewSize(1, theme.IconInlineSize()))
	return container.NewCenter(line)
}
