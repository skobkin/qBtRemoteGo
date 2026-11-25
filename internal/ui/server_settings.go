package ui

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/skobkin/qbtremotego/internal/qbt"
)

const serverPrefsTimeout = 20 * time.Second

// serverPrefsBackend is the slice of the controller the Server settings
// window needs.
type serverPrefsBackend interface {
	FetchPreferences(ctx context.Context) (qbt.Preferences, error)
	ApplyPreferences(ctx context.Context, changes map[string]any) error
	SuggestDirectories(ctx context.Context, path string) ([]string, error)
}

// serverSettingsWindow edits the qBittorrent server's global preferences.
// Edits are applied as one batch: re-fetch, confirm the diff (flagging values
// another client changed meanwhile), send only changed keys, re-read and
// report anything the server did not apply.
type serverSettingsWindow struct {
	backend serverPrefsBackend
	win     fyne.Window
	host    string

	body         *fyne.Container
	form         *prefForm
	reloadButton *widget.Button
	applyButton  *widget.Button
	busy         bool

	// Seams so tests can drive the flow synchronously.
	run        func(work func(), done func())
	confirm    func(title string, content fyne.CanvasObject, confirmText string, onConfirm func())
	showError  func(err error)
	notify     func(title, message string)
	afterApply func()
	openConfig func()
}

func (a *application) openServerSettingsWindow() {
	if a.serverSettings != nil {
		a.serverSettings.win.Show()
		a.serverSettings.win.RequestFocus()
		return
	}

	w := newServerSettingsWindow(a.fyApp, a.controller, remoteHostLabel(a.controller.Config().Connection.URL))
	w.afterApply = func() { go a.refreshNow() }
	w.openConfig = a.openSettingsWindow
	a.serverSettings = w
	w.win.SetOnClosed(func() {
		w.dispose()
		if a.serverSettings == w {
			a.serverSettings = nil
		}
	})
	w.win.Show()
	w.load()
}

func newServerSettingsWindow(fyApp fyne.App, backend serverPrefsBackend, host string) *serverSettingsWindow {
	w := &serverSettingsWindow{backend: backend, host: host}
	w.win = fyApp.NewWindow("Server settings — " + host)
	w.win.Resize(fyne.NewSize(640, 560))

	w.run = func(work func(), done func()) {
		go func() {
			work()
			fyne.Do(done)
		}()
	}
	w.confirm = func(title string, content fyne.CanvasObject, confirmText string, onConfirm func()) {
		d := dialog.NewCustomConfirm(title, confirmText, "Cancel", content, func(ok bool) {
			if ok {
				onConfirm()
			}
		}, w.win)
		d.Show()
	}
	w.showError = func(err error) { dialog.ShowError(err, w.win) }
	w.notify = func(title, message string) { dialog.ShowInformation(title, message, w.win) }
	w.afterApply = func() {}
	w.openConfig = func() {}

	banner := widget.NewLabel(fmt.Sprintf("Changes apply to the qBittorrent server %s for all of its clients, not only this app.", host))
	banner.Wrapping = fyne.TextWrapWord
	bannerRow := container.NewBorder(nil, nil, widget.NewIcon(theme.WarningIcon()), nil, banner)

	w.body = container.NewStack()
	w.reloadButton = widget.NewButtonWithIcon("Reload", theme.ViewRefreshIcon(), w.requestReload)
	closeButton := widget.NewButton("Close", w.requestClose)
	w.applyButton = widget.NewButtonWithIcon("Apply…", theme.ConfirmIcon(), w.startApply)
	w.applyButton.Importance = widget.HighImportance
	buttons := container.NewHBox(w.reloadButton, layout.NewSpacer(), closeButton, w.applyButton)

	top := container.NewVBox(bannerRow, widget.NewSeparator())
	bottom := container.NewVBox(widget.NewSeparator(), buttons)
	w.win.SetContent(container.NewBorder(top, bottom, nil, nil, w.body))

	w.win.SetCloseIntercept(w.requestClose)
	w.win.Canvas().SetOnTypedKey(func(event *fyne.KeyEvent) {
		if event.Name != fyne.KeyEscape {
			return
		}
		// Fyne dialogs ignore Esc: close the topmost one (as a cancel) first.
		if top := w.win.Canvas().Overlays().Top(); top != nil {
			w.win.Canvas().Overlays().Remove(top)
			return
		}
		w.requestClose()
	})
	w.syncButtons()

	return w
}

// remoteHostLabel names the server in the window title and warnings.
func remoteHostLabel(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Host == "" {
		if rawURL = strings.TrimSpace(rawURL); rawURL != "" {
			return rawURL
		}
		return "(not configured)"
	}

	return parsed.Host
}

func (w *serverSettingsWindow) setBody(obj fyne.CanvasObject) {
	w.body.Objects = []fyne.CanvasObject{obj}
	w.body.Refresh()
}

func (w *serverSettingsWindow) setBusy(busy bool) {
	w.busy = busy
	w.syncButtons()
}

func (w *serverSettingsWindow) syncButtons() {
	setWidgetEnabled(w.reloadButton, !w.busy)
	setWidgetEnabled(w.applyButton, !w.busy && w.form != nil && len(w.form.fields) > 0)
}

// load fetches the preferences and rebuilds the form.
func (w *serverSettingsWindow) load() {
	w.setBusy(true)
	w.setBody(container.NewCenter(widget.NewLabel("Loading server settings…")))

	var prefs qbt.Preferences
	var err error
	w.run(func() {
		ctx, cancel := context.WithTimeout(context.Background(), serverPrefsTimeout)
		defer cancel()
		prefs, err = w.backend.FetchPreferences(ctx)
	}, func() {
		w.setBusy(false)
		w.dispose()
		if err != nil {
			w.setBody(w.loadErrorView(err))
			w.syncButtons()
			return
		}
		w.form = newPrefForm(serverPrefTabs(w.backend.SuggestDirectories), prefs)
		w.setBody(w.form.tabs)
		w.syncButtons()
	})
}

// dispose releases the current form's background resources and drops it.
func (w *serverSettingsWindow) dispose() {
	if w.form != nil {
		w.form.close()
		w.form = nil
	}
}

func (w *serverSettingsWindow) loadErrorView(err error) fyne.CanvasObject {
	message := widget.NewLabel("Could not load the server settings:\n" + err.Error())
	message.Wrapping = fyne.TextWrapWord
	retry := widget.NewButtonWithIcon("Retry", theme.ViewRefreshIcon(), w.load)
	settings := widget.NewButtonWithIcon("Connection settings", theme.SettingsIcon(), func() { w.openConfig() })

	return container.NewVBox(message, container.NewHBox(retry, settings))
}

// dirty reports unapplied edits; an invalid field counts as an edit.
func (w *serverSettingsWindow) dirty() bool {
	if w.form == nil {
		return false
	}
	edited, err := w.form.edited()
	if err != nil {
		return true
	}

	return len(changedFields(w.form.fields, w.form.baseline, edited)) > 0
}

func (w *serverSettingsWindow) requestClose() {
	if !w.dirty() {
		w.win.Close()
		return
	}
	w.confirm("Discard changes?", widget.NewLabel("The edited server settings have not been applied."), "Discard", w.win.Close)
}

func (w *serverSettingsWindow) requestReload() {
	if !w.dirty() {
		w.load()
		return
	}
	w.confirm("Discard changes?", widget.NewLabel("Reloading replaces the edited values with the server's current settings."), "Reload", w.load)
}

// startApply validates the form, re-fetches the server state to detect
// concurrent edits and asks for confirmation.
func (w *serverSettingsWindow) startApply() {
	if w.form == nil || w.busy {
		return
	}
	edited, err := w.form.edited()
	if err != nil {
		w.showError(err)
		return
	}
	changed := changedFields(w.form.fields, w.form.baseline, edited)
	if len(changed) == 0 {
		w.notify("No changes", "None of the server settings were changed.")
		return
	}

	w.setBusy(true)
	var fresh qbt.Preferences
	w.run(func() {
		ctx, cancel := context.WithTimeout(context.Background(), serverPrefsTimeout)
		defer cancel()
		fresh, err = w.backend.FetchPreferences(ctx)
	}, func() {
		w.setBusy(false)
		if err != nil {
			w.showError(fmt.Errorf("check the server's current settings: %w", err))
			return
		}
		freshValues := decodePrefValues(w.form.fields, fresh)
		conflicts := conflictingFields(changed, w.form.baseline, freshValues)
		lines := changeSummaryLines(changed, w.form.baseline, edited, freshValues, conflicts)

		intro := fmt.Sprintf("Apply these changes to %s? They affect every client of this server.", w.host)
		content := []fyne.CanvasObject{widget.NewLabel(intro)}
		for _, line := range lines {
			label := widget.NewLabel(line)
			label.Wrapping = fyne.TextWrapWord
			content = append(content, label)
		}
		confirmText := "Apply"
		if len(conflicts) > 0 {
			warning := widget.NewLabelWithStyle(
				"Some values were changed on the server since this window loaded them. Apply anyway to overwrite them, or cancel and Reload.",
				fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
			warning.Wrapping = fyne.TextWrapWord
			content = append(content, warning)
			confirmText = "Apply anyway"
		}
		w.confirm("Apply server settings", container.NewVBox(content...), confirmText, func() {
			w.sendChanges(changed, edited)
		})
	})
}

// changeSummaryLines describes each change as "name: old → new", marking
// values another client changed since the window loaded.
func changeSummaryLines(changed []*prefField, baseline, edited, fresh prefValues, conflicts []*prefField) []string {
	conflicting := map[*prefField]bool{}
	for _, field := range conflicts {
		conflicting[field] = true
	}
	lines := make([]string, 0, len(changed))
	for _, field := range changed {
		line := fmt.Sprintf("%s: %s → %s", field.name, field.format(baseline), field.format(edited))
		if conflicting[field] {
			line += fmt.Sprintf("  (server now: %s)", field.format(fresh))
		}
		lines = append(lines, line)
	}

	return lines
}

// sendChanges applies the changed fields, re-reads the preferences and
// reports what the server did not take.
func (w *serverSettingsWindow) sendChanges(changed []*prefField, edited prefValues) {
	payload := changesPayload(changed, edited)
	w.setBusy(true)

	var applyErr, rereadErr error
	var reread qbt.Preferences
	w.run(func() {
		ctx, cancel := context.WithTimeout(context.Background(), serverPrefsTimeout)
		defer cancel()
		applyErr = w.backend.ApplyPreferences(ctx, payload)
		// Re-read even after an error: the server applies keys one by one,
		// so a rejected request can still have changed some of them.
		reread, rereadErr = w.backend.FetchPreferences(ctx)
	}, func() {
		w.setBusy(false)
		defer w.afterApply()

		if rereadErr != nil {
			if applyErr != nil {
				w.showError(fmt.Errorf("apply server settings: %w", errors.Join(applyErr, rereadErr)))
				return
			}
			// Applied, but the result could not be verified: treat the sent
			// values as the new baseline so the form stops reporting them.
			for key, value := range payload {
				w.form.baseline[key] = value
			}
			w.showError(fmt.Errorf("the settings were sent, but re-reading them failed: %w", rereadErr))
			return
		}

		values := decodePrefValues(w.form.fields, reread)
		w.form.baseline = values
		w.form.load(values)

		if applyErr != nil {
			w.showError(fmt.Errorf("the server rejected the changes (the form now shows its current settings): %w", applyErr))
			return
		}
		if unapplied := unappliedFields(changed, edited, values); len(unapplied) > 0 {
			lines := make([]string, 0, len(unapplied))
			for _, field := range unapplied {
				lines = append(lines, fmt.Sprintf("%s: sent %s, server has %s", field.name, field.format(edited), field.format(values)))
			}
			w.notify("Some changes were not applied", strings.Join(lines, "\n"))
			return
		}
		w.notify("Server settings applied", fmt.Sprintf("Applied %d change(s) to %s.", len(changed), w.host))
	})
}
