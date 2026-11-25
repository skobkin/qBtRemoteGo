package ui

import (
	"fmt"
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	appcore "github.com/skobkin/qbtremotego/internal/app"
)

// Progress fill colours for states that should not read as "active download".
// They match the status chip palette so a row's bar and chip agree.
var (
	progressCompleteColor = color.NRGBA{R: 0x1e, G: 0x7e, B: 0x3e, A: 0xff}
	progressStoppedColor  = color.NRGBA{R: 0x64, G: 0x6d, B: 0x78, A: 0xff}
	progressErrorColor    = color.NRGBA{R: 0xc9, G: 0x2a, B: 0x2a, A: 0xff}
)

// tableSpeedText renders a transfer speed for a table cell, leaving idle
// (zero) speeds empty so active torrents stand out.
func tableSpeedText(speed int64) string {
	if speed <= 0 {
		return ""
	}
	return appcore.HumanSpeed(speed)
}

// tableETAText renders an ETA for a table cell, leaving the "infinite"
// sentinel empty: qBittorrent reports it for every idle or seeding torrent.
func tableETAText(seconds int64) string {
	eta := appcore.HumanETA(seconds)
	if eta == "∞" {
		return ""
	}
	return eta
}

// progressFillColor picks the progress bar colour for a torrent: errors red,
// stopped torrents grey, finished ones green, everything else the theme
// primary colour.
func progressFillColor(state string, progress float64) color.Color {
	switch appcore.StatusLabel(state) {
	case "Error", "Missing files":
		return progressErrorColor
	case "Stopped":
		return progressStoppedColor
	}
	if progress >= 1 {
		return progressCompleteColor
	}
	return theme.Color(theme.ColorNamePrimary)
}

// stateProgressBar is a lightweight progress bar whose fill colour is set per
// row. widget.ProgressBar always paints the theme primary colour, and a theme
// override per row would be far heavier for a list of hundreds of rows.
type stateProgressBar struct {
	widget.BaseWidget
	track *canvas.Rectangle
	fill  *canvas.Rectangle
	label *canvas.Text
	value float64
}

func newStateProgressBar() *stateProgressBar {
	b := &stateProgressBar{
		track: canvas.NewRectangle(theme.Color(theme.ColorNameInputBackground)),
		fill:  canvas.NewRectangle(theme.Color(theme.ColorNamePrimary)),
		label: canvas.NewText("0%", theme.Color(theme.ColorNameForeground)),
	}
	radius := theme.InputRadiusSize()
	b.track.CornerRadius = radius
	b.fill.CornerRadius = radius
	b.label.Alignment = fyne.TextAlignCenter
	b.label.TextSize = theme.TextSize()
	b.ExtendBaseWidget(b)
	return b
}

// SetProgress updates the value (clamped to 0..1) and the fill colour.
func (b *stateProgressBar) SetProgress(value float64, fill color.Color) {
	if math.IsNaN(value) {
		value = 0
	}
	b.value = math.Max(0, math.Min(1, value))
	b.fill.FillColor = fill
	b.label.Text = fmt.Sprintf("%d%%", int(math.Floor(b.value*100)))
	// Over a mostly filled bar the label sits on the dark fill colour; over a
	// mostly empty one it sits on the track, so it follows the theme.
	if b.value >= 0.5 {
		b.label.Color = color.White
	} else {
		b.label.Color = theme.Color(theme.ColorNameForeground)
	}
	b.Refresh()
}

func (b *stateProgressBar) CreateRenderer() fyne.WidgetRenderer {
	return &stateProgressBarRenderer{bar: b}
}

type stateProgressBarRenderer struct {
	bar *stateProgressBar
}

func (r *stateProgressBarRenderer) Layout(size fyne.Size) {
	b := r.bar
	b.track.Resize(size)
	b.track.Move(fyne.NewPos(0, 0))
	b.fill.Resize(fyne.NewSize(size.Width*float32(b.value), size.Height))
	b.fill.Move(fyne.NewPos(0, 0))
	labelSize := b.label.MinSize()
	b.label.Resize(fyne.NewSize(size.Width, labelSize.Height))
	b.label.Move(fyne.NewPos(0, (size.Height-labelSize.Height)/2))
}

func (r *stateProgressBarRenderer) MinSize() fyne.Size {
	labelSize := r.bar.label.MinSize()
	pad := theme.InnerPadding()
	return fyne.NewSize(labelSize.Width+pad*2, labelSize.Height+pad)
}

func (r *stateProgressBarRenderer) Refresh() {
	b := r.bar
	b.track.FillColor = theme.Color(theme.ColorNameInputBackground)
	r.Layout(b.Size())
	b.track.Refresh()
	b.fill.Refresh()
	b.label.Refresh()
}

func (r *stateProgressBarRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.bar.track, r.bar.fill, r.bar.label}
}

func (r *stateProgressBarRenderer) Destroy() {}
