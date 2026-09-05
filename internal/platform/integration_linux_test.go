//go:build linux

package platform

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skobkin/qbtremotego/internal/config"
)

func TestSyncDesktopEntryEnabledMimeTypes(t *testing.T) {
	exePath := "/tmp/qbtremotego"
	tests := []struct {
		name string
		cfg  config.IntegrationConfig
		want string
	}{
		{
			name: "magnet only",
			cfg: config.IntegrationConfig{
				RegisterMagnetHandler: true,
			},
			want: "MimeType=x-scheme-handler/magnet;",
		},
		{
			name: "torrent only",
			cfg: config.IntegrationConfig{
				RegisterTorrentHandler: true,
			},
			want: "MimeType=application/x-bittorrent;",
		},
		{
			name: "magnet and torrent",
			cfg: config.IntegrationConfig{
				RegisterMagnetHandler:  true,
				RegisterTorrentHandler: true,
			},
			want: "MimeType=x-scheme-handler/magnet;application/x-bittorrent;",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dataDir := t.TempDir()
			t.Setenv("XDG_DATA_HOME", dataDir)
			t.Setenv("PATH", "")

			if err := syncDesktopEntry(exePath, tc.cfg, slog.Default()); err != nil {
				t.Fatalf("syncDesktopEntry() error = %v", err)
			}

			desktopPath := filepath.Join(dataDir, "applications", handlerDesktopFileName)
			// #nosec G304 -- desktopPath is built from the test temp dir and a fixed filename.
			data, err := os.ReadFile(desktopPath)
			if err != nil {
				t.Fatalf("read desktop entry: %v", err)
			}
			if !strings.Contains(string(data), tc.want) {
				t.Fatalf("desktop entry = %q, want substring %q", string(data), tc.want)
			}
		})
	}
}

func TestSyncDesktopEntryRemovesFileWhenHandlersDisabled(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataDir)
	t.Setenv("PATH", "")

	logger := slog.Default()
	exePath := "/tmp/qbtremotego"
	if err := syncDesktopEntry(exePath, config.IntegrationConfig{
		RegisterMagnetHandler: true,
	}, logger); err != nil {
		t.Fatalf("syncDesktopEntry(enable) error = %v", err)
	}

	if err := syncDesktopEntry(exePath, config.IntegrationConfig{}, logger); err != nil {
		t.Fatalf("syncDesktopEntry(disable) error = %v", err)
	}

	desktopPath := filepath.Join(dataDir, "applications", handlerDesktopFileName)
	if _, err := os.Stat(desktopPath); !os.IsNotExist(err) {
		t.Fatalf("desktop entry still exists: err = %v", err)
	}
}

func TestLegacyHandlerMigration(t *testing.T) {
	const legacy = "[Desktop Entry]\nType=Application\nName=qBtRemoteGo\nExec=\"/old location/qbtremotego\" %U\nTerminal=false\nNoDisplay=true\nMimeType=x-scheme-handler/magnet;application/x-bittorrent;\n"
	tests := []struct {
		name    string
		content string
		symlink bool
		remove  bool
	}{
		{name: "both", content: legacy, remove: true},
		{name: "magnet", content: strings.ReplaceAll(legacy, "application/x-bittorrent;", ""), remove: true},
		{name: "torrent", content: strings.ReplaceAll(legacy, "x-scheme-handler/magnet;", ""), remove: true},
		{name: "visible launcher", content: strings.ReplaceAll(legacy, "NoDisplay=true\n", "")},
		{name: "customized", content: legacy + "Icon=custom\n"},
		{name: "other MIME", content: strings.ReplaceAll(legacy, "application/x-bittorrent;", "text/plain;")},
		{name: "relative executable", content: strings.ReplaceAll(legacy, "/old location/qbtremotego", "qbtremotego")},
		{name: "symlink", content: legacy, symlink: true},
		{name: "arbitrary file", content: "user content"},
	}
	for _, tc := range tests {
		for _, enabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/enabled=%t", tc.name, enabled), func(t *testing.T) {
				dataDir := t.TempDir()
				t.Setenv("XDG_DATA_HOME", dataDir)
				t.Setenv("PATH", "")
				applicationsDir := filepath.Join(dataDir, "applications")
				if err := os.MkdirAll(applicationsDir, 0o750); err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(applicationsDir, "qbtremotego.desktop")
				target := path
				if tc.symlink {
					target = filepath.Join(dataDir, "user.desktop")
				}
				if err := os.WriteFile(target, []byte(tc.content), 0o600); err != nil {
					t.Fatal(err)
				}
				if tc.symlink {
					if err := os.Symlink(target, path); err != nil {
						t.Fatal(err)
					}
				}
				if err := syncDesktopEntry("/new/qbtremotego", config.IntegrationConfig{RegisterMagnetHandler: enabled}, slog.Default()); err != nil {
					t.Fatal(err)
				}
				if tc.remove {
					if _, err := os.Lstat(path); !os.IsNotExist(err) {
						t.Fatalf("legacy entry was not removed: %v", err)
					}
				} else {
					// #nosec G304 -- path uses a test temp directory and a fixed desktop filename.
					data, err := os.ReadFile(path)
					if err != nil || string(data) != tc.content {
						t.Fatalf("user entry changed: %q, %v", data, err)
					}
				}
			})
		}
	}
}

func TestSyncDesktopEntryRegistersHandlerName(t *testing.T) {
	dataDir := t.TempDir()
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "commands")
	t.Setenv("XDG_DATA_HOME", dataDir)
	t.Setenv("PATH", binDir)
	t.Setenv("COMMAND_LOG", logPath)
	for _, name := range []string{"xdg-mime", "update-desktop-database"} {
		script := "#!/bin/sh\nprintf '%s\\n' \"$0 $*\" >> \"$COMMAND_LOG\"\n"
		// #nosec G306 -- the stub must be executable; binDir is a test temp directory.
		if err := os.WriteFile(filepath.Join(binDir, name), []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := syncDesktopEntry("/tmp/qbtremotego", config.IntegrationConfig{
		RegisterMagnetHandler: true, RegisterTorrentHandler: true,
	}, slog.Default()); err != nil {
		t.Fatal(err)
	}
	// #nosec G304 -- logPath is a fixed filename inside a test temp directory.
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(binDir, "update-desktop-database") + " " + filepath.Join(dataDir, "applications") + "\n" +
		filepath.Join(binDir, "xdg-mime") + " default qbtremotego-handler.desktop x-scheme-handler/magnet\n" +
		filepath.Join(binDir, "xdg-mime") + " default qbtremotego-handler.desktop application/x-bittorrent\n"
	if string(data) != want {
		t.Fatalf("commands = %q, want %q", data, want)
	}
}

func TestSyncAutostartKeepsApplicationName(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configDir)
	path := filepath.Join(configDir, "autostart", "qbtremotego.desktop")
	if err := syncAutostart("/tmp/qbtremotego", true, slog.Default()); err != nil {
		t.Fatal(err)
	}
	// #nosec G304 -- path uses a test temp directory and a fixed desktop filename.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "X-GNOME-Autostart-enabled=true\n") || strings.Contains(string(data), "MimeType=") {
		t.Fatalf("unexpected autostart content: %q", data)
	}
	if _, err := os.Stat(filepath.Join(configDir, "autostart", "qbtremotego-handler.desktop")); !os.IsNotExist(err) {
		t.Fatalf("unexpected handler-named autostart: %v", err)
	}
	if err := syncAutostart("/tmp/qbtremotego", false, slog.Default()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("autostart entry was not removed: %v", err)
	}
}
