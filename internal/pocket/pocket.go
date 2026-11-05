// Package pocket is the bearer-token JSON client shared by every sibling
// Pocket app (Habits, Bookshelf, Travel, Kitchen, Garage, Invoicing, Split,
// Inventory, Money). They all answer /healthz, /api/v1/auth/me and speak the
// same {"error": "..."} on failure.
package pocket

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"time"
)

// Client is a bearer-token JSON client for one Pocket app.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// New creates a client.
func New(baseURL, token string, timeout time.Duration) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), Token: token, HTTP: &http.Client{Timeout: timeout}}
}

// Conn hands a source the client for one fetch: the same URL and token every
// time (a direct connection) or a fresh Pocket Core link token per call, so a
// link switched off in Core stops the next fetch.
type Conn func(ctx context.Context) (*Client, error)

// Fixed is a direct connection: one URL and one token.
func Fixed(baseURL, token string, timeout time.Duration) Conn {
	c := New(baseURL, token, timeout)
	return func(context.Context) (*Client, error) { return c, nil }
}

// ErrNotConnected means the person has no working connection to the app
// (switched off, or the Pocket Core link is off): the source is skipped for
// the day rather than recorded as failed.
var ErrNotConnected = errors.New("not connected")

// Configured reports whether URL and token are set.
func (c *Client) Configured() bool { return c != nil && c.BaseURL != "" && c.Token != "" }

// ErrNotFound is a 404 from the sibling app.
var ErrNotFound = errors.New("not found")

// HTTPError is a sibling app's answer other than 401 and 404: the status and
// its {"error": "..."} message, if any.
type HTTPError struct {
	Status  int
	Message string
}

func (e *HTTPError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("HTTP %d: %s", e.Status, e.Message)
	}
	return fmt.Sprintf("HTTP %d", e.Status)
}

// Do performs a JSON request.
func (c *Client) Do(ctx context.Context, method, path string, q url.Values, body any, v any) error {
	if !c.Configured() {
		return errors.New("not configured")
	}
	u := c.BaseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	return decodeAnswer(res, v)
}

// PostFile sends one file as the multipart field "file", with any other form
// fields, and decodes the JSON answer (Pocket Voice's dictation).
func (c *Client) PostFile(ctx context.Context, path, filename, contentType string, data []byte, fields map[string]string, v any) error {
	if !c.Configured() {
		return errors.New("not configured")
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, val := range fields {
		if val != "" {
			_ = mw.WriteField(k, val)
		}
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", `form-data; name="file"; filename="`+strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(filename)+`"`)
	h.Set("Content-Type", contentType)
	fw, err := mw.CreatePart(h)
	if err != nil {
		return err
	}
	if _, err := fw.Write(data); err != nil {
		return err
	}
	if err := mw.Close(); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", mw.FormDataContentType())
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	return decodeAnswer(res, v)
}

// decodeAnswer reads a Pocket app's answer: the JSON into v, or its
// {"error": "..."} as the error.
func decodeAnswer(res *http.Response, v any) error {
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized {
		return errors.New("token rejected (401)")
	}
	if res.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if res.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		b, _ := io.ReadAll(io.LimitReader(res.Body, 2000))
		_ = json.Unmarshal(b, &e)
		return &HTTPError{Status: res.StatusCode, Message: e.Error}
	}
	if v == nil || res.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.NewDecoder(res.Body).Decode(v)
}

// Get performs a GET.
func (c *Client) Get(ctx context.Context, path string, q url.Values, v any) error {
	return c.Do(ctx, http.MethodGet, path, q, nil, v)
}

// Health is the connection test for any Pocket app: /healthz plus a token check.
func (c *Client) Health(ctx context.Context) (string, error) {
	var out struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}
	if !c.Configured() {
		return "", errors.New("not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/healthz", nil)
	if err != nil {
		return "", err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", res.StatusCode)
	}
	_ = json.NewDecoder(res.Body).Decode(&out)
	var me map[string]any
	if err := c.Get(ctx, "/api/v1/auth/me", nil, &me); err != nil {
		return out.Version, err
	}
	return out.Version, nil
}
