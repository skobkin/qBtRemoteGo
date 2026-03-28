package ui

import (
	"fmt"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	appcore "github.com/skobkin/qbtremotego/internal/app"
	"github.com/skobkin/qbtremotego/internal/config"
	"github.com/skobkin/qbtremotego/internal/qbt"
)

type torrentColumnSpec struct {
	key          string
	label        string
	defaultWidth float32
	minWidth     float32
	resizable    bool
}

var torrentColumnSpecs = []torrentColumnSpec{
	{key: "select", label: "", defaultWidth: 36, minWidth: 36, resizable: false},
	{key: "name", label: "Name", defaultWidth: 420, minWidth: 220, resizable: true},
	{key: "size", label: "Size", defaultWidth: 110, minWidth: 90, resizable: true},
	{key: "progress", label: "Progress", defaultWidth: 180, minWidth: 140, resizable: true},
	{key: "status", label: "Status", defaultWidth: 140, minWidth: 110, resizable: true},
	{key: "down", label: "Down", defaultWidth: 110, minWidth: 90, resizable: true},
	{key: "up", label: "Up", defaultWidth: 110, minWidth: 90, resizable: true},
	{key: "eta", label: "ETA", defaultWidth: 90, minWidth: 80, resizable: true},
	{key: "added", label: "Added", defaultWidth: 90, minWidth: 80, resizable: true},
}

type torrentTableCell struct {
	widget.BaseWidget
	app        *application
	root       *fyne.Container
	checkWrap  *fyne.Container
	check      *widget.Check
	text       *hoverLabel
	progress   *widget.ProgressBar
	progressCt *fyne.Container
	statusBG   *canvas.Rectangle
	statusTx   *widget.Label
	statusCt   *fyne.Container
}

type torrentHeaderCell struct {
	widget.BaseWidget
	label  *widget.Label
	handle *columnResizeHandle
	root   *fyne.Container
}

type columnResizeHandle struct {
	widget.BaseWidget
	app            *application
	spec           torrentColumnSpec
	indicator      *canvas.Rectangle
	dragStartX     float32
	dragStartWidth float32
	dragging       bool
}

type hoverLabel struct {
	widget.BaseWidget
	canvas   fyne.Canvas
	label    *widget.Label
	fullText string
	popup    *widget.PopUp
}

func (a *application) buildTorrentTable() *widget.Table {
	a.columnWidths = mergeColumnWidths(a.controller.Config().UI.ColumnWidths)

	table := widget.NewTable(
		func() (int, int) {
			return len(a.visibleTorrents), len(torrentColumnSpecs)
		},
		func() fyne.CanvasObject {
			return newTorrentTableCell(a)
		},
		func(id widget.TableCellID, item fyne.CanvasObject) {
			cell := item.(*torrentTableCell)
			if id.Row < 0 || id.Row >= len(a.visibleTorrents) || id.Col < 0 || id.Col >= len(torrentColumnSpecs) {
				cell.clear()
				return
			}
			a.updateTorrentTableCell(cell, a.visibleTorrents[id.Row], id.Col)
		},
	)
	table.ShowHeaderRow = true
	table.ShowHeaderColumn = false
	table.CreateHeader = func() fyne.CanvasObject {
		return newTorrentHeaderCell(a)
	}
	table.UpdateHeader = func(id widget.TableCellID, item fyne.CanvasObject) {
		header := item.(*torrentHeaderCell)
		if id.Row != -1 || id.Col < 0 || id.Col >= len(torrentColumnSpecs) {
			header.setColumn(torrentColumnSpecs[0])
			return
		}
		header.setColumn(torrentColumnSpecs[id.Col])
	}
	table.HideSeparators = false
	a.table = table
	a.applyColumnWidths()

	return table
}

func (a *application) updateTorrentTableCell(cell *torrentTableCell, torrent qbt.Torrent, column int) {
	switch torrentColumnSpecs[column].key {
	case "select":
		cell.showCheck(torrent.Hash, a.selection[torrent.Hash])
	case "name":
		cell.showText(torrent.Name, torrent.Name, fyne.TextAlignLeading)
	case "size":
		size := appcore.HumanBytes(torrent.Size)
		cell.showText(size, size, fyne.TextAlignTrailing)
	case "progress":
		cell.showProgress(torrent.Progress, fmt.Sprintf("%.1f%%", torrent.Progress*100))
	case "status":
		label := appcore.StatusLabel(torrent.State)
		cell.showStatus(label, statusColor(torrent.State))
	case "down":
		speed := appcore.HumanSpeed(torrent.DLSpeed)
		cell.showText(speed, speed, fyne.TextAlignTrailing)
	case "up":
		speed := appcore.HumanSpeed(torrent.UPSpeed)
		cell.showText(speed, speed, fyne.TextAlignTrailing)
	case "eta":
		eta := appcore.HumanETA(torrent.ETASeconds)
		cell.showText(eta, eta, fyne.TextAlignTrailing)
	case "added":
		display := appcore.HumanAdded(torrent.AddedAt)
		hover := ""
		if !torrent.AddedAt.IsZero() {
			hover = torrent.AddedAt.Local().Format("2006-01-02 15:04")
		}
		cell.showText(display, hover, fyne.TextAlignTrailing)
	default:
		cell.clear()
	}
}

func (a *application) applyColumnWidths() {
	if a.table == nil {
		return
	}
	for index, spec := range torrentColumnSpecs {
		a.table.SetColumnWidth(index, a.columnWidth(spec))
	}
}

func (a *application) columnWidth(spec torrentColumnSpec) float32 {
	if !spec.resizable {
		return spec.defaultWidth
	}
	width, ok := a.columnWidths[spec.key]
	if !ok || width < spec.minWidth {
		return spec.defaultWidth
	}
	return width
}

func (a *application) setColumnWidth(spec torrentColumnSpec, width float32, persist bool) {
	if !spec.resizable {
		return
	}
	if width < spec.minWidth {
		width = spec.minWidth
	}
	if a.columnWidths == nil {
		a.columnWidths = make(map[string]float32, len(torrentColumnSpecs))
	}
	a.columnWidths[spec.key] = width
	if a.table != nil {
		index := torrentColumnIndex(spec.key)
		if index >= 0 {
			a.table.SetColumnWidth(index, width)
		}
	}
	if persist {
		a.persistColumnWidths()
	}
}

func (a *application) persistColumnWidths() {
	cfg := a.controller.Config()
	cfg.UI.ColumnWidths = cloneColumnWidths(a.columnWidths)
	if err := a.controller.SaveLocalUI(cfg); err != nil {
		a.logger.Warn("persist column widths", "error", err)
	}
}

func cloneColumnWidths(in map[string]float32) map[string]float32 {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]float32, len(in))
	for key, width := range in {
		out[key] = width
	}
	return out
}

func mergeColumnWidths(saved map[string]float32) map[string]float32 {
	out := make(map[string]float32, len(torrentColumnSpecs))
	for _, spec := range torrentColumnSpecs {
		if !spec.resizable {
			continue
		}
		out[spec.key] = spec.defaultWidth
	}
	for key, width := range saved {
		index := torrentColumnIndex(key)
		if index < 0 {
			continue
		}
		spec := torrentColumnSpecs[index]
		if !spec.resizable || width < spec.minWidth {
			continue
		}
		out[key] = width
	}
	return out
}

func torrentColumnIndex(key string) int {
	for index, spec := range torrentColumnSpecs {
		if spec.key == key {
			return index
		}
	}
	return -1
}

func newTorrentTableCell(app *application) *torrentTableCell {
	cell := &torrentTableCell{
		app:      app,
		check:    widget.NewCheck("", nil),
		text:     newHoverLabel(app.window.Canvas()),
		progress: widget.NewProgressBar(),
		statusBG: canvas.NewRectangle(color.Transparent),
		statusTx: widget.NewLabel(""),
	}
	cell.text.label.Truncation = fyne.TextTruncateEllipsis
	cell.statusTx.Alignment = fyne.TextAlignCenter
	cell.statusCt = container.NewMax(cell.statusBG, container.NewCenter(cell.statusTx))
	cell.progressCt = container.NewMax(cell.progress)
	cell.checkWrap = container.NewCenter(cell.check)
	cell.root = container.NewMax(cell.checkWrap, cell.text, cell.progressCt, cell.statusCt)
	cell.ExtendBaseWidget(cell)
	cell.clear()
	return cell
}

func (c *torrentTableCell) clear() {
	c.check.OnChanged = nil
	c.checkWrap.Hide()
	c.text.hidePopup()
	c.text.Hide()
	c.progressCt.Hide()
	c.statusCt.Hide()
}

func (c *torrentTableCell) showCheck(hash string, checked bool) {
	c.clear()
	c.check.SetChecked(checked)
	c.check.OnChanged = func(selected bool) {
		if selected {
			c.app.selection[hash] = true
			return
		}
		delete(c.app.selection, hash)
	}
	c.checkWrap.Show()
}

func (c *torrentTableCell) showText(display string, hover string, alignment fyne.TextAlign) {
	c.clear()
	c.text.SetAlignment(alignment)
	c.text.SetText(display, hover)
	c.text.Show()
}

func (c *torrentTableCell) showProgress(value float64, _ string) {
	c.clear()
	c.progress.SetValue(value)
	c.progressCt.Show()
}

func (c *torrentTableCell) showStatus(text string, fill color.Color) {
	c.clear()
	c.statusTx.SetText(text)
	c.statusBG.FillColor = fill
	c.statusBG.Refresh()
	c.statusCt.Show()
}

func (c *torrentTableCell) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(c.root)
}

func newTorrentHeaderCell(app *application) *torrentHeaderCell {
	cell := &torrentHeaderCell{
		label:  widget.NewLabel(""),
		handle: newColumnResizeHandle(app),
	}
	cell.label.TextStyle = fyne.TextStyle{Bold: true}
	cell.root = container.NewBorder(nil, widget.NewSeparator(), nil, cell.handle, cell.label)
	cell.ExtendBaseWidget(cell)
	return cell
}

func (c *torrentHeaderCell) setColumn(spec torrentColumnSpec) {
	c.label.SetText(spec.label)
	c.label.Alignment = fyne.TextAlignLeading
	if strings.TrimSpace(spec.label) == "" {
		c.label.Alignment = fyne.TextAlignCenter
	}
	c.handle.setColumn(spec)
}

func (c *torrentHeaderCell) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(c.root)
}

func newColumnResizeHandle(app *application) *columnResizeHandle {
	handle := &columnResizeHandle{
		app:       app,
		indicator: canvas.NewRectangle(color.Transparent),
	}
	handle.ExtendBaseWidget(handle)
	return handle
}

func (h *columnResizeHandle) setColumn(spec torrentColumnSpec) {
	h.spec = spec
	if spec.resizable {
		h.Show()
		return
	}
	h.Hide()
}

func (h *columnResizeHandle) Cursor() desktop.Cursor {
	if h.spec.resizable {
		return desktop.HResizeCursor
	}
	return desktop.DefaultCursor
}

func (h *columnResizeHandle) Dragged(event *fyne.DragEvent) {
	if !h.spec.resizable {
		return
	}
	if !h.dragging {
		h.dragging = true
		h.dragStartX = event.Position.X
		h.dragStartWidth = h.app.columnWidth(h.spec)
	}
	h.app.setColumnWidth(h.spec, h.dragStartWidth+(event.Position.X-h.dragStartX), false)
}

func (h *columnResizeHandle) DragEnd() {
	if !h.spec.resizable {
		return
	}
	h.dragging = false
	h.indicator.FillColor = color.Transparent
	h.indicator.Refresh()
	h.app.persistColumnWidths()
}

func (h *columnResizeHandle) MouseDown(event *desktop.MouseEvent) {
	if !h.spec.resizable {
		return
	}
	h.dragStartX = event.Position.X
	h.dragStartWidth = h.app.columnWidth(h.spec)
	h.dragging = true
}

func (h *columnResizeHandle) MouseUp(*desktop.MouseEvent) {
}

func (h *columnResizeHandle) MouseIn(*desktop.MouseEvent) {
	if !h.spec.resizable {
		return
	}
	h.indicator.FillColor = theme.Color(theme.ColorNamePrimary)
	h.indicator.Refresh()
}

func (h *columnResizeHandle) MouseMoved(*desktop.MouseEvent) {
}

func (h *columnResizeHandle) MouseOut() {
	if h.dragging {
		return
	}
	h.indicator.FillColor = color.Transparent
	h.indicator.Refresh()
}

func (h *columnResizeHandle) MinSize() fyne.Size {
	return fyne.NewSize(8, 1)
}

func (h *columnResizeHandle) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(h.indicator)
}

func newHoverLabel(canvas fyne.Canvas) *hoverLabel {
	label := widget.NewLabel("")
	label.Truncation = fyne.TextTruncateEllipsis
	h := &hoverLabel{
		canvas: canvas,
		label:  label,
	}
	h.ExtendBaseWidget(h)
	return h
}

func (h *hoverLabel) SetAlignment(alignment fyne.TextAlign) {
	h.label.Alignment = alignment
	h.label.Refresh()
}

func (h *hoverLabel) SetText(display string, hover string) {
	h.hidePopup()
	h.label.SetText(display)
	if strings.TrimSpace(hover) == "" {
		h.fullText = display
		return
	}
	h.fullText = hover
}

func (h *hoverLabel) MouseIn(*desktop.MouseEvent) {
	if strings.TrimSpace(h.fullText) == "" {
		return
	}
	if h.fullText == h.label.Text && h.label.MinSize().Width <= h.Size().Width {
		return
	}
	content := widget.NewLabel(h.fullText)
	content.Wrapping = fyne.TextWrapWord
	h.popup = widget.NewPopUp(container.NewPadded(content), h.canvas)
	h.popup.ShowAtRelativePosition(fyne.NewPos(0, h.Size().Height), h)
}

func (h *hoverLabel) MouseMoved(*desktop.MouseEvent) {
}

func (h *hoverLabel) MouseOut() {
	h.hidePopup()
}

func (h *hoverLabel) hidePopup() {
	if h.popup == nil {
		return
	}
	h.popup.Hide()
	h.popup = nil
}

func (h *hoverLabel) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(h.label)
}

func defaultColumnWidths() map[string]float32 {
	return mergeColumnWidths(config.Default().UI.ColumnWidths)
}
