package core

// SCADA / 组态 limit-based alarm engine.
//
// This sits beside the real-time tag model (tag.go) and speaks the same
// contract: it is UI-AGNOSTIC and MUST NOT import gui, so gui can depend on it
// without an import cycle. EvaluateAlarm maps a live value plus a tag's static
// Meta limits (LoLo/Lo/Hi/HiHi) to an AlarmSeverity; AlarmDB tracks the raise /
// acknowledge / clear lifecycle of each tag's alarm, keeps a bounded history
// ring for an audit list, and fans transitions out to subscribers (a UI alarm
// banner posts them onto the event-loop thread the same way tag bindings do).
//
// Threading: AlarmDB is safe for concurrent use. Subscriber callbacks fire on
// whatever goroutine drove the transition (typically a driver-poll goroutine),
// unlocked, so a callback may freely call back into the db.

import (
	"sort"
	"sync"
	"time"
)

// AlarmSeverity classifies a value against a tag's alarm limits.
//
// Ordering (None < Low < High < LowLow < HighHigh) is the numeric severity
// rank used to sort the active-alarm list; larger is "more urgent" on each
// side, and the LoLo/HiHi trip limits outrank the Lo/Hi warning limits.
type AlarmSeverity int

const (
	None     AlarmSeverity = iota // value within limits (no alarm)
	Low                           // value <= Lo   (low warning)
	High                          // value >= Hi   (high warning)
	LowLow                        // value <= LoLo (low trip)
	HighHigh                      // value >= HiHi (high trip)
)

// String renders the short SCADA label for the severity.
func (s AlarmSeverity) String() string {
	switch s {
	case None:
		return "None"
	case Low:
		return "Lo"
	case High:
		return "Hi"
	case LowLow:
		return "LoLo"
	case HighHigh:
		return "HiHi"
	default:
		return "?"
	}
}

// IsAlarm reports whether the severity represents an active alarm condition.
func (s AlarmSeverity) IsAlarm() bool { return s != None }

// limitSet reports whether an individual alarm limit is configured.
//
// Unset convention: the zero value Meta{} arms no alarms. A limit equal to
// exactly 0 counts as unconfigured only while the engineering range is itself
// unset (Min == 0 && Max == 0) — i.e. the whole Meta is a zero value. Any
// non-zero limit is always active. Once a real range is configured, a 0 limit
// is honored as a genuine threshold, so a high-only (or low-only) setup should
// leave Min/Max at 0, or set every limit explicitly.
func limitSet(limit float64, m Meta) bool {
	return limit != 0 || m.Min != 0 || m.Max != 0
}

// EvaluateAlarm classifies value against the limits in m. Boundaries are
// inclusive: value <= LoLo -> LowLow, else <= Lo -> Low, else >= HiHi ->
// HighHigh, else >= Hi -> High, else None. Each limit is skipped when unset
// (see limitSet), so a zero Meta{} always yields None.
func EvaluateAlarm(value float64, m Meta) AlarmSeverity {
	// Low side, most-severe (trip) first.
	if limitSet(m.LoLo, m) && value <= m.LoLo {
		return LowLow
	}
	if limitSet(m.Lo, m) && value <= m.Lo {
		return Low
	}
	// High side, most-severe (trip) first.
	if limitSet(m.HiHi, m) && value >= m.HiHi {
		return HighHigh
	}
	if limitSet(m.Hi, m) && value >= m.Hi {
		return High
	}
	return None
}

// AlarmState is a snapshot of one tag's alarm at a point in its lifecycle. The
// same struct is returned by Active()/History() and delivered to subscribers.
//
// For a live active alarm, Since is the time it entered its current severity
// ("active since"); acknowledging does not reset it. A cleared snapshot
// (Active == false, Severity == None) is stamped at the clear time.
type AlarmState struct {
	Tag      string        // tag name this alarm belongs to
	Severity AlarmSeverity // current severity band
	Active   bool          // true while the alarm condition holds
	Acked    bool          // operator has acknowledged this alarm
	Since    time.Time     // time the current severity was entered / cleared
	Value    float64       // value that produced this state
}

// defaultAlarmHistory bounds the transition ring when the db is built with
// NewAlarmDB.
const defaultAlarmHistory = 256

// alarmRing is a fixed-capacity FIFO ring of past alarm transitions. When full,
// the oldest entry is overwritten.
type alarmRing struct {
	buf  []AlarmState
	head int // index of the oldest entry
	size int // number of valid entries (<= len(buf))
}

func newAlarmRing(capacity int) alarmRing {
	if capacity < 1 {
		capacity = 1
	}
	return alarmRing{buf: make([]AlarmState, capacity)}
}

func (r *alarmRing) push(s AlarmState) {
	n := len(r.buf)
	if r.size < n {
		r.buf[(r.head+r.size)%n] = s
		r.size++
		return
	}
	r.buf[r.head] = s
	r.head = (r.head + 1) % n
}

// slice returns the entries oldest -> newest as a fresh copy.
func (r *alarmRing) slice() []AlarmState {
	n := len(r.buf)
	out := make([]AlarmState, r.size)
	for i := 0; i < r.size; i++ {
		out[i] = r.buf[(r.head+i)%n]
	}
	return out
}

// AlarmDB tracks the live alarm state of many tags plus a bounded history of
// past transitions, and fans transitions out to subscribers. All methods are
// safe for concurrent use.
type AlarmDB struct {
	mu      sync.RWMutex
	cur     map[string]*AlarmState      // tag -> live active alarm (absent == normal)
	hist    alarmRing                   // bounded ring of past transitions
	subs    map[uint64]func(AlarmState) // transition subscribers
	nextSub uint64                      // next subscriber id
	now     func() time.Time            // clock (injectable for tests)
}

// NewAlarmDB returns an empty db with the default history bound.
func NewAlarmDB() *AlarmDB { return NewAlarmDBWithHistory(defaultAlarmHistory) }

// NewAlarmDBWithHistory returns an empty db whose transition history is bounded
// to at most n entries.
func NewAlarmDBWithHistory(n int) *AlarmDB {
	return &AlarmDB{
		cur:  make(map[string]*AlarmState),
		hist: newAlarmRing(n),
		subs: make(map[uint64]func(AlarmState)),
		now:  time.Now,
	}
}

// Update drives the alarm state machine for tagName toward severity sev:
//   - sev != None with no active alarm            -> raise (unacked)
//   - sev != None with a different active severity -> re-raise (restamp, unack)
//   - sev != None with the same active severity    -> refresh value only, no event
//   - sev == None with an active alarm             -> clear (return to normal)
//   - sev == None with no active alarm             -> no-op
//
// Every raise / re-raise / clear records one history entry and notifies
// subscribers; a same-severity refresh does neither (a poll re-reading an
// unchanged band does not spam the alarm list).
func (db *AlarmDB) Update(tagName string, sev AlarmSeverity, value float64) {
	db.mu.Lock()
	var (
		emit bool
		snap AlarmState
	)
	prev, ok := db.cur[tagName]
	switch {
	case sev == None:
		if ok {
			snap = AlarmState{Tag: tagName, Severity: None, Active: false, Acked: prev.Acked, Since: db.now(), Value: value}
			delete(db.cur, tagName)
			emit = true
		}
	case !ok:
		st := &AlarmState{Tag: tagName, Severity: sev, Active: true, Acked: false, Since: db.now(), Value: value}
		db.cur[tagName] = st
		snap = *st
		emit = true
	case prev.Severity != sev:
		prev.Severity = sev
		prev.Acked = false
		prev.Since = db.now()
		prev.Value = value
		snap = *prev
		emit = true
	default:
		prev.Value = value
	}
	var fan []func(AlarmState)
	if emit {
		fan = db.emitLocked(snap)
	}
	db.mu.Unlock()
	for _, s := range fan {
		s(snap)
	}
}

// Ack acknowledges tagName's active alarm. It is a no-op when the tag has no
// active alarm or is already acknowledged; otherwise it records the ack and
// notifies subscribers (so a banner can stop flashing).
func (db *AlarmDB) Ack(tagName string) {
	db.mu.Lock()
	var (
		emit bool
		snap AlarmState
	)
	if st, ok := db.cur[tagName]; ok && !st.Acked {
		st.Acked = true
		snap = *st
		emit = true
	}
	var fan []func(AlarmState)
	if emit {
		fan = db.emitLocked(snap)
	}
	db.mu.Unlock()
	for _, s := range fan {
		s(snap)
	}
}

// emitLocked records a transition in history and snapshots the subscriber set.
// The caller must hold db.mu and invoke the returned callbacks after unlocking.
func (db *AlarmDB) emitLocked(snap AlarmState) []func(AlarmState) {
	db.hist.push(snap)
	fan := make([]func(AlarmState), 0, len(db.subs))
	for _, s := range db.subs {
		fan = append(fan, s)
	}
	return fan
}

// Active returns a snapshot of every currently active alarm, ordered for an
// operator alarm list: unacknowledged before acknowledged, then higher
// severity first, then oldest first, then by tag name.
func (db *AlarmDB) Active() []AlarmState {
	db.mu.RLock()
	out := make([]AlarmState, 0, len(db.cur))
	for _, st := range db.cur {
		out = append(out, *st)
	}
	db.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Acked != b.Acked {
			return !a.Acked // unacked first
		}
		if a.Severity != b.Severity {
			return a.Severity > b.Severity // higher severity first
		}
		if !a.Since.Equal(b.Since) {
			return a.Since.Before(b.Since) // oldest first
		}
		return a.Tag < b.Tag
	})
	return out
}

// History returns the bounded ring of past transitions, oldest -> newest.
func (db *AlarmDB) History() []AlarmState {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.hist.slice()
}

// Subscribe registers fn to receive every future alarm transition (raise,
// re-raise, ack, clear). Unlike tag subscriptions it does not prime with the
// current state — call Active() for the initial snapshot. The returned
// CancelFunc is idempotent.
func (db *AlarmDB) Subscribe(fn func(AlarmState)) CancelFunc {
	db.mu.Lock()
	id := db.nextSub
	db.nextSub++
	db.subs[id] = fn
	db.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			db.mu.Lock()
			delete(db.subs, id)
			db.mu.Unlock()
		})
	}
}

// Watch subscribes to t and auto-evaluates every good-quality sample against
// t's Meta limits, driving Update. Bad/uncertain samples are ignored (a
// disconnected sensor reading 0 must not trip LoLo). The returned CancelFunc
// stops watching.
func (db *AlarmDB) Watch(t *Tag) CancelFunc {
	return t.Subscribe(func(v Value) {
		if v.Quality != QualityGood {
			return
		}
		f := v.Float()
		db.Update(t.Name(), EvaluateAlarm(f, t.Meta()), f)
	})
}
