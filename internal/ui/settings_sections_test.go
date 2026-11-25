package ui

import (
	"slices"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
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

// findSettingsTabs and formLabels walk an object tree for assertions.
func findSettingsTabs(obj fyne.CanvasObject) *container.AppTabs {
	switch o := obj.(type) {
	case *container.AppTabs:
		return o
	case *fyne.Container:
		for _, child := range o.Objects {
			if tabs := findSettingsTabs(child); tabs != nil {
				return tabs
			}
		}
	}
	return nil
}

func formLabels(obj fyne.CanvasObject) []string {
	var out []string
	switch o := obj.(type) {
	case *widget.Form:
		for _, item := range o.Items {
			out = append(out, item.Text)
		}
	case *fyne.Container:
		for _, child := range o.Objects {
			out = append(out, formLabels(child)...)
		}
	case fyne.Widget:
		for _, child := range test.WidgetRenderer(o).Objects() {
			out = append(out, formLabels(child)...)
		}
	}
	return out
}

func TestSettingsWindowUsesFocusedTabs(t *testing.T) {
	app := newTorrentTableTestApp(t)
	app.openSettingsWindow()
	t.Cleanup(app.settingsWindow.Close)

	tabs := findSettingsTabs(app.settingsWindow.Content())
	if tabs == nil {
		t.Fatal("settings window has no tabs")
	}
	var titles []string
	for _, item := range tabs.Items {
		titles = append(titles, item.Text)
		for _, label := range formLabels(item.Content) {
			if label == "Sort by" || label == "Descending order" {
				t.Fatalf("tab %q still offers %q; sorting lives in the column headers", item.Text, label)
			}
		}
	}
	want := []string{"Connection", "Torrent list", "Adding", "Integration", "Application"}
	if !slices.Equal(titles, want) {
		t.Fatalf("tabs %v, want %v", titles, want)
	}
}

func TestSettingsTabSections(t *testing.T) {
	test.NewTempApp(t)
	compact, details, mode := widget.NewCheck("", nil), widget.NewCheck("", nil), widget.NewSelect(nil, nil)
	active, background := widget.NewEntry(), widget.NewEntry()
	list := torrentListSettingsContent(torrentListSettingsControls{
		compactRows: compact, detailsEnabled: details, detailsMode: mode,
		activePoll: active, backgroundPoll: background,
	})
	if got := sectionOf(list, compact); got != "Display" {
		t.Fatalf("compact rows in %q, want Display", got)
	}
	if got := sectionOf(list, background); got != "Refresh" {
		t.Fatalf("background poll in %q, want Refresh", got)
	}

	minimized, updates, level, toFile := widget.NewCheck("", nil), widget.NewCheck("", nil), widget.NewSelect(nil, nil), widget.NewCheck("", nil)
	appTab := applicationSettingsContent(applicationSettingsControls{
		startMinimized: minimized, checkUpdates: updates, logLevel: level, logToFile: toFile,
	})
	for obj, want := range map[fyne.CanvasObject]string{minimized: "Startup", updates: "Updates", toFile: "Logging"} {
		if got := sectionOf(appTab, obj); got != want {
			t.Fatalf("control in %q, want %q", got, want)
		}
	}
}
