package ui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// detailsStatusMessages carries the per-tab status copy of a details view.
type detailsStatusMessages struct {
	loading      string
	failedPrefix string
	idle         string
}

// detailsStatusChrome is the shared loading/error/idle presentation of a
// details tab: one message label plus a Retry button, built once.
type detailsStatusChrome struct {
	label *widget.Label
	retry *widget.Button
	root  *fyne.Container
}

func newDetailsStatusChrome(app *application) *detailsStatusChrome {
	c := &detailsStatusChrome{
		label: widget.NewLabel(""),
	}
	c.label.Wrapping = fyne.TextWrapWord
	c.label.Truncation = fyne.TextTruncateEllipsis
	// The VBox below stretches the label to the full panel width, so the text
	// centers itself through alignment. Inside a Center container the label
	// would be sized to its minimum instead — and a wrapped label's minimum is
	// a one-character column whose height grows by one line per character, so
	// the message collapses and its inflated minimum can drag the bottom
	// details pane's split height up.
	c.label.Alignment = fyne.TextAlignCenter
	c.retry = widget.NewButton("Retry", func() { app.retryActiveDetailsLoad() })
	c.root = container.NewVBox(
		container.NewPadded(c.label),
		container.NewCenter(container.NewPadded(c.retry)),
	)

	return c
}

// present renders the dataset's state into root: the status chrome while the
// dataset is loading, failed or idle, the content once loaded. The chrome
// replaces the content in root instead of stacking over it — a translucent
// overlay would leave the previous dataset's rows visible and tappable
// underneath. Reports whether the chrome took over the root.
func (c *detailsStatusChrome) present(
	root *fyne.Container,
	content fyne.CanvasObject,
	msgs detailsStatusMessages,
	dataset *detailsDatasetState,
) bool {
	switch {
	case dataset == nil || (dataset.Loading && !dataset.Loaded):
		c.label.SetText(msgs.loading)
		c.retry.Hide()
	case strings.TrimSpace(dataset.Error) != "":
		c.label.SetText(msgs.failedPrefix + "\n" + dataset.Error)
		c.retry.Show()
	case !dataset.Loaded:
		c.label.SetText(msgs.idle)
		c.retry.Hide()
	default:
		root.Objects = []fyne.CanvasObject{content}

		return false
	}
	root.Objects = []fyne.CanvasObject{c.root}

	return true
}
