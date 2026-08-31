package qbt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitRemotePath(t *testing.T) {
	parent, prefix := splitRemotePath("/home/user/downloads/abc")
	if parent != "/home/user/downloads/" || prefix != "abc" {
		t.Fatalf("unexpected split: %q %q", parent, prefix)
	}

	parent, prefix = splitRemotePath("C:\\data\\")
	if parent != "C:\\data\\" || prefix != "" {
		t.Fatalf("unexpected trailing split: %q %q", parent, prefix)
	}
}

func TestNewClientRejectsUnsupportedURLs(t *testing.T) {
	t.Parallel()

	credentialedURL := (&url.URL{
		Scheme: "https",
		User:   url.UserPassword("user", "pass"),
		Host:   "example.com",
	}).String()

	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "unsupported scheme",
			url:  "ftp://example.com",
			want: "connection URL must use http or https",
		},
		{
			name: "embedded credentials",
			url:  credentialedURL,
			want: "connection URL must not include embedded credentials",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := NewClient(ClientConfig{URL: tc.url}, slog.Default())
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q, got %v", tc.want, err)
			}
		})
	}
}

func TestExecuteRejectsUnexpectedRequestTarget(t *testing.T) {
	t.Parallel()

	client, err := NewClient(ClientConfig{URL: "https://example.com/qbt"}, slog.Default())
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	req, err := client.newRequest(context.Background(), http.MethodGet, "app/version", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	req.URL = &url.URL{
		Scheme: "https",
		Host:   "evil.example",
		Path:   "/qbt/api/v2/app/version",
	}

	resp, err := client.execute(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err == nil {
		t.Fatal("expected execute to reject unexpected host")
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		t.Fatalf("expected validation error before transport call, got %v", err)
	}
	if !strings.Contains(err.Error(), "unexpected host") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoginAcceptsLegacyOKBody(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = io.WriteString(w, "Ok.")
		case "/api/v2/app/version":
			_, _ = io.WriteString(w, "5.1.2")
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		URL:      server.URL,
		Username: "user",
		Password: "pass",
	}, slog.Default())
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	if err := client.TestConnection(context.Background()); err != nil {
		t.Fatalf("test connection: %v", err)
	}
}

func TestLoginAcceptsNoContentWithPortNamedSessionCookie(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			http.SetCookie(w, &http.Cookie{
				Name:     "QBT_SID_8112",
				Value:    "test-value",
				Path:     "/",
				HttpOnly: true,
				SameSite: http.SameSiteStrictMode,
			})
			w.WriteHeader(http.StatusNoContent)
		case "/api/v2/app/version":
			cookie, err := r.Cookie("QBT_SID_8112")
			if err != nil {
				t.Fatalf("expected non-legacy session cookie: %v", err)
			}
			if cookie.Value != "test-value" {
				t.Fatalf("unexpected cookie value: %q", cookie.Value)
			}
			_, _ = io.WriteString(w, "5.2.0")
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		URL:      server.URL,
		Username: "user",
		Password: "pass",
	}, slog.Default())
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	if err := client.TestConnection(context.Background()); err != nil {
		t.Fatalf("test connection: %v", err)
	}
}

func TestLoginRejectsFailsBody(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = io.WriteString(w, "Fails.")
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		URL:      server.URL,
		Username: "user",
		Password: "bad-pass",
	}, slog.Default())
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	err = client.TestConnection(context.Background())
	if err == nil {
		t.Fatal("expected authentication failure")
	}
	if !strings.Contains(err.Error(), "authentication failed: Fails.") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoginRejectsErrorStatus(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			http.Error(w, "forbidden", http.StatusForbidden)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		URL:      server.URL,
		Username: "user",
		Password: "bad-pass",
	}, slog.Default())
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	err = client.TestConnection(context.Background())
	if err == nil {
		t.Fatal("expected login status failure")
	}
	if !strings.Contains(err.Error(), "auth/login returned 403 Forbidden") {
		t.Fatalf("expected auth/login status in error, got %v", err)
	}
}

const testAPIKey = "qbt_" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestAPIKeySkipsLoginAndSendsBearer(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/auth/login" {
			t.Fatalf("API key auth must never call auth/login")
		}
		switch r.URL.Path {
		case "/api/v2/app/version":
			if got := r.Header.Get("Authorization"); got != "Bearer "+testAPIKey {
				t.Fatalf("unexpected Authorization header: %q", got)
			}
			_, _ = io.WriteString(w, "5.2.0")
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		URL:    server.URL,
		APIKey: testAPIKey,
	}, slog.Default())
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	if err := client.TestConnection(context.Background()); err != nil {
		t.Fatalf("test connection: %v", err)
	}
}

func TestAPIKeySendsBearerOnAllRequestKinds(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+testAPIKey {
			t.Fatalf("unexpected Authorization header on %s: %q", r.URL.Path, got)
		}
		switch r.URL.Path {
		case "/api/v2/torrents/info":
			_, _ = io.WriteString(w, `[]`)
		case "/api/v2/torrents/stop":
			_, _ = io.WriteString(w, "{}")
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		URL:    server.URL,
		APIKey: testAPIKey,
	}, slog.Default())
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	if _, err := client.Torrents(context.Background()); err != nil {
		t.Fatalf("fetch torrents: %v", err)
	}
	if err := client.Stop(context.Background(), []string{"abc"}); err != nil {
		t.Fatalf("stop torrent: %v", err)
	}
}

func TestNewClientRejectsMalformedAPIKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		apiKey string
		want   string
	}{
		{
			name:   "too short",
			apiKey: "qbt_abc",
			want:   "invalid API key",
		},
		{
			name:   "missing prefix",
			apiKey: strings.Repeat("a", 32),
			want:   "invalid API key",
		},
		{
			name:   "invalid character in secret part",
			apiKey: "qbt_" + strings.Repeat("a", 27) + "!",
			want:   "invalid API key",
		},
		{
			name:   "valid key",
			apiKey: testAPIKey,
			want:   "",
		},
		{
			name:   "digits and mixed case are valid",
			apiKey: "qbt_" + strings.Repeat("zZ9", 9) + "x",
			want:   "",
		},
		{
			name:   "surrounding whitespace is trimmed",
			apiKey: "  " + testAPIKey + "\n",
			want:   "",
		},
		{
			name:   "empty key means password auth",
			apiKey: "",
			want:   "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := NewClient(ClientConfig{URL: "https://example.com", APIKey: tc.apiKey}, slog.Default())
			if tc.want == "" {
				if err != nil {
					t.Fatalf("expected success, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q, got %v", tc.want, err)
			}
		})
	}
}

func TestTestConnectionAPIKeyFailureHint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status int
	}{
		{
			name:   "unauthorized",
			status: http.StatusUnauthorized,
		},
		{
			name:   "forbidden",
			status: http.StatusForbidden,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v2/app/version" {
					t.Fatalf("unexpected path: %s", r.URL.Path)
				}
				http.Error(w, "denied", tc.status)
			}))
			defer server.Close()

			client, err := NewClient(ClientConfig{
				URL:    server.URL,
				APIKey: testAPIKey,
			}, slog.Default())
			if err != nil {
				t.Fatalf("new client: %v", err)
			}

			err = client.TestConnection(context.Background())
			if err == nil {
				t.Fatal("expected connection failure")
			}
			message := err.Error()
			if !strings.Contains(message, "may have failed") {
				t.Fatalf("expected cautious API key hint, got %v", err)
			}
			if !strings.Contains(message, "v5.2.0") {
				t.Fatalf("expected version requirement in hint, got %v", err)
			}
			if !strings.Contains(message, "proxy") {
				t.Fatalf("expected server/proxy denial mention, got %v", err)
			}
			if got := strings.Count(message, http.StatusText(tc.status)); got != 1 {
				t.Fatalf("expected status text exactly once, got %d in: %s", got, message)
			}
		})
	}

	t.Run("password mode keeps generic error", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api/v2/auth/login":
				_, _ = io.WriteString(w, "Ok.")
			case "/api/v2/app/version":
				http.Error(w, "denied", http.StatusForbidden)
			default:
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
		}))
		defer server.Close()

		client, err := NewClient(ClientConfig{
			URL:      server.URL,
			Username: "user",
			Password: "pass",
		}, slog.Default())
		if err != nil {
			t.Fatalf("new client: %v", err)
		}

		err = client.TestConnection(context.Background())
		if err == nil {
			t.Fatal("expected connection failure")
		}
		message := err.Error()
		if !strings.Contains(message, "app/version returned 403 Forbidden") {
			t.Fatalf("expected generic error, got %v", err)
		}
		if strings.Contains(message, "may have failed") {
			t.Fatalf("did not expect API key hint in password mode, got %v", err)
		}
	})
}

func TestDirectorySuggestionsFiltersPrefix(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/qbt/api/v2/auth/login":
			_, _ = io.WriteString(w, "Ok.")
		case "/qbt/api/v2/app/getDirectoryContent":
			_, _ = io.WriteString(w, `["/data/alpha","/data/beta","/data/alpine"]`)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		URL:      server.URL + "/qbt",
		Username: "user",
		Password: "pass",
	}, slog.Default())
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	items, err := client.DirectorySuggestions(context.Background(), "/data/al")
	if err != nil {
		t.Fatalf("directory suggestions: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("unexpected items: %#v", items)
	}
}

func TestDefaultSavePathLoadsServerValue(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = io.WriteString(w, "Ok.")
		case "/api/v2/app/defaultSavePath":
			_, _ = io.WriteString(w, "/srv/downloads")
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		URL:      server.URL,
		Username: "user",
		Password: "pass",
	}, slog.Default())
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	path, err := client.DefaultSavePath(context.Background())
	if err != nil {
		t.Fatalf("default save path: %v", err)
	}
	if path != "/srv/downloads" {
		t.Fatalf("unexpected path: %q", path)
	}
}

func TestTorrentsDecodesMagnetURI(t *testing.T) {
	var torrent Torrent
	if err := json.Unmarshal([]byte(`{"hash":"abc","name":"sample","magnet_uri":"magnet:?xt=urn:btih:abc"}`), &torrent); err != nil {
		t.Fatalf("decode torrent: %v", err)
	}
	if torrent.MagnetURI != "magnet:?xt=urn:btih:abc" {
		t.Fatalf("unexpected magnet URI: %q", torrent.MagnetURI)
	}
}

func TestForceRecheckPostsHashes(t *testing.T) {
	testHashesAction(t, "/api/v2/torrents/recheck", func(ctx context.Context, client *Client, hashes []string) error {
		return client.ForceRecheck(ctx, hashes)
	})
}

func TestForceReannouncePostsHashes(t *testing.T) {
	testHashesAction(t, "/api/v2/torrents/reannounce", func(ctx context.Context, client *Client, hashes []string) error {
		return client.ForceReannounce(ctx, hashes)
	})
}

func TestSetLocationPostsHashesAndLocation(t *testing.T) {
	values := testHashesAction(t, "/api/v2/torrents/setLocation", func(ctx context.Context, client *Client, hashes []string) error {
		return client.SetLocation(ctx, hashes, "/data/new")
	})
	if got := values.Get("location"); got != "/data/new" {
		t.Fatalf("unexpected location: %q", got)
	}
}

func TestRenamePostsHashAndName(t *testing.T) {
	var (
		contentType string
		payload     string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = io.WriteString(w, "Ok.")
		case "/api/v2/torrents/rename":
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method: %s", r.Method)
			}
			contentType = r.Header.Get("Content-Type")
			data, _ := io.ReadAll(r.Body)
			payload = string(data)
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		URL:      server.URL,
		Username: "user",
		Password: "pass",
	}, slog.Default())
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	if err := client.Rename(context.Background(), "a", "New name"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if contentType != "application/x-www-form-urlencoded" {
		t.Fatalf("unexpected content type: %q", contentType)
	}
	values, err := url.ParseQuery(payload)
	if err != nil {
		t.Fatalf("parse payload: %v", err)
	}
	if got := values.Get("hash"); got != "a" {
		t.Fatalf("unexpected hash: %q", got)
	}
	if got := values.Get("name"); got != "New name" {
		t.Fatalf("unexpected name: %q", got)
	}
}

func testHashesAction(t *testing.T, wantPath string, action func(context.Context, *Client, []string) error) url.Values {
	t.Helper()

	var payload string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = io.WriteString(w, "Ok.")
		case wantPath:
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method: %s", r.Method)
			}
			data, _ := io.ReadAll(r.Body)
			payload = string(data)
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		URL:      server.URL,
		Username: "user",
		Password: "pass",
	}, slog.Default())
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	if err := action(context.Background(), client, []string{"a", "b"}); err != nil {
		t.Fatalf("post hashes action: %v", err)
	}
	values, err := url.ParseQuery(payload)
	if err != nil {
		t.Fatalf("parse payload: %v", err)
	}
	if got := values.Get("hashes"); got != "a|b" {
		t.Fatalf("unexpected hashes: %q", got)
	}
	return values
}

func TestAddTorrentEncodesMagnetFields(t *testing.T) {
	var (
		contentType string
		payload     []byte
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = io.WriteString(w, "Ok.")
		case "/api/v2/torrents/add":
			contentType = r.Header.Get("Content-Type")
			payload, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		URL:      server.URL,
		Username: "user",
		Password: "pass",
	}, slog.Default())
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	dl := 12
	req := AddRequest{
		SourceType:         SourceMagnet,
		MagnetLinks:        []string{"magnet:?xt=urn:btih:abc"},
		ManagementMode:     "auto",
		SavePath:           "/data",
		StartTorrent:       false,
		SequentialDownload: true,
		DownloadLimitKiB:   &dl,
		ContentLayout:      "Original",
	}

	if err := client.AddTorrent(context.Background(), req); err != nil {
		t.Fatalf("add torrent: %v", err)
	}

	if !strings.HasPrefix(contentType, "multipart/form-data; boundary=") {
		t.Fatalf("unexpected content type: %q", contentType)
	}
	boundary := strings.TrimPrefix(contentType, "multipart/form-data; boundary=")
	reader := multipart.NewReader(bytes.NewReader(payload), boundary)
	fields := map[string]string{}
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("next part: %v", err)
		}
		data, _ := io.ReadAll(part)
		fields[part.FormName()] = string(data)
	}

	if fields["urls"] == "" || fields["autoTMM"] != "true" || fields["stopped"] != "true" {
		t.Fatalf("unexpected fields: %#v", fields)
	}
	if fields["dlLimit"] != "12288" {
		t.Fatalf("unexpected dlLimit: %q", fields["dlLimit"])
	}
}

func TestAddTorrentUploadsFile(t *testing.T) {
	var sawFilename string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = io.WriteString(w, "Ok.")
		case "/api/v2/torrents/add":
			mediaType := r.Header.Get("Content-Type")
			boundary := strings.TrimPrefix(mediaType, "multipart/form-data; boundary=")
			reader := multipart.NewReader(r.Body, boundary)
			for {
				part, err := reader.NextPart()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatalf("read multipart: %v", err)
				}
				if part.FormName() == "torrents" {
					sawFilename = part.FileName()
				}
			}
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		URL:      server.URL,
		Username: "user",
		Password: "pass",
	}, slog.Default())
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "sample.torrent")
	if err := os.WriteFile(path, []byte("torrent-bytes"), 0o600); err != nil {
		t.Fatalf("write torrent: %v", err)
	}

	if err := client.AddTorrent(context.Background(), AddRequest{
		SourceType:      SourceTorrentFile,
		TorrentFilePath: path,
		StartTorrent:    true,
	}); err != nil {
		t.Fatalf("add torrent file: %v", err)
	}

	if sawFilename != "sample.torrent" {
		t.Fatalf("unexpected filename: %q", sawFilename)
	}
}

func TestServerStateLoadsFreeSpaceAndSlowMode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = io.WriteString(w, "Ok.")
		case "/api/v2/sync/maindata":
			if got := r.URL.Query().Get("rid"); got != "0" {
				t.Fatalf("unexpected rid: %q", got)
			}
			_, _ = io.WriteString(w, `{"server_state":{"free_space_on_disk":5368709120,"use_alt_speed_limits":true}}`)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		URL:      server.URL,
		Username: "user",
		Password: "pass",
	}, slog.Default())
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	state, err := client.ServerState(context.Background())
	if err != nil {
		t.Fatalf("server state: %v", err)
	}
	if state.FreeSpaceOnDisk != 5368709120 || !state.UseAltSpeedLimits {
		t.Fatalf("unexpected state: %#v", state)
	}
}
