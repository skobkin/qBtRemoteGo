package platform

import (
	"errors"
	"strings"
)

// ErrInstanceAlreadyRunning indicates another process already owns the app instance lock.
var ErrInstanceAlreadyRunning = errors.New("instance already running")

// ErrInstanceLockUnsupported indicates the current platform has no lock backend implementation.
var ErrInstanceLockUnsupported = errors.New("instance lock unsupported")

// InstanceLock represents an acquired single-instance lock.
type InstanceLock interface {
	Release() error
}

func AcquireInstanceLock(appID string) (InstanceLock, error) {
	return acquireInstanceLock(normalizeInstanceLockComponent(appID, "app"))
}

func normalizeInstanceLockComponent(raw, fallback string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}

	var b strings.Builder
	b.Grow(len(raw))
	for _, r := range raw {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}

	normalized := strings.Trim(b.String(), "_-.")
	if normalized == "" {
		return fallback
	}

	return normalized
}
