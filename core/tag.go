package core

// Real-time TAG data model for SCADA / 组态 (HMI) screens.
//
// This is the "scada" contract shared between the industrial widgets and the
// designer binding layer. It is UI-AGNOSTIC by design: it lives in core and
// MUST NOT import gui, so gui can depend on it without an import cycle. (It
// would read as package scada in isolation; it ships in core purely to sit
// below gui in the import graph — Tag, TagDB, Value, etc. are used as
// core.Tag, core.Value, ... from the widget and binding code.)
//
// Threading: notifications fire on whatever goroutine calls SetValue/Publish
// (typically a background driver-poll goroutine). The model itself knows
// nothing about gui or the main thread. The gui bridge (gui/tagbind.go) is
// what marshals each callback onto the event-loop thread via gui.Post, e.g.:
//
//	func BindTagFloat(t *core.Tag, set func(float64)) core.CancelFunc {
//	    return t.Subscribe(func(v core.Value) {
//	        f := v.Float()
//	        gui.Post(func() { set(f) }) // -> main thread, next frame
//	    })
//	}
//	// e.g. BindTagFloat(tag, tank.SetLevel) / BindTagBool(tag, lamp.SetOn)
//	// For eased motion the bridge may Post a gui.NewAnimation onto SetLevel.
//
// codegen would emit those Bind* calls from the designer's tag bindings.

import (
	"fmt"
	"sync"
	"time"
)

// Quality models OPC-style sample quality.
type Quality uint8

const (
	QualityBad Quality = iota
	QualityUncertain
	QualityGood
)

// Value is one immutable sample: payload plus provenance.
type Value struct {
	Raw     interface{} // comparable scalar: float64 | bool | int64 | string
	Quality Quality
	Time    time.Time
}

// Float coerces the payload to float64 (gauges, tanks, charts read float64).
func (v Value) Float() float64 {
	switch x := v.Raw.(type) {
	case float64:
		return x
	case int64:
		return float64(x)
	case int:
		return float64(x)
	case bool:
		if x {
			return 1
		}
		return 0
	}
	return 0
}

// Bool coerces the payload to bool (lamps, valves, digital I/O).
func (v Value) Bool() bool {
	if b, ok := v.Raw.(bool); ok {
		return b
	}
	return v.Float() != 0
}

// Int coerces the payload to int64.
func (v Value) Int() int64 { return int64(v.Float()) }

// String renders the payload for text/label widgets.
func (v Value) String() string { return fmt.Sprint(v.Raw) }

// Meta is static engineering metadata (drives tank %, gauge span, alarms).
type Meta struct {
	Unit               string  // "℃", "bar", "rpm"
	Min, Max           float64 // engineering range
	LoLo, Lo, Hi, HiHi float64 // alarm limits
	Desc               string  // human-readable description
}

// Subscriber is invoked with each new sample. Subscribe returns a CancelFunc.
type Subscriber func(Value)

// CancelFunc removes a subscription; it is safe to call more than once.
type CancelFunc func()

// Tag is a single named real-time data point: a live value plus subscriber
// fan-out. All methods are safe for concurrent use (driver poller, simulator,
// UI thread).
type Tag struct {
	name string
	meta Meta

	mu   sync.RWMutex
	cur  Value
	subs map[uint64]Subscriber
	next uint64
}

func newTag(name string, m Meta) *Tag {
	return &Tag{name: name, meta: m, subs: make(map[uint64]Subscriber)}
}

// Name returns the tag's registry key.
func (t *Tag) Name() string { return t.name }

// Meta returns the tag's static engineering metadata.
func (t *Tag) Meta() Meta { return t.meta }

// Value returns the latest sample.
func (t *Tag) Value() Value {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.cur
}

// SetValue stamps the payload QualityGood at time.Now and publishes it.
// Callable from any goroutine.
func (t *Tag) SetValue(raw interface{}) {
	t.Publish(Value{Raw: raw, Quality: QualityGood, Time: time.Now()})
}

// Publish stores a fully-formed sample (a driver may set Quality/Time itself)
// and fans out to subscribers — but only when the sample changes. If the
// incoming Raw and Quality both equal the current sample, Publish is a no-op:
// a poll loop that re-reads an unchanged value does not wake the UI
// (notify on change, not on same value).
//
// The subscriber set is snapshotted under the lock and invoked unlocked, so a
// callback that (un)subscribes cannot deadlock. Callable from any goroutine.
func (t *Tag) Publish(v Value) {
	t.mu.Lock()
	if rawEqual(t.cur.Raw, v.Raw) && t.cur.Quality == v.Quality {
		t.mu.Unlock()
		return
	}
	t.cur = v
	fan := make([]Subscriber, 0, len(t.subs))
	for _, s := range t.subs {
		fan = append(fan, s)
	}
	t.mu.Unlock()
	for _, s := range fan {
		s(v)
	}
}

// Subscribe registers fn and immediately primes it with the current sample so
// a freshly-bound widget paints live data at once. The returned CancelFunc is
// idempotent and safe to call from any goroutine (dynamic screens add and
// remove bindings at runtime).
func (t *Tag) Subscribe(fn Subscriber) CancelFunc {
	t.mu.Lock()
	id := t.next
	t.next++
	t.subs[id] = fn
	cur := t.cur
	t.mu.Unlock()
	fn(cur) // prime outside the lock
	var once sync.Once
	return func() {
		once.Do(func() {
			t.mu.Lock()
			delete(t.subs, id)
			t.mu.Unlock()
		})
	}
}

// rawEqual compares two scalar payloads. Tag payloads are the comparable
// scalars documented on Value.Raw, so == is well defined; a differing dynamic
// type counts as a change.
func rawEqual(a, b interface{}) bool { return a == b }

// TagDB is the process-wide tag registry: a thread-safe name -> *Tag map.
type TagDB struct {
	mu   sync.RWMutex
	tags map[string]*Tag
}

// NewTagDB returns an empty registry.
func NewTagDB() *TagDB { return &TagDB{tags: make(map[string]*Tag)} }

// Get returns the tag or (nil, false).
func (db *TagDB) Get(name string) (*Tag, bool) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	t, ok := db.tags[name]
	return t, ok
}

// GetOrCreate atomically returns the existing tag or creates one with meta.
// meta is applied only on creation; a later call for an existing name returns
// the original tag and ignores the passed meta.
func (db *TagDB) GetOrCreate(name string, meta Meta) *Tag {
	db.mu.Lock()
	defer db.mu.Unlock()
	if t, ok := db.tags[name]; ok {
		return t
	}
	t := newTag(name, meta)
	db.tags[name] = t
	return t
}

// SetValue pushes a value by name (a driver need not hold a *Tag). The tag is
// created on first use with zero Meta.
func (db *TagDB) SetValue(name string, raw interface{}) {
	db.GetOrCreate(name, Meta{}).SetValue(raw)
}

// All returns a snapshot slice of every tag (for a tag browser / bind picker).
func (db *TagDB) All() []*Tag {
	db.mu.RLock()
	defer db.mu.RUnlock()
	out := make([]*Tag, 0, len(db.tags))
	for _, t := range db.tags {
		out = append(out, t)
	}
	return out
}

// Default is the singleton registry the designer binding resolves against.
var Default = NewTagDB()
