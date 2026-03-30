//go:build windows

package platform

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows"
)

type windowsInstanceLock struct {
	handle windows.Handle
}

func acquireInstanceLock(appID string) (InstanceLock, error) {
	sid, err := windowsCurrentUserSID()
	if err != nil {
		return nil, err
	}

	namePtr, err := windows.UTF16PtrFromString(windowsInstanceMutexName(appID, sid))
	if err != nil {
		return nil, fmt.Errorf("encode instance mutex name: %w", err)
	}

	handle, err := windows.CreateMutex(nil, false, namePtr)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		if handle != 0 {
			_ = windows.CloseHandle(handle)
		}

		return nil, ErrInstanceAlreadyRunning
	}
	if err != nil {
		if handle != 0 {
			_ = windows.CloseHandle(handle)
		}

		return nil, fmt.Errorf("create instance mutex: %w", err)
	}

	return &windowsInstanceLock{handle: handle}, nil
}

func (l *windowsInstanceLock) Release() error {
	if l == nil || l.handle == 0 {
		return nil
	}

	err := windows.CloseHandle(l.handle)
	l.handle = 0
	if err != nil {
		return fmt.Errorf("close instance mutex handle: %w", err)
	}

	return nil
}

func windowsCurrentUserSID() (string, error) {
	token := windows.GetCurrentProcessToken()
	tokenUser, err := token.GetTokenUser()
	if err != nil {
		return "", fmt.Errorf("read current user token: %w", err)
	}

	return tokenUser.User.Sid.String(), nil
}

func windowsInstanceMutexName(appID, userSID string) string {
	return `Local\` + normalizeInstanceLockComponent(appID, "app") + `-single-instance-v1-` +
		normalizeInstanceLockComponent(userSID, "sid")
}
