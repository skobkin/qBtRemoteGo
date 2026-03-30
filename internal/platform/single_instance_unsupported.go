//go:build !unix && !windows

package platform

import (
	"fmt"
	"runtime"
)

func acquireInstanceLock(_ string) (InstanceLock, error) {
	return nil, fmt.Errorf("%w on %s", ErrInstanceLockUnsupported, runtime.GOOS)
}
