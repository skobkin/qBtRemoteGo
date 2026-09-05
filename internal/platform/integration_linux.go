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

const (
	handlerDesktopFileName       = "qbtremotego-handler.desktop"
	autostartDesktopFileName     = "qbtremotego.desktop"
	legacyHandlerDesktopFileName = "qbtremotego.desktop"
)

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
	desktopPath := filepath.Join(applicationsDir, handlerDesktopFileName)

	mimeTypes := enabledMimeTypes(cfg)
	logger.Debug("syncing linux desktop handlers", "desktop_path", desktopPath, "mime_types", mimeTypes)
	if len(mimeTypes) == 0 {
		if err := removeLegacyHandler(applicationsDir, logger); err != nil {
			return err
		}
		logger.Debug("removing linux desktop handler registration", "desktop_path", desktopPath)
		_ = os.Remove(desktopPath)

		return nil
	}

	if err := os.MkdirAll(applicationsDir, 0o750); err != nil {
		return fmt.Errorf("create applications dir: %w", err)
	}

	content := handlerDesktopContent(exePath, mimeTypes)

	// #nosec G306 -- desktop entries are expected to be readable by desktop integration tools.
	if err := os.WriteFile(desktopPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write desktop entry: %w", err)
	}
	logger.Debug("wrote linux desktop entry", "desktop_path", desktopPath)

	if err := removeLegacyHandler(applicationsDir, logger); err != nil {
		return err
	}

	if err := tryCommand(logger, "update-desktop-database", applicationsDir); err != nil {
		logger.Debug("update-desktop-database failed", "error", err)
	}
	for _, mimeType := range mimeTypes {
		if err := tryCommand(logger, "xdg-mime", "default", handlerDesktopFileName, mimeType); err != nil {
			logger.Debug("xdg-mime default failed", "mime_type", mimeType, "error", err)
			continue
		}
		logger.Debug("registered linux default handler", "desktop_file", handlerDesktopFileName, "mime_type", mimeType)
	}

	return nil
}

func handlerDesktopContent(exePath string, mimeTypes []string) string {
	return strings.Join([]string{
		"[Desktop Entry]",
		"Type=Application",
		"Name=qBtRemoteGo",
		"Exec=" + shellQuote(exePath) + " %U",
		"Terminal=false",
		"NoDisplay=true",
		"MimeType=" + strings.Join(mimeTypes, ";") + ";",
		"",
	}, "\n")
}

// Match the complete historical output, not just its name or NoDisplay flag.
// The executable may have moved since registration. Reject edited entries and
// non-regular files rather than risk removing a user's launcher.
func isLegacyHandler(content string) bool {
	lines := strings.Split(content, "\n")
	if len(lines) != 8 || !strings.HasPrefix(lines[3], "Exec=") || !strings.HasSuffix(lines[3], " %U") {
		return false
	}
	quotedPath := strings.TrimSuffix(strings.TrimPrefix(lines[3], "Exec="), " %U")
	exePath, err := strconv.Unquote(quotedPath)
	if err != nil || !filepath.IsAbs(exePath) {
		return false
	}
	for _, mimeTypes := range [][]string{
		{"x-scheme-handler/magnet"},
		{"application/x-bittorrent"},
		{"x-scheme-handler/magnet", "application/x-bittorrent"},
	} {
		if content == handlerDesktopContent(exePath, mimeTypes) {
			return true
		}
	}
	return false
}

func removeLegacyHandler(applicationsDir string, logger *slog.Logger) error {
	path := filepath.Join(applicationsDir, legacyHandlerDesktopFileName)
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect legacy desktop handler: %w", err)
	}
	if info.Mode().IsRegular() {
		// #nosec G304 -- path is the fixed legacy filename in the user's applications directory.
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read legacy desktop handler: %w", err)
		}
		if isLegacyHandler(string(data)) {
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("remove legacy desktop handler: %w", err)
			}
			logger.Info("removed legacy linux desktop handler", "desktop_path", path)
			return nil
		}
	}
	logger.Warn("preserving unrecognized user desktop entry; it may shadow the system launcher and needs manual review", "desktop_path", path)
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

func syncAutostart(exePath string, enabled bool, logger *slog.Logger) error {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("resolve user config dir: %w", err)
	}
	path := filepath.Join(configDir, "autostart", autostartDesktopFileName)
	if !enabled {
		logger.Debug("removing linux autostart entry", "path", path)
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
	logger.Debug("wrote linux autostart entry", "path", path, "exe_path", exePath)

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
