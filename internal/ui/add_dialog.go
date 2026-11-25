package ui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// Add dialog source options.
const (
	addSourceTorrentFile = "Torrent file"
	addSourceMagnet      = "Magnet links"
)

// addTorrentDialog holds the Add dialog widgets that other code (and tests)
// needs after the window is built.
type addTorrentDialog struct {
	win          fyne.Window
	sourceSelect *widget.RadioGroup
	magnetEntry  *submitEntry
	fileEntry    *submitEntry
	submit       *widget.Button
	cancel       *widget.Button
	buttons      *fyne.Container
}

// addSourceReady reports whether the active source has something to add.
func addSourceReady(magnetMode bool, magnetText string, filePath string) bool {
	if magnetMode {
		return strings.TrimSpace(magnetText) != ""
	}
	return strings.TrimSpace(filePath) != ""
}

// isSubmitShortcut matches Ctrl+Enter (Cmd+Enter on macOS).
func isSubmitShortcut(shortcut fyne.Shortcut) bool {
	custom, ok := shortcut.(*desktop.CustomShortcut)
	if !ok || custom.Modifier != fyne.KeyModifierShortcutDefault {
		return false
	}
	return custom.KeyName == fyne.KeyReturn || custom.KeyName == fyne.KeyEnter
}

// submitEntry is an entry that submits its form on Ctrl+Enter and cancels it
// on Esc. A focused entry receives these keys before the window does, so the
// dialog-level bindings alone would never fire while typing.
type submitEntry struct {
	widget.Entry
	onSubmit func()
	onCancel func()
}

func newSubmitEntry(multiLine bool, onSubmit func(), onCancel func()) *submitEntry {
	e := &submitEntry{onSubmit: onSubmit, onCancel: onCancel}
	e.MultiLine = multiLine
	if multiLine {
		e.Wrapping = fyne.TextWrapBreak
	}
	e.ExtendBaseWidget(e)
	return e
}

func (e *submitEntry) TypedShortcut(shortcut fyne.Shortcut) {
	if isSubmitShortcut(shortcut) {
		if e.onSubmit != nil {
			e.onSubmit()
		}
		return
	}
	e.Entry.TypedShortcut(shortcut)
}

func (e *submitEntry) TypedKey(event *fyne.KeyEvent) {
	if event.Name == fyne.KeyEscape && e.onCancel != nil {
		e.onCancel()
		return
	}
	e.Entry.TypedKey(event)
}
