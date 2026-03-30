package platform

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/skobkin/qbtremotego/internal/config"
)

type Manager struct {
	logger *slog.Logger
}

func NewManager(logger *slog.Logger) *Manager {
	return &Manager{logger: logger}
}

func (m *Manager) Sync(cfg config.IntegrationConfig) []error {
	exePath, err := os.Executable()
	if err != nil {
		return []error{fmt.Errorf("resolve executable path: %w", err)}
	}

	m.logger.Info(
		"syncing desktop integrations",
		"exe_path", exePath,
		"magnet_handler", cfg.RegisterMagnetHandler,
		"torrent_handler", cfg.RegisterTorrentHandler,
		"autostart", cfg.StartWithSystem,
	)

	errs := syncHandlers(exePath, cfg, m.logger)
	if err := syncAutostart(exePath, cfg.StartWithSystem, m.logger); err != nil {
		errs = append(errs, fmt.Errorf("autostart: %w", err))
	}

	if len(errs) == 0 {
		m.logger.Info("desktop integrations synced successfully")
	} else {
		m.logger.Info("desktop integrations synced with warnings", "warnings", JoinErrors(errs))
	}

	return errs
}

func JoinErrors(errs []error) string {
	if len(errs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(errs))
	for _, err := range errs {
		if err != nil {
			parts = append(parts, err.Error())
		}
	}

	return strings.Join(parts, "\n")
}
