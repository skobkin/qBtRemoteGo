//go:build unix && !windows

package platform

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const instanceLockFilename = "instance.lock"

type unixInstanceLock struct {
	file *os.File
}

func acquireInstanceLock(appID string) (InstanceLock, error) {
	lockPath, err := unixInstanceLockPath(appID)
	if err != nil {
		return nil, err
	}

	// #nosec G304,G703 -- lockPath is built from process-owned runtime/temp directories.
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open instance lock file: %w", err)
	}

	fd, err := syscallFD(file)
	if err != nil {
		_ = file.Close()

		return nil, err
	}
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if isUnixLockContention(err) {
			return nil, ErrInstanceAlreadyRunning
		}

		return nil, fmt.Errorf("acquire instance file lock: %w", err)
	}

	return &unixInstanceLock{file: file}, nil
}

func (l *unixInstanceLock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}

	fd, err := syscallFD(l.file)
	if err != nil {
		_ = l.file.Close()
		l.file = nil

		return err
	}
	unlockErr := syscall.Flock(fd, syscall.LOCK_UN)
	closeErr := l.file.Close()
	l.file = nil

	if unlockErr != nil && !errors.Is(unlockErr, syscall.EBADF) {
		return fmt.Errorf("unlock instance file lock: %w", unlockErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close instance lock file: %w", closeErr)
	}

	return nil
}

func unixInstanceLockPath(appID string) (string, error) {
	lockDir := strings.TrimSpace(os.Getenv("XDG_RUNTIME_DIR"))
	if lockDir != "" {
		lockDir = filepath.Join(lockDir, normalizeInstanceLockComponent(appID, "app"))
	} else {
		lockDir = filepath.Join(
			os.TempDir(),
			normalizeInstanceLockComponent(appID, "app")+"-"+strconv.Itoa(os.Getuid()),
		)
	}

	// #nosec G703 -- lockDir is derived from process-owned runtime/temp directories.
	if err := os.MkdirAll(lockDir, 0o700); err != nil {
		return "", fmt.Errorf("create instance lock dir: %w", err)
	}

	return filepath.Join(lockDir, instanceLockFilename), nil
}

func isUnixLockContention(err error) bool {
	return errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN)
}

func syscallFD(file *os.File) (int, error) {
	if file == nil {
		return 0, fmt.Errorf("instance lock file is nil")
	}
	fd := file.Fd()
	maxInt := uint64(^uint(0) >> 1)
	if uint64(fd) > maxInt {
		return 0, fmt.Errorf("file descriptor %d exceeds int range", fd)
	}

	return int(fd), nil
}
