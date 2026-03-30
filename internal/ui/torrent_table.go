package ui

import (
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	appcore "github.com/skobkin/qbtremotego/internal/app"
	"github.com/skobkin/qbtremotego/internal/qbt"
)

const (
	torrentRowHeight     float32 = 40
	torrentHeaderHeight  float32 = 38
	headerHandleWidth    float32 = 8
	headerCellPadding    float32 = 8
	rowCellPadding       float32 = 6
	rowEmphasisCellInset float32 = 4
)

type torrentColumnSpec struct {
	key          string
	label        string
	defaultWidth float32
	minWidth     float32
	resizable    bool
}

var torrentColumnSpecs = []torrentColumnSpec{
	{key: "name", label: "Name", defaultWidth: 420, minWidth: 220, resizable: true},
	{key: "size", label: "Size", defaultWidth: 110, minWidth: 90, resizable: true},
	{key: "progress", label: "Progress", defaultWidth: 180, minWidth: 140, resizable: true},
	{key: "status", label: "Status", defaultWidth: 140, minWidth: 110, resizable: true},
	{key: "down", label: "Down", defaultWidth: 110, minWidth: 90, resizable: true},
	{key: "up", label: "Up", defaultWidth: 110, minWidth: 90, resizable: true},
	{key: "eta", label: "ETA", defaultWidth: 90, minWidth: 80, resizable: true},
	{key: "added", label: "Added", defaultWidth: 90, minWidth: 80, resizable: true},
}

type torrentHeaderRow struct {
	widget.BaseWidget
	app       *application
	root      *fyne.Container
	labels    []*widget.Label
	handles   []*columnResizeHandle
	separator *widget.Separator
}

type torrentListRow struct {
	widget.BaseWidget
	app        *application
	background *canvas.Rectangle
	root       *fyne.Container
	content    *fyne.Container
	name       *hoverLabel
	size       *hoverLabel
	progress   *widget.ProgressBar
	statusBG   *canvas.Rectangle
	statusTx   *widget.Label
	statusCt   *fyne.Container
	down       *hoverLabel
	up         *hoverLabel
	eta        *hoverLabel
	added      *hoverLabel
	separator  *widget.Separator
	hash       string
	modifier   fyne.KeyModifier
}

type columnResizeHandle struct {
	widget.BaseWidget
	app            *application
	spec           torrentColumnSpec
	indicator      *canvas.Rectangle
	dragStartWidth float32
	pendingWidth   float32
	dragging       bool
}

type hoverLabel struct {
	widget.BaseWidget
	hoverTooltipOwner
	label    *widget.Label
	fullText string
}

type torrentHeaderLayout struct {
	app *application
}

type torrentRowLayout struct {
	app *application
}

type torrentTableLayout struct {
	app *application
}

func (a *application) buildTorrentTable() fyne.CanvasObject {
	a.columnWidths = mergeColumnWidths(a.controller.Config().UI.ColumnWidths)

	a.tableHeader = newTorrentHeaderRow(a)
	a.tablePreview = canvas.NewRectangle(theme.Color(theme.ColorNamePrimary))
	a.tablePreview.Hide()
	a.list = widget.NewList(
		func() int {
			return len(a.visibleTorrents)
		},
		func() fyne.CanvasObject {
			return newTorrentListRow(a)
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			if id < 0 || id >= len(a.visibleTorrents) {
				return
			}
			item.(*torrentListRow).setTorrent(a.visibleTorrents[id])
		},
	)
	a.list.HideSeparators = true

	content := container.New(&torrentTableLayout{app: a}, a.tableHeader, a.list, a.tablePreview)
	a.tableScroll = container.NewHScroll(content)

	return a.tableScroll
}

func (a *application) totalColumnWidth() float32 {
	total := float32(0)
	for _, spec := range torrentColumnSpecs {
		total += a.columnWidth(spec)
	}
	return total
}

func (a *application) columnBoundary(key string, widthOverride float32) float32 {
	total := float32(0)
	for _, spec := range torrentColumnSpecs {
		width := a.columnWidth(spec)
		if spec.key == key {
			if spec.resizable && widthOverride >= spec.minWidth {
				width = widthOverride
			}
			total += width
			return total
		}
		total += width
	}
	return total
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
	if a.tableHeader != nil {
		a.tableHeader.Refresh()
	}
	if a.list != nil {
		a.list.Refresh()
	}
	if a.tableScroll != nil {
		a.tableScroll.Refresh()
	}
	if persist {
		a.persistColumnWidths()
	}
}

func (a *application) previewColumnWidth(spec torrentColumnSpec, width float32) {
	if !spec.resizable || a.tablePreview == nil {
		return
	}
	if width < spec.minWidth {
		width = spec.minWidth
	}
	a.previewX = a.columnBoundary(spec.key, width)
	headerHeight := torrentHeaderHeight
	if a.tableHeader != nil {
		if size := a.tableHeader.Size(); size.Height > 0 {
			headerHeight = size.Height
		}
	}
	a.tablePreview.Move(fyne.NewPos(a.previewX-1, 0))
	a.tablePreview.Resize(fyne.NewSize(2, headerHeight))
	a.tablePreview.Show()
	a.tablePreview.Refresh()
}

func (a *application) hideColumnPreview() {
	if a.tablePreview == nil {
		return
	}
	a.previewX = 0
	a.tablePreview.Hide()
	a.tablePreview.Refresh()
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

func newTorrentHeaderRow(app *application) *torrentHeaderRow {
	row := &torrentHeaderRow{
		app:       app,
		labels:    make([]*widget.Label, 0, len(torrentColumnSpecs)),
		handles:   make([]*columnResizeHandle, 0, len(torrentColumnSpecs)),
		separator: widget.NewSeparator(),
	}
	objects := make([]fyne.CanvasObject, 0, len(torrentColumnSpecs)*2+1)
	for _, spec := range torrentColumnSpecs {
		label := widget.NewLabel(spec.label)
		label.TextStyle = fyne.TextStyle{Bold: true}
		if strings.TrimSpace(spec.label) == "" {
			label.Alignment = fyne.TextAlignCenter
		} else {
			label.Alignment = fyne.TextAlignLeading
		}
		handle := newColumnResizeHandle(app)
		handle.setColumn(spec)
		row.labels = append(row.labels, label)
		row.handles = append(row.handles, handle)
		objects = append(objects, label, handle)
	}
	objects = append(objects, row.separator)
	row.root = container.New(&torrentHeaderLayout{app: app}, objects...)
	row.ExtendBaseWidget(row)
	return row
}

func (r *torrentHeaderRow) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(r.root)
}

func newTorrentListRow(app *application) *torrentListRow {
	row := &torrentListRow{
		app:        app,
		background: canvas.NewRectangle(theme.Color(theme.ColorNameSelection)),
		name:       newHoverLabel(app.tooltipManager),
		size:       newHoverLabel(app.tooltipManager),
		progress:   widget.NewProgressBar(),
		statusBG:   canvas.NewRectangle(color.Transparent),
		statusTx:   widget.NewLabel(""),
		down:       newHoverLabel(app.tooltipManager),
		up:         newHoverLabel(app.tooltipManager),
		eta:        newHoverLabel(app.tooltipManager),
		added:      newHoverLabel(app.tooltipManager),
		separator:  widget.NewSeparator(),
	}
	row.background.Hide()
	row.name.label.Truncation = fyne.TextTruncateEllipsis
	row.size.label.Truncation = fyne.TextTruncateEllipsis
	row.down.label.Truncation = fyne.TextTruncateEllipsis
	row.up.label.Truncation = fyne.TextTruncateEllipsis
	row.eta.label.Truncation = fyne.TextTruncateEllipsis
	row.added.label.Truncation = fyne.TextTruncateEllipsis
	row.statusTx.Alignment = fyne.TextAlignCenter
	row.statusCt = container.NewStack(row.statusBG, container.NewCenter(row.statusTx))
	row.content = container.New(&torrentRowLayout{app: app},
		row.name,
		row.size,
		row.progress,
		row.statusCt,
		row.down,
		row.up,
		row.eta,
		row.added,
		row.separator,
	)
	row.root = container.NewStack(row.background, row.content)
	row.ExtendBaseWidget(row)
	return row
}

func (r *torrentListRow) setTorrent(torrent qbt.Torrent) {
	r.hash = torrent.Hash
	r.background.FillColor = theme.Color(theme.ColorNameSelection)
	if r.app.selection[torrent.Hash] {
		r.background.Show()
	} else {
		r.background.Hide()
	}
	r.background.Refresh()

	r.name.SetAlignment(fyne.TextAlignLeading)
	r.name.SetText(torrent.Name, torrent.Name)

	size := appcore.HumanBytes(torrent.Size)
	r.size.SetAlignment(fyne.TextAlignTrailing)
	r.size.SetText(size, size)

	r.progress.SetValue(torrent.Progress)

	r.statusTx.SetText(appcore.StatusLabel(torrent.State))
	r.statusBG.FillColor = statusColor(torrent.State)
	r.statusBG.Refresh()

	down := appcore.HumanSpeed(torrent.DLSpeed)
	r.down.SetAlignment(fyne.TextAlignTrailing)
	r.down.SetText(down, down)

	up := appcore.HumanSpeed(torrent.UPSpeed)
	r.up.SetAlignment(fyne.TextAlignTrailing)
	r.up.SetText(up, up)

	eta := appcore.HumanETA(torrent.ETASeconds)
	r.eta.SetAlignment(fyne.TextAlignTrailing)
	r.eta.SetText(eta, eta)

	added := appcore.HumanAdded(torrent.AddedAt)
	hover := ""
	if !torrent.AddedAt.IsZero() {
		hover = torrent.AddedAt.Local().Format("2006-01-02 15:04")
	}
	r.added.SetAlignment(fyne.TextAlignTrailing)
	r.added.SetText(added, hover)
}

func (r *torrentListRow) MinSize() fyne.Size {
	return fyne.NewSize(r.app.totalColumnWidth(), torrentRowHeight)
}

func (r *torrentListRow) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(r.root)
}

func (r *torrentListRow) Tapped(*fyne.PointEvent) {
	r.app.applyTorrentSelection(r.hash, r.modifier)
}

func (r *torrentListRow) TappedSecondary(event *fyne.PointEvent) {
	r.app.prepareTorrentContextSelection(r.hash)
	menu := fyne.NewMenu("",
		fyne.NewMenuItem("Start", func() {
			r.app.startSelectedTorrents()
		}),
		fyne.NewMenuItem("Stop", func() {
			r.app.stopSelectedTorrents()
		}),
		fyne.NewMenuItem("Remove", func() {
			r.app.confirmDelete()
		}),
	)
	widget.ShowPopUpMenuAtPosition(menu, r.app.window.Canvas(), event.AbsolutePosition)
}

func (r *torrentListRow) MouseDown(event *desktop.MouseEvent) {
	r.modifier = event.Modifier
}

func (r *torrentListRow) MouseUp(*desktop.MouseEvent) {
}

func (l *torrentHeaderLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	x := float32(0)
	for index, spec := range torrentColumnSpecs {
		width := l.app.columnWidth(spec)
		label := objects[index*2]
		handle := objects[index*2+1]

		labelRightPad := headerCellPadding
		if spec.resizable {
			labelRightPad += headerHandleWidth
		}
		labelWidth := clampMinZero(width - headerCellPadding - labelRightPad)
		label.Move(fyne.NewPos(x+headerCellPadding, 0))
		label.Resize(fyne.NewSize(labelWidth, clampMinZero(size.Height-1)))

		handle.Move(fyne.NewPos(x+width-headerHandleWidth, 0))
		handle.Resize(fyne.NewSize(headerHandleWidth, clampMinZero(size.Height-1)))
		x += width
	}

	separator := objects[len(objects)-1]
	separator.Move(fyne.NewPos(0, clampMinZero(size.Height-1)))
	separator.Resize(fyne.NewSize(x, 1))
}

func (l *torrentHeaderLayout) MinSize(_ []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(l.app.totalColumnWidth(), torrentHeaderHeight)
}

func (l *torrentRowLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	x := float32(0)
	for index, spec := range torrentColumnSpecs {
		width := l.app.columnWidth(spec)
		obj := objects[index]

		inset := rowCellPadding
		if spec.key == "progress" || spec.key == "status" {
			inset = rowEmphasisCellInset
		}
		obj.Move(fyne.NewPos(x+inset, 1))
		obj.Resize(fyne.NewSize(clampMinZero(width-inset*2), clampMinZero(size.Height-2)))
		x += width
	}

	separator := objects[len(objects)-1]
	separator.Move(fyne.NewPos(0, clampMinZero(size.Height-1)))
	separator.Resize(fyne.NewSize(x, 1))
}

func (l *torrentRowLayout) MinSize(_ []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(l.app.totalColumnWidth(), torrentRowHeight)
}

func (l *torrentTableLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	headerHeight := objects[0].MinSize().Height
	totalWidth := l.app.totalColumnWidth()

	objects[0].Move(fyne.NewPos(0, 0))
	objects[0].Resize(fyne.NewSize(totalWidth, headerHeight))

	objects[1].Move(fyne.NewPos(0, headerHeight))
	objects[1].Resize(fyne.NewSize(totalWidth, clampMinZero(size.Height-headerHeight)))

	preview := objects[2]
	if preview.Visible() {
		preview.Move(fyne.NewPos(l.app.previewX-1, 0))
		preview.Resize(fyne.NewSize(2, headerHeight))
	}
}

func (l *torrentTableLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	headerHeight := objects[0].MinSize().Height
	listHeight := objects[1].MinSize().Height
	return fyne.NewSize(l.app.totalColumnWidth(), headerHeight+listHeight)
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
		h.dragStartWidth = h.app.columnWidth(h.spec)
		h.pendingWidth = h.dragStartWidth
	}
	h.pendingWidth += event.Dragged.DX
	h.app.previewColumnWidth(h.spec, h.pendingWidth)
}

func (h *columnResizeHandle) DragEnd() {
	h.finishDrag()
}

func (h *columnResizeHandle) finishDrag() {
	if !h.spec.resizable || !h.dragging {
		return
	}
	h.app.setColumnWidth(h.spec, h.pendingWidth, true)
	h.app.hideColumnPreview()
	h.dragging = false
	h.pendingWidth = 0
	h.indicator.FillColor = color.Transparent
	h.indicator.Refresh()
}

func (h *columnResizeHandle) MouseDown(*desktop.MouseEvent) {
	if !h.spec.resizable {
		return
	}
	h.dragStartWidth = h.app.columnWidth(h.spec)
	h.pendingWidth = h.dragStartWidth
	h.dragging = true
	h.app.previewColumnWidth(h.spec, h.pendingWidth)
}

func (h *columnResizeHandle) MouseUp(*desktop.MouseEvent) {
	h.finishDrag()
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
	return fyne.NewSize(headerHandleWidth, 1)
}

func (h *columnResizeHandle) CreateRenderer() fyne.WidgetRenderer {
	return &columnResizeHandleRenderer{handle: h, objects: []fyne.CanvasObject{h.indicator}}
}

type columnResizeHandleRenderer struct {
	handle  *columnResizeHandle
	objects []fyne.CanvasObject
}

func (r *columnResizeHandleRenderer) Layout(size fyne.Size) {
	lineWidth := float32(2)
	r.handle.indicator.Move(fyne.NewPos(clampMinZero(size.Width-lineWidth), 0))
	r.handle.indicator.Resize(fyne.NewSize(lineWidth, size.Height))
}

func (r *columnResizeHandleRenderer) MinSize() fyne.Size {
	return r.handle.MinSize()
}

func (r *columnResizeHandleRenderer) Refresh() {
	canvas.Refresh(r.handle.indicator)
}

func (r *columnResizeHandleRenderer) Destroy() {
}

func (r *columnResizeHandleRenderer) Objects() []fyne.CanvasObject {
	return r.objects
}

func (r *columnResizeHandleRenderer) BackgroundColor() color.Color {
	return color.Transparent
}

func newHoverLabel(manager *hoverTooltipManager) *hoverLabel {
	label := widget.NewLabel("")
	label.Truncation = fyne.TextTruncateEllipsis
	h := &hoverLabel{
		hoverTooltipOwner: hoverTooltipOwner{manager: manager},
		label:             label,
	}
	h.ExtendBaseWidget(h)
	return h
}

func (h *hoverLabel) SetAlignment(alignment fyne.TextAlign) {
	h.label.Alignment = alignment
	h.label.Refresh()
}

func (h *hoverLabel) SetText(display string, hover string) {
	h.hideTooltip(h)
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
	content := newTextTooltip(h.fullText, 160)
	if content == nil {
		return
	}
	h.showTooltip(h, content)
}

func (h *hoverLabel) MouseMoved(*desktop.MouseEvent) {
}

func (h *hoverLabel) MouseOut() {
	h.scheduleHide(h)
}

func (h *hoverLabel) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(h.label)
}

func clampMinZero(value float32) float32 {
	if value > 0 {
		return value
	}

	return 0
}
