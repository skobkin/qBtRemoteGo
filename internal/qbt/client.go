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
)

type Client struct {
	baseURL    *url.URL
	username   string
	password   string
	apiKey     string
	httpClient *http.Client
	logger     *slog.Logger

	mu            sync.Mutex
	authenticated bool // guarded by mu; login is single-flight under mu
}

type ClientConfig struct {
	URL                  string
	Username             string
	Password             string
	APIKey               string
	SkipCertificateCheck bool
}

const (
	apiKeyPrefix = "qbt_"
	apiKeyLength = 32
)

// ValidateAPIKey reports whether key matches the qBittorrent WebAPI key format:
// 32 characters, prefix "qbt_" followed by 28 alphanumeric characters.
func ValidateAPIKey(key string) error {
	if len(key) != apiKeyLength || !strings.HasPrefix(key, apiKeyPrefix) {
		return errors.New("invalid API key: expected 32 characters starting with \"qbt_\" (generated in qBittorrent Preferences > WebUI > API Key)")
	}
	for _, r := range key[len(apiKeyPrefix):] {
		if !isASCIIAlphanumeric(r) {
			return errors.New("invalid API key: expected only alphanumeric characters after the \"qbt_\" prefix")
		}
	}

	return nil
}

func isASCIIAlphanumeric(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
}

func NewClient(cfg ClientConfig, logger *slog.Logger) (*Client, error) {
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

	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey != "" {
		if err := ValidateAPIKey(apiKey); err != nil {
			return nil, err
		}
	}

	return &Client{
		baseURL:  base,
		username: cfg.Username,
		password: cfg.Password,
		apiKey:   apiKey,
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Jar:       jar,
			Transport: transport,
		},
		logger: logger,
	}, nil
}

func (c *Client) TestConnection(ctx context.Context) error {
	_, err := c.Version(ctx)

	return err
}

// Logger exposes the logger the client was built with, so callers can verify
// which logger a cached client captured.
func (c *Client) Logger() *slog.Logger {
	return c.logger
}

// Version authenticates if needed and returns the qBittorrent server version
// reported by app/version.
func (c *Client) Version(ctx context.Context) (string, error) {
	if err := c.ensureAuthenticated(ctx); err != nil {
		return "", err
	}

	req, err := c.newRequest(ctx, http.MethodGet, "app/version", nil)
	if err != nil {
		return "", err
	}

	resp, err := c.execute(req)
	if err != nil {
		return "", fmt.Errorf("request app/version: %w", err)
	}
	defer closeAndLog(c.logger, resp.Body, "close app/version response body")

	if resp.StatusCode != http.StatusOK {
		body := readLimitedResponseText(resp.Body, 2048)
		c.invalidateSessionOnAuthFailure(resp.StatusCode)
		if c.apiKey != "" && (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden) {
			return "", fmt.Errorf(
				"app/version returned %s: %s. API-key authentication may have failed: the key might be invalid or no longer current "+
					"(qBittorrent invalidates the previous key when it is rotated), the server might not support API keys "+
					"(qBittorrent v5.2.0 or newer is required), or the server or an intermediate proxy might have denied the request",
				resp.Status, body)
		}

		return "", fmt.Errorf("app/version returned %s: %s", resp.Status, body)
	}

	return readLimitedResponseText(resp.Body, 64), nil
}

func (c *Client) Torrents(ctx context.Context) ([]Torrent, error) {
	var torrents []Torrent
	if err := c.getJSON(ctx, "torrents/info", nil, &torrents); err != nil {
		return nil, err
	}
	for i := range torrents {
		torrents[i].AddedAt = time.Unix(torrents[i].AddedUnix, 0)
	}

	return torrents, nil
}

func (c *Client) TransferInfo(ctx context.Context) (TransferInfo, error) {
	var transfer TransferInfo
	if err := c.getJSON(ctx, "transfer/info", nil, &transfer); err != nil {
		return TransferInfo{}, err
	}

	return transfer, nil
}

func (c *Client) ServerState(ctx context.Context) (ServerState, error) {
	query := url.Values{}
	query.Set("rid", "0")

	var data MainData
	if err := c.getJSON(ctx, "sync/maindata", query, &data); err != nil {
		return ServerState{}, err
	}

	return data.ServerState, nil
}

func (c *Client) TorrentProperties(ctx context.Context, hash string) (TorrentProperties, error) {
	var properties TorrentProperties
	if err := c.getJSON(ctx, "torrents/properties", hashQuery(hash), &properties); err != nil {
		return TorrentProperties{}, err
	}

	return properties, nil
}

func (c *Client) TorrentFiles(ctx context.Context, hash string) ([]TorrentFile, error) {
	var files []TorrentFile
	if err := c.getJSON(ctx, "torrents/files", hashQuery(hash), &files); err != nil {
		return nil, err
	}

	return files, nil
}

func (c *Client) TorrentTrackers(ctx context.Context, hash string) ([]TorrentTracker, error) {
	var trackers []TorrentTracker
	if err := c.getJSON(ctx, "torrents/trackers", hashQuery(hash), &trackers); err != nil {
		return nil, err
	}

	return trackers, nil
}

func (c *Client) TorrentWebSeeds(ctx context.Context, hash string) ([]TorrentWebSeed, error) {
	var webSeeds []TorrentWebSeed
	if err := c.getJSON(ctx, "torrents/webseeds", hashQuery(hash), &webSeeds); err != nil {
		return nil, err
	}

	return webSeeds, nil
}

func (c *Client) TorrentPeers(ctx context.Context, hash string, rid int) (TorrentPeersSync, error) {
	query := hashQuery(hash)
	query.Set("rid", strconv.Itoa(rid))

	var peers TorrentPeersSync
	if err := c.getJSON(ctx, "sync/torrentPeers", query, &peers); err != nil {
		return TorrentPeersSync{}, err
	}

	return peers, nil
}

func (c *Client) Categories(ctx context.Context) ([]string, error) {
	var raw map[string]json.RawMessage
	if err := c.getJSON(ctx, "torrents/categories", nil, &raw); err != nil {
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
	var tags []string
	if err := c.getJSON(ctx, "torrents/tags", nil, &tags); err != nil {
		return nil, err
	}
	slices.Sort(tags)

	return tags, nil
}

func (c *Client) DefaultSavePath(ctx context.Context) (string, error) {
	if err := c.ensureAuthenticated(ctx); err != nil {
		return "", err
	}

	req, err := c.newRequest(ctx, http.MethodGet, "app/defaultSavePath", nil)
	if err != nil {
		return "", err
	}

	resp, err := c.execute(req)
	if err != nil {
		return "", fmt.Errorf("request app/defaultSavePath: %w", err)
	}
	defer closeAndLog(c.logger, resp.Body, "close app/defaultSavePath response body")

	if resp.StatusCode != http.StatusOK {
		c.invalidateSessionOnAuthFailure(resp.StatusCode)
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))

		return "", fmt.Errorf("app/defaultSavePath returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", fmt.Errorf("read app/defaultSavePath response: %w", err)
	}

	return strings.TrimSpace(string(body)), nil
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
		c.invalidateSessionOnAuthFailure(resp.StatusCode)
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

func (c *Client) ForceRecheck(ctx context.Context, hashes []string) error {
	return c.postHashes(ctx, "torrents/recheck", hashes, nil)
}

func (c *Client) ForceReannounce(ctx context.Context, hashes []string) error {
	return c.postHashes(ctx, "torrents/reannounce", hashes, nil)
}

func (c *Client) SetLocation(ctx context.Context, hashes []string, location string) error {
	return c.postHashes(ctx, "torrents/setLocation", hashes, map[string]string{
		"location": location,
	})
}

func (c *Client) Rename(ctx context.Context, hash string, name string) error {
	return c.postForm(ctx, "torrents/rename", url.Values{
		"hash": []string{hash},
		"name": []string{name},
	})
}

func (c *Client) Delete(ctx context.Context, hashes []string, deleteFiles bool) error {
	return c.postHashes(ctx, "torrents/delete", hashes, map[string]string{
		"deleteFiles": strconv.FormatBool(deleteFiles),
	})
}

func (c *Client) DirectorySuggestions(ctx context.Context, path string) ([]string, error) {
	parent, prefix := splitRemotePath(path)
	if parent == "" {
		return nil, nil
	}

	query := url.Values{}
	query.Set("dirPath", parent)
	query.Set("mode", "dirs")

	var raw []string
	if err := c.getJSON(ctx, "app/getDirectoryContent", query, &raw); err != nil {
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

	form := url.Values{}
	form.Set("hashes", strings.Join(hashes, "|"))
	for key, value := range extra {
		form.Set(key, value)
	}

	return c.postForm(ctx, endpoint, form)
}

func (c *Client) postForm(ctx context.Context, endpoint string, form url.Values) error {
	if err := c.ensureAuthenticated(ctx); err != nil {
		return err
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
		c.invalidateSessionOnAuthFailure(resp.StatusCode)
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

		return fmt.Errorf("%s returned %s: %s", endpoint, resp.Status, strings.TrimSpace(string(body)))
	}

	return nil
}

// invalidateSessionOnAuthFailure drops the cached login after the server
// rejected a request with an authentication error, so the next request performs
// a fresh login instead of failing until the client is rebuilt. API keys
// authenticate statelessly and never need this.
func (c *Client) invalidateSessionOnAuthFailure(statusCode int) {
	if c.apiKey != "" {
		return
	}
	if statusCode != http.StatusUnauthorized && statusCode != http.StatusForbidden {
		return
	}

	c.mu.Lock()
	c.authenticated = false
	c.mu.Unlock()
}

// ensureAuthenticated performs a login when needed. The mu lock is held across
// the whole check → login → flag update sequence so concurrent fetch goroutines
// sharing one cached client perform a single login instead of racing each other
// into duplicate auth/login calls (each of which can invalidate the previous
// session server-side). A stale 401 from an older session can still clear the
// flag after another goroutine's fresh login, costing one redundant login; that
// path self-heals on the next request.
func (c *Client) ensureAuthenticated(ctx context.Context) error {
	if c.apiKey != "" {
		// API keys authenticate statelessly via the Authorization header; qBittorrent
		// rejects them on the auth endpoints, so login must never be attempted.
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.authenticated {
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

	switch resp.StatusCode {
	case http.StatusOK:
		text := readLimitedResponseText(resp.Body, 2048)
		if !strings.EqualFold(text, "ok.") && !strings.EqualFold(text, "ok") {
			return fmt.Errorf("authentication failed: %s", text)
		}
	case http.StatusNoContent:
	default:
		text := readLimitedResponseText(resp.Body, 2048)

		return fmt.Errorf("auth/login returned %s: %s", resp.Status, text)
	}

	c.authenticated = true

	return nil
}

func readLimitedResponseText(body io.Reader, limit int64) string {
	data, _ := io.ReadAll(io.LimitReader(body, limit))

	return strings.TrimSpace(string(data))
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
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
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
		c.invalidateSessionOnAuthFailure(resp.StatusCode)
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

		return fmt.Errorf("%s returned %s: %s", req.URL.Path, resp.Status, strings.TrimSpace(string(body)))
	}

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("decode %s: %w", req.URL.Path, err)
	}

	return nil
}

// getJSON performs an authenticated GET against an API endpoint and decodes the
// JSON response into target; the shared request path for all read endpoints.
func (c *Client) getJSON(ctx context.Context, endpoint string, query url.Values, target any) error {
	if err := c.ensureAuthenticated(ctx); err != nil {
		return err
	}

	req, err := c.newRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	if len(query) > 0 {
		req.URL.RawQuery = query.Encode()
	}

	return c.doJSON(req, target)
}

func hashQuery(hash string) url.Values {
	query := url.Values{}
	query.Set("hash", strings.TrimSpace(hash))

	return query
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
