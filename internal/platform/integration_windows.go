//go:build windows

package platform

import (
	"fmt"
	"log/slog"

	"github.com/skobkin/qbtremotego/internal/config"
	"golang.org/x/sys/windows/registry"
)

func syncHandlers(exePath string, cfg config.IntegrationConfig, _ *slog.Logger) []error {
	var errs []error
	if err := syncMagnetHandler(exePath, cfg.RegisterMagnetHandler); err != nil {
		errs = append(errs, fmt.Errorf("magnet handler: %w", err))
	}
	if err := syncTorrentHandler(exePath, cfg.RegisterTorrentHandler); err != nil {
		errs = append(errs, fmt.Errorf(".torrent handler: %w", err))
	}

	return errs
}

func syncMagnetHandler(exePath string, enabled bool) error {
	if !enabled {
		return deleteKey(registry.CURRENT_USER, `Software\Classes\magnet`)
	}

	if err := writeString(registry.CURRENT_USER, `Software\Classes\magnet`, "", "URL:Magnet URI"); err != nil {
		return err
	}
	if err := writeString(registry.CURRENT_USER, `Software\Classes\magnet`, "URL Protocol", ""); err != nil {
		return err
	}
	return writeString(registry.CURRENT_USER, `Software\Classes\magnet\shell\open\command`, "", fmt.Sprintf(`"%s" "%%1"`, exePath))
}

func syncTorrentHandler(exePath string, enabled bool) error {
	if !enabled {
		_ = deleteKey(registry.CURRENT_USER, `Software\Classes\.torrent`)
		return deleteKey(registry.CURRENT_USER, `Software\Classes\qbtremotego.torrent`)
	}

	if err := writeString(registry.CURRENT_USER, `Software\Classes\.torrent`, "", "qbtremotego.torrent"); err != nil {
		return err
	}
	if err := writeString(registry.CURRENT_USER, `Software\Classes\qbtremotego.torrent`, "", "qBtRemoteGo Torrent"); err != nil {
		return err
	}
	return writeString(registry.CURRENT_USER, `Software\Classes\qbtremotego.torrent\shell\open\command`, "", fmt.Sprintf(`"%s" "%%1"`, exePath))
}

func syncAutostart(exePath string, enabled bool, _ *slog.Logger) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open Run key: %w", err)
	}
	defer key.Close()

	if !enabled {
		if err := key.DeleteValue("qBtRemoteGo"); err != nil && err != registry.ErrNotExist {
			return fmt.Errorf("delete autostart value: %w", err)
		}
		return nil
	}

	if err := key.SetStringValue("qBtRemoteGo", fmt.Sprintf(`"%s"`, exePath)); err != nil {
		return fmt.Errorf("set autostart value: %w", err)
	}

	return nil
}

func writeString(root registry.Key, path string, name string, value string) error {
	key, _, err := registry.CreateKey(root, path, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open registry key %s: %w", path, err)
	}
	defer key.Close()

	if err := key.SetStringValue(name, value); err != nil {
		return fmt.Errorf("set registry value %s/%s: %w", path, name, err)
	}
	return nil
}

func deleteKey(root registry.Key, path string) error {
	if err := registry.DeleteKey(root, path); err != nil && err != registry.ErrNotExist {
		return fmt.Errorf("delete key %s: %w", path, err)
	}
	return nil
}
