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

type detailsPeerTabView struct {
	app   *application
	root  *fyne.Container
	table *detailsTableView
}

type detailsTrackersTabView struct {
	app   *application
	root  *fyne.Container
	table *detailsTableView
}

type detailsWebSeedsTabView struct {
	app   *application
	root  *fyne.Container
	table *detailsTableView
}

func newDetailsPeerTabView(app *application) *detailsPeerTabView {
	return &detailsPeerTabView{
		app:   app,
		root:  container.NewStack(),
		table: newDetailsTableView(detailsPeerColumnSpecs),
	}
}

func (v *detailsPeerTabView) Root() fyne.CanvasObject {
	return v.root
}

func (v *detailsPeerTabView) Refresh() {
	state := v.app.detailsState.Peers
	switch {
	case state.Loading && !state.Loaded:
		v.root.Objects = []fyne.CanvasObject{detailsStatusState("Loading peers...")}
	case strings.TrimSpace(state.Error) != "":
		v.root.Objects = []fyne.CanvasObject{detailsErrorState("Failed to load peers:\n"+state.Error, v.app.retryActiveDetailsLoad)}
	case !state.Loaded:
		v.root.Objects = []fyne.CanvasObject{detailsStatusState("Peers will load when this tab becomes active.")}
	default:
		rows := make([][]string, 0, len(state.Peers))
		for _, peer := range state.Peers {
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
		v.table.SetRows(rows)
		v.root.Objects = []fyne.CanvasObject{v.table.Root()}
	}
	v.root.Refresh()
}

func newDetailsTrackersTabView(app *application) *detailsTrackersTabView {
	return &detailsTrackersTabView{
		app:   app,
		root:  container.NewStack(),
		table: newDetailsTableView(detailsTrackerColumnSpecs),
	}
}

func (v *detailsTrackersTabView) Root() fyne.CanvasObject {
	return v.root
}

func (v *detailsTrackersTabView) Refresh() {
	state := v.app.detailsState.Trackers
	switch {
	case state.Loading && !state.Loaded:
		v.root.Objects = []fyne.CanvasObject{detailsStatusState("Loading trackers...")}
	case strings.TrimSpace(state.Error) != "":
		v.root.Objects = []fyne.CanvasObject{detailsErrorState("Failed to load trackers:\n"+state.Error, v.app.retryActiveDetailsLoad)}
	case !state.Loaded:
		v.root.Objects = []fyne.CanvasObject{detailsStatusState("Trackers will load when this tab becomes active.")}
	default:
		rows := make([][]string, 0, len(state.Trackers))
		for _, tracker := range state.Trackers {
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
		v.table.SetRows(rows)
		v.root.Objects = []fyne.CanvasObject{v.table.Root()}
	}
	v.root.Refresh()
}

func newDetailsWebSeedsTabView(app *application) *detailsWebSeedsTabView {
	return &detailsWebSeedsTabView{
		app:   app,
		root:  container.NewStack(),
		table: newDetailsTableView(detailsWebSeedColumnSpecs),
	}
}

func (v *detailsWebSeedsTabView) Root() fyne.CanvasObject {
	return v.root
}

func (v *detailsWebSeedsTabView) Refresh() {
	state := v.app.detailsState.WebSeeds
	switch {
	case state.Loading && !state.Loaded:
		v.root.Objects = []fyne.CanvasObject{detailsStatusState("Loading HTTP sources...")}
	case strings.TrimSpace(state.Error) != "":
		v.root.Objects = []fyne.CanvasObject{detailsErrorState("Failed to load HTTP sources:\n"+state.Error, v.app.retryActiveDetailsLoad)}
	case !state.Loaded:
		v.root.Objects = []fyne.CanvasObject{detailsStatusState("HTTP sources will load when this tab becomes active.")}
	default:
		rows := make([][]string, 0, len(state.WebSeeds))
		for _, webSeed := range state.WebSeeds {
			rows = append(rows, []string{webSeed.URL})
		}
		v.table.SetRows(rows)
		v.root.Objects = []fyne.CanvasObject{v.table.Root()}
	}
	v.root.Refresh()
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
