package ui

import (
	"slices"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
)

func TestHoverIconTapRunsHandler(t *testing.T) {
	test.NewTempApp(t)
	icon := newHoverIcon(nil, theme.InfoIcon(), "tip")
	if icon.Cursor() != desktop.DefaultCursor {
		t.Fatal("non-interactive icon should keep the default cursor")
	}

	tapped := 0
	icon.SetOnTapped(func() { tapped++ })
	test.Tap(icon)

	if tapped != 1 {
		t.Fatalf("tap handler ran %d times, want 1", tapped)
	}
	if icon.Cursor() != desktop.PointerCursor {
		t.Fatal("interactive icon should show the pointer cursor")
	}
}

func TestTraySlowModeItemFollowsServerState(t *testing.T) {
	app := &application{trayState: trayState{
		speedItem:    fyne.NewMenuItem("speed", nil),
		showItem:     fyne.NewMenuItem("show", nil),
		slowModeItem: fyne.NewMenuItem("Slow mode", nil),
		updateItem:   fyne.NewMenuItem("update", nil),
		quitItem:     fyne.NewMenuItem("quit", nil),
	}}

	if !slices.Contains(app.trayMenu().Items, app.trayState.slowModeItem) {
		t.Fatal("tray menu lacks the slow mode item")
	}

	app.serverState.UseAltSpeedLimits = true
	app.syncTraySlowMode()
	if !app.trayState.slowModeItem.Checked {
		t.Fatal("slow mode item unchecked while alternative limits are on")
	}

	app.serverState.UseAltSpeedLimits = false
	app.syncTraySlowMode()
	if app.trayState.slowModeItem.Checked {
		t.Fatal("slow mode item checked while alternative limits are off")
	}
}
