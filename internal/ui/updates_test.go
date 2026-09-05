package ui

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	updates "github.com/skobkin/go4updates"
)

// stubUpdateChecker records lifecycle calls and serves a scripted CheckNow.
type stubUpdateChecker struct {
	mu       sync.Mutex
	running  bool
	started  int
	stopped  int
	checkNow func(context.Context) (updates.Result, error)
}

func (s *stubUpdateChecker) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

func (s *stubUpdateChecker) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.started++
	s.running = true
}

func (s *stubUpdateChecker) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopped++
	s.running = false
}

func (s *stubUpdateChecker) CheckNow(ctx context.Context) (updates.Result, error) {
	s.mu.Lock()
	check := s.checkNow
	s.mu.Unlock()
	if check == nil {
		return updates.Result{}, nil
	}
	return check(ctx)
}

// The tray menu must contain the update action between the window and quit
// entries. updateTray republishes the menu on every poll via trayMenu, so
// the composition is shared.
func TestTrayMenuIncludesUpdateItem(t *testing.T) {
	test.NewTempApp(t)
	app := &application{}
	app.trayState = trayState{
		speedItem:  fyne.NewMenuItem("Down 0 B/s | Up 0 B/s", nil),
		showItem:   fyne.NewMenuItem("Open main window", nil),
		updateItem: fyne.NewMenuItem("Check for updates…", nil),
		quitItem:   fyne.NewMenuItem("Quit application", nil),
	}

	menu := app.trayMenu()
	items := menu.Items
	if len(items) != 4 {
		t.Fatalf("menu items = %d, want 4", len(items))
	}
	if items[2].Label != "Check for updates…" {
		t.Fatalf("item[2] = %q, want the update action", items[2].Label)
	}
	if items[3].Label != "Quit application" {
		t.Fatalf("item[3] = %q, want the quit action", items[3].Label)
	}
}

// A manual check while another is in flight must be a no-op: the busy guard
// protects against native tray backends that deliver duplicate activations.
func TestCheckForUpdatesBusyGuard(t *testing.T) {
	test.NewTempApp(t)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })

	checks := 0
	stub := &stubUpdateChecker{
		checkNow: func(context.Context) (updates.Result, error) {
			checks++
			<-release
			return updates.Result{}, nil
		},
	}
	app := &application{logger: slog.New(slog.DiscardHandler), window: fyne.CurrentApp().NewWindow("busy")}
	app.updateChecker = stub

	app.checkForUpdates()

	// Wait until the in-flight check reached the stub.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && checks == 0 {
		time.Sleep(5 * time.Millisecond)
	}

	app.checkForUpdates()

	// The first check is still in flight; the second must not have called
	// CheckNow again.
	if checks != 1 {
		t.Fatalf("CheckNow calls = %d, want 1", checks)
	}
	if !app.checkingUpdates.Load() {
		t.Fatal("expected the busy flag to be set while the check runs")
	}
}

// A manual check on a hidden (tray-minimized) window must reveal it before
// presenting the result.
func TestPresentManualCheckResultRevealsHiddenWindow(t *testing.T) {
	test.NewTempApp(t)
	win := fyne.CurrentApp().NewWindow("reveal")
	app := &application{logger: slog.New(slog.DiscardHandler), window: win}
	app.windowVisible.Store(false)

	app.presentManualCheckResult(updates.Result{Status: updates.StatusUpToDate}, nil)

	if !app.windowVisible.Load() {
		t.Fatal("expected the hidden window to be revealed")
	}
}

func TestPresentManualCheckResultShowsError(t *testing.T) {
	test.NewTempApp(t)
	win := fyne.CurrentApp().NewWindow("err")
	app := &application{logger: slog.New(slog.DiscardHandler), window: win}
	app.windowVisible.Store(true)

	// Must not panic under the test driver; the error dialog is attached to
	// the window and dismissed with it.
	app.presentManualCheckResult(updates.Result{}, context.Canceled)
}
