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
	if stop := controller.credentialRetryStep(); !stop {
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

	if stop := controller.credentialRetryStep(); !stop {
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

	if stop := controller.credentialRetryStep(); !stop {
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
