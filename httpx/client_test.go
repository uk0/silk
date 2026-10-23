package httpx

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newClient(t *testing.T, o Options) *Client {
	t.Helper()
	c, e := NewClient(o)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(c.CloseIdleConnections)
	return c
}
func TestHTTPMethodsHeadersBodyStatusAndTypedFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/error" {
			w.WriteHeader(422)
			fmt.Fprint(w, `{"error":"invalid"}`)
			return
		}
		if r.Method != "POST" || r.Header.Get("Content-Type") != "application/json" || r.Header.Get("X-Test") != "yes" {
			t.Errorf("bad request %s %v", r.Method, r.Header)
		}
		var v map[string]any
		if err := json.NewDecoder(r.Body).Decode(&v); err != nil || v["name"] != "中文😀" {
			t.Errorf("body=%v err=%v", v, err)
		}
		w.Header().Set("X-Result", "ok")
		fmt.Fprint(w, `{"items":[{"name":"中文😀","n":9223372036854775807,"f":1.25,"ok":true}],"a/b":{"~key":7},"null":null}`)
	}))
	defer server.Close()
	c := newClient(t, Options{})
	req, err := JSONRequest("POST", server.URL, map[string]any{"name": "中文😀"})
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Test", "yes")
	response, err := c.Do(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode() != 200 || response.Header().Get("X-Result") != "ok" {
		t.Fatal(response)
	}
	if v, e := response.Str("/items/0/name"); v != "中文😀" || e != nil {
		t.Fatal(v, e)
	}
	if v, e := response.Int("/items/0/n"); v != 9223372036854775807 || e != nil {
		t.Fatal(v, e)
	}
	if v, e := response.Float("/items/0/f"); v != 1.25 || e != nil {
		t.Fatal(v, e)
	}
	if v, e := response.Bool("/items/0/ok"); !v || e != nil {
		t.Fatal(v, e)
	}
	if v, e := response.Int("/a~1b/~0key"); v != 7 || e != nil {
		t.Fatal(v, e)
	}
	for _, pointer := range []string{"/missing", "/items/1", "/null/key"} {
		if _, e := response.JSON(pointer); !errors.Is(e, ErrMissingField) {
			t.Fatal(pointer, e)
		}
	}
	for _, pointer := range []string{"items", "/items/00", "/items/-", "/~2", "/~"} {
		if _, e := response.JSON(pointer); !errors.Is(e, ErrJSONPointer) {
			t.Fatal(pointer, e)
		}
	}
	if _, e := response.Int("/items/0/name"); !errors.Is(e, ErrType) {
		t.Fatal(e)
	}
	if _, e := response.Int("/items/0/f"); e == nil {
		t.Fatal("fraction truncated")
	}
	bytes := response.Bytes()
	bytes[0] = 'x'
	header := response.Header()
	header.Set("X-Result", "changed")
	if response.Text()[0] != '{' || response.Header().Get("X-Result") != "ok" {
		t.Fatal("response accessors share mutable data")
	}
	value, _ := response.JSON("")
	value.(map[string]any)["items"] = "changed"
	if _, e := response.Str("/items/0/name"); e != nil {
		t.Fatal("JSON accessors share mutable data")
	}
	response, err = c.Get(context.Background(), server.URL+"/error")
	var status *StatusError
	if !errors.As(err, &status) || status.StatusCode != 422 || response == nil {
		t.Fatal(response, err)
	}
	if v, e := response.Str("/error"); v != "invalid" || e != nil {
		t.Fatal(v, e)
	}
}
func TestBodyLimitTimeoutCancellationAndTLS(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/slow":
			<-r.Context().Done()
		case "/gzip":
			w.Header().Set("Content-Encoding", "gzip")
			g := gzip.NewWriter(w)
			_, _ = g.Write([]byte(strings.Repeat("a", 1025)))
			_ = g.Close()
		case "/chunked":
			w.(http.Flusher).Flush()
			fmt.Fprint(w, strings.Repeat("a", 1025))
		default:
			fmt.Fprint(w, strings.Repeat("a", 1025))
		}
	}))
	defer server.Close()
	c := newClient(t, Options{Timeout: 50 * time.Millisecond, MaxResponseBytes: 1024})
	for _, path := range []string{"/", "/gzip", "/chunked"} {
		if _, err := c.Get(context.Background(), server.URL+path); !errors.Is(err, ErrResponseTooLarge) {
			t.Fatal(path, err)
		}
	}
	if _, err := c.Get(context.Background(), server.URL+"/slow"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Get(ctx, server.URL); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	tls := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "secure") }))
	defer tls.Close()
	if _, err := c.Get(context.Background(), tls.URL); err == nil {
		t.Fatal("TLS verification bypassed")
	}
	trusted := newClient(t, Options{HTTPClient: tls.Client()})
	if r, err := trusted.Get(context.Background(), tls.URL); err != nil || r.Text() != "secure" {
		t.Fatal(r, err)
	}
}
func TestAsyncRequestSnapshotAndCompletion(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"header":%q}`, r.Header.Get("X-Test"))
	}))
	defer server.Close()
	c := newClient(t, Options{})
	req := Request{URL: server.URL, Header: http.Header{"X-Test": {"before"}}, Body: []byte("before")}
	f, err := c.DoAsync(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Test", "after")
	req.Body[0] = 'x'
	<-entered
	close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	response, err := f.Await(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if v, e := response.Str("/header"); v != "before" || e != nil {
		t.Fatal(v, e)
	}
}
func TestExtractStrictTypesAndBadInput(t *testing.T) {
	r := &Response{body: []byte(`{"s":"hi","i":123,"f":2.5,"b":true,"o":{"x":1}}`)}
	for _, tc := range []struct {
		kind, pointer string
		want          any
	}{{"str", "/s", "hi"}, {"str", "", r.Text()}, {"int", "/i", int64(123)}, {"float", "/f", 2.5}, {"bool", "/b", true}, {"json", "/o", `{"x":1}`}} {
		got, err := r.Extract(tc.kind, tc.pointer)
		if err != nil || got != tc.want {
			t.Fatal(tc, got, err)
		}
	}
	for _, body := range []string{"", `{} []`, `{broken}`, `1e9999`, `9223372036854775808`, `"12"`, `null`} {
		r := &Response{body: []byte(body)}
		if _, err := r.Int(""); err == nil {
			t.Fatal("invalid integer accepted", body)
		}
	}
	if _, err := (&Response{body: []byte(`1e9999`)}).Float(""); err == nil {
		t.Fatal("infinity accepted")
	}
	c := newClient(t, Options{})
	for _, url := range []string{"file:///etc/passwd", "/relative", "https://", "%"} {
		if _, err := c.Get(context.Background(), url); err == nil {
			t.Fatal(url)
		}
	}
	if _, err := c.Get(nil, "https://example.invalid"); err == nil {
		t.Fatal("nil context")
	}
	for _, o := range []Options{{Timeout: -1}, {MaxResponseBytes: -1}, {MaxResponseBytes: 9223372036854775807}} {
		if _, err := NewClient(o); err == nil {
			t.Fatal(o)
		}
	}
}
