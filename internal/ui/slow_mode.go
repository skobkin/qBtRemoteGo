package ui

import (
	"context"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
)

// toggleSlowMode flips the server's alternative speed limits and refreshes so
// the status icon and tray item pick up the new state.
func (a *application) toggleSlowMode() {
	go func() {
		err := a.controller.ToggleSlowMode(context.Background())
		fyne.Do(func() {
			if err != nil {
				a.logger.Warn("toggle slow mode", "remote", a.controller.Config().Connection.URL, "error", err)
				dialog.ShowError(err, a.window)
				return
			}
			a.refreshNow()
		})
	}()
}

// syncTraySlowMode mirrors the server's alternative-limits state in the tray
// item's check mark.
func (a *application) syncTraySlowMode() {
	if a.trayState.slowModeItem == nil {
		return
	}
	a.trayState.slowModeItem.Checked = a.serverState.UseAltSpeedLimits
}
