//go:build windows

package platform

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/skobkin/qbtremotego/internal/config"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func syncHandlers(exePath string, cfg config.IntegrationConfig, logger *slog.Logger) []error {
	var errs []error
	associationsChanged := false

	if err := syncMagnetHandler(exePath, cfg.RegisterMagnetHandler); err != nil {
		errs = append(errs, fmt.Errorf("magnet handler: %w", err))
	} else {
		associationsChanged = true
		logger.Debug("synced windows magnet handler", "enabled", cfg.RegisterMagnetHandler, "exe_path", exePath)
	}
	if err := syncTorrentHandler(exePath, cfg.RegisterTorrentHandler); err != nil {
		errs = append(errs, fmt.Errorf(".torrent handler: %w", err))
	} else {
		associationsChanged = true
		logger.Debug("synced windows torrent handler", "enabled", cfg.RegisterTorrentHandler, "exe_path", exePath)
	}
	if err := syncDefaultAppsRegistration(exePath, cfg); err != nil {
		errs = append(errs, fmt.Errorf("default apps registration: %w", err))
	} else {
		associationsChanged = true
		logger.Debug(
			"synced windows default apps registration",
			"magnet_enabled", cfg.RegisterMagnetHandler,
			"torrent_enabled", cfg.RegisterTorrentHandler,
		)
	}
	if associationsChanged {
		notifyAssociationChanged()
	}
	if warning := defaultHandlerWarning(
		"magnet links",
		cfg.RegisterMagnetHandler,
		windowsMagnetUserChoicePath,
		windowsMagnetProgID,
	); warning != nil {
		errs = append(errs, fmt.Errorf("magnet handler: %w", warning))
	}
	if warning := defaultHandlerWarning(
		".torrent files",
		cfg.RegisterTorrentHandler,
		windowsTorrentUserChoicePath,
		windowsTorrentProgID,
	); warning != nil {
		errs = append(errs, fmt.Errorf(".torrent handler: %w", warning))
	}

	return errs
}

func syncMagnetHandler(exePath string, enabled bool) error {
	command := windowsHandlerCommand(exePath)
	icon := windowsDefaultIcon(exePath)

	if !enabled {
		if err := deleteKeyTree(registry.CURRENT_USER, windowsMagnetSchemePath); err != nil {
			return err
		}
		return deleteKeyTree(registry.CURRENT_USER, windowsMagnetProgIDPath)
	}

	if err := writeProtocolClass(registry.CURRENT_USER, windowsMagnetSchemePath, "URL:Magnet URI", command, icon); err != nil {
		return err
	}

	return writeProtocolClass(registry.CURRENT_USER, windowsMagnetProgIDPath, "qBtRemoteGo Magnet Link", command, icon)
}

func syncTorrentHandler(exePath string, enabled bool) error {
	command := windowsHandlerCommand(exePath)
	icon := windowsDefaultIcon(exePath)

	if !enabled {
		if err := deleteKeyTree(registry.CURRENT_USER, windowsTorrentExtensionPath); err != nil {
			return err
		}
		return deleteKeyTree(registry.CURRENT_USER, windowsTorrentProgIDPath)
	}

	if err := writeString(registry.CURRENT_USER, windowsTorrentExtensionPath, "", windowsTorrentProgID); err != nil {
		return err
	}

	return writeFileClass(registry.CURRENT_USER, windowsTorrentProgIDPath, "qBtRemoteGo Torrent", command, icon)
}

func syncAutostart(exePath string, enabled bool, logger *slog.Logger) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open Run key: %w", err)
	}
	defer key.Close()

	if !enabled {
		if err := key.DeleteValue("qBtRemoteGo"); err != nil && err != registry.ErrNotExist {
			return fmt.Errorf("delete autostart value: %w", err)
		}
		logger.Debug("removed windows autostart entry")
		return nil
	}

	if err := key.SetStringValue("qBtRemoteGo", fmt.Sprintf(`"%s"`, exePath)); err != nil {
		return fmt.Errorf("set autostart value: %w", err)
	}
	logger.Debug("wrote windows autostart entry", "exe_path", exePath)

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

func writeProtocolClass(root registry.Key, path string, title string, command string, icon string) error {
	if err := writeString(root, path, "", title); err != nil {
		return err
	}
	if err := writeString(root, path, "URL Protocol", ""); err != nil {
		return err
	}
	if err := writeString(root, path+`\DefaultIcon`, "", icon); err != nil {
		return err
	}
	return writeString(root, path+`\shell\open\command`, "", command)
}

func writeFileClass(root registry.Key, path string, title string, command string, icon string) error {
	if err := writeString(root, path, "", title); err != nil {
		return err
	}
	if err := writeString(root, path+`\DefaultIcon`, "", icon); err != nil {
		return err
	}
	return writeString(root, path+`\shell\open\command`, "", command)
}

func syncDefaultAppsRegistration(exePath string, cfg config.IntegrationConfig) error {
	if !cfg.RegisterMagnetHandler && !cfg.RegisterTorrentHandler {
		if err := deleteValue(registry.CURRENT_USER, windowsRegisteredApplicationsPath, windowsAppName); err != nil {
			return err
		}
		return deleteKeyTree(registry.CURRENT_USER, windowsAppRegistrationPath)
	}

	if err := writeString(registry.CURRENT_USER, windowsCapabilitiesPath, "ApplicationName", windowsAppName); err != nil {
		return err
	}
	if err := writeString(
		registry.CURRENT_USER,
		windowsCapabilitiesPath,
		"ApplicationDescription",
		"Open magnet links and .torrent files in qBtRemoteGo.",
	); err != nil {
		return err
	}
	if err := writeString(
		registry.CURRENT_USER,
		windowsCapabilitiesPath,
		"ApplicationIcon",
		windowsDefaultIcon(exePath),
	); err != nil {
		return err
	}

	if cfg.RegisterMagnetHandler {
		if err := writeString(registry.CURRENT_USER, windowsCapabilitiesPath+`\UrlAssociations`, "magnet", windowsMagnetProgID); err != nil {
			return err
		}
	} else if err := deleteKeyTree(registry.CURRENT_USER, windowsCapabilitiesPath+`\UrlAssociations`); err != nil {
		return err
	}

	if cfg.RegisterTorrentHandler {
		if err := writeString(registry.CURRENT_USER, windowsCapabilitiesPath+`\FileAssociations`, ".torrent", windowsTorrentProgID); err != nil {
			return err
		}
	} else if err := deleteKeyTree(registry.CURRENT_USER, windowsCapabilitiesPath+`\FileAssociations`); err != nil {
		return err
	}

	return writeString(registry.CURRENT_USER, windowsRegisteredApplicationsPath, windowsAppName, windowsCapabilitiesPath)
}

func deleteValue(root registry.Key, path string, name string) error {
	key, err := registry.OpenKey(root, path, registry.SET_VALUE)
	if err != nil {
		if err == registry.ErrNotExist {
			return nil
		}
		return fmt.Errorf("open registry key %s: %w", path, err)
	}
	defer key.Close()

	if err := key.DeleteValue(name); err != nil && err != registry.ErrNotExist {
		return fmt.Errorf("delete registry value %s/%s: %w", path, name, err)
	}
	return nil
}

func deleteKeyTree(root registry.Key, path string) error {
	key, err := registry.OpenKey(root, path, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		if err == registry.ErrNotExist {
			return nil
		}
		return fmt.Errorf("open registry key %s: %w", path, err)
	}

	subKeys, err := key.ReadSubKeyNames(-1)
	key.Close()
	if err != nil {
		return fmt.Errorf("list registry subkeys %s: %w", path, err)
	}

	for _, subKey := range subKeys {
		if err := deleteKeyTree(root, path+`\`+subKey); err != nil {
			return err
		}
	}

	if err := registry.DeleteKey(root, path); err != nil && err != registry.ErrNotExist {
		return fmt.Errorf("delete key %s: %w", path, err)
	}
	return nil
}

func defaultHandlerWarning(subject string, enabled bool, userChoicePath string, expectedProgID string) error {
	if !enabled {
		return nil
	}

	currentProgID, err := currentUserChoiceProgID(registry.CURRENT_USER, userChoicePath)
	if err != nil {
		return fmt.Errorf("inspect current default: %w", err)
	}
	if currentProgID == "" || isOurWindowsHandlerProgID(expectedProgID, currentProgID) {
		return nil
	}

	return errors.New(windowsDefaultSelectionWarning(subject, currentProgID))
}

func currentUserChoiceProgID(root registry.Key, path string) (string, error) {
	key, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		if err == registry.ErrNotExist {
			return "", nil
		}
		return "", fmt.Errorf("open registry key %s: %w", path, err)
	}
	defer key.Close()

	value, _, err := key.GetStringValue("ProgId")
	if err != nil {
		if err == registry.ErrNotExist {
			return "", nil
		}
		return "", fmt.Errorf("read registry value %s/ProgId: %w", path, err)
	}

	return strings.TrimSpace(value), nil
}

var shChangeNotify = windows.NewLazySystemDLL("shell32.dll").NewProc("SHChangeNotify")

func notifyAssociationChanged() {
	const (
		shcneAssocChanged = 0x08000000
		shcnfIDList       = 0x0000
	)

	_, _, _ = shChangeNotify.Call(shcneAssocChanged, shcnfIDList, 0, 0)
}
