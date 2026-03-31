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
)

type detailsGeneralTabView struct {
	app  *application
	root *fyne.Container
}

func newDetailsGeneralTabView(app *application) *detailsGeneralTabView {
	return &detailsGeneralTabView{
		app:  app,
		root: container.NewStack(),
	}
}

func (v *detailsGeneralTabView) Root() fyne.CanvasObject {
	return v.root
}

func (v *detailsGeneralTabView) Refresh() {
	state := v.app.detailsState.General
	switch {
	case state.Loading && !state.Loaded:
		v.root.Objects = []fyne.CanvasObject{detailsStatusState("Loading details...")}
	case strings.TrimSpace(state.Error) != "":
		v.root.Objects = []fyne.CanvasObject{detailsStatusState("Failed to load details:\n" + state.Error)}
	case !state.Loaded:
		v.root.Objects = []fyne.CanvasObject{detailsStatusState("Details will load when this tab becomes active.")}
	default:
		v.root.Objects = []fyne.CanvasObject{container.NewVScroll(v.buildContent())}
	}
	v.root.Refresh()
}

func (v *detailsGeneralTabView) buildContent() fyne.CanvasObject {
	data := v.app.detailsState.General.Data
	progress := widget.NewProgressBar()
	progress.SetValue(data.Progress)
	progress.Resize(fyne.NewSize(0, 10))

	progressRow := container.NewBorder(
		nil,
		nil,
		widget.NewLabel("Progress"),
		widget.NewLabel(fmt.Sprintf("%.1f%%", data.Progress*100)),
		progress,
	)

	transfer := detailsSection("Transfer", []detailsField{
		{Label: "Time Active", Value: detailsDuration(data.TimeElapsed)},
		{Label: "ETA", Value: detailsETA(data.ETASeconds)},
		{Label: "Connections", Value: fmt.Sprintf("%d (%d max)", data.Connections, data.ConnectionLimit)},
		{Label: "Downloaded", Value: detailsSessionBytes(data.TotalDownloaded, data.SessionDownloaded)},
		{Label: "Uploaded", Value: detailsSessionBytes(data.TotalUploaded, data.SessionUploaded)},
		{Label: "Seeds", Value: fmt.Sprintf("%d (%d total)", data.Seeds, data.SeedsTotal)},
		{Label: "Download Speed", Value: detailsSpeedWithAverage(data.DownloadSpeed, data.AverageDownloadSpeed)},
		{Label: "Upload Speed", Value: detailsSpeedWithAverage(data.UploadSpeed, data.AverageUploadSpeed)},
		{Label: "Peers", Value: fmt.Sprintf("%d (%d total)", data.Peers, data.PeersTotal)},
		{Label: "Download Limit", Value: detailsLimit(data.DownloadLimit)},
		{Label: "Upload Limit", Value: detailsLimit(data.UploadLimit)},
		{Label: "Wasted", Value: appcore.HumanBytes(data.TotalWasted)},
		{Label: "Share Ratio", Value: detailsRatio(data.ShareRatio)},
		{Label: "Reannounce In", Value: detailsETA(data.ReannounceSeconds)},
		{Label: "Last Seen Complete", Value: detailsUnix(data.LastSeenCompleteUnix, "Never")},
		{Label: "Popularity", Value: detailsRatio(data.Popularity)},
		{Label: "Availability", Value: detailsAvailability(data.Availability)},
	})

	info := detailsSection("Information", []detailsField{
		{Label: "Total Size", Value: appcore.HumanBytes(data.TotalSize)},
		{Label: "Pieces", Value: fmt.Sprintf("%d x %s (have %d)", data.PiecesNum, appcore.HumanBytes(data.PieceSize), data.PiecesHave)},
		{Label: "Created By", Value: detailsTextOrDash(data.CreatedBy)},
		{Label: "Added On", Value: detailsUnix(data.AdditionDateUnix, "")},
		{Label: "Completed On", Value: detailsUnix(data.CompletionDateUnix, "")},
		{Label: "Created On", Value: detailsUnix(data.CreationDateUnix, "")},
		{Label: "Private", Value: detailsPrivateFlag(data.Private)},
		{Label: "Info Hash v1", Value: detailsTextOrDash(data.InfoHashV1)},
		{Label: "Info Hash v2", Value: detailsTextOrDash(data.InfoHashV2)},
		{Label: "Save Path", Value: detailsTextOrDash(data.SavePath)},
		{Label: "Comment", Value: detailsTextOrDash(data.Comment)},
	})

	return container.NewPadded(container.NewVBox(
		progressRow,
		transfer,
		info,
		layout.NewSpacer(),
	))
}

type detailsField struct {
	Label string
	Value string
}

func detailsSection(title string, fields []detailsField) fyne.CanvasObject {
	titleLabel := widget.NewLabel(title)
	titleLabel.TextStyle = fyne.TextStyle{Bold: true}
	rows := make([]fyne.CanvasObject, 0, len(fields)+1)
	rows = append(rows, titleLabel)
	for _, field := range fields {
		value := widget.NewLabel(field.Value)
		value.Wrapping = fyne.TextWrapWord
		rows = append(rows, container.NewBorder(nil, nil, widget.NewLabel(field.Label), nil, value))
	}
	return widget.NewCard("", "", container.NewVBox(rows...))
}

func detailsStatusState(text string) fyne.CanvasObject {
	label := widget.NewLabel(text)
	label.Wrapping = fyne.TextWrapWord
	return container.NewCenter(container.NewPadded(label))
}

func detailsDuration(seconds int64) string {
	if seconds < 0 {
		return "∞"
	}
	return appcore.HumanETA(seconds)
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

func detailsLimit(value int64) string {
	if value < 0 {
		return "∞"
	}
	return appcore.HumanSpeed(value)
}

func detailsRatio(value float64) string {
	if value < 0 {
		return "∞"
	}
	return fmt.Sprintf("%.2f", value)
}

func detailsAvailability(value float64) string {
	if value < 0 {
		return "?"
	}
	return fmt.Sprintf("%.2f", value)
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
