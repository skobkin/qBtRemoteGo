package credentials

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strings"

	keyring "github.com/zalando/go-keyring"
)

const (
	serviceName = "qbtremotego"
	accountName = "connection/default"
)

type State string

const (
	StateAvailable   State = "available"
	StateLocked      State = "locked"
	StateUnavailable State = "unavailable"
	StateUnsupported State = "unsupported"
)

type Credentials struct {
	Username string
	Password string
}

type Status struct {
	Backend string
	State   State
	Message string
}

type Store interface {
	Status(context.Context) Status
	Get(context.Context) (Credentials, error)
	Set(context.Context, Credentials) error
	Delete(context.Context) error
}

type Error struct {
	state State
	err   error
}

func (e *Error) Error() string {
	if e == nil || e.err == nil {
		return ""
	}

	return e.err.Error()
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}

	return e.err
}

func (e *Error) State() State {
	if e == nil {
		return StateUnavailable
	}

	return e.state
}

type keyringStore struct {
	get func(service, user string) (string, error)
	set func(service, user, password string) error
	del func(service, user string) error
}

type payload struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func NewStore() Store {
	return &keyringStore{
		get: keyring.Get,
		set: keyring.Set,
		del: keyring.Delete,
	}
}

func NewStoreForTests(
	get func(service, user string) (string, error),
	set func(service, user, password string) error,
	del func(service, user string) error,
) Store {
	return &keyringStore{get: get, set: set, del: del}
}

func (s *keyringStore) Status(ctx context.Context) Status {
	_, err := s.Get(ctx)
	switch {
	case err == nil:
		return Status{
			Backend: backendName(),
			State:   StateAvailable,
			Message: "System keychain is available.",
		}
	case errors.Is(err, keyring.ErrNotFound):
		return Status{
			Backend: backendName(),
			State:   StateAvailable,
			Message: "System keychain is available.",
		}
	default:
		var credErr *Error
		if errors.As(err, &credErr) {
			return statusForState(backendName(), credErr.State(), credErr.Error())
		}

		return statusForState(backendName(), StateUnavailable, err.Error())
	}
}

func (s *keyringStore) Get(_ context.Context) (Credentials, error) {
	raw, err := s.get(serviceName, accountName)
	if err != nil {
		return Credentials{}, classifyError("read keychain credentials", err)
	}

	var stored payload
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return Credentials{}, fmt.Errorf("decode keychain credentials: %w", err)
	}

	return Credentials(stored), nil
}

func (s *keyringStore) Set(_ context.Context, creds Credentials) error {
	//nolint:gosec // Password must be serialized before handing it to the system keychain backend.
	data, err := json.Marshal(payload{
		Username: strings.TrimSpace(creds.Username),
		Password: creds.Password,
	})
	if err != nil {
		return fmt.Errorf("encode keychain credentials: %w", err)
	}

	if err := s.set(serviceName, accountName, string(data)); err != nil {
		return classifyError("write keychain credentials", err)
	}

	return nil
}

func (s *keyringStore) Delete(_ context.Context) error {
	err := s.del(serviceName, accountName)
	if err == nil || errors.Is(err, keyring.ErrNotFound) {
		return nil
	}

	return classifyError("delete keychain credentials", err)
}

func statusForState(backend string, state State, detail string) Status {
	message := detail
	if message == "" {
		switch state {
		case StateLocked:
			message = "System keychain is locked."
		case StateUnsupported:
			message = "System keychain is not supported on this platform."
		default:
			message = "System keychain is unavailable."
		}
	}

	return Status{
		Backend: backend,
		State:   state,
		Message: message,
	}
}

func classifyError(op string, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, keyring.ErrNotFound):
		return err
	case errors.Is(err, keyring.ErrUnsupportedPlatform):
		return &Error{
			state: StateUnsupported,
			err:   fmt.Errorf("%s: %w", op, err),
		}
	default:
		text := strings.ToLower(err.Error())
		state := StateUnavailable
		if strings.Contains(text, "locked") || strings.Contains(text, "unlock") || strings.Contains(text, "prompt") {
			state = StateLocked
		}

		return &Error{
			state: state,
			err:   fmt.Errorf("%s: %w", op, err),
		}
	}
}

func backendName() string {
	switch runtime.GOOS {
	case "windows":
		return "Windows Credential Manager"
	case "linux", "freebsd", "openbsd", "netbsd", "dragonfly":
		return "Secret Service"
	default:
		return "System keychain"
	}
}
