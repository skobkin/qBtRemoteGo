//go:build linux

package platform

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/skobkin/qbtremotego/internal/config"
)

const desktopFileName = "qbtremotego.desktop"

func syncHandlers(exePath string, cfg config.IntegrationConfig, logger *slog.Logger) []error {
	if err := syncDesktopEntry(exePath, cfg, logger); err != nil {
		return []error{err}
	}

	return nil
}

func syncDesktopEntry(exePath string, cfg config.IntegrationConfig, logger *slog.Logger) error {
	applicationsDir, err := userApplicationsDir()
	if err != nil {
		return err
	}
	desktopPath := filepath.Join(applicationsDir, desktopFileName)

	mimeTypes := enabledMimeTypes(cfg)
	if len(mimeTypes) == 0 {
		_ = os.Remove(desktopPath)

		return nil
	}

	if err := os.MkdirAll(applicationsDir, 0o750); err != nil {
		return fmt.Errorf("create applications dir: %w", err)
	}

	content := strings.Join([]string{
		"[Desktop Entry]",
		"Type=Application",
		"Name=qBtRemoteGo",
		"Exec=" + shellQuote(exePath) + " %U",
		"Terminal=false",
		"NoDisplay=true",
		"MimeType=" + strings.Join(mimeTypes, ";") + ";",
		"",
	}, "\n")

	// #nosec G306 -- desktop entries are expected to be readable by desktop integration tools.
	if err := os.WriteFile(desktopPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write desktop entry: %w", err)
	}

	if err := tryCommand(logger, "update-desktop-database", applicationsDir); err != nil {
		logger.Debug("update-desktop-database failed", "error", err)
	}
	for _, mimeType := range mimeTypes {
		if err := tryCommand(logger, "xdg-mime", "default", desktopFileName, mimeType); err != nil {
			logger.Debug("xdg-mime default failed", "mime_type", mimeType, "error", err)
		}
	}

	return nil
}

func enabledMimeTypes(cfg config.IntegrationConfig) []string {
	mimeTypes := make([]string, 0, 2)
	if cfg.RegisterMagnetHandler {
		mimeTypes = append(mimeTypes, "x-scheme-handler/magnet")
	}
	if cfg.RegisterTorrentHandler {
		mimeTypes = append(mimeTypes, "application/x-bittorrent")
	}

	return mimeTypes
}

func syncAutostart(exePath string, enabled bool, _ *slog.Logger) error {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("resolve user config dir: %w", err)
	}
	path := filepath.Join(configDir, "autostart", desktopFileName)
	if !enabled {
		_ = os.Remove(path)

		return nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create autostart dir: %w", err)
	}

	content := strings.Join([]string{
		"[Desktop Entry]",
		"Type=Application",
		"Name=qBtRemoteGo",
		"Exec=" + shellQuote(exePath),
		"Terminal=false",
		"X-GNOME-Autostart-enabled=true",
		"",
	}, "\n")

	// #nosec G306 -- autostart desktop entries are expected to be readable by the desktop session.
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write autostart entry: %w", err)
	}

	return nil
}

func tryCommand(_ *slog.Logger, name string, args ...string) error {
	bin, err := exec.LookPath(name)
	if err != nil {
		return err
	}
	// #nosec G204 -- command name is resolved from a fixed allowlist in this package.
	cmd := exec.Command(bin, args...)

	return cmd.Run()
}

func userApplicationsDir() (string, error) {
	dataDir := os.Getenv("XDG_DATA_HOME")
	if dataDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir: %w", err)
		}
		dataDir = filepath.Join(home, ".local", "share")
	}

	return filepath.Join(dataDir, "applications"), nil
}

func shellQuote(value string) string {
	return strconv.Quote(value)
}
