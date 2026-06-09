package logging

import (
	"io"
	"strings"
	"testing"
)

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
