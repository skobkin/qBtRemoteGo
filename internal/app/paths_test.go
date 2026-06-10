package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePaths(t *testing.T) {
	paths, err := ResolvePaths()
	if err != nil {
		t.Fatalf("resolve paths: %v", err)
	}

	if paths.RootDir == "" {
		t.Fatal("expected non-empty RootDir")
	}
	if filepath.Base(paths.RootDir) != "qbtremotego" {
		t.Fatalf("expected RootDir to end with 'qbtremotego', got %q", paths.RootDir)
	}
	if filepath.Base(paths.LogFile) != LogFilename {
		t.Fatalf("expected LogFile to be named %q, got %q", LogFilename, paths.LogFile)
	}
	if filepath.Base(paths.ConfigFile) != ConfigFilename {
		t.Fatalf("expected ConfigFile to be %q, got %q", ConfigFilename, paths.ConfigFile)
	}
	if filepath.Dir(paths.LogFile) != paths.RootDir {
		t.Fatalf("expected LogFile to live under RootDir, got %q under %q", paths.LogFile, paths.RootDir)
	}

	// The root dir must exist after ResolvePaths returns.
	if info, statErr := os.Stat(paths.RootDir); statErr != nil || !info.IsDir() {
		t.Fatalf("expected RootDir %q to exist as a directory (stat err: %v)", paths.RootDir, statErr)
	}
}
