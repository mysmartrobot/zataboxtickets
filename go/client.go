// Package zatabox is the official Go SDK for the Zatabox Tickets REST API 
// the white-label event-ticketing platform.
//
// Zero third-party dependencies (standard library only). Full coverage of all
// 310 endpoints; the per-endpoint methods in resources_gen.go are generated from
// the canonical spec so this SDK never drifts from the API.
//
//	z := zatabox.New("vt_live_...")           // vt_test_ auto-routes to sandbox
//	data, err := z.Events.List(ctx, zatabox.WithQuery(map[string]interface{}{"q": "jazz"}))
//
// See ../spec/CONVENTIONS.md for the cross-language contract.
package zatabox

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Version of the SDK.
const Version = "0.3.0"

const (
	defaultLive    = "https://api.zatabox.com"
	defaultSandbox = "https://sandbox.zatabox.com"
)

// Error is returned for any non-2xx response (and transport failures).
type Error struct {
	Code       string          // stable machine code, e.g. "TICKET_SOLD_OUT"
	Message    string          // human-readable message
	Status     int             // HTTP status
	RequestID  string          // meta.request_id hand this to support
	Details    json.RawMessage // optional structured details
	RetryAfter string          // Retry-After header value on 429s
}

func (e *Error) Error() string {
	return fmt.Sprintf("zatabox: %s (status %d, request_id %q): %s", e.Code, e.Status, e.RequestID, e.Message)
}

// Client is the entry point for the Zatabox API.
type Client struct {
	services

	apiKey      string
	bearerToken string
	baseURL     string
	userAgent   string
	httpClient  *http.Client
	maxRetries  int
}

// Option configures a Client.
type Option func(*Client)

// WithBearerToken authenticates with a portal JWT or vt_mcp_ token.
func WithBearerToken(token string) Option { return func(c *Client) { c.bearerToken = token } }

// WithBaseURL overrides the base URL (wins over key-prefix routing).
func WithBaseURL(u string) Option { return func(c *Client) { c.baseURL = strings.TrimRight(u, "/") } }

// WithHTTPClient supplies a custom *http.Client.
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.httpClient = h } }

// WithMaxRetries sets the transient-failure retry count (default 2).
func WithMaxRetries(n int) Option { return func(c *Client) { c.maxRetries = n } }

// WithUserAgent overrides the User-Agent header.
func WithUserAgent(s string) Option { return func(c *Client) { c.userAgent = s } }

// New creates a Client. Pass a vt_live_/vt_test_ API key (test keys auto-route to
// the sandbox), or "" plus WithBearerToken for portal/MCP auth.
func New(apiKey string, opts ...Option) *Client {
	c := &Client{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		maxRetries: 2,
		userAgent:  "zatabox-go/" + Version,
	}
	for _, o := range opts {
		o(c)
	}
	if c.baseURL == "" {
		c.baseURL = resolveBaseURL(apiKey)
	}
	c.initServices()
	return c
}

// SetBearerToken swaps the credential at runtime (e.g. after a refresh).
func (c *Client) SetBearerToken(token string) { c.bearerToken = token }

func (c *Client) token() string {
	if c.apiKey != "" {
		return c.apiKey
	}
	return c.bearerToken
}

func resolveBaseURL(apiKey string) string {
	if strings.HasPrefix(apiKey, "vt_test_") || strings.HasPrefix(apiKey, "sk_test_") {
		return defaultSandbox
	}
	return defaultLive
}

// ── request options ──────────────────────────────────────────────────────────

type requestConfig struct {
	body           interface{}
	query          map[string]interface{}
	idempotencyKey string
	headers        map[string]string
}

// RequestOption customizes a single API call.
type RequestOption func(*requestConfig)

// WithBody sets the JSON request body (writes).
func WithBody(v interface{}) RequestOption { return func(c *requestConfig) { c.body = v } }

// WithQuery sets query parameters.
func WithQuery(q map[string]interface{}) RequestOption { return func(c *requestConfig) { c.query = q } }

// WithIdempotencyKey sets the Idempotency-Key header (auto-generated otherwise).
func WithIdempotencyKey(k string) RequestOption { return func(c *requestConfig) { c.idempotencyKey = k } }

// WithHeader adds a request header.
func WithHeader(k, v string) RequestOption {
	return func(c *requestConfig) {
		if c.headers == nil {
			c.headers = map[string]string{}
		}
		c.headers[k] = v
	}
}

func buildConfig(opts []RequestOption) *requestConfig {
	cfg := &requestConfig{}
	for _, o := range opts {
		o(cfg)
	}
	return cfg
}

func firstQuery(queryArgs []map[string]interface{}) map[string]interface{} {
	if len(queryArgs) > 0 {
		return queryArgs[0]
	}
	return nil
}

// ── URL building ─────────────────────────────────────────────────────────────

// enc URL-encodes a single path segment.
func enc(v string) string { return url.PathEscape(v) }

func (c *Client) url(path string, query map[string]interface{}) string {
	u := c.baseURL + path
	qs := buildQuery(query)
	if qs != "" {
		if strings.Contains(u, "?") {
			u += "&"
		} else {
			u += "?"
		}
		u += qs
	}
	return u
}

func buildQuery(q map[string]interface{}) string {
	if len(q) == 0 {
		return ""
	}
	vals := url.Values{}
	for k, v := range q {
		if v == nil {
			continue
		}
		switch t := v.(type) {
		case []string:
			for _, s := range t {
				vals.Add(k, s)
			}
		case []interface{}:
			for _, s := range t {
				vals.Add(k, fmt.Sprint(s))
			}
		case bool:
			vals.Add(k, strconv.FormatBool(t))
		default:
			vals.Add(k, fmt.Sprint(v))
		}
	}
	return vals.Encode()
}

// ── transport ────────────────────────────────────────────────────────────────

// do issues a JSON request and returns the unwrapped `data` document.
func (c *Client) do(ctx context.Context, method, path string, opts []RequestOption) (json.RawMessage, error) {
	cfg := buildConfig(opts)
	status, data, hdr, err := c.execute(ctx, method, path, cfg, "application/json")
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, errorFrom(status, data, hdr)
	}
	return unwrap(data), nil
}

// doRaw issues a request and returns the raw body plus its content type (binary).
func (c *Client) doRaw(ctx context.Context, method, path string, opts []RequestOption) ([]byte, string, error) {
	cfg := buildConfig(opts)
	status, data, hdr, err := c.execute(ctx, method, path, cfg, "*/*")
	if err != nil {
		return nil, "", err
	}
	if status >= 400 {
		return nil, "", errorFrom(status, data, hdr)
	}
	return data, hdr.Get("Content-Type"), nil
}

func (c *Client) execute(ctx context.Context, method, path string, cfg *requestConfig, accept string) (int, []byte, http.Header, error) {
	fullURL := c.url(path, cfg.query)
	var bodyBytes []byte
	if cfg.body != nil && method != "GET" {
		b, err := json.Marshal(cfg.body)
		if err != nil {
			return 0, nil, nil, err
		}
		bodyBytes = b
	}

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		var reader io.Reader
		if bodyBytes != nil {
			reader = bytes.NewReader(bodyBytes)
		}
		req, err := http.NewRequestWithContext(ctx, method, fullURL, reader)
		if err != nil {
			return 0, nil, nil, err
		}
		req.Header.Set("Accept", accept)
		req.Header.Set("User-Agent", c.userAgent)
		req.Header.Set("Authorization", "Bearer "+c.token())
		if bodyBytes != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if method != "GET" {
			key := cfg.idempotencyKey
			if key == "" {
				key = newUUID()
			}
			req.Header.Set("Idempotency-Key", key)
		}
		for k, v := range cfg.headers {
			req.Header.Set(k, v)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			if attempt < c.maxRetries {
				time.Sleep(backoff(attempt))
				continue
			}
			return 0, nil, nil, &Error{Code: "NETWORK_ERROR", Message: err.Error()}
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode >= 500 && attempt < c.maxRetries {
			lastErr = errorFrom(resp.StatusCode, data, resp.Header)
			time.Sleep(backoff(attempt))
			continue
		}
		return resp.StatusCode, data, resp.Header, nil
	}
	if lastErr != nil {
		var e *Error
		if errors.As(lastErr, &e) {
			return 0, nil, nil, e
		}
		return 0, nil, nil, &Error{Code: "NETWORK_ERROR", Message: lastErr.Error()}
	}
	return 0, nil, nil, &Error{Code: "NETWORK_ERROR", Message: "request failed"}
}

func backoff(attempt int) time.Duration {
	return time.Duration(1<<uint(attempt)) * 200 * time.Millisecond
}

func unwrap(b []byte) json.RawMessage {
	if len(b) == 0 {
		return nil
	}
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(b, &env); err == nil && len(env.Data) > 0 {
		return env.Data
	}
	return json.RawMessage(b)
}

func errorFrom(status int, body []byte, hdr http.Header) *Error {
	var env struct {
		Error struct {
			Code    string          `json:"code"`
			Message string          `json:"message"`
			Details json.RawMessage `json:"details"`
		} `json:"error"`
		Meta struct {
			RequestID string `json:"request_id"`
		} `json:"meta"`
	}
	_ = json.Unmarshal(body, &env)
	code := env.Error.Code
	if code == "" {
		if status == 429 {
			code = "RATE_LIMITED"
		} else {
			code = fmt.Sprintf("HTTP_%d", status)
		}
	}
	msg := env.Error.Message
	if msg == "" {
		msg = string(body)
	}
	e := &Error{Code: code, Message: msg, Status: status, RequestID: env.Meta.RequestID, Details: env.Error.Details}
	if status == 429 && hdr != nil {
		e.RetryAfter = hdr.Get("Retry-After")
	}
	return e
}

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// ── pagination ───────────────────────────────────────────────────────────────

// ListFunc is the signature of every generated list method (e.g. c.Organizer.Events).
type ListFunc func(ctx context.Context, opts ...RequestOption) (json.RawMessage, error)

// Paginate calls list repeatedly, following either cursor shape, invoking fn for
// each page until the cursor is exhausted (or fn returns an error).
//
//	err := z.Paginate(ctx, z.Organizer.Events, map[string]interface{}{"limit": 50}, func(page json.RawMessage) error {
//		// decode page ...
//		return nil
//	})
func (c *Client) Paginate(ctx context.Context, list ListFunc, query map[string]interface{}, fn func(json.RawMessage) error) error {
	cursor, _ := query["cursor"].(string)
	for {
		q := map[string]interface{}{}
		for k, v := range query {
			q[k] = v
		}
		if cursor != "" {
			q["cursor"] = cursor
		}
		page, err := list(ctx, WithQuery(q))
		if err != nil {
			return err
		}
		if err := fn(page); err != nil {
			return err
		}
		cursor = nextCursor(page)
		if cursor == "" {
			break
		}
	}
	return nil
}

func nextCursor(page json.RawMessage) string {
	var p struct {
		Pagination struct {
			Cursor string `json:"cursor"`
		} `json:"pagination"`
		NextCursor string `json:"nextCursor"`
		Meta       struct {
			Cursor string `json:"cursor"`
		} `json:"meta"`
	}
	_ = json.Unmarshal(page, &p)
	if p.Pagination.Cursor != "" {
		return p.Pagination.Cursor
	}
	if p.NextCursor != "" {
		return p.NextCursor
	}
	return p.Meta.Cursor
}

// ── webhooks ─────────────────────────────────────────────────────────────────

// Verify validates an inbound webhook signature; returns the raw event JSON or an error.
func (s *WebhooksService) Verify(payload []byte, signatureHeader, secret string) (json.RawMessage, error) {
	return verifyWebhook(payload, signatureHeader, secret, 300)
}

func verifyWebhook(payload []byte, signatureHeader, secret string, toleranceSec int64) (json.RawMessage, error) {
	if signatureHeader == "" {
		return nil, &Error{Code: "MISSING_SIGNATURE", Message: "Signature header is required."}
	}
	if secret == "" {
		return nil, errors.New("verifyWebhook: pass the endpoint secret")
	}
	var t, sig string
	for _, piece := range strings.Split(signatureHeader, ",") {
		kv := strings.SplitN(piece, "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch strings.TrimSpace(kv[0]) {
		case "t":
			t = kv[1]
		case "v1":
			sig = kv[1]
		}
	}
	if t == "" || sig == "" {
		return nil, &Error{Code: "INVALID_SIGNATURE", Message: "Malformed signature header."}
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(t + "." + string(payload)))
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(sig)) {
		return nil, &Error{Code: "INVALID_SIGNATURE", Message: "Signature mismatch."}
	}
	if toleranceSec > 0 {
		if ts, err := strconv.ParseInt(t, 10, 64); err == nil {
			d := time.Now().Unix() - ts
			if d < 0 {
				d = -d
			}
			if d > toleranceSec {
				return nil, &Error{Code: "SIGNATURE_EXPIRED", Message: "Timestamp outside tolerance."}
			}
		}
	}
	return json.RawMessage(payload), nil
}
