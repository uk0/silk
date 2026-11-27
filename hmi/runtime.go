package hmi

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/httpx"
	"github.com/uk0/silk/point"
)

type Store interface {
	point.SampleStore
	Define(context.Context, point.Definition) error
}

type stalePoint struct {
	name string
	age  time.Duration
}

// RuntimeDiagnostics is an owned snapshot of active runtime faults. HTTP
// source IDs and storage retention faults are in separate namespaces.
type RuntimeDiagnostics struct {
	Sources        map[string]string
	RetentionError string
	retentionKey   string
}

type Runtime struct {
	Tags                *core.TagDB
	project             Project
	store               Store
	pointIndex          map[string]int
	writers             map[string]string
	sources             []*point.HTTPSource
	configs             []Source
	alarms              *alarmMonitor
	stalePoints         []stalePoint
	mu                  sync.Mutex
	publishMu           sync.Mutex
	sourceErrors        map[string]string
	retentionError      string
	retentionErrorKey   string
	diagnosticObservers map[*diagnosticObserver]struct{}
	cancel              context.CancelFunc
	done                chan struct{}
	started, closed     bool
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
	r := &Runtime{
		Tags: core.NewTagDB(), project: p, store: store,
		sourceErrors: make(map[string]string), retentionErrorKey: retentionDiagnosticKey(p.Sources),
		pointIndex: make(map[string]int, len(p.Points)), writers: make(map[string]string),
	}
	for i, d := range p.Points {
		r.pointIndex[d.Name] = i
		if d.StaleMS > 0 {
			r.stalePoints = append(r.stalePoints, stalePoint{d.Name, time.Duration(d.StaleMS) * time.Millisecond})
		}
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
				value, err = point.NormalizeSample(stored, d.Kind)
				if err != nil {
					return nil, fmt.Errorf("hmi: restored point %q: %w", d.Name, err)
				}
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
			r.writers[m.Name] = s.ID
			t, _ := r.Tags.Get(m.Name)
			v := t.Value()
			v.Quality = core.QualityUncertain
			t.Observe(v)
		}
	}
	r.Tick(time.Now())
	r.alarms, e = newAlarmMonitor(p, r.Tags, time.Now)
	if e != nil {
		return nil, e
	}
	return r, nil
}

// Publish is the SDK entry point for database-backed manual/driver points.
// HTTP-owned targets reject a second writer. Calls are serialized; persistence
// succeeds before live notification. Do not call Stop from a point callback.
func (r *Runtime) Publish(ctx context.Context, name string, v core.Value) error {
	if r == nil || ctx == nil {
		return fmt.Errorf("hmi: invalid runtime/context")
	}
	r.publishMu.Lock()
	defer r.publishMu.Unlock()
	return r.publishLocked(ctx, name, v, nil)
}

// publishLocked is called only under the runtime-managed writer lock. Direct
// mutation of Tags bypasses persistence/ownership and is not an operator API.
func (r *Runtime) publishLocked(ctx context.Context, name string, v core.Value, expected *PointToken) error {
	r.mu.Lock()
	closed := r.closed
	r.mu.Unlock()
	if closed {
		return fmt.Errorf("hmi: runtime closed")
	}
	if _, owned := r.writers[name]; owned {
		return fmt.Errorf("%w: point owned by HTTP source", ErrPointReadOnly)
	}
	i, found := r.pointIndex[name]
	if !found {
		return ErrPointNotFound
	}
	kind := r.project.Points[i].Kind
	tag, _ := r.Tags.Get(name)
	if expected != nil {
		_, revision := tag.Snapshot()
		if expected.runtime != r || expected.tag != tag || expected.revision != revision {
			return ErrPointConflict
		}
	}
	v, err := point.NormalizeSample(v, kind)
	if err != nil {
		return fmt.Errorf("hmi: point %q: %w", name, err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.store != nil {
		if err := r.store.Publish(ctx, name, v); err != nil {
			return err
		}
	}
	tag.Observe(v)
	return nil
}

const runtimeServiceInterval = 100 * time.Millisecond

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
					r.setSourceDiagnostic(c.ID, e)
				})
			}(s, cfg)
		}
		if pruner, ok := r.store.(pointPruner); ok && r.project.RetentionDays > 0 {
			wg.Go(func() { r.runRetention(ctx, pruner) })
		}
		var ticks <-chan time.Time
		if ticker := r.newServiceTicker(); ticker != nil {
			defer ticker.Stop()
			ticks = ticker.C
		}
		for {
			select {
			case now := <-ticks:
				r.Tick(now)
			case <-ctx.Done():
				wg.Wait()
				close(r.done)
				return
			}
		}
	}()
	return nil
}

func (r *Runtime) newServiceTicker() *time.Ticker {
	if len(r.stalePoints) == 0 && r.alarms == nil {
		return nil
	}
	return time.NewTicker(runtimeServiceInterval)
}

type pointPruner interface {
	Prune(context.Context, time.Time, int) (int64, error)
}

// Retention is independent of the freshness/alarm clock. A slow storage
// operation cannot delay time-sensitive point quality or alarm transitions.
func (r *Runtime) runRetention(ctx context.Context, pruner pointPruner) {
	age := time.Duration(r.project.RetentionDays) * 24 * time.Hour
	due := time.NewTimer(runtimeServiceInterval)
	defer due.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-due.C:
			if ctx.Err() != nil {
				return
			}
			_, err := pruner.Prune(ctx, now.Add(-age), 10000)
			if ctx.Err() != nil {
				return
			}
			r.setRetentionDiagnostic(err)
			// Avoid catch-up executions if the previous prune was slow.
			due.Reset(time.Minute)
		}
	}
}

// Tick marks aged samples Uncertain atomically. A single runtime clock serves
// the configured freshness policies and alarm dwell; no timer per widget.
func (r *Runtime) Tick(now time.Time) {
	for _, d := range r.stalePoints {
		if t, ok := r.Tags.Get(d.name); ok {
			t.MarkStale(now.Add(-d.age))
		}
	}
	r.alarms.tick(now)
}

// Diagnostics returns the current faults; caller mutations do not affect the
// runtime. RetentionError is empty when storage retention has no active fault.
func (r *Runtime) Diagnostics() RuntimeDiagnostics {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]string, len(r.sourceErrors))
	for k, v := range r.sourceErrors {
		out[k] = v
	}
	return RuntimeDiagnostics{Sources: out, RetentionError: r.retentionError, retentionKey: r.retentionErrorKey}
}

// Errors retains the flat diagnostics view for existing clients. Source keys
// are always their configured IDs. The retention key is "storage" unless a
// configured enabled source has that ID; in that case it uses a distinct, stable
// display key. Use Diagnostics when processing specific categories.
func (r *Runtime) Errors() map[string]string {
	diagnostics := r.Diagnostics()
	return diagnostics.withRetention(diagnostics.Sources)
}

// ErrorMap returns the flat compatibility view of this same diagnostic
// snapshot without modifying Sources. This avoids taking a second snapshot
// when an application needs both categorized and legacy diagnostics.
func (d RuntimeDiagnostics) ErrorMap() map[string]string {
	out := make(map[string]string, len(d.Sources)+1)
	for key, value := range d.Sources {
		out[key] = value
	}
	return d.withRetention(out)
}

func (d RuntimeDiagnostics) withRetention(out map[string]string) map[string]string {
	if d.RetentionError != "" {
		key := d.retentionKey
		if key == "" {
			key = "storage"
		}
		out[key] = d.RetentionError
	}
	return out
}

func retentionDiagnosticKey(sources []Source) string {
	used := make(map[string]bool, len(sources))
	for _, source := range sources {
		if source.Enabled {
			used[source.ID] = true
		}
	}
	if !used["storage"] {
		return "storage"
	}
	if !used["storage (retention)"] {
		return "storage (retention)"
	}
	for i := 2; ; i++ {
		key := fmt.Sprintf("storage (retention %d)", i)
		if !used[key] {
			return key
		}
	}
}
func (r *Runtime) Stop() {
	r.mu.Lock()
	r.closed = true
	for observer := range r.diagnosticObservers {
		observer.fn.Store(nil)
	}
	r.diagnosticObservers = nil
	cancel, done := r.cancel, r.done
	r.mu.Unlock()
	r.alarms.close()
	if cancel != nil {
		cancel()
		<-done
	}
	r.publishMu.Lock()
	r.publishMu.Unlock()
}
