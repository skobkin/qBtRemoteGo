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
)

const desktopFileName = "qbtremotego.desktop"

func syncMagnetHandler(exePath string, enabled bool, logger *slog.Logger) error {
	return syncDesktopEntry(exePath, enabled, logger)
}

func syncTorrentHandler(exePath string, enabled bool, logger *slog.Logger) error {
	return syncDesktopEntry(exePath, enabled, logger)
}

func syncDesktopEntry(exePath string, enabled bool, logger *slog.Logger) error {
	applicationsDir, err := userApplicationsDir()
	if err != nil {
		return err
	}
	desktopPath := filepath.Join(applicationsDir, desktopFileName)

	if !enabled {
		_ = os.Remove(desktopPath)
		return nil
	}

	if err := os.MkdirAll(applicationsDir, 0o755); err != nil {
		return fmt.Errorf("create applications dir: %w", err)
	}

	content := strings.Join([]string{
		"[Desktop Entry]",
		"Type=Application",
		"Name=qBtRemoteGo",
		"Exec=" + shellQuote(exePath) + " %U",
		"Terminal=false",
		"NoDisplay=true",
		"MimeType=x-scheme-handler/magnet;application/x-bittorrent;",
		"",
	}, "\n")

	if err := os.WriteFile(desktopPath, []byte(content), 0o755); err != nil {
		return fmt.Errorf("write desktop entry: %w", err)
	}

	if err := tryCommand(logger, "update-desktop-database", applicationsDir); err != nil {
		logger.Debug("update-desktop-database failed", "error", err)
	}
	if err := tryCommand(logger, "xdg-mime", "default", desktopFileName, "x-scheme-handler/magnet"); err != nil {
		logger.Debug("xdg-mime default magnet failed", "error", err)
	}
	if err := tryCommand(logger, "xdg-mime", "default", desktopFileName, "application/x-bittorrent"); err != nil {
		logger.Debug("xdg-mime default bittorrent failed", "error", err)
	}

	return nil
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

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
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

	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		return fmt.Errorf("write autostart entry: %w", err)
	}

	return nil
}

func tryCommand(_ *slog.Logger, name string, args ...string) error {
	bin, err := exec.LookPath(name)
	if err != nil {
		return err
	}
	cmd := exec.Command(bin, args...)
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
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
