package ui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	appcore "github.com/skobkin/qbtremotego/internal/app"
	"github.com/skobkin/qbtremotego/internal/qbt"
)

// mainKeyAction is what a plain key press does in the main window when no
// widget (such as the search box) has keyboard focus.
type mainKeyAction int

const (
	keyActionNone mainKeyAction = iota
	keyActionRemove
	keyActionClearSelection
	keyActionOpenDetails
	keyActionRefresh
)

func mainKeyActionFor(key fyne.KeyName) mainKeyAction {
	switch key {
	case fyne.KeyDelete:
		return keyActionRemove
	case fyne.KeyEscape:
		return keyActionClearSelection
	case fyne.KeyReturn, fyne.KeyEnter:
		return keyActionOpenDetails
	case fyne.KeyF5:
		return keyActionRefresh
	default:
		return keyActionNone
	}
}

// mainShortcut pairs a main-window shortcut with its handler.
type mainShortcut struct {
	shortcut fyne.Shortcut
	handler  func(fyne.Shortcut)
}

// mainShortcuts lists the main window's modifier shortcuts. They use the
// platform modifier (Ctrl, or Cmd on macOS). A focused entry receives its own
// clipboard shortcuts first, so pasting into the search box still pastes text.
func (a *application) mainShortcuts() []mainShortcut {
	mod := fyne.KeyModifierShortcutDefault
	return []mainShortcut{
		// Fyne reports Shift+Del as the secondary Cut shortcut.
		{&fyne.ShortcutCut{Secondary: true}, a.handleCutShortcut},
		{&desktop.CustomShortcut{KeyName: fyne.KeyN, Modifier: mod}, func(fyne.Shortcut) {
			a.openAddWindow(nil)
		}},
		{&desktop.CustomShortcut{KeyName: fyne.KeyO, Modifier: mod}, func(fyne.Shortcut) {
			a.openAddWindowWith(&appcore.AddDialogPrefill{SourceType: qbt.SourceTorrentFile}, addWindowOptions{browse: true})
		}},
		{&desktop.CustomShortcut{KeyName: fyne.KeyF, Modifier: mod}, func(fyne.Shortcut) {
			if a.filterEntry != nil {
				a.window.Canvas().Focus(a.filterEntry)
			}
		}},
		{&fyne.ShortcutPaste{}, func(fyne.Shortcut) { a.pasteFromClipboard() }},
	}
}

// installMainShortcuts wires the main window's keyboard shortcuts. Plain keys reach handleMainKey only while no widget (such as the
// search box) has keyboard focus.
func (a *application) installMainShortcuts() {
	c := a.window.Canvas()
	c.SetOnTypedKey(a.handleMainKey)
	for _, entry := range a.mainShortcuts() {
		c.AddShortcut(entry.shortcut, entry.handler)
	}
}

// handleCutShortcut handles the Cut shortcut while no widget has focus. Only
// its secondary form (Shift+Del) means "remove with files"; Ctrl+X is ignored
// because it shares the shortcut name.
func (a *application) handleCutShortcut(shortcut fyne.Shortcut) {
	cut, ok := shortcut.(*fyne.ShortcutCut)
	if !ok || !cut.Secondary || len(a.selection) == 0 {
		return
	}
	a.confirmDeleteWith(true)
}

func (a *application) handleMainKey(event *fyne.KeyEvent) {
	switch mainKeyActionFor(event.Name) {
	case keyActionRemove:
		if len(a.selection) > 0 {
			a.confirmDeleteWith(false)
		}
	case keyActionClearSelection:
		a.clearSelection()
	case keyActionOpenDetails:
		if hashes := a.selectedHashes(); len(hashes) > 0 {
			a.openTorrentDetails(hashes[0])
		}
	case keyActionRefresh:
		a.refreshNow()
	case keyActionNone:
	}
}

// clearSelection deselects every torrent.
func (a *application) clearSelection() {
	if len(a.selection) == 0 {
		return
	}
	a.selection = map[string]bool{}
	a.selectionAnchor = ""
	a.refreshTorrentSelection()
}

// pasteFromClipboard opens prefilled Add dialogs for magnet links and existing
// .torrent paths on the clipboard; anything else is ignored.
func (a *application) pasteFromClipboard() {
	clipboard := a.fyApp.Clipboard()
	if clipboard == nil {
		return
	}
	batch := clipboardInvocation(clipboard.Content())
	if batch.Empty() {
		a.logger.Debug("clipboard paste ignored: no magnet links or .torrent paths")
		return
	}
	a.handleInvocation(batch)
}

// clipboardInvocation parses clipboard text, one candidate per line (paths
// may contain spaces), with the same rules as command-line arguments.
func clipboardInvocation(text string) appcore.InvocationBatch {
	lines := strings.FieldsFunc(text, func(r rune) bool { return r == '\n' || r == '\r' })
	return appcore.ParseInvocationArgs(lines)
}
