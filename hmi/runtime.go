package hmi

import (
	"context"
	"fmt"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/httpx"
	"github.com/uk0/silk/point"
	"net/http"
	"sync"
	"time"
)

type Store interface {
	point.SampleStore
	Define(context.Context, point.Definition) error
}
type Runtime struct {
	Tags            *core.TagDB
	project         Project
	store           Store
	sources         []*point.HTTPSource
	configs         []Source
	mu              sync.Mutex
	publishMu       sync.Mutex
	errors          map[string]string
	cancel          context.CancelFunc
	done            chan struct{}
	started, closed bool
}

// NewRuntime is explicit setup, not a deserializer: optional Store definitions
// are written here. Call on a worker if persistence can block. Headers supplies
// secrets at runtime; they never enter the saved Project.
func NewRuntime(ctx context.Context, p Project, store Store, headers func(string) http.Header) (*Runtime, error) {
	if ctx == nil {
		return nil, fmt.Errorf("hmi: nil context")
	}
	b, e := p.JSON()
	if e != nil {
		return nil, e
	}
	p, e = Parse(b)
	if e != nil {
		return nil, e
	}
	r := &Runtime{Tags: core.NewTagDB(), project: p, store: store, errors: map[string]string{}}
	for _, d := range p.Points {
		if store != nil {
			if e = store.Define(ctx, d.Definition); e != nil {
				return nil, e
			}
		}
		raw := d.Initial
		if raw == nil {
			switch d.Kind {
			case point.Bool:
				raw = false
			case point.String:
				raw = ""
			default:
				raw = 0.
			}
		}
		raw, e = point.Convert(raw, d.Kind)
		if e != nil {
			return nil, e
		}
		value := core.Value{Raw: raw, Quality: core.QualityGood, Time: time.Now()}
		if reader, ok := store.(interface {
			LoadSample(context.Context, string) (core.Value, bool, error)
		}); ok {
			stored, found, err := reader.LoadSample(ctx, d.Name)
			if err != nil {
				return nil, err
			}
			if found {
				value = stored
			}
		}
		r.Tags.GetOrCreate(d.Name, core.Meta{Unit: d.Unit, Desc: d.Description}).Observe(value)
	}
	for _, s := range p.Sources {
		if !s.Enabled {
			continue
		}
		h := http.Header{}
		for k, v := range s.Headers {
			h.Set(k, v)
		}
		if headers != nil {
			for k, v := range headers(s.ID) {
				h[k] = append([]string(nil), v...)
			}
		}
		client, e := httpx.NewClient(httpx.Options{Timeout: time.Duration(s.TimeoutMS) * time.Millisecond})
		if e != nil {
			return nil, e
		}
		source, e := point.NewHTTPSource(client, httpx.Request{URL: s.URL, Method: s.Method, Header: h, Body: []byte(s.Body)}, s.Mappings, r.Tags, store)
		if e != nil {
			return nil, e
		}
		r.sources = append(r.sources, source)
		r.configs = append(r.configs, s)
		for _, m := range s.Mappings {
			t, _ := r.Tags.Get(m.Name)
			v := t.Value()
			v.Quality = core.QualityUncertain
			t.Observe(v)
		}
	}
	r.Tick(time.Now())
	return r, nil
}

// Publish is the SDK entry point for database-backed manual/driver points.
// HTTP-owned targets reject a second writer. Calls are serialized; persistence
// succeeds before live notification. Do not call Stop from a point callback.
func (r *Runtime) Publish(ctx context.Context, name string, v core.Value) error {
	if ctx == nil {
		return fmt.Errorf("hmi: nil context")
	}
	r.publishMu.Lock()
	defer r.publishMu.Unlock()
	r.mu.Lock()
	closed := r.closed
	r.mu.Unlock()
	if closed {
		return fmt.Errorf("hmi: runtime closed")
	}
	for _, source := range r.configs {
		for _, m := range source.Mappings {
			if m.Name == name {
				return fmt.Errorf("hmi: point owned by HTTP source")
			}
		}
	}
	var kind point.Kind
	for _, p := range r.project.Points {
		if p.Name == name {
			kind = p.Kind
			break
		}
	}
	if kind == "" || v.Quality > core.QualityGood || v.Time.IsZero() {
		return fmt.Errorf("hmi: invalid point/sample")
	}
	if v.Raw != nil || v.Quality == core.QualityGood {
		raw, err := point.Convert(v.Raw, kind)
		if err != nil {
			return err
		}
		v.Raw = raw
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.store != nil {
		if err := r.store.Publish(ctx, name, v); err != nil {
			return err
		}
	}
	tag, _ := r.Tags.Get(name)
	tag.Observe(v)
	return nil
}
func (r *Runtime) Start(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("hmi: nil context")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started || r.closed {
		return fmt.Errorf("hmi: runtime already started/closed")
	}
	ctx, r.cancel = context.WithCancel(ctx)
	r.done = make(chan struct{})
	r.started = true
	go func() {
		var wg sync.WaitGroup
		for i, s := range r.sources {
			cfg := r.configs[i]
			wg.Add(1)
			go func(s *point.HTTPSource, c Source) {
				defer wg.Done()
				_ = s.RunObserved(ctx, time.Duration(c.IntervalMS)*time.Millisecond, c.RepeatWrites, func(e error) {
					r.mu.Lock()
					if e == nil {
						delete(r.errors, c.ID)
					} else {
						r.errors[c.ID] = e.Error()
					}
					r.mu.Unlock()
				})
			}(s, cfg)
		}
		nextPrune := time.Time{}
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case now := <-ticker.C:
				r.Tick(now)
				if r.project.RetentionDays > 0 && !now.Before(nextPrune) {
					nextPrune = now.Add(time.Minute)
					if store, ok := r.store.(interface {
						Prune(context.Context, time.Time, int) (int64, error)
					}); ok {
						_, e := store.Prune(ctx, now.Add(-time.Duration(r.project.RetentionDays)*24*time.Hour), 10000)
						r.mu.Lock()
						if e != nil {
							r.errors["storage"] = e.Error()
						} else {
							delete(r.errors, "storage")
						}
						r.mu.Unlock()
					}
				}
			case <-ctx.Done():
				wg.Wait()
				close(r.done)
				return
			}
		}
	}()
	return nil
}

// Tick marks aged samples Uncertain atomically. A single runtime clock serves
// all points, not one timer per widget. No sources means only this bounded clock.
func (r *Runtime) Tick(now time.Time) {
	for _, d := range r.project.Points {
		if d.StaleMS > 0 {
			if t, ok := r.Tags.Get(d.Name); ok {
				t.MarkStale(now.Add(-time.Duration(d.StaleMS) * time.Millisecond))
			}
		}
	}
}
func (r *Runtime) Errors() map[string]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]string{}
	for k, v := range r.errors {
		out[k] = v
	}
	return out
}
func (r *Runtime) Stop() {
	r.mu.Lock()
	r.closed = true
	cancel, done := r.cancel, r.done
	r.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
	r.publishMu.Lock()
	r.publishMu.Unlock()
}
