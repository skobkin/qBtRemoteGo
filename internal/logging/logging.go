package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/skobkin/qbtremotego/internal/config"
)

// Manager owns app logger configuration and the optional log file lifecycle.
type Manager struct {
	mu     sync.RWMutex
	logger *slog.Logger
	file   *os.File
}

var supportedLevels = []string{"debug", "info", "warn", "error"}

// New constructs a Manager and configures it to log to stdout only.
// It is a thin wrapper around Configure used at startup, before the
// application knows whether the user wants file logging.
func New(cfg config.LoggingConfig) (*Manager, error) {
	m := &Manager{}
	if err := m.Configure(cfg, ""); err != nil {
		return nil, err
	}
	return m, nil
}

// Configure (re)builds the underlying slog logger. When cfg.LogToFile
// is true and filePath is non-empty, the log file is opened in append
// mode (0600) and writes are mirrored to stdout via fanoutWriter. When
// LogToFile is false, only stdout is used. Calling Configure again
// closes any previously-opened log file before opening a new one.
func (m *Manager) Configure(cfg config.LoggingConfig, filePath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.file != nil {
		_ = m.file.Close()
		m.file = nil
	}

	level, err := parseLevel(cfg.Level)
	if err != nil {
		return err
	}

	writer := io.Writer(os.Stdout)
	if cfg.LogToFile && filePath != "" {
		cleanPath := filepath.Clean(filePath)
		// #nosec G304 -- path is resolved by app runtime and points to user config dir.
		file, err := os.OpenFile(cleanPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return fmt.Errorf("open log file: %w", err)
		}
		m.file = file
		writer = newFanoutWriter(os.Stdout, file)
	}

	h := slog.NewTextHandler(writer, &slog.HandlerOptions{Level: level})
	m.logger = slog.New(h)
	slog.SetDefault(m.logger)

	return nil
}

// Logger returns the default logger with a component tag attached.
func (m *Manager) Logger(component string) *slog.Logger {
	if m == nil || m.logger == nil {
		return slog.Default().With("component", component)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.logger.With("component", component)
}

// Close releases the log file (if any). Safe to call when no file is open.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.file != nil {
		if err := m.file.Close(); err != nil {
			return err
		}
		m.file = nil
	}

	return nil
}

func SupportedLevels() []string {
	return append([]string(nil), supportedLevels...)
}

func NormalizeLevel(raw string) string {
	level := strings.ToLower(strings.TrimSpace(raw))
	if level == "" {
		return "info"
	}
	if slices.Contains(supportedLevels, level) {
		return level
	}

	return ""
}

func parseLevel(raw string) (slog.Leveler, error) {
	switch NormalizeLevel(raw) {
	case "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return nil, fmt.Errorf("unsupported log level %q", raw)
	}
}
