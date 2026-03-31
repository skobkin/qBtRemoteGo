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

type autocompleteFetchResponse struct {
	items []string
	err   error
}

type autocompleteFetchRequest struct {
	query    string
	response chan autocompleteFetchResponse
}

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

	requests := make(chan autocompleteFetchRequest, 2)
	fetcher := func(_ context.Context, query string) ([]string, error) {
		response := make(chan autocompleteFetchResponse, 1)
		requests <- autocompleteFetchRequest{query: query, response: response}
		result := <-response
		return result.items, result.err
	}

	entry := newPathAutocompleteEntryWithDelay(nil, 5*time.Millisecond, fetcher, nil)
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
	first := waitForRequest(t, requests, "/data/a")

	fyne.DoAndWait(func() {
		entry.SetText("/data/al")
	})
	second := waitForRequest(t, requests, "/data/al")

	second.response <- autocompleteFetchResponse{items: []string{"/data/alpine"}}
	waitForCondition(t, func() bool {
		return len(entry.suggestions) == 1 && entry.suggestions[0] == "/data/alpine"
	})

	first.response <- autocompleteFetchResponse{items: []string{"/data/alpha"}}
	time.Sleep(20 * time.Millisecond)

	if len(entry.suggestions) != 1 || entry.suggestions[0] != "/data/alpine" {
		t.Fatalf("stale suggestions overwrote current state: %#v", entry.suggestions)
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

func waitForRequest(t *testing.T, requests <-chan autocompleteFetchRequest, query string) autocompleteFetchRequest {
	t.Helper()

	select {
	case request := <-requests:
		if request.query != query {
			t.Fatalf("unexpected request query: got %q want %q", request.query, query)
		}
		return request
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for request %q", query)
	}
	return autocompleteFetchRequest{}
}

func waitForCondition(t *testing.T, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("condition not met before timeout")
}
