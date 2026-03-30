package ui

import (
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const tooltipHideDelay = 200 * time.Millisecond

type tooltipOverlay struct {
	widget.BaseWidget
	layer *fyne.Container
}

func newTooltipOverlay() *tooltipOverlay {
	o := &tooltipOverlay{
		layer: container.NewWithoutLayout(),
	}
	o.ExtendBaseWidget(o)

	return o
}

func (o *tooltipOverlay) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(o.layer)
}

func (o *tooltipOverlay) MinSize() fyne.Size {
	return fyne.Size{}
}

type hoverTooltipOwner struct {
	hovered bool
	hide    *time.Timer
	manager *hoverTooltipManager
}

func (o *hoverTooltipOwner) showTooltip(owner fyne.CanvasObject, content fyne.CanvasObject) {
	if o == nil || o.manager == nil || content == nil {
		return
	}

	o.hovered = true
	o.cancelHide()
	o.manager.Show(owner, content)
}

func (o *hoverTooltipOwner) scheduleHide(owner fyne.CanvasObject) {
	if o == nil {
		return
	}

	o.hovered = false
	o.cancelHide()
	o.hide = time.AfterFunc(tooltipHideDelay, func() {
		fyne.Do(func() {
			if o.hovered {
				return
			}
			o.hideTooltip(owner)
		})
	})
}

func (o *hoverTooltipOwner) hideTooltip(owner fyne.CanvasObject) {
	if o == nil {
		return
	}

	o.cancelHide()
	if o.manager == nil {
		return
	}
	o.manager.Hide(owner)
}

func (o *hoverTooltipOwner) cancelHide() {
	if o == nil || o.hide == nil {
		return
	}

	o.hide.Stop()
	o.hide = nil
}

type hoverTooltipManager struct {
	layer *tooltipOverlay
	owner fyne.CanvasObject
}

func newHoverTooltipManager(layer *tooltipOverlay) *hoverTooltipManager {
	if layer == nil {
		return nil
	}

	return &hoverTooltipManager{layer: layer}
}

func (m *hoverTooltipManager) Show(owner fyne.CanvasObject, content fyne.CanvasObject) {
	if m == nil || m.layer == nil || owner == nil || content == nil {
		return
	}

	app := fyne.CurrentApp()
	if app == nil {
		return
	}

	driver := app.Driver()
	if driver == nil {
		return
	}

	cnv := driver.CanvasForObject(owner)
	if cnv == nil {
		return
	}

	layerSize := m.layer.Size()
	layerPos := driver.AbsolutePositionForObject(m.layer)
	ownerPos := driver.AbsolutePositionForObject(owner).Subtract(layerPos)
	if layerSize.Width <= 0 || layerSize.Height <= 0 {
		layerSize = cnv.Size()
	}

	bubble := newTooltipBubble(content)
	bubble.Resize(bubble.MinSize())
	bubble.Move(tooltipPopupPosition(ownerPos, owner.Size(), bubble.Size(), layerSize))

	m.layer.layer.Objects = []fyne.CanvasObject{bubble}
	m.owner = owner
	m.layer.layer.Refresh()
}

func (m *hoverTooltipManager) Hide(owner fyne.CanvasObject) {
	if m == nil || m.layer == nil || m.owner == nil {
		return
	}
	if owner != nil && owner != m.owner {
		return
	}

	m.layer.layer.Objects = nil
	m.owner = nil
	m.layer.layer.Refresh()
}

func newTextTooltip(text string, minWidth float32) fyne.CanvasObject {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	label := widget.NewLabel(wrapTooltipText(text, minWidth-theme.Padding()*2))
	padded := container.NewPadded(label)
	size := padded.MinSize()
	if size.Width < minWidth {
		size.Width = minWidth
	}

	return container.NewGridWrap(size, padded)
}

func wrapTooltipText(text string, width float32) string {
	if width <= 0 {
		return strings.TrimSpace(text)
	}

	paragraphs := strings.Split(strings.TrimSpace(text), "\n")
	lines := make([]string, 0, len(paragraphs))
	spaceWidth := fyne.MeasureText(" ", theme.TextSize(), fyne.TextStyle{}).Width

	for _, paragraph := range paragraphs {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			lines = append(lines, "")
			continue
		}

		words := strings.Fields(paragraph)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}

		current := words[0]
		currentWidth := fyne.MeasureText(current, theme.TextSize(), fyne.TextStyle{}).Width

		for _, word := range words[1:] {
			wordWidth := fyne.MeasureText(word, theme.TextSize(), fyne.TextStyle{}).Width
			if currentWidth+spaceWidth+wordWidth <= width {
				current = fmt.Sprintf("%s %s", current, word)
				currentWidth += spaceWidth + wordWidth
				continue
			}

			lines = append(lines, current)
			current = word
			currentWidth = wordWidth
		}

		lines = append(lines, current)
	}

	return strings.Join(lines, "\n")
}

func newTooltipBubble(content fyne.CanvasObject) *fyne.Container {
	bgColor := theme.DefaultTheme().Color(theme.ColorNameOverlayBackground, theme.VariantDark)
	if app := fyne.CurrentApp(); app != nil {
		bgColor = app.Settings().Theme().Color(theme.ColorNameOverlayBackground, app.Settings().ThemeVariant())
	}

	bg := canvas.NewRectangle(bgColor)
	bg.CornerRadius = theme.Padding()

	return container.NewStack(bg, content)
}

func tooltipPopupPosition(anchorPos fyne.Position, anchorSize, popupSize, canvasSize fyne.Size) fyne.Position {
	gap := theme.Padding()
	edge := theme.Padding()
	x := anchorPos.X
	yBelow := anchorPos.Y + anchorSize.Height + gap
	yAbove := anchorPos.Y - popupSize.Height - gap

	minX := edge
	maxX := canvasSize.Width - popupSize.Width - edge
	if maxX < minX {
		minX = 0
		maxX = canvasSize.Width - popupSize.Width
	}
	if maxX < 0 {
		maxX = 0
	}

	minY := edge
	maxY := canvasSize.Height - popupSize.Height - edge
	if maxY < minY {
		minY = 0
		maxY = canvasSize.Height - popupSize.Height
	}
	if maxY < 0 {
		maxY = 0
	}

	availableBelow := maxY - yBelow
	availableAbove := yAbove - minY

	y := yBelow
	switch {
	case yBelow <= maxY:
	case yAbove >= minY:
		y = yAbove
	default:
		if availableAbove > availableBelow {
			y = minY
		} else {
			y = maxY
		}
	}

	if x > maxX {
		x = maxX
	}
	if x < minX {
		x = minX
	}
	if y > maxY {
		y = maxY
	}
	if y < minY {
		y = minY
	}

	return fyne.NewPos(x, y)
}
