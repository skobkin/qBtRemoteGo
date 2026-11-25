package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

// sectionOf returns the title of the settings section containing target, or
// "" when it is not inside one.
func sectionOf(root fyne.CanvasObject, target fyne.CanvasObject) string {
	var found string
	var walk func(obj fyne.CanvasObject, section string)
	walk = func(obj fyne.CanvasObject, section string) {
		if found != "" {
			return
		}
		if s, ok := obj.(*settingsSection); ok {
			section = s.title
		}
		if obj == target {
			found = section
			return
		}
		switch o := obj.(type) {
		case *fyne.Container:
			for _, child := range o.Objects {
				walk(child, section)
			}
		case fyne.Widget:
			for _, child := range test.WidgetRenderer(o).Objects() {
				walk(child, section)
			}
		}
	}
	walk(root, "")
	return found
}

func TestIntegrationSettingsGroupsNotifications(t *testing.T) {
	test.NewTempApp(t)
	magnet, torrent, autostart, notifyCheck := widget.NewCheck("", nil), widget.NewCheck("", nil), widget.NewCheck("", nil), widget.NewCheck("", nil)

	content := integrationSettingsContent(integrationSettingsControls{
		registerMagnet:   magnet,
		registerTorrent:  torrent,
		startWithSystem:  autostart,
		notifyOnComplete: notifyCheck,
	})

	if got := sectionOf(content, notifyCheck); got != "Notifications" {
		t.Fatalf("notify option in section %q, want Notifications", got)
	}
	if got := sectionOf(content, magnet); got != "System integration" {
		t.Fatalf("magnet handler in section %q, want System integration", got)
	}
}
