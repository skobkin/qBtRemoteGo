package platform

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const activationStateFilename = "activation.json"

type ActivationServer struct {
	server    *http.Server
	listener  net.Listener
	statePath string
	closeOnce sync.Once
}

type activationState struct {
	Address string `json:"address"`
	Token   string `json:"token"`
}

type activationRequest struct {
	Args []string `json:"args"`
}

func StartActivationServer(appID string, logger *slog.Logger, handler func([]string)) (*ActivationServer, error) {
	statePath, err := activationStatePath(appID)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(statePath), 0o700); err != nil {
		return nil, fmt.Errorf("create activation state dir: %w", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen for activations: %w", err)
	}

	token, err := randomToken()
	if err != nil {
		_ = listener.Close()
		return nil, err
	}

	state := activationState{
		Address: listener.Addr().String(),
		Token:   token,
	}
	if err := writeActivationState(statePath, state); err != nil {
		_ = listener.Close()
		return nil, err
	}

	server := &ActivationServer{
		statePath: statePath,
		listener:  listener,
	}
	server.server = &http.Server{
		ReadHeaderTimeout: 2 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || r.URL.Path != "/activate" {
				http.NotFound(w, r)
				return
			}
			if r.Header.Get("X-QBtRemoteGo-Token") != state.Token {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}

			defer func() { _ = r.Body.Close() }()
			var req activationRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}

			handler(req.Args)
			w.WriteHeader(http.StatusNoContent)
		}),
	}

	go func() {
		if err := server.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Warn("activation server stopped", "error", err)
		}
	}()

	return server, nil
}

func (s *ActivationServer) Close() error {
	if s == nil {
		return nil
	}

	var closeErr error
	s.closeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		shutdownErr := s.server.Shutdown(ctx)
		removeErr := os.Remove(s.statePath)
		if shutdownErr != nil && !errors.Is(shutdownErr, http.ErrServerClosed) {
			closeErr = fmt.Errorf("shutdown activation server: %w", shutdownErr)
			return
		}
		if removeErr != nil && !os.IsNotExist(removeErr) {
			closeErr = fmt.Errorf("remove activation state: %w", removeErr)
		}
	})

	return closeErr
}

func ForwardActivation(appID string, args []string) error {
	statePath, err := activationStatePath(appID)
	if err != nil {
		return err
	}

	state, err := readActivationState(statePath)
	if err != nil {
		return err
	}

	body, err := json.Marshal(activationRequest{Args: args})
	if err != nil {
		return fmt.Errorf("encode activation request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, "http://"+state.Address+"/activate", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create activation request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-QBtRemoteGo-Token", state.Token)

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send activation request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("activation request failed: status %d: %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}

	return nil
}

func activationStatePath(appID string) (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}

	return filepath.Join(configDir, normalizeInstanceLockComponent(appID, "app"), activationStateFilename), nil
}

func writeActivationState(path string, state activationState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode activation state: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write activation state: %w", err)
	}

	return nil
}

func readActivationState(path string) (activationState, error) {
	// #nosec G304 -- activation state path is derived from the user's config directory and app-owned filenames.
	data, err := os.ReadFile(path)
	if err != nil {
		return activationState{}, fmt.Errorf("read activation state: %w", err)
	}

	var state activationState
	if err := json.Unmarshal(data, &state); err != nil {
		return activationState{}, fmt.Errorf("decode activation state: %w", err)
	}
	if strings.TrimSpace(state.Address) == "" || strings.TrimSpace(state.Token) == "" {
		return activationState{}, fmt.Errorf("activation state is incomplete")
	}

	return state, nil
}

func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate activation token: %w", err)
	}

	return hex.EncodeToString(buf), nil
}
