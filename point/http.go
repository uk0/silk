package point

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/httpx"
	"github.com/uk0/silk/jsonx"
)

// Mapping extracts one JSON Pointer, applies steps, and enforces the final type.
type Mapping struct {
	Name    string `json:"name"`
	Pointer string `json:"pointer"`
	Kind    Kind   `json:"kind"`
	Steps   []Step `json:"steps,omitempty"`
}
type compiledMapping struct {
	name     string
	kind     Kind
	path     jsonx.CompiledPointer
	pipeline *Pipeline
}

// Validate checks a response mapping before its source can issue any requests.
func (m Mapping) Validate() error {
	_, err := m.compile()
	return err
}

func (m Mapping) compile() (compiledMapping, error) {
	if err := (Definition{Name: m.Name, Kind: m.Kind}).Validate(); err != nil {
		return compiledMapping{}, err
	}
	if len(m.Pointer) > 4096 {
		return compiledMapping{}, fmt.Errorf("point: JSON pointer exceeds 4096 bytes: %w", jsonx.ErrPointer)
	}
	path, err := jsonx.CompilePointer(m.Pointer)
	if err != nil {
		return compiledMapping{}, fmt.Errorf("point: response path: %w", err)
	}
	pipeline, err := NewPipeline(m.Steps)
	if err != nil {
		return compiledMapping{}, err
	}
	return compiledMapping{name: m.Name, kind: m.Kind, path: path, pipeline: pipeline}, nil
}

// HTTPSource is inert until Poll or Run. One response feeds all mappings. A
// source has one owner worker; do not concurrently Poll/Run the same instance.
// UI widgets subscribe to Tags, never make per-widget requests. Credentials are
// runtime request headers and are not serialized by this type.
type HTTPSource struct {
	client   *httpx.Client
	request  httpx.Request
	mappings []compiledMapping
	tags     *core.TagDB
	store    SampleStore
}

func NewHTTPSource(client *httpx.Client, req httpx.Request, mappings []Mapping, tags *core.TagDB, store SampleStore) (*HTTPSource, error) {
	if tags == nil || len(mappings) == 0 || len(mappings) > 128 || len(req.Body) > 1<<20 {
		return nil, fmt.Errorf("point: invalid HTTP source bounds")
	}
	u, e := url.Parse(req.URL)
	if e != nil || u.Host == "" || u.User != nil || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("point: invalid HTTP URL")
	}
	if req.Method == "" {
		req.Method = http.MethodGet
	}
	req.Method = strings.ToUpper(req.Method)
	switch req.Method {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD":
	default:
		return nil, fmt.Errorf("point: unsupported HTTP method")
	}
	req.Header = req.Header.Clone()
	req.Body = append([]byte(nil), req.Body...)
	if client == nil {
		client = httpx.Default()
	}
	s := &HTTPSource{client: client, request: req, tags: tags, store: store}
	names := map[string]bool{}
	for _, m := range mappings {
		if names[m.Name] {
			return nil, fmt.Errorf("point: duplicate HTTP target")
		}
		compiled, err := m.compile()
		if err != nil {
			return nil, err
		}
		names[m.Name] = true
		s.mappings = append(s.mappings, compiled)
	}
	return s, nil
}
func (s *HTTPSource) deliver(ctx context.Context, m compiledMapping, v core.Value) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	if s.store != nil {
		if e := s.store.Publish(ctx, m.name, v); e != nil {
			return e
		}
	}
	s.tags.GetOrCreate(m.name, core.Meta{}).Observe(v)
	return nil
}

// Poll makes exactly one request, without retries. Successful fields may publish
// alongside Bad fields; this is NOT an atomic multi-point PLC scan. Bad samples
// retain the last payload, never manufacture a normal zero. Storage errors are
// returned before updating the corresponding live point.
func (s *HTTPSource) Poll(ctx context.Context) error {
	_, err := s.poll(ctx)
	return err
}

// poll also reports whether a Good sample reached its destination. HTTP status
// alone is not enough: parsing, conversion or all writes can still fail.
func (s *HTTPSource) poll(ctx context.Context) (hasGood bool, err error) {
	if ctx == nil {
		return false, fmt.Errorf("point: nil context")
	}
	response, requestErr := s.client.Do(ctx, s.request)
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	var doc *jsonx.Document
	if requestErr == nil {
		doc, requestErr = jsonx.Parse(response.Bytes())
	}
	now := time.Now()
	var first error
	for _, m := range s.mappings {
		var v any
		e := requestErr
		if e == nil {
			var node jsonx.Node
			node, e = m.path.Lookup(doc)
			if e == nil {
				v = node.Value()
			}
		}
		if e == nil {
			v, e = m.pipeline.Apply(v)
		}
		sample := core.Value{Raw: v, Quality: core.QualityGood, Time: now}
		if e == nil {
			sample, e = NormalizeSample(sample, m.kind)
		}
		if e != nil {
			sample = core.Value{Quality: core.QualityBad, Time: now}
			last := s.tags.GetOrCreate(m.name, core.Meta{}).Value().Raw
			if last != nil {
				retained := sample
				retained.Raw = last
				if normalized, err := NormalizeSample(retained, m.kind); err == nil {
					sample = normalized
				}
			}
			if first == nil {
				first = fmt.Errorf("point: source field %s failed: %w", m.name, e)
			}
		}
		if e = s.deliver(ctx, m, sample); e != nil {
			if first == nil {
				first = e
			}
		} else if sample.Quality == core.QualityGood {
			hasGood = true
		}
	}
	return hasGood, first
}

// Run is blocking: start it in an application-owned worker. Fixed delay after
// completion prevents overlapping requests. Errors are reported on partial
// failures without slowing healthy mappings; polls delivering no Good samples
// back off exponentially within a bound. POST/etc require explicit repeat
// consent; read-style POST endpoints may opt in. Cancellation is joined by Run's
// return; no goroutines or queued completions survive it. onError must not block.
func (s *HTTPSource) Run(ctx context.Context, interval time.Duration, repeatWrites bool, onError func(error)) error {
	return s.RunObserved(ctx, interval, repeatWrites, func(e error) {
		if e != nil && onError != nil {
			onError(e)
		}
	})
}

// RunObserved reports each poll completion, including recovery (nil). The
// callback runs on the source worker; it must not block or synchronously join it.
func (s *HTTPSource) RunObserved(ctx context.Context, interval time.Duration, repeatWrites bool, observe func(error)) error {
	if ctx == nil || interval < 100*time.Millisecond || interval > time.Hour {
		return fmt.Errorf("point: invalid polling interval/context")
	}
	if s.request.Method != "GET" && s.request.Method != "HEAD" && !repeatWrites {
		return fmt.Errorf("point: repeating non-read HTTP method requires opt-in")
	}
	delay := interval
	for {
		if e := ctx.Err(); e != nil {
			return e
		}
		hasGood, e := s.poll(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if observe != nil {
			observe(e)
		}
		if e != nil && !hasGood {
			delay = min(max(interval, 30*time.Second), max(interval, delay*2))
		} else {
			delay = interval
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// SampleStore is the optional persistence seam; conversion/HTTP/GUI do not
// import SQLite. point/sqlite.Store implements it, as can another storage SDK.
type SampleStore interface {
	Publish(context.Context, string, core.Value) error
}
