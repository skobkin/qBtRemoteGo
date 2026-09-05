package ui

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"

	updates "github.com/skobkin/go4updates"
	"github.com/skobkin/go4updates/releasefmt"
	fyneupdate "github.com/skobkin/go4updates/ui/fyne/update"

	appcore "github.com/skobkin/qbtremotego/internal/app"
	appupdates "github.com/skobkin/qbtremotego/internal/updates"
)

// updateChecker is the seam between the UI and the update manager; the
// production implementation is *appupdates.Manager and tests substitute a
// stub.
type updateChecker interface {
	Running() bool
	Start()
	Stop()
	CheckNow(ctx context.Context) (updates.Result, error)
}

// newUpdateChecker builds the production update manager; kept as a variable
// so tests can stub construction without a Forgejo server.
var newUpdateChecker = func(logger *slog.Logger, onAvailable func(updates.Result)) (updateChecker, error) {
	return appupdates.New(appupdates.Options{
		CurrentVersion:    appcore.BuildVersion(),
		Logger:            logger,
		OnUpdateAvailable: onAvailable,
	})
}

// setupUpdateChecker creates the update manager for the application. A nil
// checker is valid: every entry point degrades to a diagnostic dialog or a
// log line.
func (a *application) setupUpdateChecker() {
	checker, err := newUpdateChecker(a.logManager.Logger("updates"), nil)
	if err != nil {
		a.logger.Warn("initialize update checker", "error", err)
		return
	}
	a.updateChecker = checker
}

// checkForUpdates runs one manual update check and presents the outcome.
// Runs on the UI thread; the network round-trip happens in a goroutine.
func (a *application) checkForUpdates() {
	if a.updateChecker == nil {
		dialog.ShowError(errors.New("update checking is unavailable"), a.window)
		return
	}
	if !a.checkingUpdates.CompareAndSwap(false, true) {
		return
	}
	a.setUpdateItemDisabled(true)
	go func() {
		result, err := a.updateChecker.CheckNow(context.Background())
		fyne.Do(func() {
			a.checkingUpdates.Store(false)
			a.setUpdateItemDisabled(false)
			a.presentManualCheckResult(result, err)
		})
	}()
}

// presentManualCheckResult shows the manual-check outcome: unlike the
// automatic path, every result is surfaced, including failures, because the
// user explicitly asked.
func (a *application) presentManualCheckResult(result updates.Result, err error) {
	a.revealMainWindowIfHidden()
	if err != nil {
		a.logger.Warn("manual update check failed", "error", err)
		dialog.ShowError(fmt.Errorf("check for updates failed:\n%w", err), a.window)
		return
	}
	a.showUpdateDialog(result)
}

// revealMainWindowIfHidden makes the main window visible before a dialog is
// attached to it: a dialog on a hidden (tray-minimized) window is invisible
// on most platforms.
func (a *application) revealMainWindowIfHidden() {
	if !a.windowVisible.Load() {
		a.showMainWindow()
	}
}

// showUpdateDialog renders the go4updates release dialog for the fetched
// result. Must run on the UI thread.
func (a *application) showUpdateDialog(result updates.Result) {
	fyneupdate.ShowDialog(a.window, result, fyneupdate.Options{
		Format: releasefmt.Options{
			// The linker requires an absolute repository URL; the feed's
			// value is derived from the configured server.
			Linker:         releasefmt.ForgejoLinker(result.Feed.RepositoryURL),
			ShortCommitSHA: true,
		},
		// Markdown options stay zero: remote release-note images are not
		// fetched.
	})
}

// setUpdateItemDisabled toggles the tray item while a manual check is in
// flight. fyne.MenuItem is a plain data struct without Refresh, so the menu
// is republished — the same idiom updateTray uses. Native tray backends may
// not repaint immediately; the busy guard in checkForUpdates is the actual
// protection.
func (a *application) setUpdateItemDisabled(disabled bool) {
	if !a.trayAvailable || a.trayState.updateItem == nil || a.trayState.desktopApp == nil {
		return
	}
	a.trayState.updateItem.Disabled = disabled
	a.trayState.desktopApp.SetSystemTrayMenu(a.trayMenu())
}
