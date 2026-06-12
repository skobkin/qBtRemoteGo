package ui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

type hoverIcon struct {
	widget.BaseWidget
	hoverTooltipOwner
	icon    *widget.Icon
	tooltip string
}

func newHoverIcon(manager *hoverTooltipManager, res fyne.Resource, tooltip string) *hoverIcon {
	h := &hoverIcon{
		hoverTooltipOwner: hoverTooltipOwner{manager: manager},
		icon:              widget.NewIcon(res),
		tooltip:           strings.TrimSpace(tooltip),
	}
	h.ExtendBaseWidget(h)
	return h
}

func (h *hoverIcon) SetState(res fyne.Resource, tooltip string) {
	h.hideTooltip(h)
	h.icon.SetResource(res)
	h.tooltip = strings.TrimSpace(tooltip)
}

func (h *hoverIcon) MouseIn(*desktop.MouseEvent) {
	content := newTextTooltip(h.tooltip, tooltipMaxWidthFor(h, tooltipMaxWidthRatio))
	if content == nil {
		return
	}

	h.showTooltip(h, content)
}

func (h *hoverIcon) MouseMoved(*desktop.MouseEvent) {
}

func (h *hoverIcon) MouseOut() {
	h.scheduleHide(h)
}

func (h *hoverIcon) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(h.icon)
}
