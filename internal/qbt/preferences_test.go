package qbt

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func newPreferencesTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/auth/login" {
			_, _ = io.WriteString(w, "Ok.")
			return
		}
		handler(w, r)
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(ClientConfig{URL: server.URL, Username: "user", Password: "pass"}, slog.Default())
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	return client
}

func TestPreferencesKeepsRawValuesAndUnknownKeys(t *testing.T) {
	t.Parallel()

	client := newPreferencesTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/app/preferences" || r.Method != http.MethodGet {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"dl_limit":2147482624,"scheduler_enabled":true,"locale":"en","max_ratio":1.5,"future_key":{"a":1}}`)
	})

	prefs, err := client.Preferences(context.Background())
	if err != nil {
		t.Fatalf("preferences: %v", err)
	}

	if !prefs.Has("future_key") || prefs.Has("missing") {
		t.Fatalf("Has: future_key=%v missing=%v", prefs.Has("future_key"), prefs.Has("missing"))
	}
	if v, ok := prefs.Int("dl_limit"); !ok || v != 2147482624 {
		t.Fatalf("Int(dl_limit) = %d, %v", v, ok)
	}
	if v, ok := prefs.Bool("scheduler_enabled"); !ok || !v {
		t.Fatalf("Bool(scheduler_enabled) = %v, %v", v, ok)
	}
	if v, ok := prefs.String("locale"); !ok || v != "en" {
		t.Fatalf("String(locale) = %q, %v", v, ok)
	}
	if v, ok := prefs.Float("max_ratio"); !ok || v != 1.5 {
		t.Fatalf("Float(max_ratio) = %v, %v", v, ok)
	}
}

func TestPreferencesAccessorsRejectMismatchedTypes(t *testing.T) {
	t.Parallel()

	prefs := Preferences{
		"flag":   json.RawMessage(`true`),
		"number": json.RawMessage(`42`),
		"text":   json.RawMessage(`"x"`),
		"frac":   json.RawMessage(`1.5`),
	}
	if _, ok := prefs.Int("flag"); ok {
		t.Fatal("Int accepted a bool")
	}
	if _, ok := prefs.Int("frac"); ok {
		t.Fatal("Int accepted a fractional number")
	}
	if _, ok := prefs.Bool("number"); ok {
		t.Fatal("Bool accepted a number")
	}
	if _, ok := prefs.String("number"); ok {
		t.Fatal("String accepted a number")
	}
	if _, ok := prefs.Int("absent"); ok {
		t.Fatal("Int reported an absent key")
	}
}

func TestSetPreferencesPostsOnlyGivenKeysAsJSONForm(t *testing.T) {
	t.Parallel()

	var got map[string]any
	client := newPreferencesTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/app/setPreferences" || r.Method != http.MethodPost {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/x-www-form-urlencoded") {
			t.Errorf("content type %q", ct)
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		if err := json.Unmarshal([]byte(r.PostForm.Get("json")), &got); err != nil {
			t.Errorf("decode json field %q: %v", r.PostForm.Get("json"), err)
		}
	})

	err := client.SetPreferences(context.Background(), map[string]any{
		"dl_limit":           1048576,
		"schedule_from_hour": 8,
		"schedule_from_min":  0,
		"scheduler_enabled":  true,
	})
	if err != nil {
		t.Fatalf("set preferences: %v", err)
	}

	want := map[string]any{
		"dl_limit":           float64(1048576),
		"schedule_from_hour": float64(8),
		"schedule_from_min":  float64(0),
		"scheduler_enabled":  true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("posted %v, want %v", got, want)
	}
}

func TestSetPreferencesSurfacesServerRejection(t *testing.T) {
	t.Parallel()

	client := newPreferencesTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, "WebUI username must be at least 3 characters long")
	})

	err := client.SetPreferences(context.Background(), map[string]any{"web_ui_username": "ab"})
	if err == nil || !strings.Contains(err.Error(), "at least 3 characters") {
		t.Fatalf("error %v, want server message", err)
	}
}

func TestSetPreferencesRejectsEmptyChanges(t *testing.T) {
	t.Parallel()

	client := newPreferencesTestClient(t, func(_ http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s", r.URL.Path)
	})

	if err := client.SetPreferences(context.Background(), nil); err == nil {
		t.Fatal("empty change set was sent")
	}
}

func TestRotateAPIKeyPostsAndReturnsNewKey(t *testing.T) {
	t.Parallel()
	const key = "qbt_abcdefghijklmnopqrstuvwxyz12"
	client := newPreferencesTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/app/rotateAPIKey" || r.Method != http.MethodPost {
			http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusNotFound)
			return
		}
		_, _ = io.WriteString(w, `{"apiKey":"`+key+`"}`)
	})

	got, err := client.RotateAPIKey(context.Background())
	if err != nil || got != key {
		t.Fatalf("RotateAPIKey = %q, %v", got, err)
	}
}

func TestRotateAPIKeyRejectsMalformedKey(t *testing.T) {
	t.Parallel()
	client := newPreferencesTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"apiKey":""}`)
	})

	if got, err := client.RotateAPIKey(context.Background()); err == nil {
		t.Fatalf("RotateAPIKey accepted %q", got)
	}
}

func TestRotateAPIKeySurfacesServerError(t *testing.T) {
	t.Parallel()
	client := newPreferencesTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "Forbidden", http.StatusForbidden)
	})

	if _, err := client.RotateAPIKey(context.Background()); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("error = %v, want 403", err)
	}
}
