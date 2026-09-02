package ui

import (
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

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
	contentRowHeight    float32 = 36
	contentHeaderHeight float32 = 34
	// contentExpanderWidth is the fixed width of the expand/collapse toggle;
	// file rows reserve the same space so names align at equal depth.
	contentExpanderWidth float32 = 24
)

type detailsContentTabView struct {
	app     *application
	root    *fyne.Container
	content *fyne.Container
	filter  *widget.Entry
	status  *detailsStatusChrome
	msgs    detailsStatusMessages
	header  *detailsContentHeader
	list    *widget.List
	scroll  *container.Scroll
	rows    []*contentVisibleRow
	tree    *contentTree
}

type contentColumnSpec struct {
	key   string
	label string
	width float32
}

var contentColumnSpecs = []contentColumnSpec{
	{key: "checkbox", label: "", width: 34},
	{key: "name", label: "Name", width: 240},
	{key: "size", label: "Total Size", width: 90},
	{key: "progress", label: "Progress", width: 110},
	{key: "priority", label: "Priority", width: 110},
	{key: "remaining", label: "Remaining", width: 90},
	{key: "availability", label: "Availability", width: 90},
}

type contentTree struct {
	root *contentNode
}

type contentNode struct {
	id   string
	name string
	path string
	// lowerPath is path lowercased once at build time for filter matching.
	lowerPath    string
	isDir        bool
	parent       *contentNode
	children     []*contentNode
	file         *qbt.TorrentFile
	size         int64
	progress     float64
	remaining    int64
	availability float64
	priority     int
	hasAvail     bool
}

type contentVisibleRow struct {
	node      *contentNode
	depth     int
	filtering bool
}

type detailsContentHeader struct {
	widget.BaseWidget
	root *fyne.Container
}

type detailsContentRow struct {
	widget.BaseWidget
	app     *application
	root    *fyne.Container
	hoverBG *canvas.Rectangle
	// nameCell is the name column: the expander toggle plus the name label,
	// indented by tree depth.
	nameCell      *fyne.Container
	checkbox      *triStateCheck
	expander      *contentExpanderToggle
	name          *hoverLabel
	size          *hoverLabel
	progress      *widget.ProgressBar
	priority      *hoverLabel
	remaining     *hoverLabel
	availability  *hoverLabel
	current       *contentVisibleRow
	hovered       bool
	hoverTimer    *time.Timer
	hoverOutDelay time.Duration
}

type contentHeaderLayout struct{}
type contentRowLayout struct{}
type contentTableLayout struct{}

func newDetailsContentTabView(app *application) *detailsContentTabView {
	v := &detailsContentTabView{
		app:  app,
		root: container.NewStack(),
	}
	v.filter = widget.NewEntry()
	v.filter.SetPlaceHolder("Filter files...")
	v.filter.OnChanged = func(value string) {
		v.app.setContentFilter(value)
	}
	v.header = newDetailsContentHeader()
	v.list = widget.NewList(
		func() int { return len(v.rows) },
		func() fyne.CanvasObject { return newDetailsContentRow(app) },
		func(id widget.ListItemID, item fyne.CanvasObject) {
			if id < 0 || id >= len(v.rows) {
				return
			}
			item.(*detailsContentRow).SetRow(v.rows[id])
		},
	)
	v.list.HideSeparators = true
	table := container.New(&contentTableLayout{}, v.header, v.list)
	v.scroll = container.NewHScroll(table)
	// Chrome is built once; Refresh only updates text and visibility so the
	// filter entry and scroll position survive poll ticks.
	v.content = container.NewBorder(v.filter, nil, nil, nil, v.scroll)
	v.status = newDetailsStatusChrome(app)
	v.msgs = detailsStatusMessages{
		loading:      "Loading content...",
		failedPrefix: "Failed to load content:",
		idle:         "Content will load when this tab becomes active.",
	}
	return v
}

func (v *detailsContentTabView) Root() fyne.CanvasObject {
	return v.root
}

func (v *detailsContentTabView) Refresh() {
	state := v.app.detailsState
	dataset := state.activeDataset(detailsTabContent)
	if dataset != nil && v.filter.Text != state.Content.Filter {
		// Fires only for external resets; the resulting OnChanged is absorbed
		// by setContentFilter's equality short-circuit.
		v.filter.SetText(state.Content.Filter)
	}
	if v.status.present(v.root, v.content, v.msgs, dataset) {
		v.root.Refresh()

		return
	}
	v.tree = buildContentTree(state.Content.Files)
	v.rows = v.tree.visibleRows(strings.TrimSpace(state.Content.Filter), state.Content.Expanded)
	v.list.Refresh()
	v.root.Refresh()
}

func buildContentTree(files []qbt.TorrentFile) *contentTree {
	root := &contentNode{id: "", name: "", path: "", isDir: true, priority: contentPriorityMixed, availability: -1}
	for i := range files {
		file := files[i]
		parts := strings.Split(strings.Trim(file.Name, "/"), "/")
		parent := root
		currentPath := ""
		for index, part := range parts {
			currentPath = path.Join(currentPath, part)
			last := index == len(parts)-1
			child := findContentChild(parent, part, last)
			if child == nil {
				child = &contentNode{
					id:        currentPath,
					name:      part,
					path:      currentPath,
					lowerPath: strings.ToLower(currentPath),
					isDir:     !last,
					parent:    parent,
					priority:  contentPriorityMixed,
					hasAvail:  last && file.Availability >= 0,
				}
				parent.children = append(parent.children, child)
			}
			if last {
				fileCopy := file
				child.file = &fileCopy
				child.size = file.Size
				// Trust the server-reported progress even for skipped files;
				// faking 100% would hide real download state.
				child.progress = file.Progress
				child.remaining = int64(float64(file.Size) * (1 - file.Progress))
				child.availability = file.Availability
				child.priority = file.Priority
				child.hasAvail = file.Availability >= 0
			}
			parent = child
		}
	}
	sortContentChildren(root)
	recalculateContentNode(root)
	return &contentTree{root: root}
}

func findContentChild(parent *contentNode, name string, last bool) *contentNode {
	for _, child := range parent.children {
		if child.name == name && child.isDir != last {
			return child
		}
	}
	return nil
}

func sortContentChildren(node *contentNode) {
	sort.Slice(node.children, func(i, j int) bool {
		if node.children[i].isDir != node.children[j].isDir {
			return node.children[i].isDir
		}
		return strings.ToLower(node.children[i].name) < strings.ToLower(node.children[j].name)
	})
	for _, child := range node.children {
		sortContentChildren(child)
	}
}

func recalculateContentNode(node *contentNode) {
	if !node.isDir {
		return
	}
	var (
		size         int64
		activeSize   int64
		remaining    int64
		weightedProg float64
		weightedAv   float64
		availSize    int64
		prio         int
		hasPrio      bool
	)
	for _, child := range node.children {
		if child.isDir {
			recalculateContentNode(child)
		}
		size += child.size
		if child.priority != contentPriorityIgnored {
			activeSize += child.size
			weightedProg += child.progress * float64(child.size)
			remaining += child.remaining
			if child.hasAvail {
				weightedAv += child.availability * float64(child.size)
				availSize += child.size
			}
		}
		if !hasPrio {
			prio = child.priority
			hasPrio = true
		} else if prio != child.priority {
			prio = contentPriorityMixed
		}
	}
	node.size = size
	node.remaining = remaining
	node.priority = prio
	if activeSize > 0 {
		node.progress = weightedProg / float64(activeSize)
	} else {
		node.progress = 1
	}
	if availSize > 0 {
		node.availability = weightedAv / float64(availSize)
		node.hasAvail = true
	} else {
		node.availability = -1
		node.hasAvail = false
	}
}

func (t *contentTree) visibleRows(filter string, expanded map[string]bool) []*contentVisibleRow {
	if t == nil || t.root == nil {
		return nil
	}
	filter = strings.ToLower(strings.TrimSpace(filter))
	out := make([]*contentVisibleRow, 0)
	for _, child := range t.root.children {
		t.walkVisible(child, 0, filter, expanded, &out)
	}
	return out
}

func (t *contentTree) walkVisible(node *contentNode, depth int, filter string, expanded map[string]bool, out *[]*contentVisibleRow) {
	if node == nil {
		return
	}
	// lowerPath is precomputed at build time; probing whether a subtree
	// matches happens at most once per pruned node via subtreeMatches instead
	// of re-walking the whole subtree on every refresh.
	if filter == "" || strings.Contains(node.lowerPath, filter) || subtreeMatches(node, filter) {
		*out = append(*out, &contentVisibleRow{node: node, depth: depth, filtering: filter != ""})
		if contentRowExpanded(node, filter != "", expanded) {
			for _, child := range node.children {
				t.walkVisible(child, depth+1, filter, expanded, out)
			}
		}
	}
}

// subtreeMatches reports whether node or any of its descendants matches the
// filter.
func subtreeMatches(node *contentNode, filter string) bool {
	if strings.Contains(node.lowerPath, filter) {
		return true
	}
	for _, child := range node.children {
		if subtreeMatches(child, filter) {
			return true
		}
	}
	return false
}

// contentRowExpanded reports whether a directory row's children are shown.
// While filtering, directories are forced open regardless of the saved
// expansion state so matches stay reachable.
func contentRowExpanded(node *contentNode, filtering bool, expanded map[string]bool) bool {
	return node.isDir && (filtering || expanded[node.path])
}

func newDetailsContentHeader() *detailsContentHeader {
	h := &detailsContentHeader{}
	objects := make([]fyne.CanvasObject, 0, len(contentColumnSpecs))
	for _, spec := range contentColumnSpecs {
		label := widget.NewLabel(spec.label)
		label.TextStyle = fyne.TextStyle{Bold: true}
		label.Truncation = fyne.TextTruncateEllipsis
		objects = append(objects, label)
	}
	h.root = container.New(&contentHeaderLayout{}, objects...)
	h.ExtendBaseWidget(h)
	return h
}

func (h *detailsContentHeader) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(h.root)
}

// contentExpanderToggle is a compact expand/collapse control drawn as a glyph.
// A plain Button is too wide for the fixed name column and steals space from
// long file names.
type contentExpanderToggle struct {
	widget.BaseWidget
	label    *widget.Label
	onTapped func()
}

func newContentExpanderToggle(onTapped func()) *contentExpanderToggle {
	toggle := &contentExpanderToggle{
		label:    widget.NewLabel(""),
		onTapped: onTapped,
	}
	toggle.label.Truncation = fyne.TextTruncateEllipsis
	toggle.ExtendBaseWidget(toggle)
	return toggle
}

func (t *contentExpanderToggle) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(t.label)
}

func (t *contentExpanderToggle) MinSize() fyne.Size {
	return fyne.NewSize(contentExpanderWidth, widget.NewLabel("").MinSize().Height)
}

func (t *contentExpanderToggle) Tapped(*fyne.PointEvent) {
	if t.onTapped != nil {
		t.onTapped()
	}
}

func (t *contentExpanderToggle) SetExpanded(open bool) {
	if open {
		t.label.SetText("▾")
	} else {
		t.label.SetText("▸")
	}
}

func newDetailsContentRow(app *application) *detailsContentRow {
	row := &detailsContentRow{
		app:           app,
		hoverBG:       canvas.NewRectangle(theme.Color(theme.ColorNameHover)),
		hoverOutDelay: rowHoverOutDelay,
		progress:      widget.NewProgressBar(),
	}
	row.hoverBG.Hide()
	row.checkbox = newTriStateCheck(row)
	// Cells carry instant tooltips: like the torrent table's non-name cells,
	// the tooltip appears only when the value is wider than its column.
	row.name = newHoverLabel(app.tooltipManager, 0, row)
	row.size = newHoverLabel(app.tooltipManager, 0, row)
	row.priority = newHoverLabel(app.tooltipManager, 0, row)
	row.remaining = newHoverLabel(app.tooltipManager, 0, row)
	row.availability = newHoverLabel(app.tooltipManager, 0, row)
	row.size.SetAlignment(fyne.TextAlignTrailing)
	row.remaining.SetAlignment(fyne.TextAlignTrailing)
	row.availability.SetAlignment(fyne.TextAlignTrailing)
	row.expander = newContentExpanderToggle(nil)
	row.nameCell = container.New(&contentNameLayout{row: row}, row.expander, row.name)
	content := container.New(&contentRowLayout{},
		row.checkbox,
		row.nameCell,
		row.size,
		row.progress,
		row.priority,
		row.remaining,
		row.availability,
	)
	row.root = container.NewStack(row.hoverBG, content)
	row.ExtendBaseWidget(row)
	return row
}

// hoverIn shows the row highlight. Hover-capable cells swallow the list's own
// row hover tint (the driver delivers hover to the deepest Hoverable), so the
// row reproduces it and the cells forward their enter/leave events here.
func (r *detailsContentRow) hoverIn() {
	if r.hoverTimer != nil {
		r.hoverTimer.Stop()
		r.hoverTimer = nil
	}
	if r.hovered {
		return
	}
	r.hovered = true
	r.hoverBG.FillColor = theme.Color(theme.ColorNameHover)
	r.hoverBG.Show()
	r.hoverBG.Refresh()
}

func (r *detailsContentRow) hoverOut() {
	if r.hoverTimer != nil {
		r.hoverTimer.Stop()
	}
	r.hoverTimer = time.AfterFunc(r.hoverOutDelay, func() {
		fyne.Do(r.finishHoverOut)
	})
}

// finishHoverOut performs the delayed part of hoverOut on the UI thread.
func (r *detailsContentRow) finishHoverOut() {
	if !r.hovered {
		r.hoverTimer = nil

		return
	}
	r.hovered = false
	r.hoverBG.Hide()
	r.hoverBG.Refresh()
	r.hoverTimer = nil
}

func (r *detailsContentRow) MouseIn(*desktop.MouseEvent) {
	r.hoverIn()
}

func (r *detailsContentRow) MouseMoved(*desktop.MouseEvent) {
}

func (r *detailsContentRow) MouseOut() {
	r.hoverOut()
}

func (r *detailsContentRow) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(r.root)
}

func (r *detailsContentRow) SetRow(row *contentVisibleRow) {
	r.current = row
	if row == nil || row.node == nil {
		return
	}
	node := row.node
	r.checkbox.SetState(contentCheckState(node.priority))
	// The name's hover text is the full path: the label shows the entry name
	// truncated by the column, so the tooltip is what disambiguates it.
	r.name.SetText(node.name, node.path)
	size := appcore.HumanBytes(node.size)
	r.size.SetText(size, size)
	r.progress.SetValue(node.progress)
	priority := contentPriorityLabel(node.priority)
	r.priority.SetText(priority, priority)
	remaining := appcore.HumanBytes(node.remaining)
	r.remaining.SetText(remaining, remaining)
	availability := contentAvailabilityLabel(node)
	r.availability.SetText(availability, availability)
	if node.isDir && !row.filtering {
		nodePath := node.path
		r.expander.onTapped = func() {
			r.app.toggleContentNode(nodePath)
		}
		r.expander.SetExpanded(contentRowExpanded(node, row.filtering, r.app.detailsState.Content.Expanded))
		r.expander.Show()
	} else {
		// While filtering, directories are forced open, so a toggle would be
		// invisible yet still rewrite the saved expansion state; drop the
		// handler along with the chevron.
		r.expander.onTapped = nil
		r.expander.Hide()
	}
	// Recycled rows are resized by the list before UpdateCell runs, which
	// silently skips layout; re-run it at the stored size.
	r.root.Refresh()
}

// contentNameLayout indents the name cell by tree depth and gives the name
// label the remaining width so truncation applies inside the column.
type contentNameLayout struct {
	row *detailsContentRow
}

func (l *contentNameLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) != 2 {
		return
	}
	expander, name := objects[0], objects[1]
	x := float32(0)
	if l.row != nil && l.row.current != nil {
		x = float32(l.row.current.depth) * theme.IconInlineSize()
	}
	// Always reserve the toggle width so file rows align with directory rows.
	expander.Move(fyne.NewPos(x, 0))
	expander.Resize(fyne.NewSize(contentExpanderWidth, size.Height))
	x += contentExpanderWidth
	name.Move(fyne.NewPos(x, 0))
	name.Resize(fyne.NewSize(size.Width-x, size.Height))
}

func (l *contentNameLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	min := fyne.NewSize(contentExpanderWidth, 0)
	if len(objects) == 2 {
		nameMin := objects[1].MinSize()
		min.Width += nameMin.Width
		min.Height = nameMin.Height
	}
	return min
}

func (l *contentHeaderLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	layoutContentColumns(objects, size)
}

func (l *contentHeaderLayout) MinSize(_ []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(totalContentWidth(), contentHeaderHeight)
}

func (l *contentRowLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	layoutContentColumns(objects, size)
}

func (l *contentRowLayout) MinSize(_ []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(totalContentWidth(), contentRowHeight)
}

func (l *contentTableLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) != 2 {
		return
	}
	header := objects[0]
	list := objects[1]
	header.Move(fyne.NewPos(0, 0))
	header.Resize(fyne.NewSize(totalContentWidth(), contentHeaderHeight))
	list.Move(fyne.NewPos(0, contentHeaderHeight))
	list.Resize(fyne.NewSize(totalContentWidth(), size.Height-contentHeaderHeight))
}

func (l *contentTableLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	if len(objects) != 2 {
		return fyne.Size{}
	}
	return fyne.NewSize(totalContentWidth(), contentHeaderHeight+objects[1].MinSize().Height)
}

func layoutContentColumns(objects []fyne.CanvasObject, size fyne.Size) {
	x := float32(0)
	for index, spec := range contentColumnSpecs {
		obj := objects[index]
		obj.Move(fyne.NewPos(x+4, 1))
		obj.Resize(fyne.NewSize(spec.width-8, size.Height-2))
		x += spec.width
	}
}

func totalContentWidth() float32 {
	total := float32(0)
	for _, spec := range contentColumnSpecs {
		total += spec.width
	}
	return total
}

const (
	contentPriorityIgnored = 0
	contentPriorityNormal  = 1
	contentPriorityHigh    = 6
	contentPriorityMaximum = 7
	contentPriorityMixed   = -1
)

func contentPriorityLabel(priority int) string {
	switch priority {
	case contentPriorityIgnored:
		return "Do not download"
	case contentPriorityNormal:
		return "Normal"
	case contentPriorityHigh:
		return "High"
	case contentPriorityMaximum:
		return "Maximum"
	case contentPriorityMixed:
		return "Mixed"
	default:
		return fmt.Sprintf("%d", priority)
	}
}

func contentCheckState(priority int) checkState {
	switch priority {
	case contentPriorityIgnored:
		return checkStateUnchecked
	case contentPriorityMixed:
		return checkStateMixed
	default:
		return checkStateChecked
	}
}

func contentAvailabilityLabel(node *contentNode) string {
	if node == nil || !node.hasAvail || node.availability < 0 {
		return "?"
	}
	return fmt.Sprintf("%.1f%%", node.availability*100)
}
