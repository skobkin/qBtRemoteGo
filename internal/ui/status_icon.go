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
	// onTapped makes the icon interactive when set (pointer cursor, taps).
	onTapped func()
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

// SetOnTapped makes the icon clickable; nil makes it purely informational.
func (h *hoverIcon) SetOnTapped(tapped func()) {
	h.onTapped = tapped
}

func (h *hoverIcon) Tapped(*fyne.PointEvent) {
	if h.onTapped != nil {
		h.onTapped()
	}
}

func (h *hoverIcon) Cursor() desktop.Cursor {
	if h.onTapped != nil {
		return desktop.PointerCursor
	}
	return desktop.DefaultCursor
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
