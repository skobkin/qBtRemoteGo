package app

import (
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/skobkin/qbtremotego/internal/config"
	"github.com/skobkin/qbtremotego/internal/credentials"
	keyring "github.com/zalando/go-keyring"
)

// newKeychainPendingController builds a controller whose config claims
// credentials are stored in the keychain (marker set) while the store answers
// ErrNotFound — the boot-race shape.
func newKeychainPendingController(t *testing.T, store credentials.Store) *Controller {
	t.Helper()

	cfg := config.Default()
	cfg.Connection.CredentialStorage = config.CredentialStorageKeychain
	cfg.Connection.KeychainHasCredentials = true

	return newTestController(t, cfg, store)
}

func newNotStoredStore() credentials.Store {
	return credentials.NewStoreForTests(
		func(service, user string) (string, error) { return "", keyring.ErrNotFound },
		func(service, user, password string) error { return nil },
		func(service, user string) error { return nil },
	)
}

func TestNewControllerReturnsWhileKeychainHangs(t *testing.T) {
	oldTimeout := startupCredentialTimeout
	startupCredentialTimeout = 50 * time.Millisecond
	t.Cleanup(func() { startupCredentialTimeout = oldTimeout })

	// The keychain holds the credentials but never answers until released,
	// like a wallet waiting for an unlock prompt.
	release := make(chan struct{})
	store := credentials.NewStoreForTests(
		func(service, user string) (string, error) {
			<-release

			return `{"username":"demo","password":"secret"}`, nil
		},
		nil,
		nil,
	)

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := config.Default()
	cfg.Connection.CredentialStorage = config.CredentialStorageKeychain
	cfg.Connection.KeychainHasCredentials = true
	if err := config.Save(path, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	controller, err := newController(path, slog.New(slog.NewTextHandler(io.Discard, nil)), store)
	if err != nil {
		t.Fatalf("new controller: %v", err)
	}
	controller.platform = nil
	t.Cleanup(func() {
		controller.stopCredentialRetries()
		// Unblock the abandoned Get goroutine so it can drain and exit.
		close(release)
	})

	if got := controller.SessionCredentials(); got != (credentials.Credentials{}) {
		t.Fatalf("expected empty session credentials while the keychain hangs, got %#v", got)
	}
	if !controller.credentialsRetryPending() {
		t.Fatal("expected a background retry to be pending")
	}
	status := controller.CredentialStatus()
	if status.State != credentials.StateUnavailable {
		t.Fatalf("unexpected status state: %q", status.State)
	}
	if status.Backend == "" {
		t.Fatal("expected the status to name the backend")
	}
}

func TestCredentialRetryStepLoadsCredentialsAfterRecovery(t *testing.T) {
	var recovered atomic.Bool
	store := credentials.NewStoreForTests(
		func(service, user string) (string, error) {
			if !recovered.Load() {
				return "", keyring.ErrNotFound
			}

			return `{"username":"demo","password":"secret"}`, nil
		},
		nil,
		nil,
	)
	controller := newKeychainPendingController(t, store)

	recovered.Store(true)
	if stop := controller.credentialRetryStep(1); !stop {
		t.Fatal("expected a successful load to stop the retry")
	}

	got := controller.SessionCredentials()
	if got.Username != "demo" || got.Password != "secret" {
		t.Fatalf("expected the keychain credentials to be applied, got %#v", got)
	}
	status := controller.CredentialStatus()
	if status.State != credentials.StateAvailable {
		t.Fatalf("unexpected status state after recovery: %q", status.State)
	}
	if !controller.Config().Connection.KeychainHasCredentials {
		t.Fatal("expected the marker to survive recovery")
	}
}

func TestCredentialRetryStepDoesNotClobberSavedCredentials(t *testing.T) {
	var recovered atomic.Bool
	store := credentials.NewStoreForTests(
		func(service, user string) (string, error) {
			if !recovered.Load() {
				return "", keyring.ErrNotFound
			}

			return `{"username":"demo","password":"secret"}`, nil
		},
		nil,
		nil,
	)
	controller := newKeychainPendingController(t, store)

	// The user re-enters credentials while the retry is still waiting.
	saved := credentials.Credentials{Username: "fresh", Password: "pw"}
	controller.setSessionCredentials(saved)
	recovered.Store(true)

	if stop := controller.credentialRetryStep(1); !stop {
		t.Fatal("expected the step to stop when credentials are already loaded")
	}
	if got := controller.SessionCredentials(); got != saved {
		t.Fatalf("retry clobbered the saved credentials: got %#v, want %#v", got, saved)
	}
}

func TestCredentialRetryStepStopsWhenStorageModeChanges(t *testing.T) {
	controller := newKeychainPendingController(t, newNotStoredStore())

	controller.stateMu.Lock()
	controller.config.Connection.CredentialStorage = config.CredentialStorageNone
	controller.stateMu.Unlock()

	if stop := controller.credentialRetryStep(1); !stop {
		t.Fatal("expected the step to stop when the storage mode is no longer keychain")
	}
}

func TestCredentialRetryLoopIsBoundedAndSignalsDone(t *testing.T) {
	oldInterval := retryCredentialInterval
	oldCap := maxCredentialRetries
	retryCredentialInterval = time.Millisecond
	maxCredentialRetries = 3
	t.Cleanup(func() {
		retryCredentialInterval = oldInterval
		maxCredentialRetries = oldCap
	})

	controller := newKeychainPendingController(t, newNotStoredStore())

	select {
	case <-controller.credentialRetryDone:
	case <-time.After(5 * time.Second):
		t.Fatal("expected the retry loop to finish after the attempt cap")
	}

	status := controller.CredentialStatus()
	if !strings.Contains(status.Message, "Gave up waiting for the system keychain") {
		t.Fatalf("expected a terminal actionable status, got %q", status.Message)
	}
	if controller.credentialsRetryPending() {
		t.Fatal("expected the pending flag to be cleared after the loop finished")
	}
}

func TestClientWaitsWhenKeychainLoadPending(t *testing.T) {
	for _, method := range []config.AuthMethod{config.AuthMethodAPIKey, config.AuthMethodPassword} {
		t.Run(string(method), func(t *testing.T) {
			cfg := config.Default()
			cfg.Connection.URL = "http://localhost:8080"
			cfg.Connection.CredentialStorage = config.CredentialStorageKeychain
			cfg.Connection.KeychainHasCredentials = true
			cfg.Connection.AuthMethod = method
			controller := newTestController(t, cfg, newNotStoredStore())

			_, err := controller.client()
			var waitErr *CredentialUnavailableError
			if !errors.As(err, &waitErr) {
				t.Fatalf("expected a CredentialUnavailableError, got %v", err)
			}
			if waitErr.Status.Backend == "" {
				t.Fatal("expected the carried status to name the backend")
			}
		})
	}
}

func TestClientKeepsPlainErrorWhenNothingStored(t *testing.T) {
	cfg := config.Default()
	cfg.Connection.CredentialStorage = config.CredentialStorageKeychain
	// No marker: the keychain was reachable and simply holds nothing, so
	// fetches must keep the actionable "no API key is stored" error instead of
	// a waiting hint.
	controller := newTestController(t, cfg, newNotStoredStore())

	_, err := controller.client()
	var waitErr *CredentialUnavailableError
	if errors.As(err, &waitErr) {
		t.Fatal("did not expect a waiting error when nothing is stored")
	}
	if err == nil || !strings.Contains(err.Error(), "no API key is stored") {
		t.Fatalf("expected the plain nothing-stored error, got %v", err)
	}
}

// Configs saved before the stored-credentials marker existed carry
// CredentialStorageKeychain without the marker, so a failed startup read must
// still start the background retry for them — the wallet may simply not be
// ready yet.
func TestLegacyKeychainConfigWithoutMarkerStartsRetry(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantMsg string
	}{
		{name: "locked keychain", err: errors.New("collection is locked")},
		{name: "timed out keychain", err: errKeychainTimeout},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Default()
			cfg.Connection.CredentialStorage = config.CredentialStorageKeychain
			// KeychainHasCredentials stays false: the marker did not exist when
			// released builds wrote this config.
			store := credentials.NewStoreForTests(
				func(service, user string) (string, error) { return "", tc.err },
				nil,
				nil,
			)
			controller := newTestController(t, cfg, store)

			if !controller.credentialsRetryPending() {
				t.Fatal("expected a background retry for a marker-less keychain config")
			}
			if !controller.CredentialRetryActive() {
				t.Fatal("expected the exported retry state to report active")
			}
			if state := controller.CredentialStatus().State; state == credentials.StateAvailable {
				t.Fatalf("expected a non-available status for %v, got %q", tc.err, state)
			}
		})
	}
}

// newMarkerlessKeychainController builds a controller whose config claims
// keychain storage but predates the stored-credentials marker — the shape
// released builds wrote — saved at a known path so tests can assert what a
// load persisted.
func newMarkerlessKeychainController(t *testing.T, store credentials.Store) (*Controller, string) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := config.Default()
	cfg.Connection.CredentialStorage = config.CredentialStorageKeychain
	cfg.Connection.AuthMethod = config.AuthMethodAPIKey
	if err := config.Save(path, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	controller, err := newController(path, slog.New(slog.NewTextHandler(io.Discard, nil)), store)
	if err != nil {
		t.Fatalf("new controller: %v", err)
	}
	controller.platform = nil
	t.Cleanup(controller.stopCredentialRetries)

	return controller, path
}

// A marker-less config cannot trust a "not found": the boot race looks
// identical to a keychain that holds nothing. The startup read must start the
// bounded grace probes while keeping the available classification — the plain
// nothing-stored error stays, and no waiting hint may appear for credentials
// that may genuinely not exist.
func TestMarkerlessNotFoundStartsGraceRetryWithoutWaitingHint(t *testing.T) {
	controller, _ := newMarkerlessKeychainController(t, newNotStoredStore())

	if !controller.credentialsRetryPending() {
		t.Fatal("expected a grace retry for a marker-less not-found startup read")
	}
	if got := controller.CredentialStatus().State; got != credentials.StateAvailable {
		t.Fatalf("unexpected status state: %q", got)
	}

	_, err := controller.client()
	var waitErr *CredentialUnavailableError
	if errors.As(err, &waitErr) {
		t.Fatal("did not expect a waiting error while the grace probes run")
	}
	if err == nil || !strings.Contains(err.Error(), "no API key is stored") {
		t.Fatalf("expected the plain nothing-stored error, got %v", err)
	}
}

// The grace probes must stay bounded and must never escalate to the terminal
// gave-up status: for a config without the marker an absent entry most
// plausibly means nothing is stored, so the available classification is the
// correct resting state.
func TestCredentialRetryStepBoundsGraceForMarkerlessNotFound(t *testing.T) {
	controller, _ := newMarkerlessKeychainController(t, newNotStoredStore())

	for attempt := 1; attempt < graceNotFoundRetries; attempt++ {
		if stop := controller.credentialRetryStep(attempt); stop {
			t.Fatalf("attempt %d: expected the grace probe to continue", attempt)
		}
		if got := controller.CredentialStatus().State; got != credentials.StateAvailable {
			t.Fatalf("attempt %d: unexpected status state %q", attempt, got)
		}
	}
	if stop := controller.credentialRetryStep(graceNotFoundRetries); !stop {
		t.Fatal("expected the grace probes to stop after the bound")
	}

	status := controller.CredentialStatus()
	if status.State != credentials.StateAvailable {
		t.Fatalf("grace exhaustion must keep the nothing-stored state, got %q", status.State)
	}
	if strings.Contains(status.Message, "Gave up waiting") {
		t.Fatalf("a marker-less config must not end in the terminal gave-up status, got %q", status.Message)
	}
}

// A successful load must persist the marker for configs saved before it
// existed, so later boots can tell the boot race from "nothing stored".
func TestSuccessfulStartupLoadBackfillsMarker(t *testing.T) {
	store := credentials.NewStoreForTests(
		func(service, user string) (string, error) { return `{"api_key":"` + testAPIKey + `"}`, nil },
		nil,
		nil,
	)
	controller, path := newMarkerlessKeychainController(t, store)

	if got := controller.SessionCredentials().APIKey; got != testAPIKey {
		t.Fatalf("expected the keychain credentials to load, got %#v", controller.SessionCredentials())
	}
	disk, err := config.Load(path)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if !disk.Connection.KeychainHasCredentials {
		t.Fatal("expected a successful startup load to persist the marker")
	}
}

// The same backfill applies when the credentials only show up after the boot
// race clears and a grace probe finds them.
func TestSuccessfulRetryLoadBackfillsMarker(t *testing.T) {
	var recovered atomic.Bool
	store := credentials.NewStoreForTests(
		func(service, user string) (string, error) {
			if !recovered.Load() {
				return "", keyring.ErrNotFound
			}

			return `{"api_key":"` + testAPIKey + `"}`, nil
		},
		nil,
		nil,
	)
	controller, path := newMarkerlessKeychainController(t, store)
	if !controller.credentialsRetryPending() {
		t.Fatal("expected the grace retry to be probing after the failed startup read")
	}

	recovered.Store(true)
	if stop := controller.credentialRetryStep(1); !stop {
		t.Fatal("expected a successful load to stop the retry")
	}

	disk, err := config.Load(path)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if !disk.Connection.KeychainHasCredentials {
		t.Fatal("expected a successful retry load to persist the marker")
	}
}

// After the retry loop ends in a terminal state (an unreadable payload, an
// unsupported keychain, or the exhausted attempt cap) the stored marker is
// still set and the session is still empty; client creation must surface the
// actionable status instead of building a client with blank credentials.
func TestClientSurfacesTerminalKeychainStatus(t *testing.T) {
	for _, method := range []config.AuthMethod{config.AuthMethodAPIKey, config.AuthMethodPassword} {
		t.Run(string(method), func(t *testing.T) {
			cfg := config.Default()
			cfg.Connection.URL = "http://localhost:8080"
			cfg.Connection.CredentialStorage = config.CredentialStorageKeychain
			cfg.Connection.KeychainHasCredentials = true
			cfg.Connection.AuthMethod = method
			controller := newTestController(t, cfg, newNotStoredStore())
			controller.stopCredentialRetries()

			controller.setCredentialStatus(credentials.Status{
				Backend: "Secret Service",
				State:   credentials.StateUnavailable,
				Message: "Gave up waiting for the system keychain; open Settings > Connection and re-enter the credentials.",
			})

			_, err := controller.client()
			var waitErr *CredentialUnavailableError
			if !errors.As(err, &waitErr) {
				t.Fatalf("expected a CredentialUnavailableError, got %v", err)
			}
			if waitErr.Waiting {
				t.Fatal("a terminal status must not be reported as still waiting")
			}
			if !strings.Contains(err.Error(), "Gave up waiting") {
				t.Fatalf("expected the actionable terminal message, got %v", err)
			}
		})
	}
}

// A marker-less keychain config with a failing keychain used to fall through
// to normal client construction, which logs in with blank password
// credentials.
func TestClientWaitsForMarkerlessKeychainFailure(t *testing.T) {
	cfg := config.Default()
	cfg.Connection.URL = "http://localhost:8080"
	cfg.Connection.CredentialStorage = config.CredentialStorageKeychain
	cfg.Connection.AuthMethod = config.AuthMethodPassword
	store := credentials.NewStoreForTests(
		func(service, user string) (string, error) { return "", errors.New("collection is locked") },
		nil,
		nil,
	)
	controller := newTestController(t, cfg, store)

	_, err := controller.client()
	var waitErr *CredentialUnavailableError
	if !errors.As(err, &waitErr) {
		t.Fatalf("expected a CredentialUnavailableError, got %v", err)
	}
	if !waitErr.Waiting {
		t.Fatal("expected the error to report an active retry")
	}
	if waitErr.Status.Backend == "" {
		t.Fatal("expected the carried status to name the backend")
	}
}

func TestCredentialUnavailableErrorMessage(t *testing.T) {
	waiting := &CredentialUnavailableError{
		Status:  credentials.Status{Message: "keychain locked"},
		Waiting: true,
	}
	if got, want := waiting.Error(), "waiting for system keychain: keychain locked"; got != want {
		t.Fatalf("unexpected waiting message: got %q, want %q", got, want)
	}

	terminal := &CredentialUnavailableError{
		Status: credentials.Status{Message: "Gave up waiting for the system keychain"},
	}
	if got, want := terminal.Error(), "Gave up waiting for the system keychain"; got != want {
		t.Fatalf("unexpected terminal message: got %q, want %q", got, want)
	}

	fallback := &CredentialUnavailableError{Waiting: true}
	if got, want := fallback.Error(), "waiting for system keychain: credentials are not loaded yet"; got != want {
		t.Fatalf("unexpected fallback message: got %q, want %q", got, want)
	}
}

func TestKeychainLoadPendingPredicate(t *testing.T) {
	controller := newTestController(t, config.Default(), newNotStoredStore())

	tests := []struct {
		name     string
		mode     config.CredentialStorageMode
		marker   bool
		session  credentials.Credentials
		state    credentials.State
		wantPend bool
	}{
		{
			name:     "stored marker with unavailable state stays pending",
			mode:     config.CredentialStorageKeychain,
			marker:   true,
			state:    credentials.StateUnavailable,
			wantPend: true,
		},
		{
			name:     "marker-less locked state stays pending",
			mode:     config.CredentialStorageKeychain,
			marker:   false,
			state:    credentials.StateLocked,
			wantPend: true,
		},
		{
			name:     "marker-less not-found still probes",
			mode:     config.CredentialStorageKeychain,
			marker:   false,
			state:    credentials.StateAvailable,
			wantPend: true,
		},
		{
			name:   "stored marker with a reachable empty keychain is not pending",
			mode:   config.CredentialStorageKeychain,
			marker: true,
			state:  credentials.StateAvailable,
		},
		{
			name:    "loaded credentials stop the pending state",
			mode:    config.CredentialStorageKeychain,
			marker:  true,
			session: credentials.Credentials{APIKey: testAPIKey},
			state:   credentials.StateUnavailable,
		},
		{
			name:     "non-keychain storage is never pending",
			mode:     config.CredentialStoragePlaintext,
			marker:   true,
			state:    credentials.StateUnavailable,
			wantPend: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			controller.stateMu.Lock()
			controller.config.Connection.CredentialStorage = tc.mode
			controller.config.Connection.KeychainHasCredentials = tc.marker
			controller.sessionCredentials = tc.session
			controller.credentialStatus = credentials.Status{Backend: "Secret Service", State: tc.state}
			controller.stateMu.Unlock()

			if got := controller.keychainLoadPending(); got != tc.wantPend {
				t.Fatalf("keychainLoadPending() = %v, want %v", got, tc.wantPend)
			}
		})
	}
}
