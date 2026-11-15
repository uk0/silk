package hmi

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/point"
)

var (
	ErrPointNotFound      = errors.New("hmi: point not found")
	ErrPointReadOnly      = errors.New("hmi: point is read-only")
	ErrPointConflict      = errors.New("hmi: point changed; refresh before writing")
	ErrHistoryUnavailable = errors.New("hmi: point history is unavailable")
)

// PointToken is an opaque, runtime/point-local optimistic-write baseline.
// It is deliberately not serializable or reusable across runtime restarts.
type PointToken struct {
	runtime  *Runtime
	tag      *core.Tag
	revision uint64
}

// PointState owns its scalar sample and definition; Token never enters JSON.
// Writer is the enabled HTTP source ID, or empty for manual/driver points.
type PointState struct {
	Definition point.Definition
	Value      core.Value
	Revision   uint64
	Writer     string
	Writable   bool
	Token      PointToken `json:"-"`
}

// PointState reads live memory only, with no database/network work or writer
// lock. A slow durable write cannot block this read behind its store operation.
func (r *Runtime) PointState(name string) (PointState, error) {
	if r == nil {
		return PointState{}, fmt.Errorf("hmi: runtime closed")
	}
	i, ok := r.pointIndex[name]
	if !ok {
		return PointState{}, ErrPointNotFound
	}
	tag, ok := r.Tags.Get(name)
	if !ok {
		return PointState{}, ErrPointNotFound
	}
	v, revision := tag.Snapshot()
	r.mu.Lock()
	closed := r.closed
	r.mu.Unlock()
	owner, owned := r.writers[name]
	return PointState{Definition: r.project.Points[i].Definition, Value: v, Revision: revision, Writer: owner, Writable: !closed && !owned, Token: PointToken{r, tag, revision}}, nil
}

// PointStates returns an owned, project-ordered view. Each individual sample
// and revision is atomic; this is not a transaction across different points.
func (r *Runtime) PointStates() []PointState {
	if r == nil {
		return nil
	}
	out := make([]PointState, 0, len(r.project.Points))
	for _, d := range r.project.Points {
		if v, err := r.PointState(d.Name); err == nil {
			out = append(out, v)
		}
	}
	return out
}

// SubscribePoints primes each known point, then reports value/quality changes.
// Heartbeats stay silent; explicit PointState/PointStates reads reveal their
// newer time/revision. Callbacks run on publisher goroutines and must not block
// or re-enter runtime writes/Stop. This latest-state stream is not an alarm log.
func (r *Runtime) SubscribePoints(fn func(PointState)) core.CancelFunc {
	if r == nil || fn == nil {
		return func() {}
	}
	r.mu.Lock()
	closed := r.closed
	r.mu.Unlock()
	if closed {
		return func() {}
	}
	cancels := make([]core.CancelFunc, 0, len(r.project.Points))
	for _, d := range r.project.Points {
		name := d.Name
		tag, _ := r.Tags.Get(name)
		cancels = append(cancels, tag.Subscribe(func(core.Value) {
			// Concurrent tag callbacks may arrive out of order. Read the current
			// atomic sample rather than attaching a newer revision to old data.
			if v, err := r.PointState(name); err == nil {
				fn(v)
			}
		}))
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			for _, cancel := range cancels {
				cancel()
			}
		})
	}
}

// WritePoint explicitly writes a new Good, typed manual/driver sample. The
// token is checked at the start of the serialized runtime write, before store
// I/O; a successful durable commit precedes live publication. HTTP-owned points
// reject operator writes. Do not mutate Tags directly as a competing writer.
// Cancellation is not a rollback guarantee: refresh before any manual retry.
func (r *Runtime) WritePoint(ctx context.Context, name string, expected PointToken, raw any) (PointState, error) {
	if r == nil || ctx == nil {
		return PointState{}, fmt.Errorf("hmi: invalid runtime/context")
	}
	r.publishMu.Lock()
	defer r.publishMu.Unlock()
	if text, ok := raw.(string); ok && len(text) > 1<<20 {
		return PointState{}, fmt.Errorf("hmi: string sample exceeds 1 MiB")
	}
	err := r.publishLocked(ctx, name, core.Value{Raw: raw, Quality: core.QualityGood, Time: time.Now()}, &expected)
	if err != nil {
		return PointState{}, err
	}
	return r.PointState(name)
}

func normalizePointSample(v core.Value, kind point.Kind) (core.Value, error) {
	if v.Quality > core.QualityGood || v.Time.IsZero() || !time.Unix(0, v.Time.UnixNano()).Equal(v.Time) {
		return core.Value{}, fmt.Errorf("hmi: invalid point sample quality/time")
	}
	if v.Raw != nil || v.Quality == core.QualityGood {
		raw, err := point.Convert(v.Raw, kind)
		if err != nil {
			return core.Value{}, err
		}
		v.Raw = raw
	}
	if text, ok := v.Raw.(string); ok && len(text) > 1<<20 {
		return core.Value{}, fmt.Errorf("hmi: string sample exceeds 1 MiB")
	}
	return v, nil
}

func (r *Runtime) SupportsHistory() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	closed := r.closed
	r.mu.Unlock()
	_, ok := r.store.(point.HistoryReader)
	return ok && !closed
}

func (r *Runtime) historyReader(ctx context.Context, name string) (point.HistoryReader, error) {
	if ctx == nil {
		return nil, fmt.Errorf("hmi: nil history context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r == nil {
		return nil, fmt.Errorf("hmi: runtime closed")
	}
	r.mu.Lock()
	closed := r.closed
	r.mu.Unlock()
	if closed {
		return nil, fmt.Errorf("hmi: runtime closed")
	}
	if _, ok := r.pointIndex[name]; !ok {
		return nil, ErrPointNotFound
	}
	reader, ok := r.store.(point.HistoryReader)
	if !ok {
		return nil, ErrHistoryUnavailable
	}
	return reader, nil
}

// History and RecentHistory perform store I/O; call on a cancellable worker.
func (r *Runtime) History(ctx context.Context, name string, from, to time.Time, limit int) ([]point.SampleRecord, error) {
	if from.After(to) || limit < 1 || limit > 10000 || !time.Unix(0, from.UnixNano()).Equal(from) || !time.Unix(0, to.UnixNano()).Equal(to) {
		return nil, fmt.Errorf("hmi: invalid history range/limit")
	}
	reader, err := r.historyReader(ctx, name)
	if err != nil {
		return nil, err
	}
	rows, err := reader.ReadHistory(ctx, name, from, to, limit)
	if err != nil {
		return nil, err
	}
	return r.checkHistory(ctx, name, rows, limit, false, &from, &to)
}
func (r *Runtime) RecentHistory(ctx context.Context, name string, limit int) ([]point.SampleRecord, error) {
	if limit < 1 || limit > 10000 {
		return nil, fmt.Errorf("hmi: invalid history limit")
	}
	reader, err := r.historyReader(ctx, name)
	if err != nil {
		return nil, err
	}
	rows, err := reader.RecentHistory(ctx, name, limit)
	if err != nil {
		return nil, err
	}
	return r.checkHistory(ctx, name, rows, limit, true, nil, nil)
}

func (r *Runtime) checkHistory(ctx context.Context, name string, rows []point.SampleRecord, limit int, descending bool, from, to *time.Time) ([]point.SampleRecord, error) {
	if len(rows) > limit {
		return nil, fmt.Errorf("hmi: history reader exceeded requested limit")
	}
	out := make([]point.SampleRecord, len(rows))
	kind := r.project.Points[r.pointIndex[name]].Kind
	for i, row := range rows {
		if i&63 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if row.Name != name {
			return nil, fmt.Errorf("hmi: history returned another point")
		}
		v, err := normalizePointSample(row.Value, kind)
		if err != nil {
			return nil, err
		}
		if from != nil && (v.Time.Before(*from) || v.Time.After(*to)) {
			return nil, fmt.Errorf("hmi: history returned a sample outside requested range")
		}
		if i > 0 {
			previous := out[i-1].Value.Time
			if (descending && v.Time.After(previous)) || (!descending && v.Time.Before(previous)) {
				return nil, fmt.Errorf("hmi: history returned unordered observations")
			}
		}
		out[i] = point.SampleRecord{Name: name, Value: v}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
