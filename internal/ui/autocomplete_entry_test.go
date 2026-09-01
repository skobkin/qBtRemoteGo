package ui

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
)

func TestFilterMatchingPaths(t *testing.T) {
	paths := []string{"", "/data/Alpha", "/data/alpine", "/other/path", "/data/alpha"}

	items := filterMatchingPaths(paths, "/data/al")

	if len(items) != 2 {
		t.Fatalf("unexpected match count: %#v", items)
	}
	if items[0] != "/data/Alpha" || items[1] != "/data/alpine" {
		t.Fatalf("unexpected matches: %#v", items)
	}
}

func TestPathAutocompleteEntryKeyboardSelection(t *testing.T) {
	test.NewTempApp(t)

	entry := newPathAutocompleteEntryWithDelay([]string{"/data/alpha", "/data/alpine"}, 0, nil, nil)
	defer entry.Close()

	win := test.NewWindow(entry)
	defer win.Close()
	win.Resize(fyne.NewSize(420, 300))
	entry.Resize(fyne.NewSize(320, entry.MinSize().Height))
	entry.Move(fyne.NewPos(20, 20))
	entry.FocusGained()

	fyne.DoAndWait(func() {
		entry.SetText("/data/al")
	})

	if !entry.popupVisible() {
		t.Fatalf("expected popup to be visible")
	}
	if entry.selectedIndex() != -1 {
		t.Fatalf("expected no preselected suggestion, got %d", entry.selectedIndex())
	}

	fyne.DoAndWait(func() {
		entry.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
		entry.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
		entry.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEnter})
	})

	if entry.Text != "/data/alpine" {
		t.Fatalf("unexpected accepted suggestion: %q", entry.Text)
	}
	if entry.CursorColumn != len([]rune("/data/alpine")) {
		t.Fatalf("expected cursor at end of accepted suggestion, got %d", entry.CursorColumn)
	}
	if entry.popupVisible() {
		t.Fatalf("expected popup to hide after accepting suggestion")
	}
}

func TestPathAutocompleteEntryMouseSelection(t *testing.T) {
	test.NewTempApp(t)

	entry := newPathAutocompleteEntryWithDelay([]string{"/data/alpha", "/data/alpine"}, 0, nil, nil)
	defer entry.Close()

	win := test.NewWindow(entry)
	defer win.Close()
	win.Resize(fyne.NewSize(420, 300))
	entry.Resize(fyne.NewSize(320, entry.MinSize().Height))
	entry.Move(fyne.NewPos(20, 20))
	entry.FocusGained()

	fyne.DoAndWait(func() {
		entry.SetText("/data/al")
	})

	if !entry.popupVisible() {
		t.Fatalf("expected popup to be visible")
	}

	test.TapCanvas(
		win.Canvas(),
		entry.popupPosition().Add(fyne.NewPos(24, entry.MinSize().Height/2)),
	)

	if entry.Text != "/data/alpha" {
		t.Fatalf("unexpected accepted suggestion from mouse click: %q", entry.Text)
	}
}

func TestPathAutocompleteEntryIgnoresStaleRemoteResults(t *testing.T) {
	test.NewTempApp(t)

	// The debounce timer is parked for the whole test so fetches only run when
	// the test drives them; every state mutation stays on the test goroutine.
	fetcher := func(_ context.Context, query string) ([]string, error) {
		if query == "/data/a" {
			return []string{"/data/alpha"}, nil
		}
		return []string{"/data/alpine"}, nil
	}

	entry := newPathAutocompleteEntryWithDelay(nil, time.Hour, fetcher, nil)
	defer entry.Close()

	win := test.NewWindow(entry)
	defer win.Close()
	win.Resize(fyne.NewSize(420, 300))
	entry.Resize(fyne.NewSize(320, entry.MinSize().Height))
	entry.Move(fyne.NewPos(20, 20))
	entry.FocusGained()

	fyne.DoAndWait(func() {
		entry.SetText("/data/a")
	})
	staleGen := entry.currentGeneration()
	staleQuery := "/data/a"

	fyne.DoAndWait(func() {
		entry.SetText("/data/al")
	})
	currentGen := entry.currentGeneration()

	// The superseded fetch is discarded before it even calls the fetcher.
	fyne.DoAndWait(func() {
		entry.runRemoteFetch(staleGen, staleQuery)
	})
	if len(entry.suggestions) != 0 {
		t.Fatalf("stale suggestions overwrote current state: %#v", entry.suggestions)
	}

	fyne.DoAndWait(func() {
		entry.runRemoteFetch(currentGen, "/data/al")
	})
	if len(entry.suggestions) != 1 || entry.suggestions[0] != "/data/alpine" {
		t.Fatalf("unexpected suggestions: %#v", entry.suggestions)
	}
}

func TestPathAutocompleteEntryDropsResultOlderThanCurrentText(t *testing.T) {
	test.NewTempApp(t)

	var entry *pathAutocompleteEntry
	fetcher := func(_ context.Context, query string) ([]string, error) {
		if query != "/data/al" {
			t.Errorf("unexpected fetch query: %q", query)
		}
		// The user keeps typing while the request is in flight; the result now
		// answers an older text and must not land when it completes.
		entry.SetText("/data/alp")
		return []string{"/data/alpine"}, nil
	}
	entry = newPathAutocompleteEntryWithDelay(nil, time.Hour, fetcher, nil)
	defer entry.Close()

	win := test.NewWindow(entry)
	defer win.Close()
	win.Resize(fyne.NewSize(420, 300))
	entry.Resize(fyne.NewSize(320, entry.MinSize().Height))
	entry.Move(fyne.NewPos(20, 20))
	entry.FocusGained()

	fyne.DoAndWait(func() {
		entry.SetText("/data/al")
	})
	fetchGen := entry.currentGeneration()

	fyne.DoAndWait(func() {
		entry.runRemoteFetch(fetchGen, "/data/al")
	})

	if len(entry.suggestions) != 0 {
		t.Fatalf("result older than the current text landed: %#v", entry.suggestions)
	}
}

func TestPathAutocompleteEntryPopupRespectsAvailableHeight(t *testing.T) {
	test.NewTempApp(t)

	entry := newPathAutocompleteEntryWithDelay(manyAutocompletePaths(), 0, nil, nil)
	defer entry.Close()

	win := test.NewWindow(entry)
	defer win.Close()
	win.Resize(fyne.NewSize(420, 150))
	entry.Resize(fyne.NewSize(320, entry.MinSize().Height))
	entry.Move(fyne.NewPos(20, 20))
	entry.FocusGained()

	fyne.DoAndWait(func() {
		entry.SetText("/data/item")
	})

	if !entry.popupVisible() {
		t.Fatalf("expected popup to be visible")
	}

	entryPos := fyne.CurrentApp().Driver().AbsolutePositionForObject(entry)
	_, size := entry.popupLayout()
	canvasPos, canvasSize := win.Canvas().InteractiveArea()
	maxBelow := canvasPos.Y + canvasSize.Height - (entryPos.Y + entry.Size().Height - theme.InputBorderSize())
	if size.Height > maxBelow {
		t.Fatalf("expected popup height %v to fit below available space %v", size.Height, maxBelow)
	}
	if size.Height >= entry.popupHeight() {
		t.Fatalf("expected popup height %v to shrink below desired height %v", size.Height, entry.popupHeight())
	}
}

func TestPathAutocompleteEntryPopupOpensAboveNearBottom(t *testing.T) {
	test.NewTempApp(t)

	entry := newPathAutocompleteEntryWithDelay(manyAutocompletePaths(), 0, nil, nil)
	defer entry.Close()

	win := test.NewWindow(entry)
	defer win.Close()
	win.Resize(fyne.NewSize(420, 260))
	entry.Resize(fyne.NewSize(320, entry.MinSize().Height))
	entry.Move(fyne.NewPos(20, 210))
	entry.FocusGained()

	fyne.DoAndWait(func() {
		entry.SetText("/data/item")
	})

	if !entry.popupVisible() {
		t.Fatalf("expected popup to be visible")
	}

	entryPos := fyne.CurrentApp().Driver().AbsolutePositionForObject(entry)
	if entry.popupPosition().Y >= entryPos.Y {
		t.Fatalf("expected popup to open above entry, popup y=%v entry y=%v", entry.popupPosition().Y, entryPos.Y)
	}
}

func TestPathAutocompleteEntryKeyboardSelectionScrollsPopup(t *testing.T) {
	test.NewTempApp(t)

	entry := newPathAutocompleteEntryWithDelay(manyAutocompletePaths(), 0, nil, nil)
	defer entry.Close()

	win := test.NewWindow(entry)
	defer win.Close()
	win.Resize(fyne.NewSize(420, 300))
	entry.Resize(fyne.NewSize(320, entry.MinSize().Height))
	entry.Move(fyne.NewPos(20, 20))
	entry.FocusGained()

	fyne.DoAndWait(func() {
		entry.SetText("/data/item")
	})

	for range 8 {
		fyne.DoAndWait(func() {
			entry.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
		})
	}

	if entry.selectedIndex() != 7 {
		t.Fatalf("expected seventh suggestion to be selected, got %d", entry.selectedIndex())
	}
	if entry.popupScroll.Offset.Y <= 0 {
		t.Fatalf("expected keyboard navigation to scroll popup, got offset %v", entry.popupScroll.Offset.Y)
	}
}

func TestPathAutocompleteEntryMouseWheelScrollsPopup(t *testing.T) {
	test.NewTempApp(t)

	entry := newPathAutocompleteEntryWithDelay(manyAutocompletePaths(), 0, nil, nil)
	defer entry.Close()

	win := test.NewWindow(entry)
	defer win.Close()
	win.Resize(fyne.NewSize(420, 300))
	entry.Resize(fyne.NewSize(320, entry.MinSize().Height))
	entry.Move(fyne.NewPos(20, 20))
	entry.FocusGained()

	fyne.DoAndWait(func() {
		entry.SetText("/data/item")
	})

	scrollPos := fyne.CurrentApp().Driver().AbsolutePositionForObject(entry.popupScroll)
	test.Scroll(win.Canvas(), scrollPos.Add(fyne.NewPos(12, 12)), 0, -entry.rowHeight()*3)

	if entry.popupScroll.Offset.Y <= 0 {
		t.Fatalf("expected mouse wheel scrolling to move popup, got offset %v", entry.popupScroll.Offset.Y)
	}
}

func TestPathAutocompleteEntryCtrlBackspaceWorksWithPopup(t *testing.T) {
	test.NewTempApp(t)

	entry := newPathAutocompleteEntryWithDelay(manyAutocompletePaths(), 0, nil, nil)
	defer entry.Close()

	win := test.NewWindow(entry)
	defer win.Close()
	win.Resize(fyne.NewSize(420, 300))
	entry.Resize(fyne.NewSize(320, entry.MinSize().Height))
	entry.Move(fyne.NewPos(20, 20))
	entry.FocusGained()

	fyne.DoAndWait(func() {
		entry.SetText("/data/item-03")
		entry.CursorColumn = len([]rune(entry.Text))
	})

	if !entry.popupVisible() {
		t.Fatalf("expected popup to be visible")
	}

	fyne.DoAndWait(func() {
		win.Canvas().Unfocus()
	})

	modifier := fyne.KeyModifierShortcutDefault
	if runtime.GOOS == "darwin" {
		modifier = fyne.KeyModifierAlt
	}

	fyne.DoAndWait(func() {
		win.Canvas().(interface{ TypedShortcut(fyne.Shortcut) }).TypedShortcut(
			&desktop.CustomShortcut{KeyName: fyne.KeyBackspace, Modifier: modifier},
		)
	})

	if entry.Text != "/data/item-" {
		t.Fatalf("expected ctrl+backspace to delete last path segment, got %q", entry.Text)
	}
}

func manyAutocompletePaths() []string {
	const count = 12
	paths := make([]string, 0, count)
	for i := range count {
		paths = append(paths, fmt.Sprintf("/data/item-%02d", i))
	}
	return paths
}
