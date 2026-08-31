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

	v.general.Refresh()
	v.content.Refresh()
	v.peers.Refresh()
	v.trackers.Refresh()
	v.webSeeds.Refresh()

	selected := v.tabItems[v.app.detailsState.ActiveTab]
	if selected == nil {
		selected = v.tabItems[detailsTabGeneral]
	}
	if v.tabs.Selected() != selected {
		v.tabs.Select(selected)
	}
	v.root.Objects = []fyne.CanvasObject{v.tabs}
	v.root.Refresh()
}

func (v *torrentDetailsView) emptyState() fyne.CanvasObject {
	text := "Select a single torrent to view details."
	if v.app.currentDetailsMode() == detailsPanelModeOverlayRight {
		text = "Double-click a torrent to open details."
	}
	label := widget.NewLabel(text)
	label.Wrapping = fyne.TextWrapWord
	return container.NewCenter(container.NewPadded(label))
}
