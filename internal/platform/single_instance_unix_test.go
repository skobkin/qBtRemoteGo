//go:build unix && !windows

package platform

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestAcquireInstanceLock_ContentionAndRelease(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	appID := "qbtremotego-test-" + strconv.Itoa(os.Getpid())

	lock1, err := AcquireInstanceLock(appID)
	if err != nil {
		t.Fatalf("acquire first lock: %v", err)
	}

	lock2, err := AcquireInstanceLock(appID)
	if !errors.Is(err, ErrInstanceAlreadyRunning) {
		t.Fatalf("expected %v, got %v", ErrInstanceAlreadyRunning, err)
	}
	if lock2 != nil {
		t.Fatalf("expected second lock to be nil, got %#v", lock2)
	}

	if err := lock1.Release(); err != nil {
		t.Fatalf("release first lock: %v", err)
	}

	lock3, err := AcquireInstanceLock(appID)
	if err != nil {
		t.Fatalf("acquire lock after release: %v", err)
	}
	if err := lock3.Release(); err != nil {
		t.Fatalf("release third lock: %v", err)
	}
}

func TestUnixInstanceLockPathPrefersXDGRuntimeDir(t *testing.T) {
	runtimeDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)

	path, err := unixInstanceLockPath("qbtremotego")
	if err != nil {
		t.Fatalf("resolve lock path: %v", err)
	}

	wantPrefix := filepath.Join(runtimeDir, "qbtremotego") + string(filepath.Separator)
	if !strings.HasPrefix(path, wantPrefix) {
		t.Fatalf("expected path prefix %q, got %q", wantPrefix, path)
	}
}

func TestUnixInstanceLockPathFallsBackToTemp(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "")

	path, err := unixInstanceLockPath("qbtremotego")
	if err != nil {
		t.Fatalf("resolve lock path: %v", err)
	}

	wantFragment := "qbtremotego-" + strconv.Itoa(os.Getuid())
	if !strings.Contains(path, wantFragment) {
		t.Fatalf("expected path to contain %q, got %q", wantFragment, path)
	}
}

func TestAcquireInstanceLock_ReleasesOnProcessExit(t *testing.T) {
	if os.Getenv("GO_WANT_INSTANCE_LOCK_HELPER") == "1" {
		runInstanceLockHelperProcess()

		return
	}

	runtimeDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	appID := "qbtremotego-crash-test-" + strconv.Itoa(os.Getpid())

	// #nosec G204,G702 -- test launches the current test binary with fixed arguments.
	cmd := exec.Command(os.Args[0], "-test.run", "^TestAcquireInstanceLock_ReleasesOnProcessExit$")
	cmd.Env = append(
		os.Environ(),
		"GO_WANT_INSTANCE_LOCK_HELPER=1",
		"INSTANCE_LOCK_HELPER_APP_ID="+appID,
		"XDG_RUNTIME_DIR="+runtimeDir,
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("create helper stdout pipe: %v", err)
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper process: %v", err)
	}

	ready := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		if scanner.Scan() {
			ready <- scanner.Text()
		}
		close(ready)
	}()

	select {
	case line, ok := <-ready:
		if !ok || strings.TrimSpace(line) != "ready" {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			t.Fatalf("helper did not report readiness, line=%q, stderr=%q", line, stderr.String())
		}
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatalf("timeout waiting for helper readiness, stderr=%q", stderr.String())
	}

	lock, err := AcquireInstanceLock(appID)
	if !errors.Is(err, ErrInstanceAlreadyRunning) {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatalf("expected contention while helper runs, err=%v", err)
	}
	if lock != nil {
		t.Fatalf("expected nil lock on contention, got %#v", lock)
	}

	if err := cmd.Process.Kill(); err != nil {
		t.Fatalf("kill helper process: %v", err)
	}
	if err := cmd.Wait(); err == nil {
		t.Fatalf("expected helper to exit due to kill")
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		lock, err = AcquireInstanceLock(appID)
		if err == nil {
			if relErr := lock.Release(); relErr != nil {
				t.Fatalf("release lock after helper exit: %v", relErr)
			}

			return
		}
		if !errors.Is(err, ErrInstanceAlreadyRunning) {
			t.Fatalf("unexpected lock acquire error after helper exit: %v", err)
		}

		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("lock remained held after helper process exit")
}

func runInstanceLockHelperProcess() {
	appID := os.Getenv("INSTANCE_LOCK_HELPER_APP_ID")
	lock, err := AcquireInstanceLock(appID)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "acquire helper lock: %v\n", err)
		os.Exit(2)
	}
	defer func() {
		_ = lock.Release()
	}()

	_, _ = fmt.Fprintln(os.Stdout, "ready")
	select {}
}
