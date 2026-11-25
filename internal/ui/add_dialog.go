package ui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
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

const (
	// addDialogMinWidth is the narrowest the Add dialog opens.
	addDialogMinWidth = float32(720)
	// addDialogMaxBodyHeight caps the form area; taller forms scroll.
	addDialogMaxBodyHeight = float32(640)
)

// addTorrentWindowSize sizes the Add dialog to its form and footer, so it
// opens without the empty space a fixed size left under a short form.
func addTorrentWindowSize(content fyne.Size, footer fyne.Size) fyne.Size {
	width := max(addDialogMinWidth, content.Width, footer.Width)
	height := min(content.Height, addDialogMaxBodyHeight) + footer.Height
	return fyne.NewSize(width, height)
}

// cappedHeightLayout gives a scroll container its content's natural size up
// to maxHeight. Fyne grows the window when a child's minimum size outgrows
// it, so expanding "Advanced" enlarges the dialog instead of scrolling inside
// a window that stays too short.
type cappedHeightLayout struct {
	maxHeight float32
}

func (l *cappedHeightLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	for _, obj := range objects {
		obj.Move(fyne.NewPos(0, 0))
		obj.Resize(size)
	}
}

func (l *cappedHeightLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	var out fyne.Size
	for _, obj := range objects {
		content := obj.MinSize()
		if scroll, ok := obj.(*container.Scroll); ok && scroll.Content != nil {
			content = scroll.Content.MinSize()
		}
		out = out.Max(fyne.NewSize(content.Width, min(content.Height, l.maxHeight)))
	}
	return out
}

// clipboardMagnets returns the clipboard lines that are magnet links.
func clipboardMagnets(text string) []string {
	var out []string
	for _, line := range strings.FieldsFunc(text, func(r rune) bool { return r == '\n' || r == '\r' }) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(line), "magnet:") {
			out = append(out, line)
		}
	}
	return out
}
