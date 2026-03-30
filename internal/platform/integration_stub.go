//go:build !linux && !windows

package platform

import (
	"log/slog"

	"github.com/skobkin/qbtremotego/internal/config"
)

func syncHandlers(_ string, _ config.IntegrationConfig, _ *slog.Logger) []error { return nil }

func syncAutostart(_ string, _ bool, _ *slog.Logger) error { return nil }
