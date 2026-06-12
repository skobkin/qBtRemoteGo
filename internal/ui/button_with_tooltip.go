package ui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

type buttonWithTooltip struct {
	widget.Button
	hoverTooltipOwner
	tooltip string
}

func newButtonWithTooltip(manager *hoverTooltipManager, icon fyne.Resource, tooltip string, tapped func()) *buttonWithTooltip {
	h := &buttonWithTooltip{
		hoverTooltipOwner: hoverTooltipOwner{manager: manager},
		tooltip:           strings.TrimSpace(tooltip),
	}
	h.Text = ""
	h.Icon = icon
	h.OnTapped = tapped
	h.ExtendBaseWidget(h)
	return h
}

func (h *buttonWithTooltip) SetTooltip(tooltip string) {
	h.hideTooltip(h)
	h.tooltip = strings.TrimSpace(tooltip)
}

func (h *buttonWithTooltip) MouseIn(ev *desktop.MouseEvent) {
	h.Button.MouseIn(ev)
	content := newTextTooltip(h.tooltip, tooltipMaxWidthFor(h, tooltipMaxWidthRatio))
	if content == nil {
		return
	}

	h.showTooltip(h, content)
}

func (h *buttonWithTooltip) MouseMoved(ev *desktop.MouseEvent) {
	h.Button.MouseMoved(ev)
}

func (h *buttonWithTooltip) MouseOut() {
	h.Button.MouseOut()
	h.scheduleHide(h)
}
