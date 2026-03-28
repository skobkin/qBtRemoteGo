package ui

import (
	"context"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
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
