package ui

import (
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func typeMainKey(app *application, name fyne.KeyName) {
	app.window.Canvas().OnTypedKey()(&fyne.KeyEvent{Name: name})
}

// typeMainShortcut runs the main-window handler registered for a shortcut.
// Fyne's test canvas cannot dispatch shortcuts itself.
func typeMainShortcut(app *application, shortcut fyne.Shortcut) {
	for _, entry := range app.mainShortcuts() {
		if entry.shortcut.ShortcutName() == shortcut.ShortcutName() {
			entry.handler(shortcut)
			return
		}
	}
	panic("no main shortcut registered for " + shortcut.ShortcutName())
}

// findChecks collects every check box in an object tree.
func findChecks(obj fyne.CanvasObject) []*widget.Check {
	var out []*widget.Check
	switch o := obj.(type) {
	case *widget.Check:
		return append(out, o)
	case *fyne.Container:
		for _, child := range o.Objects {
			out = append(out, findChecks(child)...)
		}
		return out
	case fyne.Widget:
		for _, child := range test.WidgetRenderer(o).Objects() {
			out = append(out, findChecks(child)...)
		}
	}
	return out
}

func TestMainKeyAction(t *testing.T) {
	tests := map[fyne.KeyName]mainKeyAction{
		fyne.KeyDelete: keyActionRemove,
		fyne.KeyEscape: keyActionClearSelection,
		fyne.KeyReturn: keyActionOpenDetails,
		fyne.KeyEnter:  keyActionOpenDetails,
		fyne.KeyF5:     keyActionRefresh,
		fyne.KeyA:      keyActionNone,
	}
	for key, want := range tests {
		if got := mainKeyActionFor(key); got != want {
			t.Errorf("mainKeyActionFor(%s) = %v, want %v", key, got, want)
		}
	}
}

func TestEscapeClearsSelection(t *testing.T) {
	app := newTorrentTableTestApp(t)
	app.applyTorrentSelection("hash-a", 0)

	typeMainKey(app, fyne.KeyEscape)

	if len(app.selection) != 0 {
		t.Fatalf("selection %v after Esc, want empty", app.selection)
	}
	if app.rowForTest(t, "hash-a").selectedShown {
		t.Fatal("row still shows its selection after Esc")
	}
}

func TestDeleteKeyNeedsSelection(t *testing.T) {
	app := newTorrentTableTestApp(t)

	typeMainKey(app, fyne.KeyDelete)
	if app.window.Canvas().Overlays().Top() != nil {
		t.Fatal("Del without a selection opened a dialog")
	}

	app.applyTorrentSelection("hash-a", 0)
	typeMainKey(app, fyne.KeyDelete)
	top := app.window.Canvas().Overlays().Top()
	if top == nil {
		t.Fatal("Del with a selection did not open the remove confirmation")
	}
	checks := findChecks(top)
	if len(checks) != 1 || checks[0].Checked {
		t.Fatalf("plain Del should leave 'remove files' unticked, got %d checks", len(checks))
	}
}

func TestShiftDeletePreticksRemoveFiles(t *testing.T) {
	app := newTorrentTableTestApp(t)
	app.applyTorrentSelection("hash-a", 0)

	// Fyne's GLFW driver reports Shift+Del as the secondary Cut shortcut.
	typeMainShortcut(app, &fyne.ShortcutCut{Secondary: true})

	top := app.window.Canvas().Overlays().Top()
	if top == nil {
		t.Fatal("Shift+Del did not open the remove confirmation")
	}
	checks := findChecks(top)
	if len(checks) != 1 || !checks[0].Checked {
		t.Fatal("Shift+Del should pre-tick 'Also remove content files'")
	}
}

func TestCtrlFFocusesSearch(t *testing.T) {
	app := newTorrentTableTestApp(t)

	typeMainShortcut(app, &desktop.CustomShortcut{KeyName: fyne.KeyF, Modifier: fyne.KeyModifierShortcutDefault})

	if app.window.Canvas().Focused() != app.filterEntry {
		t.Fatal("Ctrl+F did not focus the search box")
	}
}

func TestClipboardInvocationParsesMagnetsAndTorrentFiles(t *testing.T) {
	dir := t.TempDir()
	torrentPath := filepath.Join(dir, "linux.torrent")
	if err := os.WriteFile(torrentPath, []byte("d4:infod4:name1:xee"), 0o600); err != nil {
		t.Fatalf("write torrent: %v", err)
	}
	text := "magnet:?xt=urn:btih:aaa\n  magnet:?xt=urn:btih:bbb  \r\n" + torrentPath + "\n/missing/file.torrent\nnot a link"

	batch := clipboardInvocation(text)

	if len(batch.MagnetLinks) != 2 || batch.MagnetLinks[1] != "magnet:?xt=urn:btih:bbb" {
		t.Fatalf("magnets %v, want two trimmed links", batch.MagnetLinks)
	}
	if len(batch.TorrentFiles) != 1 || batch.TorrentFiles[0] != torrentPath {
		t.Fatalf("torrent files %v, want only the existing file", batch.TorrentFiles)
	}
}

func TestCtrlXDoesNotRemove(t *testing.T) {
	app := newTorrentTableTestApp(t)
	app.applyTorrentSelection("hash-a", 0)

	app.handleCutShortcut(&fyne.ShortcutCut{})

	if app.window.Canvas().Overlays().Top() != nil {
		t.Fatal("Ctrl+X must not open the remove confirmation")
	}
}
