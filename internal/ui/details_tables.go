package ui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	appcore "github.com/skobkin/qbtremotego/internal/app"
)

type detailsTableView struct {
	root    *fyne.Container
	table   *widget.Table
	headers []string
	rows    [][]string
}

func newDetailsTableView(headers []string) *detailsTableView {
	v := &detailsTableView{
		root:    container.NewStack(),
		headers: headers,
	}
	v.table = widget.NewTable(
		func() (int, int) {
			return len(v.rows), len(v.headers)
		},
		func() fyne.CanvasObject {
			label := widget.NewLabel("")
			label.Wrapping = fyne.TextWrapOff
			return label
		},
		func(id widget.TableCellID, object fyne.CanvasObject) {
			if id.Row < 0 || id.Row >= len(v.rows) || id.Col < 0 || id.Col >= len(v.headers) {
				return
			}
			object.(*widget.Label).SetText(v.rows[id.Row][id.Col])
		},
	)
	v.table.ShowHeaderRow = true
	v.table.CreateHeader = func() fyne.CanvasObject {
		label := widget.NewLabel("")
		label.TextStyle = fyne.TextStyle{Bold: true}
		return label
	}
	v.table.UpdateHeader = func(id widget.TableCellID, object fyne.CanvasObject) {
		if id.Row == -1 && id.Col >= 0 && id.Col < len(v.headers) {
			object.(*widget.Label).SetText(v.headers[id.Col])
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
		table: newDetailsTableView([]string{"IP", "Port", "Connection", "Flags", "Client", "Progress", "Down", "Up", "Downloaded", "Uploaded"}),
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
		v.root.Objects = []fyne.CanvasObject{detailsStatusState("Failed to load peers:\n" + state.Error)}
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
		table: newDetailsTableView([]string{"Tier", "URL", "Status", "Peers", "Seeds", "Leeches", "Downloaded", "Message"}),
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
		v.root.Objects = []fyne.CanvasObject{detailsStatusState("Failed to load trackers:\n" + state.Error)}
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
		table: newDetailsTableView([]string{"URL"}),
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
		v.root.Objects = []fyne.CanvasObject{detailsStatusState("Failed to load HTTP sources:\n" + state.Error)}
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
