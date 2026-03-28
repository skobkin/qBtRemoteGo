//go:build !linux && !windows

package platform

import "log/slog"

func syncMagnetHandler(_ string, _ bool, _ *slog.Logger) error { return nil }

func syncTorrentHandler(_ string, _ bool, _ *slog.Logger) error { return nil }

func syncAutostart(_ string, _ bool, _ *slog.Logger) error { return nil }
