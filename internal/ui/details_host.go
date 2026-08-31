package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type torrentDetailsHost struct {
	app    *application
	root   *fyne.Container
	table  fyne.CanvasObject
	view   *torrentDetailsView
	split  *container.Split
	panel  *detailsPanelChrome
	drawer *detailsDrawer
	mode   detailsPanelMode
}

func newTorrentDetailsHost(app *application, table fyne.CanvasObject) *torrentDetailsHost {
	h := &torrentDetailsHost{
		app:   app,
		root:  container.NewStack(),
		table: table,
	}
	h.Refresh()
	return h
}

func (h *torrentDetailsHost) Root() fyne.CanvasObject {
	return h.root
}

func (h *torrentDetailsHost) Refresh() {
	if h == nil {
		return
	}
	mode := h.app.currentDetailsMode()
	if h.view == nil || h.mode != mode {
		h.view = newTorrentDetailsView(h.app)
		h.panel = newDetailsPanelChrome(h.view.Root())
		h.drawer = newDetailsDrawer(h.app, h.panel)
		h.split = container.NewVSplit(h.table, h.panel)
		h.split.SetOffset(0.62)
		h.mode = mode
	}

	h.view.Refresh()
	switch mode {
	case detailsPanelModeBottomPane:
		h.root.Objects = []fyne.CanvasObject{h.split}
		h.panel.Show()
	case detailsPanelModeOverlayRight:
		h.root.Objects = []fyne.CanvasObject{container.NewStack(h.table, h.drawer)}
		h.panel.Show()
	default:
		h.root.Objects = []fyne.CanvasObject{h.table}
	}

	h.drawer.SetVisible(h.app.detailsState != nil && h.app.detailsState.Visible && mode == detailsPanelModeOverlayRight)
	h.root.Refresh()
}

// detailsPanelChrome is the drawer/pane panel surface: background, border and
// the tab content. As a Tappable it consumes taps landing on its own chrome so
// they never reach the drawer backdrop underneath (which closes the drawer);
// interactive children still receive their events first.
type detailsPanelChrome struct {
	widget.BaseWidget
	root *fyne.Container
}

func newDetailsPanelChrome(content fyne.CanvasObject) *detailsPanelChrome {
	bg := canvas.NewRectangle(theme.Color(theme.ColorNameInputBackground))
	border := canvas.NewRectangle(color.Transparent)
	border.StrokeColor = theme.Color(theme.ColorNameSeparator)
	border.StrokeWidth = 1
	panel := &detailsPanelChrome{
		root: container.NewStack(bg, border, content),
	}
	panel.ExtendBaseWidget(panel)
	return panel
}

func (p *detailsPanelChrome) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(p.root)
}

func (p *detailsPanelChrome) Tapped(*fyne.PointEvent) {}

type detailsDrawer struct {
	widget.BaseWidget
	app      *application
	backdrop *drawerBackdrop
	panel    fyne.CanvasObject
	visible  bool
}

func newDetailsDrawer(app *application, panel fyne.CanvasObject) *detailsDrawer {
	d := &detailsDrawer{
		app:      app,
		backdrop: newDrawerBackdrop(func() { app.closeTorrentDetails() }),
		panel:    panel,
	}
	d.ExtendBaseWidget(d)
	return d
}

func (d *detailsDrawer) SetVisible(visible bool) {
	d.visible = visible
	if visible {
		d.Show()
	} else {
		d.Hide()
	}
	d.Refresh()
}

func (d *detailsDrawer) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(container.New(&detailsDrawerLayout{}, d.backdrop, d.panel))
}

func (d *detailsDrawer) MinSize() fyne.Size {
	return fyne.Size{}
}

type detailsDrawerLayout struct{}

func (l *detailsDrawerLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) != 2 {
		return
	}
	backdrop := objects[0]
	panel := objects[1]
	backdrop.Move(fyne.NewPos(0, 0))
	backdrop.Resize(size)

	width := size.Width * 0.4
	if width < 360 {
		width = min(size.Width, 360)
	}
	if width > 720 {
		width = 720
	}
	panel.Move(fyne.NewPos(size.Width-width, 0))
	panel.Resize(fyne.NewSize(width, size.Height))
}

func (l *detailsDrawerLayout) MinSize(_ []fyne.CanvasObject) fyne.Size {
	return fyne.Size{}
}

type drawerBackdrop struct {
	widget.BaseWidget
	onTap func()
}

func newDrawerBackdrop(onTap func()) *drawerBackdrop {
	b := &drawerBackdrop{onTap: onTap}
	b.ExtendBaseWidget(b)
	return b
}

func (b *drawerBackdrop) Tapped(*fyne.PointEvent) {
	if b.onTap != nil {
		b.onTap()
	}
}

func (b *drawerBackdrop) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(color.Transparent)
	return widget.NewSimpleRenderer(bg)
}
