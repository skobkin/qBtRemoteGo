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

// torrentListSettingsControls are the Torrent list tab's inputs.
type torrentListSettingsControls struct {
	compactRows    fyne.CanvasObject
	detailsEnabled fyne.CanvasObject
	detailsMode    fyne.CanvasObject
	activePoll     fyne.CanvasObject
	backgroundPoll fyne.CanvasObject
}

// torrentListSettingsContent lays out how the list looks and how often it
// refreshes. Sorting is set from the column headers.
func torrentListSettingsContent(c torrentListSettingsControls) fyne.CanvasObject {
	return container.NewVBox(
		newSettingsSection("Display", widget.NewForm(
			widget.NewFormItem("Compact rows", c.compactRows),
			widget.NewFormItem("Enable details panel", c.detailsEnabled),
			widget.NewFormItem("Details panel mode", c.detailsMode),
		)),
		widget.NewSeparator(),
		newSettingsSection("Refresh", widget.NewForm(
			widget.NewFormItem("Update interval (seconds)", c.activePoll),
			widget.NewFormItem("Background interval (seconds)", c.backgroundPoll),
		)),
	)
}

// addingSettingsControls are the Adding tab's inputs.
type addingSettingsControls struct {
	rememberCount    fyne.CanvasObject
	rememberLastPath fyne.CanvasObject
	pathAutocomplete fyne.CanvasObject
}

// addingSettingsContent lays out the Add dialog's save location behaviour.
func addingSettingsContent(c addingSettingsControls) fyne.CanvasObject {
	return container.NewVBox(
		newSettingsSection("Save location", widget.NewForm(
			widget.NewFormItem("Number of paths to remember", c.rememberCount),
			widget.NewFormItem("Remember last save location", c.rememberLastPath),
			widget.NewFormItem("Path autocompletion", c.pathAutocomplete),
		)),
	)
}

// applicationSettingsControls are the Application tab's inputs.
type applicationSettingsControls struct {
	startMinimized fyne.CanvasObject
	checkUpdates   fyne.CanvasObject
	logLevel       fyne.CanvasObject
	logToFile      fyne.CanvasObject
}

// applicationSettingsContent lays out app-wide behaviour: startup, update
// checks and logging.
func applicationSettingsContent(c applicationSettingsControls) fyne.CanvasObject {
	return container.NewVBox(
		newSettingsSection("Startup", widget.NewForm(
			widget.NewFormItem("Start minimized to tray", c.startMinimized),
		)),
		widget.NewSeparator(),
		newSettingsSection("Updates", widget.NewForm(
			widget.NewFormItem("Check for updates automatically", c.checkUpdates),
		)),
		widget.NewSeparator(),
		newSettingsSection("Logging", widget.NewForm(
			widget.NewFormItem("Log level", c.logLevel),
			widget.NewFormItem("Log to file", c.logToFile),
		)),
	)
}
