package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"github.com/skobkin/qbtremotego/internal/config"
)

const (
	sortAscendingMarker  = " ▲"
	sortDescendingMarker = " ▼"
)

// nextSortOrder returns the sort after tapping a column header. Tapping the
// active column flips the direction. A new column starts in the direction
// people usually want first: A→Z for text, soonest first for ETA, and
// largest/newest first for sizes, progress, speeds and dates.
func nextSortOrder(column string, descending bool, tapped string) (string, bool) {
	if tapped == column {
		return column, !descending
	}
	switch tapped {
	case "name", "status", "eta":
		return tapped, false
	default:
		return tapped, true
	}
}

// sortableHeaderCell is a column header label that sorts the table when tapped.
type sortableHeaderCell struct {
	widget.BaseWidget
	label    *widget.Label
	onTapped func()
	// onSecondaryTapped opens the column menu on right-click.
	onSecondaryTapped func(position fyne.Position)
}

func newSortableHeaderCell(text string, onTapped func()) *sortableHeaderCell {
	label := widget.NewLabel(text)
	label.TextStyle = fyne.TextStyle{Bold: true}
	label.Alignment = fyne.TextAlignLeading
	label.Truncation = fyne.TextTruncateEllipsis
	cell := &sortableHeaderCell{label: label, onTapped: onTapped}
	cell.ExtendBaseWidget(cell)
	return cell
}

func (c *sortableHeaderCell) setText(text string) {
	if c.label.Text == text {
		return
	}
	c.label.SetText(text)
}

func (c *sortableHeaderCell) Tapped(*fyne.PointEvent) {
	if c.onTapped != nil {
		c.onTapped()
	}
}

func (c *sortableHeaderCell) TappedSecondary(event *fyne.PointEvent) {
	if c.onSecondaryTapped != nil {
		c.onSecondaryTapped(event.AbsolutePosition)
	}
}

func (c *sortableHeaderCell) Cursor() desktop.Cursor {
	return desktop.PointerCursor
}

func (c *sortableHeaderCell) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(c.label)
}

// setSortIndicator marks the active sort column with a direction arrow.
func (r *torrentHeaderRow) setSortIndicator(column string, descending bool) {
	for index, spec := range torrentColumnSpecs {
		text := spec.label
		if spec.key == column {
			if descending {
				text += sortDescendingMarker
			} else {
				text += sortAscendingMarker
			}
		}
		r.cells[index].setText(text)
	}
}

// sortByColumn applies a header tap: it computes the next sort, persists it
// and re-sorts the visible torrents.
func (a *application) sortByColumn(key string) {
	cfg := a.controller.Config()
	column, descending := nextSortOrder(cfg.UI.SortColumn, cfg.UI.SortDescending, key)
	if err := a.controller.SaveLocalUI(func(cfg *config.AppConfig) {
		cfg.UI.SortColumn = column
		cfg.UI.SortDescending = descending
	}); err != nil {
		a.logger.Warn("persist sort order", "column", column, "error", err)
	}
	a.refreshVisibleTorrents()
}
