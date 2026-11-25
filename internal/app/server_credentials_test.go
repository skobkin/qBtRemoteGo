package app

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	keyring "github.com/zalando/go-keyring"

	"github.com/skobkin/qbtremotego/internal/config"
	"github.com/skobkin/qbtremotego/internal/credentials"
)

var rotatedAPIKey = "qbt_" + strings.Repeat("b", 28)

// newRotateServer serves app/rotateAPIKey for requests carrying the expected
// bearer key and records the bearer key of every request.
func newRotateServer(t *testing.T, bearers *[]string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*bearers = append(*bearers, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = io.WriteString(w, "Ok.")
		case "/api/v2/app/rotateAPIKey":
			_, _ = io.WriteString(w, `{"apiKey":"`+rotatedAPIKey+`"}`)
		case "/api/v2/app/preferences":
			_, _ = io.WriteString(w, `{}`)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	return server
}

func notFoundStore() credentials.Store {
	return credentials.NewStoreForTests(
		func(service, user string) (string, error) { return "", keyring.ErrNotFound },
		func(service, user, password string) error { return nil },
		func(service, user string) error { return nil },
	)
}

func TestRotateAPIKeyReplacesThePlaintextKeyTheAppUses(t *testing.T) {
	var bearers []string
	server := newRotateServer(t, &bearers)
	cfg := config.Default()
	cfg.Connection.URL = server.URL
	cfg.Connection.AuthMethod = config.AuthMethodAPIKey
	cfg.Connection.CredentialStorage = config.CredentialStoragePlaintext
	cfg.Connection.APIKey = testAPIKey
	controller := newTestController(t, cfg, notFoundStore())

	key, err := controller.RotateAPIKey(context.Background())
	if err != nil || key != rotatedAPIKey {
		t.Fatalf("RotateAPIKey = %q, %v", key, err)
	}
	if got := controller.Config().Connection.APIKey; got != rotatedAPIKey {
		t.Fatalf("saved key = %q", got)
	}
	if got := controller.SessionCredentials().APIKey; got != rotatedAPIKey {
		t.Fatalf("session key = %q", got)
	}

	if _, err := controller.FetchPreferences(context.Background()); err != nil {
		t.Fatalf("fetch after rotation: %v", err)
	}
	if bearers[0] != testAPIKey || bearers[len(bearers)-1] != rotatedAPIKey {
		t.Fatalf("bearer keys = %v, want old key for rotation, new key afterwards", bearers)
	}
}

func TestRotateAPIKeySavesToTheKeychain(t *testing.T) {
	var bearers []string
	server := newRotateServer(t, &bearers)
	store, raw := newCapturingKeychainStore()
	*raw = `{"api_key":"` + testAPIKey + `"}`
	cfg := config.Default()
	cfg.Connection.URL = server.URL
	cfg.Connection.AuthMethod = config.AuthMethodAPIKey
	cfg.Connection.CredentialStorage = config.CredentialStorageKeychain
	cfg.Connection.KeychainHasCredentials = true
	controller := newTestController(t, cfg, store)
	if controller.SessionCredentials().APIKey != testAPIKey {
		t.Fatalf("startup did not load the keychain key: %#v", controller.SessionCredentials())
	}

	if _, err := controller.RotateAPIKey(context.Background()); err != nil {
		t.Fatalf("RotateAPIKey: %v", err)
	}
	if stored := decodeStoredPayload(t, *raw); stored.APIKey != rotatedAPIKey {
		t.Fatalf("keychain holds %#v", stored)
	}
	if conn := controller.Config().Connection; conn.APIKey != "" || conn.CredentialStorage != config.CredentialStorageKeychain {
		t.Fatalf("config changed: storage %q, key %q", conn.CredentialStorage, conn.APIKey)
	}
}

func TestRotateAPIKeyReportsAKeychainFailureButKeepsTheSessionWorking(t *testing.T) {
	var bearers []string
	server := newRotateServer(t, &bearers)
	payload := `{"api_key":"` + testAPIKey + `"}`
	store := credentials.NewStoreForTests(
		func(service, user string) (string, error) { return payload, nil },
		func(service, user, password string) error { return errors.New("wallet locked") },
		func(service, user string) error { return nil },
	)
	cfg := config.Default()
	cfg.Connection.URL = server.URL
	cfg.Connection.AuthMethod = config.AuthMethodAPIKey
	cfg.Connection.CredentialStorage = config.CredentialStorageKeychain
	cfg.Connection.KeychainHasCredentials = true
	controller := newTestController(t, cfg, store)

	key, err := controller.RotateAPIKey(context.Background())
	if err == nil || key != rotatedAPIKey {
		t.Fatalf("RotateAPIKey = %q, %v; want the new key and a save error", key, err)
	}
	if got := controller.SessionCredentials().APIKey; got != rotatedAPIKey {
		t.Fatalf("session key = %q, want the rotated key", got)
	}
}

func TestRotateAPIKeyLeavesPasswordLoginAlone(t *testing.T) {
	var bearers []string
	server := newRotateServer(t, &bearers)
	cfg := config.Default()
	cfg.Connection.URL = server.URL
	cfg.Connection.AuthMethod = config.AuthMethodPassword
	cfg.Connection.CredentialStorage = config.CredentialStoragePlaintext
	cfg.Connection.Username = "demo"
	cfg.Connection.Password = "secret"
	controller := newTestController(t, cfg, notFoundStore())

	if _, err := controller.RotateAPIKey(context.Background()); err != nil {
		t.Fatalf("RotateAPIKey: %v", err)
	}
	conn := controller.Config().Connection
	if conn.APIKey != "" || conn.Username != "demo" || conn.Password != "secret" {
		t.Fatalf("saved login changed: %#v", conn)
	}
	if got := controller.SessionCredentials(); got != (credentials.Credentials{Username: "demo", Password: "secret"}) {
		t.Fatalf("session = %#v", got)
	}
}

func TestUpdateLoginCredentialsKeepsUnchangedPartsAndStorageMode(t *testing.T) {
	cfg := config.Default()
	cfg.Connection.URL = "http://localhost:8080"
	cfg.Connection.AuthMethod = config.AuthMethodPassword
	cfg.Connection.CredentialStorage = config.CredentialStoragePlaintext
	cfg.Connection.Username = "demo"
	cfg.Connection.Password = "secret"
	controller := newTestController(t, cfg, notFoundStore())

	if err := controller.UpdateLoginCredentials(context.Background(), "", "new-secret"); err != nil {
		t.Fatalf("update password: %v", err)
	}
	conn := controller.Config().Connection
	if conn.Username != "demo" || conn.Password != "new-secret" || conn.CredentialStorage != config.CredentialStoragePlaintext {
		t.Fatalf("after password change: %#v", conn)
	}

	if err := controller.UpdateLoginCredentials(context.Background(), "admin", ""); err != nil {
		t.Fatalf("update username: %v", err)
	}
	if got := controller.SessionCredentials(); got != (credentials.Credentials{Username: "admin", Password: "new-secret"}) {
		t.Fatalf("session = %#v", got)
	}
	if conn := controller.Config().Connection; conn.Username != "admin" || conn.Password != "new-secret" {
		t.Fatalf("after username change: %#v", conn)
	}
}

func TestUpdateLoginCredentialsSessionOnlyDoesNotPersist(t *testing.T) {
	cfg := config.Default()
	cfg.Connection.URL = "http://localhost:8080"
	cfg.Connection.AuthMethod = config.AuthMethodPassword
	cfg.Connection.CredentialStorage = config.CredentialStorageNone
	controller := newTestController(t, cfg, notFoundStore())
	controller.setSessionCredentials(credentials.Credentials{Username: "demo", Password: "secret"})

	if err := controller.UpdateLoginCredentials(context.Background(), "", "new-secret"); err != nil {
		t.Fatalf("update: %v", err)
	}
	if got := controller.SessionCredentials(); got != (credentials.Credentials{Username: "demo", Password: "new-secret"}) {
		t.Fatalf("session = %#v", got)
	}
	if conn := controller.Config().Connection; conn.Password != "" || conn.CredentialStorage != config.CredentialStorageNone {
		t.Fatalf("session-only login was persisted: %#v", conn)
	}
}

func TestUpdateLoginCredentialsRefusesAPIKeyLogin(t *testing.T) {
	cfg := config.Default()
	cfg.Connection.URL = "http://localhost:8080"
	cfg.Connection.AuthMethod = config.AuthMethodAPIKey
	cfg.Connection.CredentialStorage = config.CredentialStoragePlaintext
	cfg.Connection.APIKey = testAPIKey
	controller := newTestController(t, cfg, notFoundStore())

	if err := controller.UpdateLoginCredentials(context.Background(), "admin", "new-secret"); err == nil {
		t.Fatal("password login saved for an app that logs in with an API key")
	}
	if controller.LoginMethod() != config.AuthMethodAPIKey {
		t.Fatalf("LoginMethod = %q", controller.LoginMethod())
	}
}
