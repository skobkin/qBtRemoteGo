package ui

import (
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	appcore "github.com/skobkin/qbtremotego/internal/app"
	"github.com/skobkin/qbtremotego/internal/qbt"
)

type detailsGeneralTabView struct {
	app         *application
	root        *fyne.Container
	content     *fyne.Container
	scroll      *container.Scroll
	progressRow *fyne.Container
	progress    *widget.ProgressBar
	progressPct *widget.Label
	transfer    *detailsGeneralSection
	info        *detailsGeneralSection
	status      *widget.Label
	retry       *widget.Button
	statusWrap  *fyne.Container
}

func newDetailsGeneralTabView(app *application) *detailsGeneralTabView {
	v := &detailsGeneralTabView{
		app:  app,
		root: container.NewStack(),
	}
	v.progress = widget.NewProgressBar()
	v.progressPct = widget.NewLabel("")
	v.progressRow = container.NewBorder(nil, nil, widget.NewLabel("Progress"), v.progressPct, v.progress)
	v.transfer = newDetailsGeneralSection("Transfer", detailsTransferSpecs)
	v.info = newDetailsGeneralSection("Information", detailsInfoSpecs)
	// The scroll is built once so its offset survives poll ticks; the content
	// below is only updated in place.
	v.scroll = container.NewVScroll(container.NewPadded(container.NewVBox(
		v.progressRow,
		v.transfer.card,
		v.info.card,
		layout.NewSpacer(),
	)))
	v.content = container.NewStack(v.scroll)
	v.status = widget.NewLabel("")
	v.status.Wrapping = fyne.TextWrapWord
	v.status.Truncation = fyne.TextTruncateEllipsis
	v.retry = widget.NewButton("Retry", func() { v.app.retryActiveDetailsLoad() })
	v.statusWrap = container.NewVBox(
		container.NewCenter(container.NewPadded(v.status)),
		container.NewCenter(v.retry),
	)
	v.root.Objects = []fyne.CanvasObject{v.content, v.statusWrap}
	return v
}

func (v *detailsGeneralTabView) Root() fyne.CanvasObject {
	return v.root
}

func (v *detailsGeneralTabView) Refresh() {
	state := v.app.detailsState
	dataset := state.activeDataset(detailsTabGeneral)
	switch {
	case state == nil || dataset == nil || (dataset.Loading && !dataset.Loaded):
		v.status.SetText("Loading details...")
		v.retry.Hide()
		v.statusWrap.Show()
	case strings.TrimSpace(dataset.Error) != "":
		v.status.SetText("Failed to load details:\n" + dataset.Error)
		v.retry.Show()
		v.statusWrap.Show()
	case !dataset.Loaded:
		v.status.SetText("Details will load when this tab becomes active.")
		v.retry.Hide()
		v.statusWrap.Show()
	default:
		data := state.General.Data
		v.progress.SetValue(data.Progress)
		v.progressPct.SetText(fmt.Sprintf("%.1f%%", data.Progress*100))
		v.transfer.update(data)
		v.info.update(data)
		v.statusWrap.Hide()
	}
	v.root.Refresh()
}

// detailsFieldSpec describes one General tab row: a fixed label and how to
// format its value from the torrent properties.
type detailsFieldSpec struct {
	label  string
	format func(qbt.TorrentProperties) string
}

// detailsGeneralSection is a build-once card of label/value rows laid out on a
// shared two-column grid so labels align within the section.
type detailsGeneralSection struct {
	card   *widget.Card
	specs  []detailsFieldSpec
	values []*widget.Label
}

func newDetailsGeneralSection(title string, specs []detailsFieldSpec) *detailsGeneralSection {
	section := &detailsGeneralSection{specs: specs}
	grid := container.New(layout.NewFormLayout())
	for _, spec := range specs {
		grid.Add(widget.NewLabel(spec.label))
		value := widget.NewLabel("")
		value.Wrapping = fyne.TextWrapWord
		section.values = append(section.values, value)
		grid.Add(value)
	}
	section.card = widget.NewCard("", title, grid)
	return section
}

func (s *detailsGeneralSection) update(data qbt.TorrentProperties) {
	for index, spec := range s.specs {
		s.values[index].SetText(spec.format(data))
	}
}

var detailsTransferSpecs = []detailsFieldSpec{
	{label: "Time Active", format: func(d qbt.TorrentProperties) string { return appcore.HumanDuration(d.TimeElapsed) }},
	{label: "ETA", format: func(d qbt.TorrentProperties) string { return detailsETA(d.ETASeconds) }},
	{label: "Connections", format: func(d qbt.TorrentProperties) string { return detailsCountLimit(d.Connections, d.ConnectionLimit) }},
	{label: "Downloaded", format: func(d qbt.TorrentProperties) string {
		return detailsSessionBytes(d.TotalDownloaded, d.SessionDownloaded)
	}},
	{label: "Uploaded", format: func(d qbt.TorrentProperties) string { return detailsSessionBytes(d.TotalUploaded, d.SessionUploaded) }},
	{label: "Seeds", format: func(d qbt.TorrentProperties) string { return fmt.Sprintf("%d (%d total)", d.Seeds, d.SeedsTotal) }},
	{label: "Download Speed", format: func(d qbt.TorrentProperties) string {
		return detailsSpeedWithAverage(d.DownloadSpeed, d.AverageDownloadSpeed)
	}},
	{label: "Upload Speed", format: func(d qbt.TorrentProperties) string {
		return detailsSpeedWithAverage(d.UploadSpeed, d.AverageUploadSpeed)
	}},
	{label: "Peers", format: func(d qbt.TorrentProperties) string { return fmt.Sprintf("%d (%d total)", d.Peers, d.PeersTotal) }},
	{label: "Download Limit", format: func(d qbt.TorrentProperties) string { return appcore.HumanSpeedLimit(d.DownloadLimit) }},
	{label: "Upload Limit", format: func(d qbt.TorrentProperties) string { return appcore.HumanSpeedLimit(d.UploadLimit) }},
	{label: "Wasted", format: func(d qbt.TorrentProperties) string { return appcore.HumanBytes(d.TotalWasted) }},
	{label: "Share Ratio", format: func(d qbt.TorrentProperties) string { return detailsRatio(d.ShareRatio) }},
	{label: "Reannounce In", format: func(d qbt.TorrentProperties) string { return detailsETA(d.ReannounceSeconds) }},
	{label: "Last Seen Complete", format: func(d qbt.TorrentProperties) string { return detailsUnix(d.LastSeenCompleteUnix, "Never") }},
	{label: "Popularity", format: func(d qbt.TorrentProperties) string { return detailsRatio(d.Popularity) }},
	{label: "Availability", format: func(d qbt.TorrentProperties) string { return detailsAvailability(d.Availability) }},
}

var detailsInfoSpecs = []detailsFieldSpec{
	{label: "Total Size", format: func(d qbt.TorrentProperties) string { return appcore.HumanBytes(d.TotalSize) }},
	{label: "Pieces", format: func(d qbt.TorrentProperties) string {
		return fmt.Sprintf("%d x %s (have %d)", d.PiecesNum, appcore.HumanBytes(d.PieceSize), d.PiecesHave)
	}},
	{label: "Created By", format: func(d qbt.TorrentProperties) string { return detailsTextOrDash(d.CreatedBy) }},
	{label: "Added On", format: func(d qbt.TorrentProperties) string { return detailsUnix(d.AdditionDateUnix, "") }},
	{label: "Completed On", format: func(d qbt.TorrentProperties) string { return detailsUnix(d.CompletionDateUnix, "Never") }},
	{label: "Created On", format: func(d qbt.TorrentProperties) string { return detailsUnix(d.CreationDateUnix, "") }},
	{label: "Private", format: func(d qbt.TorrentProperties) string { return detailsPrivateFlag(d.Private) }},
	{label: "Info Hash v1", format: func(d qbt.TorrentProperties) string { return detailsTextOrDash(d.InfoHashV1) }},
	{label: "Info Hash v2", format: func(d qbt.TorrentProperties) string { return detailsTextOrDash(d.InfoHashV2) }},
	{label: "Save Path", format: func(d qbt.TorrentProperties) string { return detailsTextOrDash(d.SavePath) }},
	{label: "Comment", format: func(d qbt.TorrentProperties) string { return detailsTextOrDash(d.Comment) }},
}

func detailsETA(seconds int64) string {
	if seconds < 0 {
		return "∞"
	}
	if seconds == 0 {
		return ""
	}
	return appcore.HumanETA(seconds)
}

func detailsSessionBytes(total int64, session int64) string {
	return fmt.Sprintf("%s (%s this session)", appcore.HumanBytes(total), appcore.HumanBytes(session))
}

func detailsSpeedWithAverage(current int64, avg int64) string {
	avgText := "?"
	if avg >= 0 {
		avgText = appcore.HumanSpeed(avg)
	}
	return fmt.Sprintf("%s (%s avg.)", appcore.HumanSpeed(current), avgText)
}

// detailsCountLimit renders a live count with its configured maximum; a
// non-positive maximum means unlimited, matching the speed-limit sentinel.
func detailsCountLimit(count int, limit int) string {
	maxText := "∞"
	if limit > 0 {
		maxText = fmt.Sprintf("%d", limit)
	}
	return fmt.Sprintf("%d (%s max)", count, maxText)
}

func detailsRatio(value float64) string {
	if value < 0 {
		return "∞"
	}
	return fmt.Sprintf("%.2f", value)
}

// detailsAvailability renders the availability ratio as a percentage, matching
// the content tree's Availability column.
func detailsAvailability(value float64) string {
	if value < 0 {
		return "?"
	}
	return fmt.Sprintf("%.1f%%", value*100)
}

func detailsUnix(unix int64, fallback string) string {
	if unix <= 0 {
		return fallback
	}
	return time.Unix(unix, 0).Local().Format("1/2/2006, 3:04:05 PM")
}

func detailsTextOrDash(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return "-"
	}
	return text
}

func detailsPrivateFlag(value *bool) string {
	if value == nil {
		return "-"
	}
	if *value {
		return "Yes"
	}
	return "No"
}
