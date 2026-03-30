package platform

import (
	"log/slog"
	"reflect"
	"testing"
	"time"
)

func TestActivationServerForwardsArgs(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	received := make(chan []string, 1)
	server, err := StartActivationServer("qbtremotego-test", slog.Default(), func(args []string) {
		received <- append([]string(nil), args...)
	})
	if err != nil {
		t.Fatalf("StartActivationServer() error = %v", err)
	}
	defer func() {
		if err := server.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	}()

	want := []string{"magnet:?xt=urn:btih:abc", "/tmp/example.torrent"}
	if err := ForwardActivation("qbtremotego-test", want); err != nil {
		t.Fatalf("ForwardActivation() error = %v", err)
	}

	select {
	case got := <-received:
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("received args = %#v, want %#v", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for activation delivery")
	}
}

func TestForwardActivationRejectsStaleState(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	server, err := StartActivationServer("qbtremotego-test", slog.Default(), func(_ []string) {})
	if err != nil {
		t.Fatalf("StartActivationServer() error = %v", err)
	}
	if err := server.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := ForwardActivation("qbtremotego-test", []string{"magnet:?xt=urn:btih:abc"}); err == nil {
		t.Fatal("ForwardActivation() error = nil, want non-nil")
	}
}
