package ui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

type buttonWithTooltip struct {
	widget.Button
	canvas  fyne.Canvas
	tooltip string
	popup   *widget.PopUp
}

func newButtonWithTooltip(canvas fyne.Canvas, icon fyne.Resource, tooltip string, tapped func()) *buttonWithTooltip {
	h := &buttonWithTooltip{
		canvas:  canvas,
		tooltip: strings.TrimSpace(tooltip),
	}
	h.Text = ""
	h.Icon = icon
	h.OnTapped = tapped
	h.ExtendBaseWidget(h)
	return h
}

func (h *buttonWithTooltip) SetTooltip(tooltip string) {
	h.hidePopup()
	h.tooltip = strings.TrimSpace(tooltip)
}

func (h *buttonWithTooltip) MouseIn(ev *desktop.MouseEvent) {
	h.Button.MouseIn(ev)
	if h.tooltip == "" {
		return
	}

	content := widget.NewLabel(h.tooltip)
	popupContent := container.NewPadded(content)
	h.popup = widget.NewPopUp(popupContent, h.canvas)
	size := popupContent.MinSize()
	if size.Width < 120 {
		size.Width = 120
	}
	h.popup.Resize(size)
	h.popup.ShowAtRelativePosition(fyne.NewPos(0, h.Size().Height), h)
}

func (h *buttonWithTooltip) MouseMoved(ev *desktop.MouseEvent) {
	h.Button.MouseMoved(ev)
}

func (h *buttonWithTooltip) MouseOut() {
	h.Button.MouseOut()
	h.hidePopup()
}

func (h *buttonWithTooltip) hidePopup() {
	if h.popup == nil {
		return
	}
	h.popup.Hide()
	h.popup = nil
}
