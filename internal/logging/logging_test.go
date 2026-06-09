package logging

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skobkin/qbtremotego/internal/config"
)

func TestConfigureStdoutOnly(t *testing.T) {
	m := &Manager{}
	if err := m.Configure(config.LoggingConfig{Level: "info"}, ""); err != nil {
		t.Fatalf("configure: %v", err)
	}
	defer m.Close()

	if m.file != nil {
		t.Fatal("expected no file when LogToFile is false")
	}
}

func TestConfigureWithFile(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "qbtremotego.log")

	m := &Manager{}
	if err := m.Configure(config.LoggingConfig{Level: "debug", LogToFile: true}, logPath); err != nil {
		t.Fatalf("configure: %v", err)
	}
	defer m.Close()

	if m.file == nil {
		t.Fatal("expected file handle to be opened")
	}

	m.Logger("test").Info("hello", "k", "v")

	// Close explicitly to flush before reading.
	if err := m.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// #nosec G304 -- test reads back a temp log file written in this test.
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(data), "hello") {
		t.Fatalf("expected log to contain 'hello', got: %q", string(data))
	}
	if !strings.Contains(string(data), "k=v") {
		t.Fatalf("expected log to contain 'k=v', got: %q", string(data))
	}
}

func TestConfigureReconfigureClosesPreviousFile(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.log")
	second := filepath.Join(dir, "second.log")

	m := &Manager{}
	if err := m.Configure(config.LoggingConfig{Level: "info", LogToFile: true}, first); err != nil {
		t.Fatalf("configure first: %v", err)
	}
	if err := m.Configure(config.LoggingConfig{Level: "info", LogToFile: true}, second); err != nil {
		t.Fatalf("configure second: %v", err)
	}
	defer m.Close()

	// Writing now should hit the second file only.
	m.Logger("test").Info("second")

	if err := m.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if _, err := os.Stat(first); err == nil {
		// first may exist if it was created during OpenFile; the
		// invariant we want is that no new content was appended
		// after the reconfigure.
		// #nosec G304 -- test reads back a temp log file written in this test.
		data, _ := os.ReadFile(first)
		if strings.Contains(string(data), "second") {
			t.Fatalf("unexpected write to first log after reconfigure: %q", string(data))
		}
	}

	// #nosec G304 -- test reads back a temp log file written in this test.
	data, err := os.ReadFile(second)
	if err != nil {
		t.Fatalf("read second: %v", err)
	}
	if !strings.Contains(string(data), "second") {
		t.Fatalf("expected 'second' in second log, got: %q", string(data))
	}
}

func TestConfigureInvalidLevel(t *testing.T) {
	m := &Manager{}
	if err := m.Configure(config.LoggingConfig{Level: "loud"}, ""); err == nil {
		t.Fatal("expected error for invalid level")
	}
}

func TestConfigureFileOpenFailure(t *testing.T) {
	// Use a path inside a non-existent parent directory that cannot
	// be created (file used as a directory).
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("not a dir"), 0o600); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	badPath := filepath.Join(blocker, "qbtremotego.log")

	m := &Manager{}
	err := m.Configure(config.LoggingConfig{Level: "info", LogToFile: true}, badPath)
	if err == nil {
		t.Fatal("expected error when log dir is not a directory")
	}
	if m.file != nil {
		t.Fatal("expected no file handle to be retained after a failed open")
	}
}

func TestLoggerNilSafe(t *testing.T) {
	// Manager can be called on a nil receiver; it falls back to
	// slog.Default(). Matches the historical behaviour.
	var m *Manager
	l := m.Logger("anything")
	if l == nil {
		t.Fatal("expected non-nil logger from nil manager")
	}
}

func TestFanoutWriterWritesToAll(t *testing.T) {
	var a, b strings.Builder
	w := newFanoutWriter(&a, &b)
	if _, err := w.Write([]byte("hi")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if a.String() != "hi" || b.String() != "hi" {
		t.Fatalf("expected both writers to receive 'hi', got a=%q b=%q", a.String(), b.String())
	}
}

func TestFanoutWriterShortWriteReported(t *testing.T) {
	short := &shortWriter{max: 2}
	w := newFanoutWriter(short)
	n, err := w.Write([]byte("hello"))
	if err == nil {
		t.Fatal("expected error from short writer")
	}
	if n != 0 {
		t.Fatalf("expected n=0 on error, got %d", n)
	}
}

type shortWriter struct{ max int }

func (s *shortWriter) Write(p []byte) (int, error) {
	if len(p) > s.max {
		return s.max, io.ErrShortWrite
	}
	return len(p), nil
}

func TestCloseNilSafe(t *testing.T) {
	m := &Manager{}
	if err := m.Close(); err != nil {
		t.Fatalf("close on fresh manager should not error: %v", err)
	}
}

func TestNormalizeLevel(t *testing.T) {
	cases := map[string]string{
		"":        "info",
		"info":    "info",
		"INFO":    "info",
		"debug":   "debug",
		"  debug": "debug",
		"warn":    "warn",
		"error":   "error",
		"loud":    "",
	}
	for in, want := range cases {
		if got := NormalizeLevel(in); got != want {
			t.Errorf("NormalizeLevel(%q) = %q, want %q", in, got, want)
		}
	}
}
