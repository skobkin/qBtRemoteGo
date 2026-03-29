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

	var errs []error
	if err := syncMagnetHandler(exePath, cfg.RegisterMagnetHandler, m.logger); err != nil {
		errs = append(errs, fmt.Errorf("magnet handler: %w", err))
	}
	if err := syncTorrentHandler(exePath, cfg.RegisterTorrentHandler, m.logger); err != nil {
		errs = append(errs, fmt.Errorf(".torrent handler: %w", err))
	}
	if err := syncAutostart(exePath, cfg.StartWithSystem, m.logger); err != nil {
		errs = append(errs, fmt.Errorf("autostart: %w", err))
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
