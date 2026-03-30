package qbt

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/skobkin/qbtremotego/internal/config"
)

type Client struct {
	baseURL    *url.URL
	username   string
	password   string
	httpClient *http.Client
	logger     *slog.Logger

	mu            sync.Mutex
	authenticated bool
}

func NewClient(cfg config.ConnectionConfig, logger *slog.Logger) (*Client, error) {
	base, err := url.Parse(strings.TrimSpace(cfg.URL))
	if err != nil {
		return nil, fmt.Errorf("parse URL: %w", err)
	}
	if base.Scheme == "" || base.Host == "" {
		return nil, errors.New("connection URL must include scheme and host")
	}
	if base.Scheme != "http" && base.Scheme != "https" {
		return nil, errors.New("connection URL must use http or https")
	}
	if base.User != nil {
		return nil, errors.New("connection URL must not include embedded credentials")
	}
	base.RawQuery = ""
	base.Fragment = ""

	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("create cookie jar: %w", err)
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if cfg.SkipCertificateCheck {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}

	return &Client{
		baseURL:  base,
		username: cfg.Username,
		password: cfg.Password,
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Jar:       jar,
			Transport: transport,
		},
		logger: logger,
	}, nil
}

func (c *Client) TestConnection(ctx context.Context) error {
	if err := c.ensureAuthenticated(ctx); err != nil {
		return err
	}

	req, err := c.newRequest(ctx, http.MethodGet, "app/version", nil)
	if err != nil {
		return err
	}

	resp, err := c.execute(req)
	if err != nil {
		return fmt.Errorf("request app/version: %w", err)
	}
	defer closeAndLog(c.logger, resp.Body, "close app/version response body")

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))

		return fmt.Errorf("app/version returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	return nil
}

func (c *Client) Torrents(ctx context.Context) ([]Torrent, error) {
	if err := c.ensureAuthenticated(ctx); err != nil {
		return nil, err
	}

	req, err := c.newRequest(ctx, http.MethodGet, "torrents/info", nil)
	if err != nil {
		return nil, err
	}

	var torrents []Torrent
	if err := c.doJSON(req, &torrents); err != nil {
		return nil, err
	}
	for i := range torrents {
		torrents[i].AddedAt = time.Unix(torrents[i].AddedUnix, 0)
	}

	return torrents, nil
}

func (c *Client) TransferInfo(ctx context.Context) (TransferInfo, error) {
	if err := c.ensureAuthenticated(ctx); err != nil {
		return TransferInfo{}, err
	}

	req, err := c.newRequest(ctx, http.MethodGet, "transfer/info", nil)
	if err != nil {
		return TransferInfo{}, err
	}

	var transfer TransferInfo
	if err := c.doJSON(req, &transfer); err != nil {
		return TransferInfo{}, err
	}

	return transfer, nil
}

func (c *Client) ServerState(ctx context.Context) (ServerState, error) {
	if err := c.ensureAuthenticated(ctx); err != nil {
		return ServerState{}, err
	}

	req, err := c.newRequest(ctx, http.MethodGet, "sync/maindata", nil)
	if err != nil {
		return ServerState{}, err
	}
	query := req.URL.Query()
	query.Set("rid", "0")
	req.URL.RawQuery = query.Encode()

	var data MainData
	if err := c.doJSON(req, &data); err != nil {
		return ServerState{}, err
	}

	return data.ServerState, nil
}

func (c *Client) Categories(ctx context.Context) ([]string, error) {
	if err := c.ensureAuthenticated(ctx); err != nil {
		return nil, err
	}

	req, err := c.newRequest(ctx, http.MethodGet, "torrents/categories", nil)
	if err != nil {
		return nil, err
	}

	var raw map[string]json.RawMessage
	if err := c.doJSON(req, &raw); err != nil {
		return nil, err
	}

	out := make([]string, 0, len(raw))
	for name := range raw {
		out = append(out, name)
	}
	slices.Sort(out)

	return out, nil
}

func (c *Client) Tags(ctx context.Context) ([]string, error) {
	if err := c.ensureAuthenticated(ctx); err != nil {
		return nil, err
	}

	req, err := c.newRequest(ctx, http.MethodGet, "torrents/tags", nil)
	if err != nil {
		return nil, err
	}

	var tags []string
	if err := c.doJSON(req, &tags); err != nil {
		return nil, err
	}
	slices.Sort(tags)

	return tags, nil
}

func (c *Client) AddTorrent(ctx context.Context, req AddRequest) error {
	if err := c.ensureAuthenticated(ctx); err != nil {
		return err
	}

	body, contentType, err := c.buildAddTorrentBody(req)
	if err != nil {
		return err
	}

	httpReq, err := c.newRequest(ctx, http.MethodPost, "torrents/add", body)
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", contentType)

	resp, err := c.execute(httpReq)
	if err != nil {
		return fmt.Errorf("request torrents/add: %w", err)
	}
	defer closeAndLog(c.logger, resp.Body, "close torrents/add response body")

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

		return fmt.Errorf("torrents/add returned %s: %s", resp.Status, strings.TrimSpace(string(bodyBytes)))
	}

	return nil
}

func (c *Client) buildAddTorrentBody(req AddRequest) (_ *bytes.Buffer, contentType string, err error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	defer func() {
		closeErr := writer.Close()
		if err == nil && closeErr != nil {
			err = fmt.Errorf("close multipart writer: %w", closeErr)
		}
	}()

	switch req.SourceType {
	case SourceMagnet:
		if err := writer.WriteField("urls", strings.Join(req.MagnetLinks, "\n")); err != nil {
			return nil, "", fmt.Errorf("write urls field: %w", err)
		}
	case SourceTorrentFile:
		file, err := os.Open(filepath.Clean(req.TorrentFilePath))
		if err != nil {
			return nil, "", fmt.Errorf("open torrent file: %w", err)
		}
		defer closeAndLog(c.logger, file, "close torrent file")

		part, err := writer.CreateFormFile("torrents", filepath.Base(req.TorrentFilePath))
		if err != nil {
			return nil, "", fmt.Errorf("create form file: %w", err)
		}
		if _, err := io.Copy(part, file); err != nil {
			return nil, "", fmt.Errorf("copy torrent file: %w", err)
		}
	default:
		return nil, "", fmt.Errorf("unsupported source type %q", req.SourceType)
	}

	fields := map[string]string{
		"autoTMM":            strconv.FormatBool(strings.EqualFold(req.ManagementMode, "auto")),
		"savepath":           req.SavePath,
		"rename":             req.Rename,
		"category":           req.Category,
		"tags":               strings.Join(req.Tags, ","),
		"stopped":            strconv.FormatBool(!req.StartTorrent),
		"addToTopOfQueue":    strconv.FormatBool(req.TopOfQueue),
		"stopCondition":      req.StopCondition,
		"skip_checking":      strconv.FormatBool(req.SkipHashCheck),
		"contentLayout":      req.ContentLayout,
		"sequentialDownload": strconv.FormatBool(req.SequentialDownload),
		"firstLastPiecePrio": strconv.FormatBool(req.FirstLastPieceFirst),
	}

	for key, value := range fields {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if err := writer.WriteField(key, value); err != nil {
			return nil, "", fmt.Errorf("write field %s: %w", key, err)
		}
	}
	if req.DownloadLimitKiB != nil {
		if err := writer.WriteField("dlLimit", strconv.Itoa(*req.DownloadLimitKiB*1024)); err != nil {
			return nil, "", fmt.Errorf("write dlLimit: %w", err)
		}
	}
	if req.UploadLimitKiB != nil {
		if err := writer.WriteField("upLimit", strconv.Itoa(*req.UploadLimitKiB*1024)); err != nil {
			return nil, "", fmt.Errorf("write upLimit: %w", err)
		}
	}

	return body, writer.FormDataContentType(), nil
}

func (c *Client) Start(ctx context.Context, hashes []string) error {
	return c.postHashes(ctx, "torrents/start", hashes, nil)
}

func (c *Client) Stop(ctx context.Context, hashes []string) error {
	return c.postHashes(ctx, "torrents/stop", hashes, nil)
}

func (c *Client) Delete(ctx context.Context, hashes []string, deleteFiles bool) error {
	return c.postHashes(ctx, "torrents/delete", hashes, map[string]string{
		"deleteFiles": strconv.FormatBool(deleteFiles),
	})
}

func (c *Client) DirectorySuggestions(ctx context.Context, path string) ([]string, error) {
	if err := c.ensureAuthenticated(ctx); err != nil {
		return nil, err
	}

	parent, prefix := splitRemotePath(path)
	if parent == "" {
		return nil, nil
	}

	values := url.Values{}
	values.Set("dirPath", parent)
	values.Set("mode", "dirs")

	req, err := c.newRequest(ctx, http.MethodGet, "app/getDirectoryContent", nil)
	if err != nil {
		return nil, err
	}
	req.URL.RawQuery = values.Encode()

	var raw []string
	if err := c.doJSON(req, &raw); err != nil {
		return nil, err
	}

	out := make([]string, 0, len(raw))
	for _, item := range raw {
		name := pathBase(item)
		if prefix != "" && !strings.HasPrefix(strings.ToLower(name), strings.ToLower(prefix)) {
			continue
		}
		out = append(out, item)
	}
	slices.Sort(out)

	return out, nil
}

func splitRemotePath(path string) (string, string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", ""
	}
	if strings.HasSuffix(path, "/") || strings.HasSuffix(path, "\\") {
		return path, ""
	}

	idx := strings.LastIndexAny(path, `/\`)
	if idx < 0 {
		return path, ""
	}

	return path[:idx+1], path[idx+1:]
}

func pathBase(path string) string {
	path = strings.TrimRight(path, `/\`)
	idx := strings.LastIndexAny(path, `/\`)
	if idx < 0 {
		return path
	}

	return path[idx+1:]
}

func (c *Client) postHashes(ctx context.Context, endpoint string, hashes []string, extra map[string]string) error {
	if len(hashes) == 0 {
		return nil
	}
	if err := c.ensureAuthenticated(ctx); err != nil {
		return err
	}

	form := url.Values{}
	form.Set("hashes", strings.Join(hashes, "|"))
	for key, value := range extra {
		form.Set(key, value)
	}

	req, err := c.newRequest(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.execute(req)
	if err != nil {
		return fmt.Errorf("request %s: %w", endpoint, err)
	}
	defer closeAndLog(c.logger, resp.Body, "close "+endpoint+" response body")

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

		return fmt.Errorf("%s returned %s: %s", endpoint, resp.Status, strings.TrimSpace(string(body)))
	}

	return nil
}

func (c *Client) ensureAuthenticated(ctx context.Context) error {
	c.mu.Lock()
	alreadyAuthenticated := c.authenticated
	c.mu.Unlock()
	if alreadyAuthenticated {
		return nil
	}

	form := url.Values{}
	form.Set("username", c.username)
	form.Set("password", c.password)

	req, err := c.newRequest(ctx, http.MethodPost, "auth/login", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.execute(req)
	if err != nil {
		return fmt.Errorf("request auth/login: %w", err)
	}
	defer closeAndLog(c.logger, resp.Body, "close auth/login response body")

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	text := strings.TrimSpace(string(body))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("auth/login returned %s: %s", resp.Status, text)
	}
	if !strings.EqualFold(text, "ok.") && !strings.EqualFold(text, "ok") {
		return fmt.Errorf("authentication failed: %s", text)
	}

	c.mu.Lock()
	c.authenticated = true
	c.mu.Unlock()

	return nil
}

func (c *Client) newRequest(ctx context.Context, method string, endpoint string, body io.Reader) (*http.Request, error) {
	path := strings.TrimRight(c.baseURL.Path, "/") + "/api/v2/" + strings.TrimLeft(endpoint, "/")
	target := *c.baseURL
	target.Path = path
	target.RawPath = ""
	target.RawQuery = ""
	target.Fragment = ""

	req, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return nil, fmt.Errorf("build request %s: %w", endpoint, err)
	}

	return req, nil
}

func (c *Client) doJSON(req *http.Request, target any) error {
	resp, err := c.execute(req)
	if err != nil {
		return fmt.Errorf("request %s %s: %w", req.Method, req.URL.String(), err)
	}
	defer closeAndLog(c.logger, resp.Body, "close "+req.URL.Path+" response body")

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

		return fmt.Errorf("%s returned %s: %s", req.URL.Path, resp.Status, strings.TrimSpace(string(body)))
	}

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("decode %s: %w", req.URL.Path, err)
	}

	return nil
}

func (c *Client) execute(req *http.Request) (*http.Response, error) {
	if req == nil || req.URL == nil {
		return nil, errors.New("request URL is empty")
	}

	expectedPathPrefix := strings.TrimRight(c.baseURL.Path, "/") + "/api/v2/"
	if req.URL.Scheme != c.baseURL.Scheme || req.URL.Host != c.baseURL.Host {
		return nil, fmt.Errorf("refusing request to unexpected host %s", req.URL.Redacted())
	}
	if !strings.HasPrefix(req.URL.Path, expectedPathPrefix) {
		return nil, fmt.Errorf("refusing request outside qBittorrent API: %s", req.URL.Path)
	}

	// #nosec G704 -- qbtremotego is a desktop client that intentionally talks to a
	// user-configured qBittorrent server over http(s); requests are built from the
	// validated base URL in newRequest and enforced again here before dispatch.
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func closeAndLog(logger *slog.Logger, closer io.Closer, action string) {
	if err := closer.Close(); err != nil && logger != nil {
		logger.Warn(action, "error", err)
	}
}
