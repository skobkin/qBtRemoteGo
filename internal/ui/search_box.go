package ui

import (
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/skobkin/qbtremotego/internal/config"
)

const (
	searchDebounce   = 200 * time.Millisecond
	searchFieldName  = "name"
	searchFieldPath  = "location"
	searchEntryWidth = 240
)

// searchEntry is the toolbar search field: Esc clears it.
type searchEntry struct {
	widget.Entry
	onEscape func()
}

func newSearchEntry(onEscape func()) *searchEntry {
	e := &searchEntry{onEscape: onEscape}
	e.ExtendBaseWidget(e)
	return e
}

func (e *searchEntry) TypedKey(event *fyne.KeyEvent) {
	if event.Name == fyne.KeyEscape {
		e.SetText("")
		if e.onEscape != nil {
			e.onEscape()
		}
		return
	}
	e.Entry.TypedKey(event)
}

// buildSearchBox assembles the single search control: a field picker button,
// the search entry and an inline clear button.
func (a *application) buildSearchBox() fyne.CanvasObject {
	a.filterField = searchFieldName
	if strings.EqualFold(a.controller.Config().UI.FilterBy, searchFieldPath) {
		a.filterField = searchFieldPath
	}

	a.filterEntry = newSearchEntry(func() {
		a.applyFilterQueryNow("")
	})
	a.filterClear = widget.NewButtonWithIcon("", theme.ContentClearIcon(), func() {
		a.filterEntry.SetText("")
		a.applyFilterQueryNow("")
	})
	a.filterClear.Importance = widget.LowImportance
	a.filterClear.Hide()
	a.filterEntry.ActionItem = a.filterClear
	a.filterEntry.OnChanged = func(value string) {
		a.syncFilterClear(value)
		if a.filterTimer != nil {
			a.filterTimer.Stop()
		}
		a.filterTimer = time.AfterFunc(searchDebounce, func() {
			fyne.Do(func() {
				a.filterQuery = value
				a.refreshVisibleTorrents()
			})
		})
	}

	a.filterFieldButton = widget.NewButtonWithIcon("", theme.MenuDropDownIcon(), nil)
	a.filterFieldButton.IconPlacement = widget.ButtonIconTrailingText
	a.filterFieldButton.OnTapped = a.showSearchFieldMenu
	a.syncSearchFieldControls()

	entry := container.NewGridWrap(fyne.NewSize(searchEntryWidth, a.filterEntry.MinSize().Height), a.filterEntry)
	return container.NewBorder(nil, nil, a.filterFieldButton, nil, entry)
}

// applyFilterQueryNow applies a search immediately, skipping the typing
// debounce (used by clear and Esc).
func (a *application) applyFilterQueryNow(query string) {
	if a.filterTimer != nil {
		a.filterTimer.Stop()
	}
	a.filterQuery = query
	a.syncFilterClear(query)
	a.refreshVisibleTorrents()
}

func (a *application) syncFilterClear(query string) {
	if a.filterClear == nil {
		return
	}
	if query == "" {
		a.filterClear.Hide()
	} else {
		a.filterClear.Show()
	}
}

func (a *application) showSearchFieldMenu() {
	nameItem := fyne.NewMenuItem("Name", func() { a.setFilterField(searchFieldName) })
	nameItem.Checked = a.filterField == searchFieldName
	pathItem := fyne.NewMenuItem("Location", func() { a.setFilterField(searchFieldPath) })
	pathItem.Checked = a.filterField == searchFieldPath

	canvas := fyne.CurrentApp().Driver().CanvasForObject(a.filterFieldButton)
	if canvas == nil {
		return
	}
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(a.filterFieldButton)
	pos.Y += a.filterFieldButton.Size().Height
	widget.ShowPopUpMenuAtPosition(fyne.NewMenu("", nameItem, pathItem), canvas, pos)
}

// setFilterField switches what the search matches against and persists it.
func (a *application) setFilterField(field string) {
	a.filterField = field
	if err := a.controller.SaveLocalUI(func(cfg *config.AppConfig) {
		cfg.UI.FilterBy = field
	}); err != nil {
		a.logger.Warn("persist filter_by", "field", field, "error", err)
	}
	a.syncSearchFieldControls()
	a.refreshVisibleTorrents()
}

func (a *application) syncSearchFieldControls() {
	if a.filterField == searchFieldPath {
		a.filterFieldButton.SetText("Location")
		a.filterEntry.SetPlaceHolder("Search locations…")
		return
	}
	a.filterFieldButton.SetText("Name")
	a.filterEntry.SetPlaceHolder("Search names…")
}
