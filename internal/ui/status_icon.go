package ui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

type hoverIcon struct {
	widget.BaseWidget
	canvas  fyne.Canvas
	icon    *widget.Icon
	tooltip string
	popup   *widget.PopUp
}

func newHoverIcon(canvas fyne.Canvas, res fyne.Resource, tooltip string) *hoverIcon {
	h := &hoverIcon{
		canvas:  canvas,
		icon:    widget.NewIcon(res),
		tooltip: strings.TrimSpace(tooltip),
	}
	h.ExtendBaseWidget(h)
	return h
}

func (h *hoverIcon) SetState(res fyne.Resource, tooltip string) {
	h.hidePopup()
	h.icon.SetResource(res)
	h.tooltip = strings.TrimSpace(tooltip)
}

func (h *hoverIcon) MouseIn(*desktop.MouseEvent) {
	if strings.TrimSpace(h.tooltip) == "" {
		return
	}
	content := widget.NewLabel(h.tooltip)
	content.Wrapping = fyne.TextWrapWord
	popupContent := container.NewPadded(content)
	h.popup = widget.NewPopUp(popupContent, h.canvas)
	size := popupContent.MinSize()
	if size.Width < 180 {
		size.Width = 180
	}
	h.popup.Resize(size)
	h.popup.ShowAtRelativePosition(fyne.NewPos(0, h.Size().Height), h)
}

func (h *hoverIcon) MouseMoved(*desktop.MouseEvent) {
}

func (h *hoverIcon) MouseOut() {
	h.hidePopup()
}

func (h *hoverIcon) hidePopup() {
	if h.popup == nil {
		return
	}
	h.popup.Hide()
	h.popup = nil
}

func (h *hoverIcon) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(h.icon)
}
