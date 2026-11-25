package ui

import (
	"runtime"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/skobkin/qbtremotego/internal/platform"
)

// settingsSection is a titled group of settings inside a tab.
type settingsSection struct {
	widget.BaseWidget
	title string
	root  fyne.CanvasObject
}

func newSettingsSection(title string, content ...fyne.CanvasObject) *settingsSection {
	heading := widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	objects := append([]fyne.CanvasObject{heading}, content...)
	s := &settingsSection{title: title, root: container.NewVBox(objects...)}
	s.ExtendBaseWidget(s)
	return s
}

func (s *settingsSection) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(s.root)
}

// integrationSettingsControls are the Integration tab's inputs.
type integrationSettingsControls struct {
	registerMagnet   fyne.CanvasObject
	registerTorrent  fyne.CanvasObject
	startWithSystem  fyne.CanvasObject
	notifyOnComplete fyne.CanvasObject
}

// integrationSettingsContent lays out the Integration tab: OS integration
// (handlers, autostart) and desktop notifications as separate sections.
func integrationSettingsContent(c integrationSettingsControls) fyne.CanvasObject {
	system := []fyne.CanvasObject{widget.NewForm(
		widget.NewFormItem("Register magnet handler", c.registerMagnet),
		widget.NewFormItem("Register .torrent handler", c.registerTorrent),
		widget.NewFormItem("Start with the system", c.startWithSystem),
	)}
	if runtime.GOOS == "windows" {
		hint := widget.NewLabel(platform.WindowsDefaultAppsSelectionHint())
		hint.Wrapping = fyne.TextWrapWord
		system = append(system, hint)
	}

	return container.NewVBox(
		newSettingsSection("System integration", system...),
		widget.NewSeparator(),
		newSettingsSection("Notifications", widget.NewForm(
			widget.NewFormItem("Notify when downloads finish", c.notifyOnComplete),
		)),
	)
}
