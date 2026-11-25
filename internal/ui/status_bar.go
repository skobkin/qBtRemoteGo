package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	appcore "github.com/skobkin/qbtremotego/internal/app"
)

// statusBarGap is the spacing between status bar items. Fyne's default
// container padding would spread the short segments across the bar.
const statusBarGap = float32(4)

// statusBarModel is the text of every status bar segment. Empty optional
// segments are hidden.
type statusBarModel struct {
	Count     string
	Selection string
	Down      string
	Up        string
	Free      string
	Notice    string
	Error     string
}

// statusModel derives the status bar content from the application state.
func (a *application) statusModel() statusBarModel {
	model := statusBarModel{
		Count: torrentCountText(len(a.visibleTorrents), len(a.allTorrents)),
		Down:  speedWithLimitText(a.transfer.DownloadSpeed, a.transfer.DownloadLimit),
		Up:    speedWithLimitText(a.transfer.UploadSpeed, a.transfer.UploadLimit),
		// The keychain hint and the error are independent: the hint describes
		// the background credential retry, the error the last failed request.
		Notice: a.credentialWait,
		Error:  a.lastError,
	}
	if selected, size := a.selectionSummary(); selected > 0 {
		model.Selection = fmt.Sprintf("%d selected · %s", selected, appcore.HumanBytes(size))
	}
	if a.serverStateKnown {
		model.Free = appcore.HumanBytes(a.serverState.FreeSpaceOnDisk) + " free"
	}
	return model
}

func torrentCountText(visible int, total int) string {
	if visible != total {
		return fmt.Sprintf("%d of %d shown", visible, total)
	}
	if total == 1 {
		return "1 torrent"
	}
	return fmt.Sprintf("%d torrents", total)
}

// speedWithLimitText renders a speed, appending the limit only when one is set.
func speedWithLimitText(speed int64, limit int64) string {
	text := appcore.HumanSpeed(speed)
	if limit > 0 {
		text += " / " + appcore.HumanSpeed(limit)
	}
	return text
}

// selectionSummary counts the selected torrents and their total size.
func (a *application) selectionSummary() (int, int64) {
	if len(a.selection) == 0 {
		return 0, 0
	}
	count := 0
	var size int64
	for _, torrent := range a.allTorrents {
		if a.selection[torrent.Hash] {
			count++
			size += torrent.Size
		}
	}
	return count, size
}

// statusBar is the structured bottom bar: separate, tightly spaced segments
// instead of one long text label.
type statusBar struct {
	root           fyne.CanvasObject
	count          *widget.Label
	selection      *widget.Label
	down           *widget.Label
	up             *widget.Label
	free           *widget.Label
	notice         *widget.Label
	errorIcon      *hoverIcon
	selectionGroup *fyne.Container
	freeGroup      *fyne.Container
	noticeGroup    *fyne.Container
	errorGroup     *fyne.Container
}

func newStatusBar(manager *hoverTooltipManager) *statusBar {
	b := &statusBar{
		count:     widget.NewLabel(""),
		selection: widget.NewLabel(""),
		down:      widget.NewLabel(""),
		up:        widget.NewLabel(""),
		free:      widget.NewLabel(""),
		notice:    widget.NewLabel(""),
		errorIcon: newHoverIcon(manager, theme.WarningIcon(), ""),
	}
	b.notice.Truncation = fyne.TextTruncateEllipsis

	// Each optional group carries its leading separator so hiding the group
	// also hides the rule in front of it.
	b.selectionGroup = statusGroup(b.selection)
	b.freeGroup = statusGroup(b.free)
	b.noticeGroup = statusGroup(b.notice)
	b.errorGroup = statusGroup(b.errorIcon, widget.NewLabel("Error"))
	transfer := statusGroup(
		widget.NewIcon(theme.DownloadIcon()), b.down,
		widget.NewIcon(theme.UploadIcon()), b.up,
	)

	b.root = container.New(layout.NewCustomPaddedHBoxLayout(statusBarGap),
		b.count,
		b.selectionGroup,
		transfer,
		b.freeGroup,
		b.noticeGroup,
		b.errorGroup,
	)
	return b
}

// statusGroup lays out one optional segment behind its leading separator.
func statusGroup(objects ...fyne.CanvasObject) *fyne.Container {
	objects = append([]fyne.CanvasObject{newStatusSeparator()}, objects...)
	return container.New(layout.NewCustomPaddedHBoxLayout(statusBarGap), objects...)
}

func newStatusSeparator() fyne.CanvasObject {
	line := canvas.NewRectangle(theme.Color(theme.ColorNameSeparator))
	line.SetMinSize(fyne.NewSize(1, theme.TextSize()))
	return container.NewCenter(line)
}

// update applies a model, hiding empty optional segments.
func (b *statusBar) update(model statusBarModel) {
	setLabel(b.count, model.Count)
	setLabel(b.down, model.Down)
	setLabel(b.up, model.Up)
	setOptional(b.selectionGroup, b.selection, model.Selection)
	setOptional(b.freeGroup, b.free, model.Free)
	setOptional(b.noticeGroup, b.notice, model.Notice)
	if model.Error == "" {
		b.errorGroup.Hide()
	} else {
		if model.Error != b.errorIcon.tooltip {
			b.errorIcon.SetState(theme.WarningIcon(), model.Error)
		}
		b.errorGroup.Show()
	}
}

func setLabel(label *widget.Label, text string) {
	if label.Text != text {
		label.SetText(text)
	}
}

func setOptional(group *fyne.Container, label *widget.Label, text string) {
	if text == "" {
		group.Hide()
		return
	}
	setLabel(label, text)
	group.Show()
}
