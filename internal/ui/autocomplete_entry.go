package ui

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const (
	pathAutocompleteDelay      = 400 * time.Millisecond
	maxAutocompletePopupItems  = 6
	maxAutocompletePopupHeight = 220
)

type autocompleteFetcher func(context.Context, string) ([]string, error)

type pathAutocompleteEntry struct {
	widget.Entry

	recentPaths []string
	fetcher     autocompleteFetcher
	status      func(string)

	debounceDelay time.Duration

	suggestions    []string
	popup          *widget.PopUp
	popupContent   *fyne.Container
	buttons        []*widget.Button
	selected       int
	suppressChange bool
	focused        bool
	popupCanvas    fyne.Canvas
	onTypedKey     func(*fyne.KeyEvent)
	onTypedRune    func(rune)

	mu            sync.Mutex
	generation    uint64
	requestGen    uint64
	requestCancel context.CancelFunc
	debounce      *time.Timer
	closed        bool
}

func newPathAutocompleteEntry(recentPaths []string, fetcher autocompleteFetcher, status func(string)) *pathAutocompleteEntry {
	return newPathAutocompleteEntryWithDelay(recentPaths, pathAutocompleteDelay, fetcher, status)
}

func newPathAutocompleteEntryWithDelay(recentPaths []string, delay time.Duration, fetcher autocompleteFetcher, status func(string)) *pathAutocompleteEntry {
	entry := &pathAutocompleteEntry{
		recentPaths:   append([]string(nil), recentPaths...),
		fetcher:       fetcher,
		status:        status,
		debounceDelay: delay,
		suggestions:   filterMatchingPaths(recentPaths, ""),
		selected:      -1,
	}
	entry.ExtendBaseWidget(entry)
	entry.Wrapping = fyne.TextWrap(fyne.TextTruncateClip)
	entry.OnChanged = entry.handleChanged
	entry.popupContent = container.NewVBox()
	return entry
}

func (e *pathAutocompleteEntry) Close() {
	e.mu.Lock()
	e.closed = true
	if e.debounce != nil {
		e.debounce.Stop()
		e.debounce = nil
	}
	if e.requestCancel != nil {
		e.requestCancel()
		e.requestCancel = nil
		e.requestGen = 0
	}
	e.mu.Unlock()
	e.hidePopup()
	e.releaseCanvasHandlers()
}

func (e *pathAutocompleteEntry) FocusGained() {
	e.focused = true
	e.Entry.FocusGained()
}

func (e *pathAutocompleteEntry) FocusLost() {
	e.focused = false
	e.Entry.FocusLost()
	if e.popupVisible() {
		e.restoreFocus()
	}
}

func (e *pathAutocompleteEntry) Move(pos fyne.Position) {
	e.Entry.Move(pos)
	e.repositionPopup()
}

func (e *pathAutocompleteEntry) Resize(size fyne.Size) {
	e.Entry.Resize(size)
	e.resizePopup()
}

func (e *pathAutocompleteEntry) AcceptsTab() bool {
	return e.popupVisible() || e.Entry.AcceptsTab()
}

func (e *pathAutocompleteEntry) TypedKey(event *fyne.KeyEvent) {
	switch event.Name {
	case fyne.KeyDown:
		if e.popupVisible() {
			e.moveSelection(1)
			return
		}
		if len(e.suggestions) == 0 {
			e.setSuggestions(filterMatchingPaths(e.recentPaths, e.Text))
		}
		e.showPopup()
		return
	case fyne.KeyUp:
		if e.popupVisible() {
			e.moveSelection(-1)
			return
		}
	case fyne.KeyReturn, fyne.KeyEnter:
		if e.popupVisible() && len(e.suggestions) > 0 {
			e.acceptSuggestion(e.highlightedIndex())
			return
		}
	case fyne.KeyEscape:
		if e.popupVisible() {
			e.hidePopup()
			return
		}
	case fyne.KeyTab:
		if e.popupVisible() && len(e.suggestions) > 0 {
			e.acceptSuggestion(e.highlightedIndex())
			return
		}
		e.hidePopup()
	case fyne.KeySpace:
		if e.popupVisible() && len(e.suggestions) > 0 {
			e.acceptSuggestion(e.highlightedIndex())
			return
		}
	}

	e.Entry.TypedKey(event)
}

func (e *pathAutocompleteEntry) handleChanged(value string) {
	if e.suppressChange {
		return
	}

	if e.status != nil {
		e.status("")
	}

	local := filterMatchingPaths(e.recentPaths, value)
	e.setSuggestions(local)
	e.scheduleRemoteFetch(value)
}

func (e *pathAutocompleteEntry) scheduleRemoteFetch(query string) {
	e.mu.Lock()
	e.generation++
	generation := e.generation
	if e.debounce != nil {
		e.debounce.Stop()
		e.debounce = nil
	}
	if e.requestCancel != nil {
		e.requestCancel()
		e.requestCancel = nil
		e.requestGen = 0
	}

	closed := e.closed
	shouldFetch := e.fetcher != nil && strings.TrimSpace(query) != ""
	delay := e.debounceDelay
	e.mu.Unlock()

	if closed || !shouldFetch {
		return
	}

	e.mu.Lock()
	e.debounce = time.AfterFunc(delay, func() {
		e.runRemoteFetch(generation, query)
	})
	e.mu.Unlock()
}

func (e *pathAutocompleteEntry) runRemoteFetch(generation uint64, query string) {
	e.mu.Lock()
	if e.closed || generation != e.generation {
		e.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	e.requestCancel = cancel
	e.requestGen = generation
	e.debounce = nil
	e.mu.Unlock()

	remote, err := e.fetcher(ctx, query)

	e.mu.Lock()
	if e.requestGen == generation {
		e.requestCancel = nil
		e.requestGen = 0
	}
	currentGeneration := e.generation
	closed := e.closed
	e.mu.Unlock()

	if closed || generation != currentGeneration {
		return
	}

	fyne.Do(func() {
		if !e.isCurrentGeneration(generation) || e.Text != query {
			return
		}
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			if e.status != nil {
				e.status("Path suggestions unavailable: " + err.Error())
			}
			return
		}
		if e.status != nil {
			e.status("")
		}
		e.setSuggestions(mergeAutocompleteSuggestions(query, remote, e.recentPaths))
	})
}

func (e *pathAutocompleteEntry) isCurrentGeneration(generation uint64) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return !e.closed && e.generation == generation
}

func (e *pathAutocompleteEntry) setSuggestions(items []string) {
	e.suggestions = append([]string(nil), items...)
	e.selected = -1
	e.refreshButtons()
	if len(e.suggestions) == 0 || !e.focused {
		e.hidePopup()
		return
	}
	e.showPopup()
}

func (e *pathAutocompleteEntry) suggestionAt(id int) string {
	if id < 0 || id >= len(e.suggestions) {
		return ""
	}
	return e.suggestions[id]
}

func (e *pathAutocompleteEntry) popupVisible() bool {
	return e.popup != nil && e.popup.Visible()
}

func (e *pathAutocompleteEntry) showPopup() {
	if len(e.suggestions) == 0 || !e.focused {
		return
	}
	canvas := fyne.CurrentApp().Driver().CanvasForObject(e)
	if canvas == nil {
		return
	}
	if e.popup == nil || e.popup.Canvas != canvas {
		if e.popup != nil {
			e.popup.Hide()
		}
		e.popup = widget.NewPopUp(e.popupContent, canvas)
	}
	e.popup.ShowAtPosition(e.popupPosition())
	e.resizePopup()
	e.installCanvasHandlers(canvas)
	canvas.Focus(e)
	e.restoreFocus()
}

func (e *pathAutocompleteEntry) hidePopup() {
	if e.popup != nil {
		e.popup.Hide()
	}
	e.releaseCanvasHandlers()
}

func (e *pathAutocompleteEntry) resizePopup() {
	if e.popup == nil || !e.popupVisible() {
		return
	}
	e.popup.Move(e.popupPosition())

	visibleItems := minInt(len(e.suggestions), maxAutocompletePopupItems)
	if visibleItems <= 0 {
		return
	}
	rowHeight := e.MinSize().Height
	height := float32(visibleItems)*rowHeight + theme.Padding()*2
	if height > maxAutocompletePopupHeight {
		height = maxAutocompletePopupHeight
	}
	e.popup.Resize(fyne.NewSize(e.Size().Width, height))
}

func (e *pathAutocompleteEntry) repositionPopup() {
	if e.popup != nil && e.popupVisible() {
		e.popup.Move(e.popupPosition())
	}
}

func (e *pathAutocompleteEntry) popupPosition() fyne.Position {
	entryPos := fyne.CurrentApp().Driver().AbsolutePositionForObject(e)
	return entryPos.Add(fyne.NewPos(0, e.Size().Height-theme.InputBorderSize()))
}

func (e *pathAutocompleteEntry) installCanvasHandlers(canvas fyne.Canvas) {
	if canvas == nil {
		return
	}
	if e.popupCanvas == canvas {
		return
	}
	e.releaseCanvasHandlers()
	e.popupCanvas = canvas
	e.onTypedKey = canvas.OnTypedKey()
	e.onTypedRune = canvas.OnTypedRune()
	canvas.SetOnTypedKey(func(event *fyne.KeyEvent) {
		if !e.popupVisible() || canvas.Focused() == e {
			if e.onTypedKey != nil {
				e.onTypedKey(event)
			}
			return
		}
		e.TypedKey(event)
	})
	canvas.SetOnTypedRune(func(r rune) {
		if !e.popupVisible() || canvas.Focused() == e {
			if e.onTypedRune != nil {
				e.onTypedRune(r)
			}
			return
		}
		e.TypedRune(r)
	})
}

func (e *pathAutocompleteEntry) releaseCanvasHandlers() {
	if e.popupCanvas == nil {
		return
	}
	e.popupCanvas.SetOnTypedKey(e.onTypedKey)
	e.popupCanvas.SetOnTypedRune(e.onTypedRune)
	e.popupCanvas = nil
	e.onTypedKey = nil
	e.onTypedRune = nil
}

func (e *pathAutocompleteEntry) moveSelection(delta int) {
	if len(e.suggestions) == 0 {
		return
	}

	index := e.selectedIndex()
	if index < 0 {
		if delta < 0 {
			index = len(e.suggestions) - 1
		} else {
			index = 0
		}
	} else {
		index += delta
		if index < 0 {
			index = 0
		}
		if index >= len(e.suggestions) {
			index = len(e.suggestions) - 1
		}
	}
	e.selectSuggestion(index)
}

func (e *pathAutocompleteEntry) selectSuggestion(index int) {
	if index < 0 || index >= len(e.suggestions) {
		return
	}
	e.selected = index
	e.refreshButtons()
}

func (e *pathAutocompleteEntry) selectedIndex() int {
	return e.selected
}

func (e *pathAutocompleteEntry) highlightedIndex() int {
	if e.selected >= 0 {
		return e.selected
	}
	if len(e.suggestions) == 0 {
		return -1
	}
	return 0
}

func (e *pathAutocompleteEntry) acceptSuggestion(index int) {
	value := e.suggestionAt(index)
	if value == "" {
		return
	}
	e.suppressChange = true
	e.SetText(value)
	e.suppressChange = false
	e.CursorRow = 0
	e.CursorColumn = utf8.RuneCountInString(value)
	e.Refresh()
	e.hidePopup()
}

func (e *pathAutocompleteEntry) refreshButtons() {
	if e.popupContent == nil {
		return
	}

	objects := make([]fyne.CanvasObject, 0, len(e.suggestions))
	e.buttons = make([]*widget.Button, 0, len(e.suggestions))
	for i, suggestion := range e.suggestions {
		index := i
		button := widget.NewButton(suggestion, func() {
			e.acceptSuggestion(index)
		})
		button.Alignment = widget.ButtonAlignLeading
		button.Importance = widget.LowImportance
		if i == e.selected {
			button.Importance = widget.HighImportance
		}
		e.buttons = append(e.buttons, button)
		objects = append(objects, button)
	}
	e.popupContent.Objects = objects
	e.popupContent.Refresh()
}

func (e *pathAutocompleteEntry) restoreFocus() {
	canvas := fyne.CurrentApp().Driver().CanvasForObject(e)
	if canvas == nil {
		return
	}
	fyne.Do(func() {
		if !e.popupVisible() {
			return
		}
		if canvas.Focused() != e {
			canvas.Focus(e)
		}
	})
}

func mergeAutocompleteSuggestions(query string, remote []string, recent []string) []string {
	return mergeUnique(remote, filterMatchingPaths(recent, query))
}

func filterMatchingPaths(paths []string, query string) []string {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return mergeUnique(paths, nil)
	}

	items := make([]string, 0, len(paths))
	for _, path := range paths {
		value := strings.TrimSpace(path)
		if value == "" {
			continue
		}
		if strings.HasPrefix(strings.ToLower(value), query) {
			items = append(items, value)
		}
	}
	return mergeUnique(items, nil)
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
