// Package httpx provides bounded HTTP responses and strict typed JSON extraction.
// It has no GUI or third-party dependencies. Requests are never retried by this
// package, especially not mutating POST/PATCH requests.
package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/uk0/silk/async"
)

const DefaultMaxResponseBytes int64 = 4 << 20

var ErrResponseTooLarge = errors.New("httpx: response exceeds size limit")

// Options is copied by NewClient. Configure the supplied http.Client and its
// Transport before use; the shared Transport/Jar must be concurrency-safe.
type Options struct {
	HTTPClient       *http.Client
	Timeout          time.Duration // zero: 30 seconds; negative: invalid
	MaxResponseBytes int64         // zero: 4 MiB; negative: invalid
	Executor         *async.Executor
}
type Client struct {
	client   http.Client
	timeout  time.Duration
	maxBytes int64
	executor *async.Executor
}

func NewClient(o Options) (*Client, error) {
	if o.Timeout < 0 || o.MaxResponseBytes < 0 || o.MaxResponseBytes == int64(^uint64(0)>>1) {
		return nil, errors.New("httpx: invalid timeout or response limit")
	}
	if o.Timeout == 0 {
		o.Timeout = 30 * time.Second
	}
	if o.MaxResponseBytes == 0 {
		o.MaxResponseBytes = DefaultMaxResponseBytes
	}
	c := &Client{timeout: o.Timeout, maxBytes: o.MaxResponseBytes, executor: o.Executor}
	if o.HTTPClient != nil {
		c.client = *o.HTTPClient
	}
	return c, nil
}

var defaultClient, _ = NewClient(Options{})

// Default returns a reusable client with a 30-second timeout and 4 MiB limit.
func Default() *Client { return defaultClient }

type Request struct {
	Method string // empty: GET
	URL    string
	Header http.Header
	Body   []byte
}

func (r Request) clone() Request { r.Header = r.Header.Clone(); r.Body = bytes.Clone(r.Body); return r }
func JSONRequest(method, url string, value any) (Request, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return Request{}, fmt.Errorf("httpx: encode request: %w", err)
	}
	return Request{Method: method, URL: url, Header: http.Header{"Content-Type": {"application/json"}, "Accept": {"application/json"}}, Body: body}, nil
}

// StatusError indicates a completed non-2xx response. Do returns its bounded
// Response as well, allowing callers to inspect an API's error payload.
type StatusError struct{ StatusCode int }

func (e *StatusError) Error() string {
	return fmt.Sprintf("httpx: HTTP %d %s", e.StatusCode, http.StatusText(e.StatusCode))
}

// Response owns its body/header. Accessors return copies; read-only methods may
// be called concurrently. JSON numbers are never silently converted to float64.
type Response struct {
	status int
	header http.Header
	body   []byte
}

func (r *Response) StatusCode() int     { return r.status }
func (r *Response) Header() http.Header { return r.header.Clone() }
func (r *Response) Bytes() []byte       { return bytes.Clone(r.body) }
func (r *Response) Text() string        { return string(r.body) }

// open shares timeout, URL validation and transport error redaction across
// buffered and streaming APIs. The caller owns body.Close and cancel.
func (c *Client) open(ctx context.Context, r Request) (*http.Response, context.Context, context.CancelFunc, error) {
	if ctx == nil {
		return nil, nil, nil, errors.New("httpx: nil context")
	}
	if c == nil || c.timeout <= 0 || c.maxBytes <= 0 {
		return nil, nil, nil, errors.New("httpx: client must be created with NewClient")
	}
	u, err := url.Parse(r.URL)
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, nil, nil, errors.New("httpx: absolute HTTP(S) URL required")
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	req, err := http.NewRequestWithContext(ctx, r.Method, r.URL, bytes.NewReader(r.Body))
	if err != nil {
		cancel()
		return nil, nil, nil, errors.New("httpx: invalid method or URL")
	}
	req.Header = r.Header.Clone()
	res, err := c.client.Do(req)
	if err != nil {
		// Do not embed credentials/query parameters from url.Error.URL in errors.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		cancel()
		return nil, nil, nil, fmt.Errorf("httpx: request failed: %w", err)
	}
	return res, ctx, cancel, nil
}

// Do is synchronous: do not call it from a GUI callback. Timeout includes body
// reads; response bodies are always closed, including failures and oversize data.
func (c *Client) Do(ctx context.Context, r Request) (*Response, error) {
	res, _, cancel, err := c.open(ctx, r)
	if err != nil {
		return nil, err
	}
	defer cancel()
	defer res.Body.Close()
	if res.ContentLength > c.maxBytes {
		return nil, ErrResponseTooLarge
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, c.maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("httpx: read response: %w", err)
	}
	if int64(len(body)) > c.maxBytes {
		return nil, ErrResponseTooLarge
	}
	response := &Response{status: res.StatusCode, header: res.Header.Clone(), body: body}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return response, &StatusError{res.StatusCode}
	}
	return response, nil
}
func (c *Client) Get(ctx context.Context, url string) (*Response, error) {
	return c.Do(ctx, Request{URL: url})
}
func (c *Client) PostJSON(ctx context.Context, url string, value any) (*Response, error) {
	r, err := JSONRequest(http.MethodPost, url, value)
	if err != nil {
		return nil, err
	}
	return c.Do(ctx, r)
}

// DoAsync snapshots request headers/body before returning. Do not concurrently
// mutate the input during this call; later changes cannot alter the request.
func (c *Client) DoAsync(ctx context.Context, r Request) (*async.Future[*Response], error) {
	if c == nil {
		return nil, errors.New("httpx: nil client")
	}
	snapshot := r.clone()
	return async.Submit(ctx, c.executor, func(ctx context.Context) (*Response, error) { return c.Do(ctx, snapshot) })
}
func (c *Client) CloseIdleConnections() { c.client.CloseIdleConnections() }

func (c *Client) GetAsync(ctx context.Context, url string) (*async.Future[*Response], error) {
	return c.DoAsync(ctx, Request{URL: url})
}
func (c *Client) PostJSONAsync(ctx context.Context, url string, value any) (*async.Future[*Response], error) {
	r, err := JSONRequest(http.MethodPost, url, value)
	if err != nil {
		return nil, err
	}
	return c.DoAsync(ctx, r)
}
