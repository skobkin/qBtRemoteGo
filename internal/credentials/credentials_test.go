package credentials

import (
	"context"
	"errors"
	"testing"

	keyring "github.com/zalando/go-keyring"
)

func TestStoreRoundTrip(t *testing.T) {
	var raw string
	store := NewStoreForTests(
		func(service, user string) (string, error) {
			if raw == "" {
				return "", keyring.ErrNotFound
			}
			return raw, nil
		},
		func(service, user, password string) error {
			raw = password
			return nil
		},
		func(service, user string) error {
			raw = ""
			return nil
		},
	)

	if err := store.Set(context.Background(), Credentials{Username: "demo", Password: "secret", APIKey: "qbt_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}); err != nil {
		t.Fatalf("set: %v", err)
	}

	got, err := store.Get(context.Background())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Username != "demo" || got.Password != "secret" || got.APIKey != "qbt_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("unexpected credentials: %#v", got)
	}

	if err := store.Delete(context.Background()); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if status := store.Status(context.Background()); status.State != StateAvailable {
		t.Fatalf("unexpected status after delete: %#v", status)
	}
}

func TestStoreReadsLegacyPayloadWithoutAPIKey(t *testing.T) {
	const raw = `{"username":"demo","password":"secret"}`
	store := NewStoreForTests(
		func(service, user string) (string, error) {
			return raw, nil
		},
		nil,
		nil,
	)

	got, err := store.Get(context.Background())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Username != "demo" || got.Password != "secret" || got.APIKey != "" {
		t.Fatalf("unexpected credentials: %#v", got)
	}
}

func TestStoreTrimsAPIKey(t *testing.T) {
	var raw string
	store := NewStoreForTests(
		func(service, user string) (string, error) {
			if raw == "" {
				return "", keyring.ErrNotFound
			}
			return raw, nil
		},
		func(service, user, password string) error {
			raw = password
			return nil
		},
		nil,
	)

	if err := store.Set(context.Background(), Credentials{APIKey: " qbt_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n"}); err != nil {
		t.Fatalf("set: %v", err)
	}

	got, err := store.Get(context.Background())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.APIKey != "qbt_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("unexpected trimmed API key: %q", got.APIKey)
	}
}

func TestStoreStatusLocked(t *testing.T) {
	store := NewStoreForTests(
		func(service, user string) (string, error) {
			return "", errors.New("collection is locked")
		},
		nil,
		nil,
	)

	status := store.Status(context.Background())
	if status.State != StateLocked {
		t.Fatalf("unexpected status: %#v", status)
	}
}

func TestStoreStatusUnsupported(t *testing.T) {
	store := NewStoreForTests(
		func(service, user string) (string, error) {
			return "", keyring.ErrUnsupportedPlatform
		},
		nil,
		nil,
	)

	status := store.Status(context.Background())
	if status.State != StateUnsupported {
		t.Fatalf("unexpected status: %#v", status)
	}
}

func TestDeleteIgnoresNotFound(t *testing.T) {
	store := NewStoreForTests(
		nil,
		nil,
		func(service, user string) error {
			return keyring.ErrNotFound
		},
	)

	if err := store.Delete(context.Background()); err != nil {
		t.Fatalf("delete: %v", err)
	}
}
