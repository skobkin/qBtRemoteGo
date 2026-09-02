package ui

import (
	"image/color"
	"strings"
	"time"

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

const (
	torrentRowHeight     float32 = 40
	compactRowHeight     float32 = 28
	torrentHeaderHeight  float32 = 38
	headerHandleWidth    float32 = 8
	headerCellPadding    float32 = 8
	rowCellPadding       float32 = 6
	rowEmphasisCellInset float32 = 4
)

// torrentDoubleTapInterval is the maximum gap between two clicks on the same
// row for them to count as a double-click opening the details.
const torrentDoubleTapInterval = 400 * time.Millisecond

// rowTapSequencer detects double-clicks from plain tap events. Fyne's
// DoubleTapped delays the first tap by its own double-click window, which made
// selection feel laggy and dropped taps; instead every tap selects instantly
// and a second tap on the same row within the interval opens the details.
// The sequencer lives on the application, not the row: List recycles rows and
// the first tap refreshes the list.
type rowTapSequencer struct {
	interval time.Duration
	lastAt   time.Time
	lastHash string
}

// record registers a tap on the given row and reports whether it completed a
// double-click. Modified taps (ctrl/shift/super) never complete one — they
// toggle or extend the selection — and reset a pending plain double-click so a
// modifier click between two plain taps cannot fuse them. Triggering resets
// the sequence, so a third tap starts a new one instead of opening the
// details twice.
func (s *rowTapSequencer) record(hash string, now time.Time, modifier fyne.KeyModifier) bool {
	if modifier != 0 {
		s.reset()

		return false
	}
	if s.interval <= 0 {
		s.interval = torrentDoubleTapInterval
	}
	if s.lastHash == hash && now.Sub(s.lastAt) <= s.interval {
		s.reset()

		return true
	}
	s.lastAt = now
	s.lastHash = hash

	return false
}

func (s *rowTapSequencer) reset() {
	s.lastAt = time.Time{}
	s.lastHash = ""
}

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
	app           *application
	background    *canvas.Rectangle
	hoverBG       *canvas.Rectangle
	root          *fyne.Container
	content       *fyne.Container
	name          *hoverLabel
	size          *hoverLabel
	progress      *widget.ProgressBar
	statusBG      *canvas.Rectangle
	statusTx      *widget.Label
	statusCt      *fyne.Container
	down          *hoverLabel
	up            *hoverLabel
	eta           *hoverLabel
	added         *hoverLabel
	separator     *widget.Separator
	hash          string
	modifier      fyne.KeyModifier
	hovered       bool
	hoverTimer    *time.Timer
	hoverOutDelay time.Duration
	// selectedShown mirrors the selection background's visibility so rows whose
	// selection state did not change skip the repaint entirely.
	selectedShown bool
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

// hoverTarget is notified when a hover-capable cell inside a row is entered or
// left so the owning row can keep its highlight alive while the cursor crosses
// the gaps between the row's cells.
type hoverTarget interface {
	hoverIn()
	hoverOut()
}

type hoverLabel struct {
	widget.BaseWidget
	hoverTooltipOwner
	label *widget.Label
	// root is what the renderer wraps: the label itself, or a stack with an
	// optional self-painted hover background (see hoverBG). Built in the
	// constructor so a renderer rebuilt by Fyne's renderer cache keeps it.
	root fyne.CanvasObject
	// hoverBG paints the hover tint when this label is a widget.Table cell:
	// a hoverable cell suppresses the table's built-in tint for the cell, so
	// the cell must reproduce it. Nil in the plain (row cell) variant.
	hoverBG   *canvas.Rectangle
	fullText  string
	showDelay time.Duration
	row       hoverTarget
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
	a.compactRows = a.controller.Config().UI.CompactRows
	// The list is built once; drop any rows registered by an earlier build so
	// the selection scan never touches dead widgets.
	a.listRows = nil

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

// torrentRowHeightValue returns the torrent row height for the current
// compact-rows setting. The flag is cached on the application (like the column
// widths) instead of being read from the controller here: MinSize runs per row
// on every layout pass and Config copies the whole config under a lock.
func (a *application) torrentRowHeightValue() float32 {
	if a != nil && a.compactRows {
		return compactRowHeight
	}
	return torrentRowHeight
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
	widths := cloneColumnWidths(a.columnWidths)
	err := a.controller.SaveLocalUI(func(cfg *config.AppConfig) {
		cfg.UI.ColumnWidths = widths
	})
	if err != nil {
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
		app:           app,
		background:    canvas.NewRectangle(theme.Color(theme.ColorNameSelection)),
		hoverBG:       canvas.NewRectangle(theme.Color(theme.ColorNameHover)),
		hoverOutDelay: rowHoverOutDelay,
	}
	row.name = newHoverLabel(app.tooltipManager, tooltipShowDelay, row)
	row.size = newHoverLabel(app.tooltipManager, 0, row)
	row.progress = widget.NewProgressBar()
	row.statusBG = canvas.NewRectangle(color.Transparent)
	row.statusTx = widget.NewLabel("")
	row.down = newHoverLabel(app.tooltipManager, 0, row)
	row.up = newHoverLabel(app.tooltipManager, 0, row)
	row.eta = newHoverLabel(app.tooltipManager, 0, row)
	row.added = newHoverLabel(app.tooltipManager, 0, row)
	row.separator = widget.NewSeparator()

	row.background.Hide()
	row.hoverBG.Hide()
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
	row.root = container.NewStack(row.background, row.hoverBG, row.content)
	row.ExtendBaseWidget(row)
	// Track the row for the targeted selection refresh. List recycles rows and
	// never destroys them while the window lives, so the slice stays bounded by
	// the visible row count.
	app.listRows = append(app.listRows, row)
	return row
}

// setSelected syncs the row's selection background. Only the background moves:
// Fyne has no cheap way to repaint a single list row (RefreshItem re-runs the
// renderer's full refresh), so the selection refresh walks the tracked rows
// and calls this directly.
func (r *torrentListRow) setSelected(selected bool) {
	if r.selectedShown == selected {
		return
	}
	r.selectedShown = selected
	if selected {
		r.background.Show()
	} else {
		r.background.Hide()
	}
	r.background.Refresh()
}

func (r *torrentListRow) setTorrent(torrent qbt.Torrent) {
	r.hash = torrent.Hash
	r.background.FillColor = theme.Color(theme.ColorNameSelection)
	r.setSelected(r.app.selection[torrent.Hash])

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
	return fyne.NewSize(r.app.totalColumnWidth(), r.app.torrentRowHeightValue())
}

func (r *torrentListRow) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(r.root)
}

func (r *torrentListRow) Tapped(*fyne.PointEvent) {
	if r.app.rowTaps.record(r.hash, time.Now(), r.modifier) {
		r.app.selectOnlyTorrent(r.hash)
		r.app.refreshTorrentSelection()
		r.app.openTorrentDetails(r.hash)

		return
	}
	r.app.applyTorrentSelection(r.hash, r.modifier)
}

func (r *torrentListRow) TappedSecondary(event *fyne.PointEvent) {
	r.app.prepareTorrentContextSelection(r.hash)
	copyNameItem := fyne.NewMenuItem("Name", func() {
		r.app.copySelectedTorrentNames()
	})
	copyMagnetItem := fyne.NewMenuItem("Magnet link", func() {
		r.app.copySelectedTorrentMagnetLinks()
	})
	if _, ok := r.app.selectedTorrentMagnetLinksText(); !ok {
		copyMagnetItem.Disabled = true
	}
	copyItem := fyne.NewMenuItem("Copy", nil)
	copyItem.ChildMenu = fyne.NewMenu("", copyNameItem, copyMagnetItem)
	renameItem := fyne.NewMenuItem("Rename", func() {
		r.app.openRenameTorrentDialog()
	})
	if _, ok := r.app.selectedRenameTarget(); !ok {
		renameItem.Disabled = true
	}

	menu := fyne.NewMenu("",
		fyne.NewMenuItem("Start", func() {
			r.app.startSelectedTorrents()
		}),
		fyne.NewMenuItem("Stop", func() {
			r.app.stopSelectedTorrents()
		}),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Set location", func() {
			r.app.openSetLocationDialog()
		}),
		renameItem,
		fyne.NewMenuItem("Force recheck", func() {
			r.app.forceRecheckSelectedTorrents()
		}),
		fyne.NewMenuItem("Force reannounce", func() {
			r.app.forceReannounceSelectedTorrents()
		}),
		copyItem,
		fyne.NewMenuItemSeparator(),
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
	return fyne.NewSize(l.app.totalColumnWidth(), l.app.torrentRowHeightValue())
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

// hoverIn marks the row as hovered and shows the hover background. If a
// pending hoverOut timer exists (cursor moving between cells in the same
// row), it is cancelled so the row stays highlighted.
func (r *torrentListRow) hoverIn() {
	if r.hoverTimer != nil {
		r.hoverTimer.Stop()
		r.hoverTimer = nil
	}
	if r.hovered {
		return
	}
	r.hovered = true
	r.hoverBG.Show()
	r.hoverBG.Refresh()
}

// rowHoverOutDelay is how long hoverOut waits before hiding the background.
// The delay lets the cursor traverse gaps between cells (e.g. the status /
// progress areas) without flashing the highlight off and back on.
const rowHoverOutDelay = 50 * time.Millisecond

// hoverOut schedules the hover background to be hidden after a short delay.
func (r *torrentListRow) hoverOut() {
	if r.hoverTimer != nil {
		r.hoverTimer.Stop()
	}
	r.hoverTimer = time.AfterFunc(r.hoverOutDelay, func() {
		fyne.Do(r.finishHoverOut)
	})
}

// finishHoverOut performs the delayed part of hoverOut on the UI thread.
func (r *torrentListRow) finishHoverOut() {
	if !r.hovered {
		r.hoverTimer = nil

		return
	}
	r.hovered = false
	r.hoverBG.Hide()
	r.hoverBG.Refresh()
	r.hoverTimer = nil
}

func newHoverLabel(manager *hoverTooltipManager, showDelay time.Duration, row hoverTarget) *hoverLabel {
	label := widget.NewLabel("")
	label.Truncation = fyne.TextTruncateEllipsis
	h := &hoverLabel{
		hoverTooltipOwner: hoverTooltipOwner{manager: manager},
		label:             label,
		root:              label,
		showDelay:         showDelay,
		row:               row,
	}
	h.ExtendBaseWidget(h)
	return h
}

// newHoverCellLabel returns a hover label for a widget.Table cell: it shows a
// tooltip when its text is truncated (like the torrent-table cells) and paints
// its own hover background, because a hoverable cell suppresses the table's
// built-in hover tint.
func newHoverCellLabel(manager *hoverTooltipManager) *hoverLabel {
	label := widget.NewLabel("")
	label.Wrapping = fyne.TextWrapOff
	label.Truncation = fyne.TextTruncateEllipsis
	bg := canvas.NewRectangle(theme.Color(theme.ColorNameHover))
	bg.CornerRadius = theme.Size(theme.SizeNameSelectionRadius)
	bg.Hide()
	h := &hoverLabel{
		hoverTooltipOwner: hoverTooltipOwner{manager: manager},
		label:             label,
		root:              container.NewStack(bg, label),
		hoverBG:           bg,
	}
	h.ExtendBaseWidget(h)
	return h
}

// showHoverBackground repaints the cell tint the table suppresses while this
// cell handles hover; the color is re-resolved so a runtime theme switch keeps
// the tint current.
func (h *hoverLabel) showHoverBackground() {
	if h.hoverBG == nil {
		return
	}
	h.hoverBG.FillColor = theme.Color(theme.ColorNameHover)
	h.hoverBG.Show()
	h.hoverBG.Refresh()
}

// hideHoverBackground clears the tint, both on mouse-out and when the label is
// rebound to new text while pooled (the cursor may sit still over the cell).
func (h *hoverLabel) hideHoverBackground() {
	if h.hoverBG == nil {
		return
	}
	h.hoverBG.Hide()
	h.hoverBG.Refresh()
}

func (h *hoverLabel) SetAlignment(alignment fyne.TextAlign) {
	h.label.Alignment = alignment
	h.label.Refresh()
}

func (h *hoverLabel) SetText(display string, hover string) {
	h.cancelShow()
	h.hideTooltip(h)
	h.hideHoverBackground()
	h.label.SetText(display)
	if strings.TrimSpace(hover) == "" {
		h.fullText = display
		return
	}
	h.fullText = hover
}

func (h *hoverLabel) MouseIn(*desktop.MouseEvent) {
	if h.row != nil {
		h.row.hoverIn()
	}
	h.showHoverBackground()
	if strings.TrimSpace(h.fullText) == "" {
		return
	}
	textWidth := fyne.MeasureText(h.fullText, theme.TextSize(), fyne.TextStyle{}).Width
	if textWidth <= h.Size().Width {
		return
	}
	content := newTextTooltip(h.fullText, tooltipMaxWidthFor(h, tooltipMaxWidthRatio))
	if content == nil {
		return
	}
	if h.showDelay > 0 {
		h.scheduleShow(h, content, h.showDelay)
		return
	}
	h.showTooltip(h, content)
}

func (h *hoverLabel) MouseMoved(*desktop.MouseEvent) {
}

func (h *hoverLabel) MouseOut() {
	if h.row != nil {
		h.row.hoverOut()
	}
	h.hideHoverBackground()
	h.scheduleHide(h)
}

func (h *hoverLabel) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(h.root)
}

func clampMinZero(value float32) float32 {
	if value > 0 {
		return value
	}

	return 0
}
