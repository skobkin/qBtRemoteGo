package ui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	appcore "github.com/skobkin/qbtremotego/internal/app"
)

// detailsColumnSpec fixes one details table column: header text and a width
// chosen so overflow ellipsizes inside the cell instead of collapsing columns.
type detailsColumnSpec struct {
	label string
	width float32
}

var (
	detailsPeerColumnSpecs = []detailsColumnSpec{
		{label: "IP", width: 130},
		{label: "Port", width: 70},
		{label: "Connection", width: 90},
		{label: "Flags", width: 70},
		{label: "Client", width: 190},
		{label: "Progress", width: 80},
		{label: "Down", width: 90},
		{label: "Up", width: 90},
		{label: "Downloaded", width: 105},
		{label: "Uploaded", width: 105},
	}

	detailsTrackerColumnSpecs = []detailsColumnSpec{
		{label: "Tier", width: 60},
		{label: "URL", width: 280},
		{label: "Status", width: 110},
		{label: "Peers", width: 70},
		{label: "Seeds", width: 70},
		{label: "Leeches", width: 70},
		{label: "Downloaded", width: 95},
		{label: "Message", width: 240},
	}

	detailsWebSeedColumnSpecs = []detailsColumnSpec{
		{label: "URL", width: 420},
	}
)

type detailsTableView struct {
	root  *fyne.Container
	table *widget.Table
	specs []detailsColumnSpec
	rows  [][]string
}

func newDetailsTableView(specs []detailsColumnSpec) *detailsTableView {
	v := &detailsTableView{
		root:  container.NewStack(),
		specs: specs,
	}
	v.table = widget.NewTable(
		func() (int, int) {
			return len(v.rows), len(v.specs)
		},
		func() fyne.CanvasObject {
			label := widget.NewLabel("")
			label.Wrapping = fyne.TextWrapOff
			label.Truncation = fyne.TextTruncateEllipsis
			return label
		},
		func(id widget.TableCellID, object fyne.CanvasObject) {
			if id.Row < 0 || id.Row >= len(v.rows) || id.Col < 0 || id.Col >= len(v.specs) {
				return
			}
			object.(*widget.Label).SetText(v.rows[id.Row][id.Col])
		},
	)
	for index, spec := range specs {
		v.table.SetColumnWidth(index, spec.width)
	}
	v.table.ShowHeaderRow = true
	v.table.CreateHeader = func() fyne.CanvasObject {
		label := widget.NewLabel("")
		label.TextStyle = fyne.TextStyle{Bold: true}
		label.Truncation = fyne.TextTruncateEllipsis
		return label
	}
	v.table.UpdateHeader = func(id widget.TableCellID, object fyne.CanvasObject) {
		if id.Row == -1 && id.Col >= 0 && id.Col < len(v.specs) {
			object.(*widget.Label).SetText(v.specs[id.Col].label)
		}
	}
	v.root.Objects = []fyne.CanvasObject{v.table}
	return v
}

func (v *detailsTableView) Root() fyne.CanvasObject {
	return v.root
}

func (v *detailsTableView) SetRows(rows [][]string) {
	v.rows = rows
	v.table.Refresh()
}

// detailsTableMessages carries the per-tab status copy of a details table view.
type detailsTableMessages struct {
	loading      string
	failedPrefix string
	idle         string
}

// detailsTableTabView is the shared tab view behind the Peers, Trackers and
// HTTP Sources tabs: one table plus one status/retry chrome, built once and
// updated in place by Refresh.
type detailsTableTabView struct {
	app    *application
	tab    detailsTabKey
	root   *fyne.Container
	table  *detailsTableView
	status *widget.Label
	retry  *widget.Button
	wrap   *fyne.Container
	msgs   detailsTableMessages
	rows   func(state *torrentDetailsState) [][]string
}

func newDetailsTableTabView(
	app *application,
	tab detailsTabKey,
	specs []detailsColumnSpec,
	msgs detailsTableMessages,
	rows func(state *torrentDetailsState) [][]string,
) *detailsTableTabView {
	v := &detailsTableTabView{
		app:   app,
		tab:   tab,
		root:  container.NewStack(),
		table: newDetailsTableView(specs),
		msgs:  msgs,
		rows:  rows,
	}
	v.status = widget.NewLabel("")
	v.status.Wrapping = fyne.TextWrapWord
	v.status.Truncation = fyne.TextTruncateEllipsis
	v.retry = widget.NewButton("Retry", func() { v.app.retryActiveDetailsLoad() })
	v.wrap = container.NewVBox(
		container.NewCenter(container.NewPadded(v.status)),
		container.NewCenter(v.retry),
	)
	v.root.Objects = []fyne.CanvasObject{v.table.Root(), v.wrap}
	return v
}

func (v *detailsTableTabView) Root() fyne.CanvasObject {
	return v.root
}

func (v *detailsTableTabView) Refresh() {
	state := v.app.detailsState
	dataset := state.activeDataset(v.tab)
	switch {
	case state == nil || dataset == nil || (dataset.Loading && !dataset.Loaded):
		v.status.SetText(v.msgs.loading)
		v.retry.Hide()
		v.wrap.Show()
	case strings.TrimSpace(dataset.Error) != "":
		v.status.SetText(v.msgs.failedPrefix + "\n" + dataset.Error)
		v.retry.Show()
		v.wrap.Show()
	case !dataset.Loaded:
		v.status.SetText(v.msgs.idle)
		v.retry.Hide()
		v.wrap.Show()
	default:
		v.table.SetRows(v.rows(state))
		v.wrap.Hide()
	}
	v.root.Refresh()
}

func newDetailsPeerTabView(app *application) *detailsTableTabView {
	return newDetailsTableTabView(app, detailsTabPeers, detailsPeerColumnSpecs,
		detailsTableMessages{
			loading:      "Loading peers...",
			failedPrefix: "Failed to load peers:",
			idle:         "Peers will load when this tab becomes active.",
		},
		peerRows)
}

func newDetailsTrackersTabView(app *application) *detailsTableTabView {
	return newDetailsTableTabView(app, detailsTabTrackers, detailsTrackerColumnSpecs,
		detailsTableMessages{
			loading:      "Loading trackers...",
			failedPrefix: "Failed to load trackers:",
			idle:         "Trackers will load when this tab becomes active.",
		},
		trackerRows)
}

func newDetailsWebSeedsTabView(app *application) *detailsTableTabView {
	return newDetailsTableTabView(app, detailsTabHTTPSources, detailsWebSeedColumnSpecs,
		detailsTableMessages{
			loading:      "Loading HTTP sources...",
			failedPrefix: "Failed to load HTTP sources:",
			idle:         "HTTP sources will load when this tab becomes active.",
		},
		webSeedRows)
}

func peerRows(state *torrentDetailsState) [][]string {
	rows := make([][]string, 0, len(state.Peers.Peers))
	for _, peer := range state.Peers.Peers {
		rows = append(rows, []string{
			peer.IP,
			fmt.Sprintf("%d", peer.Port),
			peer.Connection,
			peer.Flags,
			peer.Client,
			fmt.Sprintf("%.1f%%", peer.Progress*100),
			appcore.HumanSpeed(peer.DownloadSpeed),
			appcore.HumanSpeed(peer.UploadSpeed),
			appcore.HumanBytes(peer.TotalDownloaded),
			appcore.HumanBytes(peer.TotalUploaded),
		})
	}
	return rows
}

func trackerRows(state *torrentDetailsState) [][]string {
	rows := make([][]string, 0, len(state.Trackers.Trackers))
	for _, tracker := range state.Trackers.Trackers {
		rows = append(rows, []string{
			fmt.Sprintf("%d", tracker.Tier),
			tracker.URL,
			trackerStatusLabel(tracker.Status),
			fmt.Sprintf("%d", tracker.Peers),
			fmt.Sprintf("%d", tracker.Seeds),
			fmt.Sprintf("%d", tracker.Leeches),
			fmt.Sprintf("%d", tracker.Downloaded),
			tracker.Message,
		})
	}
	return rows
}

func webSeedRows(state *torrentDetailsState) [][]string {
	rows := make([][]string, 0, len(state.WebSeeds.WebSeeds))
	for _, webSeed := range state.WebSeeds.WebSeeds {
		rows = append(rows, []string{webSeed.URL})
	}
	return rows
}

func trackerStatusLabel(status int) string {
	switch status {
	case 0:
		return "Disabled"
	case 1:
		return "Not contacted"
	case 2:
		return "Working"
	case 3:
		return "Updating"
	case 4:
		return "Not working"
	default:
		return fmt.Sprintf("%d", status)
	}
}
