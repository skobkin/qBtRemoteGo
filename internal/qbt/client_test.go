package qbt

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
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
