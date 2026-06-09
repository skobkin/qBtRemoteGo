package app

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/skobkin/qbtremotego/internal/config"
)

// Paths stores resolved runtime file locations for the application.
type Paths struct {
	RootDir    string
	ConfigFile string
	LogFile    string
}

const (
	// ConfigFilename is the per-user config file name. The full path
	// is resolved by ResolvePaths; keep the filename here so call
	// sites and tests share a single source of truth.
	ConfigFilename = "config.json"
	// LogFilename is the per-user log file name. The full path is
	// resolved by ResolvePaths; the file is opened lazily by the
	// logging package when LogToFile is enabled.
	LogFilename = "qbtremotego.log"
)

// ResolvePaths returns the runtime paths for the application, creating
// the config directory if it does not already exist. The log directory
// is the same as the config directory; the log file is created lazily
// by the logging package when LogToFile is enabled.
func ResolvePaths() (Paths, error) {
	dir, err := config.DefaultConfigDir()
	if err != nil {
		return Paths{}, fmt.Errorf("resolve config dir: %w", err)
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return Paths{}, fmt.Errorf("create app config dir: %w", err)
	}

	return Paths{
		RootDir:    dir,
		ConfigFile: filepath.Join(dir, ConfigFilename),
		LogFile:    filepath.Join(dir, LogFilename),
	}, nil
}
