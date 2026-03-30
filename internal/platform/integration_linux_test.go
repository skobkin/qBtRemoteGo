//go:build linux

package platform

import (
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

			desktopPath := filepath.Join(dataDir, "applications", desktopFileName)
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

	desktopPath := filepath.Join(dataDir, "applications", desktopFileName)
	if _, err := os.Stat(desktopPath); !os.IsNotExist(err) {
		t.Fatalf("desktop entry still exists: err = %v", err)
	}
}
