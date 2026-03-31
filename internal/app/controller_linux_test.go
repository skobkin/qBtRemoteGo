//go:build linux

package app

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/skobkin/qbtremotego/internal/config"
	"github.com/skobkin/qbtremotego/internal/credentials"
	keyring "github.com/zalando/go-keyring"
)

func TestSaveLocalUIDoesNotSyncDesktopIntegrations(t *testing.T) {
	configHome := t.TempDir()
	dataHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("PATH", "")

	cfg := config.Default()
	cfg.Integration.RegisterMagnetHandler = true
	cfg.Integration.StartWithSystem = true

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := config.Save(path, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	controller, err := newController(path, slog.New(slog.NewTextHandler(io.Discard, nil)), credentials.NewStoreForTests(
		func(service, user string) (string, error) { return "", keyring.ErrNotFound },
		func(service, user, password string) error { return nil },
		func(service, user string) error { return nil },
	))
	if err != nil {
		t.Fatalf("new controller: %v", err)
	}

	updated := controller.Config()
	updated.UI.FilterBy = "location"
	if err := controller.SaveLocalUI(updated); err != nil {
		t.Fatalf("SaveLocalUI() error = %v", err)
	}

	desktopPath := filepath.Join(dataHome, "applications", "qbtremotego.desktop")
	if _, err := os.Stat(desktopPath); !os.IsNotExist(err) {
		t.Fatalf("desktop entry should not be created by SaveLocalUI: err = %v", err)
	}

	autostartPath := filepath.Join(configHome, "autostart", "qbtremotego.desktop")
	if _, err := os.Stat(autostartPath); !os.IsNotExist(err) {
		t.Fatalf("autostart entry should not be created by SaveLocalUI: err = %v", err)
	}
}
