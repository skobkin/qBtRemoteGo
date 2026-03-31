package ui

import (
	"fmt"
	"image/color"
	"path"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	appcore "github.com/skobkin/qbtremotego/internal/app"
	"github.com/skobkin/qbtremotego/internal/qbt"
)

const (
	contentRowHeight    float32 = 36
	contentHeaderHeight float32 = 34
)

type detailsContentTabView struct {
	app    *application
	root   *fyne.Container
	filter *widget.Entry
	header *detailsContentHeader
	list   *widget.List
	scroll *container.Scroll
	rows   []*contentVisibleRow
	tree   *contentTree
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
	id           string
	name         string
	path         string
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
	node  *contentNode
	depth int
}

type detailsContentHeader struct {
	widget.BaseWidget
	root *fyne.Container
}

type detailsContentRow struct {
	widget.BaseWidget
	app          *application
	root         *fyne.Container
	checkbox     *widget.Label
	expander     *widget.Button
	name         *widget.Label
	size         *widget.Label
	progress     *widget.ProgressBar
	priority     *widget.Label
	remaining    *widget.Label
	availability *widget.Label
	indent       *canvas.Rectangle
	row          *contentVisibleRow
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
	return v
}

func (v *detailsContentTabView) Root() fyne.CanvasObject {
	return v.root
}

func (v *detailsContentTabView) Refresh() {
	state := v.app.detailsState.Content
	if v.filter.Text != state.Filter {
		v.filter.SetText(state.Filter)
	}
	switch {
	case state.Loading && !state.Loaded:
		v.root.Objects = []fyne.CanvasObject{detailsStatusState("Loading content...")}
	case strings.TrimSpace(state.Error) != "":
		v.root.Objects = []fyne.CanvasObject{detailsStatusState("Failed to load content:\n" + state.Error)}
	case !state.Loaded:
		v.root.Objects = []fyne.CanvasObject{detailsStatusState("Content will load when this tab becomes active.")}
	default:
		v.tree = buildContentTree(state.Files)
		v.rows = v.tree.visibleRows(strings.TrimSpace(state.Filter), state.Expanded)
		v.list.Refresh()
		v.root.Objects = []fyne.CanvasObject{container.NewBorder(v.filter, nil, nil, nil, v.scroll)}
	}
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
					id:       currentPath,
					name:     part,
					path:     currentPath,
					isDir:    !last,
					parent:   parent,
					priority: contentPriorityMixed,
					hasAvail: last && file.Availability >= 0,
				}
				parent.children = append(parent.children, child)
			}
			if last {
				fileCopy := file
				child.file = &fileCopy
				child.size = file.Size
				if file.Priority == contentPriorityIgnored {
					child.progress = 1
					child.remaining = 0
				} else {
					child.progress = file.Progress
					child.remaining = int64(float64(file.Size) * (1 - file.Progress))
				}
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

func (t *contentTree) walkVisible(node *contentNode, depth int, filter string, expanded map[string]bool, out *[]*contentVisibleRow) bool {
	if node == nil {
		return false
	}
	matches := filter == "" || strings.Contains(strings.ToLower(node.path), filter)
	childMatch := false
	if node.isDir {
		for _, child := range node.children {
			if t.walkVisible(child, depth+1, filter, expanded, &[]*contentVisibleRow{}) {
				childMatch = true
				break
			}
		}
	}
	if filter != "" && !matches && !childMatch {
		return false
	}
	*out = append(*out, &contentVisibleRow{node: node, depth: depth})
	if node.isDir && (filter != "" || expanded[node.path]) {
		for _, child := range node.children {
			t.walkVisible(child, depth+1, filter, expanded, out)
		}
	}
	return true
}

func newDetailsContentHeader() *detailsContentHeader {
	h := &detailsContentHeader{}
	objects := make([]fyne.CanvasObject, 0, len(contentColumnSpecs))
	for _, spec := range contentColumnSpecs {
		label := widget.NewLabel(spec.label)
		label.TextStyle = fyne.TextStyle{Bold: true}
		objects = append(objects, label)
	}
	h.root = container.New(&contentHeaderLayout{}, objects...)
	h.ExtendBaseWidget(h)
	return h
}

func (h *detailsContentHeader) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(h.root)
}

func newDetailsContentRow(app *application) *detailsContentRow {
	row := &detailsContentRow{
		app:          app,
		checkbox:     widget.NewLabel(""),
		expander:     widget.NewButton("", nil),
		name:         widget.NewLabel(""),
		size:         widget.NewLabel(""),
		progress:     widget.NewProgressBar(),
		priority:     widget.NewLabel(""),
		remaining:    widget.NewLabel(""),
		availability: widget.NewLabel(""),
		indent:       canvas.NewRectangle(color.Transparent),
	}
	row.size.Alignment = fyne.TextAlignTrailing
	row.remaining.Alignment = fyne.TextAlignTrailing
	row.availability.Alignment = fyne.TextAlignTrailing
	row.expander.Importance = widget.LowImportance
	row.root = container.New(&contentRowLayout{},
		row.checkbox,
		container.NewHBox(row.indent, row.expander, row.name),
		row.size,
		row.progress,
		row.priority,
		row.remaining,
		row.availability,
	)
	row.ExtendBaseWidget(row)
	return row
}

func (r *detailsContentRow) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(r.root)
}

func (r *detailsContentRow) SetRow(row *contentVisibleRow) {
	r.row = row
	if row == nil || row.node == nil {
		return
	}
	node := row.node
	r.checkbox.SetText(contentCheckboxText(node.priority))
	r.name.SetText(node.name)
	r.size.SetText(appcore.HumanBytes(node.size))
	r.progress.SetValue(node.progress)
	r.priority.SetText(contentPriorityLabel(node.priority))
	r.remaining.SetText(appcore.HumanBytes(node.remaining))
	r.availability.SetText(contentAvailabilityLabel(node))
	r.indent.SetMinSize(fyne.NewSize(float32(row.depth)*theme.IconInlineSize(), 1))
	if node.isDir {
		if r.app.detailsState.Content.Expanded[node.path] {
			r.expander.SetText("v")
		} else {
			r.expander.SetText(">")
		}
		nodePath := node.path
		r.expander.OnTapped = func() {
			r.app.toggleContentNode(nodePath)
		}
		r.expander.Show()
	} else {
		r.expander.Hide()
	}
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

func contentCheckboxText(priority int) string {
	switch priority {
	case contentPriorityIgnored:
		return "[ ]"
	case contentPriorityMixed:
		return "[-]"
	default:
		return "[x]"
	}
}

func contentAvailabilityLabel(node *contentNode) string {
	if node == nil || !node.hasAvail || node.availability < 0 {
		return "?"
	}
	return fmt.Sprintf("%.1f%%", node.availability*100)
}
