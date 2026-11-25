package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func TestSubmitEntryCtrlEnterAndEscape(t *testing.T) {
	test.NewTempApp(t)
	submitted, cancelled := 0, 0
	entry := newSubmitEntry(true, func() { submitted++ }, func() { cancelled++ })

	entry.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyReturn, Modifier: fyne.KeyModifierShortcutDefault})
	entry.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyEnter, Modifier: fyne.KeyModifierShortcutDefault})
	entry.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})

	if submitted != 2 {
		t.Fatalf("submitted %d times, want 2", submitted)
	}
	if cancelled != 1 {
		t.Fatalf("cancelled %d times, want 1", cancelled)
	}

	// Plain Enter still inserts a newline in the multi-line entry.
	entry.SetText("a")
	entry.CursorColumn = 1
	entry.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if entry.Text != "a\n" {
		t.Fatalf("text %q after Enter, want a newline", entry.Text)
	}
}

func TestAddSourceReady(t *testing.T) {
	tests := []struct {
		name     string
		magnet   bool
		magnets  string
		filePath string
		want     bool
	}{
		{"empty magnets", true, "  \n ", "", false},
		{"magnet text", true, "magnet:?xt=urn:btih:aaa", "", true},
		{"magnet mode ignores file path", true, "", "/tmp/a.torrent", false},
		{"empty file path", false, "magnet:?xt=urn:btih:aaa", " ", false},
		{"file path", false, "", "/tmp/a.torrent", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := addSourceReady(tt.magnet, tt.magnets, tt.filePath); got != tt.want {
				t.Fatalf("addSourceReady = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddDialogButtonsAndSourceGate(t *testing.T) {
	app := newTorrentTableTestApp(t)
	d := app.openAddWindowWith(nil, addWindowOptions{})
	t.Cleanup(d.win.Close)

	if d.submit.Importance != widget.HighImportance {
		t.Fatal("Add should be the primary button")
	}
	buttons := d.buttons.Objects
	if buttons[len(buttons)-2] != d.cancel || buttons[len(buttons)-1] != d.submit {
		t.Fatal("buttons should read Cancel, Add")
	}
	if !d.submit.Disabled() {
		t.Fatal("Add enabled with no source")
	}

	d.magnetEntry.SetText("magnet:?xt=urn:btih:aaa")
	if d.submit.Disabled() {
		t.Fatal("Add disabled with a magnet link")
	}

	d.sourceSelect.SetSelected(addSourceTorrentFile)
	if !d.submit.Disabled() {
		t.Fatal("Add enabled in file mode with no file")
	}
	d.fileEntry.SetText("/tmp/a.torrent")
	if d.submit.Disabled() {
		t.Fatal("Add disabled with a file path")
	}
}

func TestAddDialogEscapeClosesWindow(t *testing.T) {
	app := newTorrentTableTestApp(t)
	d := app.openAddWindowWith(nil, addWindowOptions{})
	closed := false
	d.win.SetOnClosed(func() { closed = true })

	d.win.Canvas().OnTypedKey()(&fyne.KeyEvent{Name: fyne.KeyEscape})

	if !closed {
		t.Fatal("Esc did not close the Add dialog")
	}
}

func TestClipboardMagnets(t *testing.T) {
	text := "see magnet:?xt=urn:btih:inline\nmagnet:?xt=urn:btih:aaa\r\n  MAGNET:?xt=urn:btih:bbb \n/tmp/a.torrent"
	got := clipboardMagnets(text)
	if len(got) != 2 || got[0] != "magnet:?xt=urn:btih:aaa" || got[1] != "MAGNET:?xt=urn:btih:bbb" {
		t.Fatalf("clipboardMagnets = %v, want the two magnet lines", got)
	}
}

func TestAddDialogPrefillsMagnetsFromClipboard(t *testing.T) {
	app := newTorrentTableTestApp(t)
	app.fyApp.Clipboard().SetContent("magnet:?xt=urn:btih:aaa")

	d := app.openAddWindowWith(nil, addWindowOptions{})
	t.Cleanup(d.win.Close)

	if d.magnetEntry.Text != "magnet:?xt=urn:btih:aaa" {
		t.Fatalf("magnet text %q, want the clipboard link", d.magnetEntry.Text)
	}
	if !d.sourceSelect.Horizontal {
		t.Fatal("source options should sit on one row")
	}
	if d.magnetEntry.PlaceHolder == "" || d.fileEntry.PlaceHolder == "" {
		t.Fatal("source entries need placeholders")
	}
}

func TestAddDialogCategoryFillsSavePathUntilEdited(t *testing.T) {
	app := newTorrentTableTestApp(t)
	d := app.openAddWindowWith(nil, addWindowOptions{})
	t.Cleanup(d.win.Close)
	d.categoryPaths = map[string]string{"movies": "/data/movies", "iso": "/data/iso", "plain": ""}

	d.categoryEntry.SetText("movies")
	if d.savePathEntry.Text != "/data/movies" {
		t.Fatalf("save path %q after picking movies, want /data/movies", d.savePathEntry.Text)
	}

	d.categoryEntry.SetText("plain")
	if d.savePathEntry.Text != "/data/movies" {
		t.Fatalf("save path %q after a category without a path, want it unchanged", d.savePathEntry.Text)
	}

	d.savePathEntry.SetText("/mine")
	d.categoryEntry.SetText("iso")
	if d.savePathEntry.Text != "/mine" {
		t.Fatalf("save path %q after the user edited it, want /mine kept", d.savePathEntry.Text)
	}
}

func TestAddDialogTagsEntrySuggestsKnownTags(t *testing.T) {
	app := newTorrentTableTestApp(t)
	d := app.openAddWindowWith(nil, addWindowOptions{})
	t.Cleanup(d.win.Close)

	if d.tagsEntry.localSuggest == nil || d.tagsEntry.acceptOnSpace {
		t.Fatal("tags entry should be a tag autocomplete entry")
	}
}

func TestDisclosureSectionTogglesDetail(t *testing.T) {
	test.NewTempApp(t)
	var toggles []bool
	detail := widget.NewLabel("advanced")
	section := newDisclosureSection("Advanced", detail, false, func(open bool) { toggles = append(toggles, open) })

	if detail.Visible() || section.Open() {
		t.Fatal("section should start collapsed")
	}
	test.Tap(section.toggle)
	if !detail.Visible() || !section.Open() {
		t.Fatal("tapping should expand the section")
	}
	test.Tap(section.toggle)
	if detail.Visible() || section.Open() {
		t.Fatal("tapping again should collapse the section")
	}
	if len(toggles) != 2 || !toggles[0] || toggles[1] {
		t.Fatalf("toggle callbacks %v, want [true false]", toggles)
	}
}

// Collapsing Advanced while scrolled down used to leave the scroll offset
// past the shorter form, drawing it entirely above the viewport (a blank
// dialog). The toggle must clamp the offset and refit the window both ways.
func TestAddDialogAdvancedToggleRefitsWindowAndScroll(t *testing.T) {
	app := newTorrentTableTestApp(t)
	d := app.openAddWindowWith(nil, addWindowOptions{})
	t.Cleanup(d.win.Close)
	collapsed := d.win.Canvas().Size()

	test.Tap(d.advanced.toggle)
	expanded := d.win.Canvas().Size()
	if expanded.Height <= collapsed.Height {
		t.Fatalf("window height %.0f after expanding, want more than %.0f", expanded.Height, collapsed.Height)
	}

	d.bodyScroll.ScrollToOffset(fyne.NewPos(0, 300))
	test.Tap(d.advanced.toggle)

	if d.bodyScroll.Offset.Y != 0 {
		t.Fatalf("scroll offset %.0f after collapsing, want 0", d.bodyScroll.Offset.Y)
	}
	if got := d.win.Canvas().Size().Height; got != collapsed.Height {
		t.Fatalf("window height %.0f after collapsing, want %.0f", got, collapsed.Height)
	}
}
