package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type torrentDetailsView struct {
	app      *application
	root     *fyne.Container
	tabs     *container.AppTabs
	general  *detailsGeneralTabView
	content  *detailsContentTabView
	peers    *detailsTableTabView
	trackers *detailsTableTabView
	webSeeds *detailsTableTabView
	tabItems map[detailsTabKey]*container.TabItem
	empty    fyne.CanvasObject
}

func newTorrentDetailsView(app *application) *torrentDetailsView {
	v := &torrentDetailsView{
		app:      app,
		root:     container.NewStack(),
		general:  newDetailsGeneralTabView(app),
		content:  newDetailsContentTabView(app),
		peers:    newDetailsPeerTabView(app),
		trackers: newDetailsTrackersTabView(app),
		webSeeds: newDetailsWebSeedsTabView(app),
		tabItems: map[detailsTabKey]*container.TabItem{},
	}

	v.tabItems[detailsTabGeneral] = container.NewTabItem("General", v.general.Root())
	v.tabItems[detailsTabContent] = container.NewTabItem("Content", v.content.Root())
	v.tabItems[detailsTabPeers] = container.NewTabItem("Peers", v.peers.Root())
	v.tabItems[detailsTabTrackers] = container.NewTabItem("Trackers", v.trackers.Root())
	v.tabItems[detailsTabHTTPSources] = container.NewTabItem("HTTP Sources", v.webSeeds.Root())

	v.tabs = container.NewAppTabs(
		v.tabItems[detailsTabGeneral],
		v.tabItems[detailsTabContent],
		v.tabItems[detailsTabPeers],
		v.tabItems[detailsTabTrackers],
		v.tabItems[detailsTabHTTPSources],
	)
	v.tabs.SetTabLocation(container.TabLocationTop)
	v.tabs.OnSelected = func(item *container.TabItem) {
		switch item {
		case v.tabItems[detailsTabGeneral]:
			v.app.setDetailsTab(detailsTabGeneral)
		case v.tabItems[detailsTabContent]:
			v.app.setDetailsTab(detailsTabContent)
		case v.tabItems[detailsTabPeers]:
			v.app.setDetailsTab(detailsTabPeers)
		case v.tabItems[detailsTabTrackers]:
			v.app.setDetailsTab(detailsTabTrackers)
		case v.tabItems[detailsTabHTTPSources]:
			v.app.setDetailsTab(detailsTabHTTPSources)
		}
	}
	v.root.Objects = []fyne.CanvasObject{v.emptyState()}
	return v
}

func (v *torrentDetailsView) Root() fyne.CanvasObject {
	return v.root
}

// Refresh syncs the presentation to the current details state. Only the
// active tab's view is updated: Fyne keeps hidden tab contents out of the
// canvas, so they are refreshed when selected again instead of on every tick.
func (v *torrentDetailsView) Refresh() {
	hash := ""
	if v.app.detailsState != nil {
		hash = v.app.detailsState.FocusedHash
	}
	if hash == "" {
		v.root.Objects = []fyne.CanvasObject{v.emptyState()}
		v.root.Refresh()
		return
	}

	tab := v.activeTab()
	v.refreshTab(tab)

	selected := v.tabItems[tab]
	if selected == nil {
		selected = v.tabItems[detailsTabGeneral]
	}
	if v.tabs.Selected() != selected {
		v.tabs.Select(selected)
	}
	v.root.Objects = []fyne.CanvasObject{v.tabs}
	v.root.Refresh()
}

// activeTab returns the tab key to present; unknown values fall back to
// General, mirroring setActiveTab's normalization.
func (v *torrentDetailsView) activeTab() detailsTabKey {
	if v.app.detailsState == nil {
		return detailsTabGeneral
	}
	return v.app.detailsState.ActiveTab
}

func (v *torrentDetailsView) refreshTab(tab detailsTabKey) {
	switch tab {
	case detailsTabGeneral:
		v.general.Refresh()
	case detailsTabContent:
		v.content.Refresh()
	case detailsTabPeers:
		v.peers.Refresh()
	case detailsTabTrackers:
		v.trackers.Refresh()
	case detailsTabHTTPSources:
		v.webSeeds.Refresh()
	}
}

// emptyState returns the no-selection placeholder, built once. The panel mode
// is its only input besides selection state, and the host rebuilds this view
// whenever the mode changes, so caching is safe.
func (v *torrentDetailsView) emptyState() fyne.CanvasObject {
	if v.empty == nil {
		text := "Select a single torrent to view details."
		if v.app.currentDetailsMode() == detailsPanelModeOverlayRight {
			text = "Double-click a torrent to open details."
		}
		// No Wrapping: NewCenter sizes the label to its minimum, and a wrapped
		// label's minimum width is a one-character column (with a height of one
		// line per character), which renders the text vertically. Both texts
		// are short constants, so the unwrapped minimum is always readable.
		label := widget.NewLabel(text)
		v.empty = container.NewCenter(container.NewPadded(label))
	}

	return v.empty
}
