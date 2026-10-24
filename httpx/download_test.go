package httpx

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDownloadKnownUnknownLimitsAndChecksum(t *testing.T) {
	payload := strings.Repeat("Silk 中文😀\n", 10000)
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(payload)))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Probe", "original")
		switch r.URL.Path {
		case "/error":
			w.WriteHeader(503)
			fmt.Fprint(w, "do-not-save")
			return
		case "/chunked":
			w.(http.Flusher).Flush()
		case "/gzip":
			w.Header().Set("Content-Encoding", "gzip")
			zw := gzip.NewWriter(w)
			_, _ = io.WriteString(zw, payload)
			_ = zw.Close()
			return
		default:
			w.Header().Set("Content-Length", fmt.Sprint(len(payload)))
		}
		if r.Method != "HEAD" {
			_, _ = io.WriteString(w, payload)
		}
	}))
	defer server.Close()
	client, _ := NewClient(Options{MaxResponseBytes: 4}) // streaming limit is independent
	for _, path := range []string{"/known", "/chunked", "/gzip"} {
		t.Run(path, func(t *testing.T) {
			var dst bytes.Buffer
			var progress []DownloadProgress
			result, err := client.DownloadTo(context.Background(), Request{URL: server.URL + path}, &dst, DownloadOptions{ExpectedSHA256: strings.ToUpper(digest), ProgressInterval: time.Hour, OnProgress: func(p DownloadProgress) { progress = append(progress, p) }})
			if err != nil || dst.String() != payload || result.BytesWritten() != int64(len(payload)) || result.SHA256() != digest {
				t.Fatal(result, err, dst.Len())
			}
			if len(progress) != 2 || progress[0].Received != 0 || progress[1].Received != int64(len(payload)) {
				t.Fatal("initial/final throttle", progress)
			}
			wantTotal := int64(len(payload))
			if path != "/known" {
				wantTotal = -1
			}
			if result.TotalBytes() != wantTotal || progress[0].Total != wantTotal {
				t.Fatal(result.TotalBytes(), progress)
			}
			h := result.Header()
			h.Set("X-Probe", "changed")
			if result.Header().Get("X-Probe") != "original" {
				t.Fatal("mutable headers")
			}
		})
	}
	for _, path := range []string{"/known", "/chunked", "/gzip"} {
		var dst bytes.Buffer
		r, err := client.DownloadTo(context.Background(), Request{URL: server.URL + path}, &dst, DownloadOptions{MaxBytes: 17})
		if !errors.Is(err, ErrResponseTooLarge) || dst.Len() > 17 || r.BytesWritten() != int64(dst.Len()) {
			t.Fatal(path, r, err, dst.Len())
		}
	}
	var dst bytes.Buffer
	r, err := client.DownloadTo(context.Background(), Request{URL: server.URL + "/error"}, &dst, DownloadOptions{})
	var status *StatusError
	if !errors.As(err, &status) || status.StatusCode != 503 || dst.Len() != 0 || r.StatusCode() != 503 {
		t.Fatal(r, err)
	}
	r, err = client.DownloadTo(context.Background(), Request{Method: "HEAD", URL: server.URL}, &dst, DownloadOptions{})
	if err != nil || r.TotalBytes() != 0 || r.BytesWritten() != 0 {
		t.Fatal(r, err)
	}
	r, err = client.DownloadTo(context.Background(), Request{URL: server.URL}, io.Discard, DownloadOptions{ExpectedSHA256: strings.Repeat("0", 64)})
	if !errors.Is(err, ErrChecksum) || r.SHA256() != digest {
		t.Fatal(r, err)
	}
}

type downloadWriter func([]byte) (int, error)

func (w downloadWriter) Write(p []byte) (int, error) { return w(p) }

type downloadTransport func(*http.Request) (*http.Response, error)

func (f downloadTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type downloadBody struct {
	io.Reader
	closed bool
}

func (b *downloadBody) Close() error { b.closed = true; return nil }

type downloadReader func([]byte) (int, error)

func (f downloadReader) Read(p []byte) (int, error) { return f(p) }

func TestDownloadClosesBodyOnWriterReadAndProgressFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		writer io.Writer
		reader io.Reader
		want   error
	}{
		{"short", downloadWriter(func([]byte) (int, error) { return 1, nil }), strings.NewReader("abc"), io.ErrShortWrite},
		{"writer-error", downloadWriter(func([]byte) (int, error) { return 1, io.ErrClosedPipe }), strings.NewReader("abc"), io.ErrClosedPipe},
		{"read-error", io.Discard, downloadReader(func(p []byte) (int, error) { copy(p, "ab"); return 2, io.ErrUnexpectedEOF }), io.ErrUnexpectedEOF},
		{"empty-loop", io.Discard, downloadReader(func([]byte) (int, error) { return 0, nil }), io.ErrNoProgress},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := &downloadBody{Reader: tc.reader}
			client, _ := NewClient(Options{HTTPClient: &http.Client{Transport: downloadTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, ContentLength: -1, Body: body}, nil
			})}})
			r, err := client.DownloadTo(context.Background(), Request{URL: "http://example.invalid"}, tc.writer, DownloadOptions{})
			if !errors.Is(err, tc.want) || !body.closed || r.SHA256() != "" {
				t.Fatal(r, err, body.closed)
			}
		})
	}
	body := &downloadBody{Reader: strings.NewReader("abc")}
	client, _ := NewClient(Options{HTTPClient: &http.Client{Transport: downloadTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, ContentLength: 3, Body: body}, nil
	})}})
	func() {
		defer func() {
			if recover() == nil {
				t.Error("panic swallowed")
			}
		}()
		_, _ = client.DownloadTo(context.Background(), Request{URL: "http://example.invalid"}, io.Discard, DownloadOptions{OnProgress: func(DownloadProgress) { panic("progress") }})
	}()
	if !body.closed {
		t.Fatal("callback panic leaked body")
	}
}

func TestDownloadAsyncSnapshotsCancellationAndValidation(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var requests atomic.Int32
	client, _ := NewClient(Options{HTTPClient: &http.Client{Transport: downloadTransport(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		close(entered)
		<-release
		b, _ := io.ReadAll(r.Body)
		if string(b) != "original" || r.Header.Get("X-Probe") != "original" {
			t.Error("request mutated")
		}
		return &http.Response{StatusCode: 200, ContentLength: 3, Body: io.NopCloser(strings.NewReader("abc"))}, nil
	})}})
	request := Request{Method: "POST", URL: "http://example.invalid", Header: http.Header{"X-Probe": {"original"}}, Body: []byte("original")}
	var dst bytes.Buffer
	f, err := client.DownloadToAsync(context.Background(), request, &dst, DownloadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	<-entered
	request.Header.Set("X-Probe", "changed")
	copy(request.Body, "changed!")
	close(release)
	r, err := f.Await(context.Background())
	if err != nil || r.BytesWritten() != 3 || dst.String() != "abc" {
		t.Fatal(r, err)
	}
	for _, o := range []DownloadOptions{{MaxBytes: -1}, {ProgressInterval: -1}, {ExpectedSHA256: "bad"}, {ExpectedSHA256: strings.Repeat("0", 1<<20)}} {
		if _, err := client.DownloadToAsync(context.Background(), request, io.Discard, o); err == nil {
			t.Fatal("bad options admitted")
		}
	}
	if requests.Load() != 1 {
		t.Fatal("invalid options performed I/O")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Default().DownloadToAsync(ctx, Request{URL: "http://example.invalid"}, io.Discard, DownloadOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := Default().DownloadTo(nil, request, io.Discard, DownloadOptions{}); err == nil {
		t.Fatal("nil context")
	}
	if _, err := Default().DownloadTo(context.Background(), request, nil, DownloadOptions{}); err == nil {
		t.Fatal("nil writer")
	}
	if _, err := Default().DownloadTo(context.Background(), Request{URL: "file:///tmp/no"}, io.Discard, DownloadOptions{}); err == nil {
		t.Fatal("non-http")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.(http.Flusher).Flush()
		fmt.Fprint(w, "1234")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	dst.Reset()
	// Cancel after the writer accepts the payload, not from a throttled progress
	// callback: on Windows multiple reads can share the same clock tick even
	// when ProgressInterval is one nanosecond. No further read is guaranteed.
	writer := downloadWriter(func(p []byte) (int, error) {
		n, err := dst.Write(p)
		if dst.Len() >= 4 {
			cancel()
		}
		return n, err
	})
	r, err = Default().DownloadTo(ctx, Request{URL: server.URL}, writer, DownloadOptions{})
	if !errors.Is(err, context.Canceled) || r.BytesWritten() != 4 || dst.Len() != 4 {
		t.Fatal(r, err, dst.Len())
	}
}

func TestDownloadTimeoutAndTruncatedBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow" {
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			return
		}
		w.Header().Set("Content-Length", "20")
		fmt.Fprint(w, "short")
	}))
	defer server.Close()
	client, _ := NewClient(Options{Timeout: 40 * time.Millisecond})
	_, err := client.DownloadTo(context.Background(), Request{URL: server.URL + "/slow"}, io.Discard, DownloadOptions{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	r, err := Default().DownloadTo(context.Background(), Request{URL: server.URL}, io.Discard, DownloadOptions{})
	if !errors.Is(err, io.ErrUnexpectedEOF) || r.BytesWritten() != 5 || r.SHA256() != "" {
		t.Fatal(r, err)
	}
}

func TestDownloadBoundaryAndBodyClosure(t *testing.T) {
	for _, tc := range []struct {
		name        string
		size, limit int64
		status      int
		want        error
	}{
		{"exact", 17, 17, 200, nil}, {"one-over", 18, 17, 200, ErrResponseTooLarge},
		{"empty", 0, 1, 200, nil}, {"status", 20, 17, 404, &StatusError{404}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := &downloadBody{Reader: io.LimitReader(downloadZeros{}, tc.size)}
			client, _ := NewClient(Options{HTTPClient: &http.Client{Transport: downloadTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, ContentLength: -1, Body: body}, nil
			})}})
			var dst bytes.Buffer
			result, err := client.DownloadTo(context.Background(), Request{URL: "http://example.invalid"}, &dst, DownloadOptions{MaxBytes: tc.limit})
			if !body.closed || int64(dst.Len()) > tc.limit {
				t.Fatal("closure/limit", body.closed, dst.Len())
			}
			if tc.status == 404 {
				var status *StatusError
				if !errors.As(err, &status) || dst.Len() != 0 {
					t.Fatal(err, dst.Len())
				}
				return
			}
			if !errors.Is(err, tc.want) || result.BytesWritten() != min(tc.size, tc.limit) {
				t.Fatal(result, err)
			}
		})
	}
}

type downloadZeros struct{}

func (downloadZeros) Read(p []byte) (int, error) { clear(p); return len(p), nil }
func BenchmarkDownloadStreamingMemory(b *testing.B) {
	for _, size := range []int64{1 << 20, 16 << 20} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			client, _ := NewClient(Options{HTTPClient: &http.Client{Transport: downloadTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, ContentLength: size, Body: io.NopCloser(io.LimitReader(downloadZeros{}, size))}, nil
			})}})
			b.SetBytes(size)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				r, err := client.DownloadTo(context.Background(), Request{URL: "http://example.invalid"}, io.Discard, DownloadOptions{})
				if err != nil || r.BytesWritten() != size {
					b.Fatal(r, err)
				}
			}
		})
	}
}
