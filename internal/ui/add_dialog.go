package ui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
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
	win           fyne.Window
	sourceSelect  *widget.RadioGroup
	magnetEntry   *submitEntry
	fileEntry     *submitEntry
	savePathEntry *pathAutocompleteEntry
	categoryEntry *widget.SelectEntry
	tagsEntry     *pathAutocompleteEntry
	submit        *widget.Button
	cancel        *widget.Button
	buttons       *fyne.Container
	advanced      *disclosureSection
	bodyScroll    *container.Scroll
	form          *fyne.Container

	// categoryPaths maps category names to their server save paths.
	categoryPaths map[string]string
	// initialSavePath and autoSavePath are the save path values the dialog
	// filled in itself; a category may replace them, but not a user edit.
	initialSavePath string
	autoSavePath    string
}

// onCategoryChanged fills the save path from the chosen category's path while
// the user hasn't typed their own.
func (d *addTorrentDialog) onCategoryChanged(name string) {
	path := strings.TrimSpace(d.categoryPaths[strings.TrimSpace(name)])
	if path == "" || !shouldAutofillSavePath(d.savePathEntry.Text, d.initialSavePath, d.autoSavePath) {
		return
	}
	d.autoSavePath = path
	d.savePathEntry.SetText(path)
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

// addTorrentWindowSize sizes the Add dialog to fit its content (the capped
// form plus footer, as measured by the window content's minimum size) and
// the window's own padding, so it opens without empty space or a scrollbar.
func addTorrentWindowSize(contentMin fyne.Size, windowPadding float32) fyne.Size {
	return fyne.NewSize(
		max(addDialogMinWidth, contentMin.Width+windowPadding*2),
		contentMin.Height+windowPadding*2,
	)
}

// fitAddWindow resizes an Add dialog window to its content.
func fitAddWindow(win fyne.Window) {
	padding := float32(0)
	if win.Padded() {
		padding = theme.Padding()
	}
	win.Resize(addTorrentWindowSize(win.Content().MinSize(), padding))
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

// disclosureSection is a titled section that expands and collapses its
// detail. Unlike widget.Accordion it reports every toggle, so the owner can
// refit its window and scroll position when the content height changes.
type disclosureSection struct {
	toggle   *widget.Button
	detail   fyne.CanvasObject
	root     *fyne.Container
	open     bool
	onToggle func(open bool)
}

func newDisclosureSection(title string, detail fyne.CanvasObject, open bool, onToggle func(open bool)) *disclosureSection {
	s := &disclosureSection{detail: detail, onToggle: onToggle}
	s.toggle = widget.NewButton(title, func() {
		s.setOpen(!s.open)
		if s.onToggle != nil {
			s.onToggle(s.open)
		}
	})
	s.toggle.Alignment = widget.ButtonAlignLeading
	s.toggle.IconPlacement = widget.ButtonIconLeadingText
	s.toggle.Importance = widget.LowImportance
	s.root = container.NewVBox(container.NewHBox(s.toggle), detail)
	s.setOpen(open)
	return s
}

// Open reports whether the detail is shown.
func (s *disclosureSection) Open() bool {
	return s.open
}

func (s *disclosureSection) setOpen(open bool) {
	s.open = open
	if open {
		s.toggle.SetIcon(theme.MenuDropDownIcon())
		s.detail.Show()
	} else {
		s.toggle.SetIcon(theme.MenuExpandIcon())
		s.detail.Hide()
	}
	s.root.Refresh()
}

// clampScrollOffset pulls a vertical scroll offset back inside its content,
// which Fyne's Scroll doesn't do when the content shrinks.
func clampScrollOffset(scroll *container.Scroll) {
	maxOffset := max(0, scroll.Content.MinSize().Height-scroll.Size().Height)
	if scroll.Offset.Y > maxOffset {
		scroll.ScrollToOffset(fyne.NewPos(scroll.Offset.X, maxOffset))
	}
}
