package ui

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	updates "github.com/skobkin/go4updates"

	appcore "github.com/skobkin/qbtremotego/internal/app"
	"github.com/skobkin/qbtremotego/internal/config"
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

	var checks atomic.Int64
	stub := &stubUpdateChecker{
		checkNow: func(context.Context) (updates.Result, error) {
			checks.Add(1)
			<-release
			return updates.Result{}, nil
		},
	}
	app := &application{logger: slog.New(slog.DiscardHandler), window: fyne.CurrentApp().NewWindow("busy")}
	app.updateChecker = stub
	app.windowVisible.Store(true)

	app.checkForUpdates()

	// Wait until the in-flight check reached the stub.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && checks.Load() == 0 {
		time.Sleep(5 * time.Millisecond)
	}

	app.checkForUpdates()

	// The first check is still in flight; the second must not have called
	// CheckNow again.
	if got := checks.Load(); got != 1 {
		t.Fatalf("CheckNow calls = %d, want 1", got)
	}
	if !app.checkingUpdates.Load() {
		t.Fatal("expected the busy flag to be set while the check runs")
	}

	// Let the check finish and drain its presentation callback so the
	// goroutine does not leak into the following tests.
	close(release)
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && app.checkingUpdates.Load() {
		time.Sleep(5 * time.Millisecond)
	}
	if app.checkingUpdates.Load() {
		t.Fatal("expected the busy flag to clear after the check finished")
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

// newUpdateChecksTestController builds a controller whose stored config has
// the automatic-check toggle set to configured.
func newUpdateChecksTestController(t *testing.T, configured bool) *appcore.Controller {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := config.Default()
	cfg.Connection.CredentialStorage = config.CredentialStorageNone
	cfg.Updates.CheckAutomatically = configured
	if err := config.Save(path, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	controller, err := appcore.NewController(path, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("new controller: %v", err)
	}

	return controller
}

// startUpdateChecks starts periodic checks only when the build is a release
// and the config enables them.
func TestStartUpdateChecksGuards(t *testing.T) {
	tests := []struct {
		name       string
		version    string
		configured bool
		wantStarts int
	}{
		{name: "release build and enabled", version: "0.8.0", configured: true, wantStarts: 1},
		{name: "disabled by config", version: "0.8.0", configured: false, wantStarts: 0},
		{name: "development build never checks", version: "dev", configured: true, wantStarts: 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			original := appcore.Version
			appcore.Version = tc.version
			t.Cleanup(func() { appcore.Version = original })

			test.NewTempApp(t)
			stub := &stubUpdateChecker{}
			app := &application{
				logger:        slog.New(slog.DiscardHandler),
				window:        fyne.CurrentApp().NewWindow("start"),
				updateChecker: stub,
			}
			app.controller = newUpdateChecksTestController(t, tc.configured)

			app.startUpdateChecks()

			stub.mu.Lock()
			started := stub.started
			stub.mu.Unlock()
			if started != tc.wantStarts {
				t.Fatalf("starts = %d, want %d", started, tc.wantStarts)
			}
		})
	}
}

// applyUpdateCheckSetting starts and stops periodic checks only on an actual
// change and never on development builds.
func TestApplyUpdateCheckSetting(t *testing.T) {
	tests := []struct {
		name       string
		version    string
		enabled    bool
		running    bool
		wantStarts int
		wantStops  int
	}{
		{name: "enable from stopped", enabled: true, running: false, wantStarts: 1},
		{name: "disable from running", enabled: false, running: true, wantStops: 1},
		{name: "enable while running is a no-op", enabled: true, running: true},
		{name: "disable while stopped is a no-op", enabled: false, running: false},
		{name: "development build never toggles", version: "dev", enabled: true, running: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.version == "" {
				tc.version = "0.8.0"
			}
			original := appcore.Version
			appcore.Version = tc.version
			t.Cleanup(func() { appcore.Version = original })

			test.NewTempApp(t)
			stub := &stubUpdateChecker{running: tc.running}
			app := &application{
				logger:        slog.New(slog.DiscardHandler),
				window:        fyne.CurrentApp().NewWindow("toggle"),
				updateChecker: stub,
			}

			app.applyUpdateCheckSetting(tc.enabled)

			stub.mu.Lock()
			started, stopped := stub.started, stub.stopped
			stub.mu.Unlock()
			if started != tc.wantStarts || stopped != tc.wantStops {
				t.Fatalf("starts = %d (want %d), stops = %d (want %d)",
					started, tc.wantStarts, stopped, tc.wantStops)
			}
		})
	}
}

// onAutoUpdateAvailable must reveal a hidden window and stay non-blocking;
// the presentation path is shared with the manual check.
func TestOnAutoUpdateAvailableRevealsHiddenWindow(t *testing.T) {
	test.NewTempApp(t)
	win := fyne.CurrentApp().NewWindow("auto")
	app := &application{logger: slog.New(slog.DiscardHandler), window: win}
	app.windowVisible.Store(false)

	app.onAutoUpdateAvailable(updates.Result{Status: updates.StatusUpdateAvailable})

	if !app.windowVisible.Load() {
		t.Fatal("expected the hidden window to be revealed")
	}
}

func TestUpdateChecksEnabled(t *testing.T) {
	cfg := config.AppConfig{}
	cfg.Updates.CheckAutomatically = true
	if !updateChecksEnabled(cfg, "0.8.0") {
		t.Fatal("expected enabled checks for a release build")
	}
	if updateChecksEnabled(cfg, "dev") {
		t.Fatal("expected dev builds to disable automatic checks")
	}
	cfg.Updates.CheckAutomatically = false
	if updateChecksEnabled(cfg, "0.8.0") {
		t.Fatal("expected the config toggle to disable automatic checks")
	}
}
