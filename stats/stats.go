// Package stats computes live per-tag rolling statistics (min/max/avg/count)
// over a core.TagDB stream — running KPIs for FameView 统计 dashboards.
//
// A Collector subscribes to each tracked tag and folds every published sample
// into that tag's Stat. Because core.Tag.Subscribe PRIMES the callback once
// with the tag's current value at subscribe time, the first fold reflects the
// value present when Track was called: Count starts at 1 immediately after
// Track (the primed sample), before any further SetValue/Publish. Callers who
// want clean, predictable counts should set a known initial value before Track
// (or account for that one primed sample). Note also that core.Tag.Publish is
// notify-on-change: publishing a value equal to the current one is a no-op and
// is not folded.
package stats

import (
	"sync"

	"github.com/uk0/silk/core"
)

// Stat is the running aggregate for one tag. Min/Max are only meaningful once
// Count > 0; the first folded sample initializes both.
type Stat struct {
	Count               int
	Min, Max, Sum, Last float64
}

// Avg is Sum/Count, and 0 when Count == 0.
func (s Stat) Avg() float64 {
	if s.Count == 0 {
		return 0
	}
	return s.Sum / float64(s.Count)
}

// Collector folds each tracked tag's value stream into a per-tag Stat. All
// methods are safe for concurrent use: samples fold on whatever goroutine
// drives SetValue/Publish, while readers call Get from any goroutine.
type Collector struct {
	tags *core.TagDB

	mu      sync.RWMutex
	stats   map[string]*Stat
	cancels map[string]core.CancelFunc
}

// NewCollector returns a Collector reading from tags.
func NewCollector(tags *core.TagDB) *Collector {
	return &Collector{
		tags:    tags,
		stats:   make(map[string]*Stat),
		cancels: make(map[string]core.CancelFunc),
	}
}

// Track resolves (creating if needed) the named tag and subscribes to it,
// folding each value's Float() into the tag's Stat. Subscribe primes with the
// tag's current value, so that primed sample is the first fold (Count 1).
// Tracking an already-tracked name is a no-op (it does not re-subscribe).
func (c *Collector) Track(name string) {
	tag := c.tags.GetOrCreate(name, core.Meta{})

	c.mu.Lock()
	if _, ok := c.cancels[name]; ok {
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()

	// Subscribe primes fn synchronously, so it must run without holding c.mu.
	cancel := tag.Subscribe(func(v core.Value) {
		f := v.Float()
		c.mu.Lock()
		s := c.stats[name]
		if s == nil {
			s = &Stat{}
			c.stats[name] = s
		}
		if s.Count == 0 {
			s.Min, s.Max = f, f
		} else {
			if f < s.Min {
				s.Min = f
			}
			if f > s.Max {
				s.Max = f
			}
		}
		s.Count++
		s.Sum += f
		s.Last = f
		c.mu.Unlock()
	})

	c.mu.Lock()
	c.cancels[name] = cancel
	c.mu.Unlock()
}

// Get returns a snapshot copy of the named tag's Stat and whether it is
// tracked. The returned Stat is a value copy and never mutates afterward.
func (c *Collector) Get(name string) (Stat, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	s, ok := c.stats[name]
	if !ok {
		return Stat{}, false
	}
	return *s, true
}

// Reset zeroes the named tag's Stat in place without dropping the
// subscription: subsequent values keep folding from a clean slate.
func (c *Collector) Reset(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if s, ok := c.stats[name]; ok {
		*s = Stat{}
	}
}

// StopAll unsubscribes from every tracked tag. Accumulated Stats remain
// readable via Get; only the subscriptions are torn down.
func (c *Collector) StopAll() {
	c.mu.Lock()
	cancels := c.cancels
	c.cancels = make(map[string]core.CancelFunc)
	c.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}
