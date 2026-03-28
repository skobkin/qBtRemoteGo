package logging

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

func New(cfg config.LoggingConfig) (*Manager, error) {
	level, err := parseLevel(cfg.Level)
	if err != nil {
		return nil, err
	}

	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	logger := slog.New(handler)
	slog.SetDefault(logger)

	return &Manager{logger: logger}, nil
}

func (m *Manager) Logger(component string) *slog.Logger {
	if m == nil || m.logger == nil {
		return slog.Default().With("component", component)
	}
	return m.logger.With("component", component)
}

func parseLevel(raw string) (slog.Leveler, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return nil, fmt.Errorf("unsupported log level %q", raw)
	}
}
