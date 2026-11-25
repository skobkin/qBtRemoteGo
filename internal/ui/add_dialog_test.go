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
