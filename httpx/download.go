package httpx

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/uk0/silk/async"
)

const DefaultMaxDownloadBytes int64 = 256 << 20

var ErrChecksum = errors.New("httpx: SHA-256 mismatch")

// DownloadProgress counts bytes accepted by the destination, not wire bytes.
// Total is -1 when unknown (including transparently decompressed responses).
// Reaching Total does not imply success: EOF/checksum/writer errors still matter.
type DownloadProgress struct{ Received, Total int64 }

type DownloadOptions struct {
	MaxBytes         int64         // zero: 256 MiB; independent of MaxResponseBytes
	ProgressInterval time.Duration // zero: 100 ms; negative: invalid
	ExpectedSHA256   string        // optional 64 hexadecimal characters
	OnProgress       func(DownloadProgress)
}

// DownloadResult contains immutable metadata, never the downloaded payload.
// On failure it may describe a partial write; SHA256 is set only after full EOF.
type DownloadResult struct {
	status          int
	header          http.Header
	received, total int64
	sha256          string
}

func (r *DownloadResult) StatusCode() int     { return r.status }
func (r *DownloadResult) Header() http.Header { return r.header.Clone() }
func (r *DownloadResult) BytesWritten() int64 { return r.received }
func (r *DownloadResult) TotalBytes() int64   { return r.total }
func (r *DownloadResult) SHA256() string      { return r.sha256 }

func normalizeDownloadOptions(o DownloadOptions) (DownloadOptions, error) {
	if o.MaxBytes < 0 || o.MaxBytes == math.MaxInt64 || o.ProgressInterval < 0 {
		return o, errors.New("httpx: invalid download limit or progress interval")
	}
	if o.MaxBytes == 0 {
		o.MaxBytes = DefaultMaxDownloadBytes
	}
	if o.ProgressInterval == 0 {
		o.ProgressInterval = 100 * time.Millisecond
	}
	if o.ExpectedSHA256 != "" {
		if len(o.ExpectedSHA256) != sha256.Size*2 {
			return o, errors.New("httpx: expected SHA-256 must be 64 hex characters")
		}
		decoded, err := hex.DecodeString(o.ExpectedSHA256)
		if err != nil || len(decoded) != sha256.Size {
			return o, errors.New("httpx: expected SHA-256 must be 64 hex characters")
		}
		o.ExpectedSHA256 = strings.ToLower(o.ExpectedSHA256)
	}
	return o, nil
}

// DownloadTo streams a response using a fixed 32 KiB copy buffer and computes
// SHA-256. It does not allocate a response-sized body, retry, choose a filename,
// close dst, or roll back partial writes. Use a temporary file when partial data
// must not become visible, and publish it only after success and your own checks.
// Non-2xx responses never write their error bodies to dst.
//
// OnProgress runs synchronously on the calling/worker goroutine: never touch GUI
// objects from it. Initial/final snapshots bypass the interval; intermediate
// updates are throttled. Use async.Latest + gui.Post to coalesce UI delivery.
// Callbacks/writers must not block indefinitely; context cannot preempt them.
func (c *Client) DownloadTo(ctx context.Context, request Request, dst io.Writer, options DownloadOptions) (result *DownloadResult, err error) {
	if dst == nil {
		return nil, errors.New("httpx: nil download writer")
	}
	o, err := normalizeDownloadOptions(options)
	if err != nil {
		return nil, err
	}
	res, workCtx, cancel, err := c.open(ctx, request)
	if err != nil {
		return nil, err
	}
	defer cancel()
	defer res.Body.Close()
	total := res.ContentLength
	if strings.EqualFold(request.Method, "HEAD") {
		total = 0
	}
	result = &DownloadResult{status: res.StatusCode, header: res.Header.Clone(), total: total}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return result, &StatusError{res.StatusCode}
	}
	if total > o.MaxBytes {
		return result, ErrResponseTooLarge
	}
	lastBytes := int64(-1)
	var lastTime time.Time
	notify := func(force bool) {
		if o.OnProgress != nil && lastBytes != result.received && (force || time.Since(lastTime) >= o.ProgressInterval) {
			lastBytes, lastTime = result.received, time.Now()
			o.OnProgress(DownloadProgress{Received: result.received, Total: total})
		}
	}
	defer func() {
		notify(true)
		if err == nil {
			err = workCtx.Err()
		}
	}()
	notify(true)
	hash := sha256.New()
	buffer := make([]byte, 32<<10)
	emptyReads := 0
	for {
		if err = workCtx.Err(); err != nil {
			return result, err
		}
		// One extra byte detects overflow, but is NEVER written to dst.
		remaining := o.MaxBytes - result.received
		n, readErr := res.Body.Read(buffer[:min(int64(len(buffer)), remaining+1)])
		if n < 0 || n > len(buffer) || int64(n) > remaining+1 {
			return result, errors.New("httpx: invalid reader count")
		}
		if n > 0 {
			emptyReads = 0
			if err = workCtx.Err(); err != nil {
				return result, err
			}
			writeN := min(int64(n), remaining)
			if writeN > 0 {
				nw, writeErr := dst.Write(buffer[:writeN])
				if nw < 0 || int64(nw) > writeN {
					return result, errors.New("httpx: invalid writer count")
				}
				result.received += int64(nw)
				_, _ = hash.Write(buffer[:nw])
				notify(false)
				if writeErr != nil {
					return result, fmt.Errorf("httpx: write download: %w", writeErr)
				}
				if int64(nw) != writeN {
					return result, io.ErrShortWrite
				}
			}
			if int64(n) > remaining {
				return result, ErrResponseTooLarge
			}
		} else if readErr == nil {
			emptyReads++
			if emptyReads >= 100 {
				return result, io.ErrNoProgress
			}
		}
		if readErr != nil {
			if readErr != io.EOF {
				return result, fmt.Errorf("httpx: read download: %w", readErr)
			}
			if total >= 0 && result.received != total {
				return result, io.ErrUnexpectedEOF
			}
			result.sha256 = hex.EncodeToString(hash.Sum(nil))
			if o.ExpectedSHA256 != "" && result.sha256 != o.ExpectedSHA256 {
				return result, ErrChecksum
			}
			return result, nil
		}
	}
}

// DownloadToAsync snapshots request headers/body and uses the client's bounded
// executor. dst and OnProgress remain caller-owned: do not read, close or reuse
// dst until Future.Done. Cancel is cooperative, not rollback; a canceled future
// can still have written bytes. Check success before using/publishing the data.
func (c *Client) DownloadToAsync(ctx context.Context, request Request, dst io.Writer, options DownloadOptions) (*async.Future[*DownloadResult], error) {
	if c == nil || dst == nil {
		return nil, errors.New("httpx: nil client or download writer")
	}
	o, err := normalizeDownloadOptions(options)
	if err != nil {
		return nil, err
	}
	snapshot := request.clone()
	return async.Submit(ctx, c.executor, func(ctx context.Context) (*DownloadResult, error) {
		return c.DownloadTo(ctx, snapshot, dst, o)
	})
}
