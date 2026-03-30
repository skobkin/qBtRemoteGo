package qbt

import (
	"bytes"
	"context"
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

	"github.com/skobkin/qbtremotego/internal/config"
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

			_, err := NewClient(config.ConnectionConfig{URL: tc.url}, slog.Default())
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q, got %v", tc.want, err)
			}
		})
	}
}

func TestExecuteRejectsUnexpectedRequestTarget(t *testing.T) {
	t.Parallel()

	client, err := NewClient(config.ConnectionConfig{URL: "https://example.com/qbt"}, slog.Default())
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

	client, err := NewClient(config.ConnectionConfig{
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

	client, err := NewClient(config.ConnectionConfig{
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

	client, err := NewClient(config.ConnectionConfig{
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

	client, err := NewClient(config.ConnectionConfig{
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

	client, err := NewClient(config.ConnectionConfig{
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
