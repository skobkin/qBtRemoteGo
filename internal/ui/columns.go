package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"github.com/skobkin/qbtremotego/internal/config"
)

// visibleColumnSet turns the saved column list into a lookup; nil keeps the
// default set (every non-optional column).
func visibleColumnSet(saved []string) map[string]bool {
	if saved == nil {
		return nil
	}
	set := make(map[string]bool, len(saved))
	for _, key := range saved {
		set[key] = true
	}
	return set
}

// columnVisible reports whether a column is shown. The flexible Name column
// always is.
func (a *application) columnVisible(spec torrentColumnSpec) bool {
	if spec.flexible {
		return true
	}
	if a.visibleColumns == nil {
		return !spec.optional
	}
	return a.visibleColumns[spec.key]
}

// setColumnVisible shows or hides a column, saves the column set and
// re-lays out the table.
func (a *application) setColumnVisible(key string, visible bool) {
	index := torrentColumnIndex(key)
	if index < 0 || torrentColumnSpecs[index].flexible {
		return
	}
	keys := make([]string, 0, len(torrentColumnSpecs))
	set := make(map[string]bool, len(torrentColumnSpecs))
	for _, spec := range torrentColumnSpecs {
		show := a.columnVisible(spec)
		if spec.key == key {
			show = visible
		}
		if show {
			keys = append(keys, spec.key)
			set[spec.key] = true
		}
	}
	a.visibleColumns = set
	if err := a.controller.SaveLocalUI(func(cfg *config.AppConfig) {
		cfg.UI.VisibleColumns = keys
	}); err != nil {
		a.logger.Warn("persist visible columns", "column", key, "error", err)
	}
	a.refreshTableLayout()
}

// refreshTableLayout re-lays out the header, rows and scroller after the
// column set or widths change.
func (a *application) refreshTableLayout() {
	if a.tableHeader != nil {
		a.tableHeader.Refresh()
	}
	if a.list != nil {
		a.list.Refresh()
	}
	if a.tableScroll != nil {
		a.tableScroll.Refresh()
	}
}

// columnMenu lists every column that can be shown or hidden, checked when
// visible.
func (a *application) columnMenu() *fyne.Menu {
	items := make([]*fyne.MenuItem, 0, len(torrentColumnSpecs))
	for _, spec := range torrentColumnSpecs {
		if spec.flexible {
			continue
		}
		key := spec.key
		visible := a.columnVisible(spec)
		item := fyne.NewMenuItem(spec.label, func() {
			a.setColumnVisible(key, !visible)
		})
		item.Checked = visible
		items = append(items, item)
	}
	return fyne.NewMenu("", items...)
}

// showColumnMenu opens the column menu at a header right-click.
func (a *application) showColumnMenu(position fyne.Position) {
	widget.ShowPopUpMenuAtPosition(a.columnMenu(), a.window.Canvas(), position)
}
