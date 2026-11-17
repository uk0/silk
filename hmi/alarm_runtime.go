package hmi

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/point"
)

const AlarmHistoryLimit = 512

var (
	ErrAlarmNotFound = errors.New("hmi: alarm not found")
	ErrAlarmConflict = errors.New("hmi: alarm occurrence is no longer current")
	ErrAlarmsClosed  = errors.New("hmi: alarm runtime closed")
)

type alarmIdentity struct{ marker byte } // distinct allocations across runtimes

// AlarmToken identifies one occurrence in one runtime. It cannot acknowledge a
// later occurrence, a different rule, or a restarted session.
type AlarmToken struct {
	owner      *alarmIdentity
	activation uint64
}

// AlarmState is a value snapshot. Active may remain true after ConditionActive
// becomes false when an unacknowledged alarm is latched. Value is the last
// valid observed scalar; use Quality to distinguish stale/failed observations.
type AlarmState struct {
	ID, Point       string
	Priority        AlarmPriority
	Active          bool
	ConditionActive bool
	Acknowledged    bool
	Quality         core.Quality
	Value           any
	Since           time.Time
	Token           AlarmToken `json:"-"`
}

type AlarmEventKind string

const (
	AlarmRaised       AlarmEventKind = "raised"
	AlarmReturned     AlarmEventKind = "returned"
	AlarmAcknowledged AlarmEventKind = "acknowledged"
	AlarmCleared      AlarmEventKind = "cleared"
)

// AlarmEvent is a transient session audit record, not a persisted point write.
// Sequence starts at one, is ordered within a runtime, and can be used as a cursor.
type AlarmEvent struct {
	Sequence uint64
	At       time.Time
	Kind     AlarmEventKind
	State    AlarmState
}

type alarmCandidate uint8

const (
	alarmIdle alarmCandidate = iota
	alarmRising
	alarmReturning
	alarmLatchClearing
)

type liveAlarmRule struct {
	rule    compiledAlarmRule
	input   *alarmInput
	state   AlarmState
	pending alarmCandidate
	due     time.Time
}

type alarmInput struct {
	tag   *core.Tag
	kind  point.Kind
	stale time.Duration
	rules []*liveAlarmRule
}

type alarmListener struct {
	owner   atomic.Pointer[alarmMonitor]
	deliver atomic.Pointer[func()]
}

func (l *alarmListener) cancel() {
	l.deliver.Store(nil)
	if m := l.owner.Swap(nil); m != nil {
		m.mu.Lock()
		delete(m.listeners, l)
		m.mu.Unlock()
	}
}

type alarmMonitor struct {
	mu           sync.Mutex
	identity     *alarmIdentity
	closed       bool
	lastNow      time.Time
	now          func() time.Time
	rules        []*liveAlarmRule
	byID         map[string]*liveAlarmRule
	inputs       []*alarmInput
	cancel       []core.CancelFunc
	sequence     uint64
	revision     uint64
	listeners    map[*alarmListener]struct{}
	history      [AlarmHistoryLimit]AlarmEvent
	head, length int
}

func newAlarmMonitor(p Project, tags *core.TagDB, now func() time.Time) (*alarmMonitor, error) {
	if len(p.Alarms) == 0 {
		return nil, nil
	}
	kinds := make(map[string]point.Kind, len(p.Points))
	byPoint := make(map[string]*alarmInput, len(p.Points))
	for _, d := range p.Points {
		t, _ := tags.Get(d.Name)
		kinds[d.Name] = d.Kind
		byPoint[d.Name] = &alarmInput{tag: t, kind: d.Kind, stale: time.Duration(d.StaleMS) * time.Millisecond}
	}
	compiled, err := compileAlarmRules(p.Alarms, kinds)
	if err != nil {
		return nil, err
	}
	m := &alarmMonitor{identity: new(alarmIdentity), now: now, byID: make(map[string]*liveAlarmRule, len(compiled))}
	for _, c := range compiled {
		in := byPoint[c.point]
		if len(in.rules) == 0 {
			m.inputs = append(m.inputs, in)
		}
		r := &liveAlarmRule{rule: c, input: in, state: AlarmState{ID: c.id, Point: c.point, Priority: c.priority}}
		in.rules = append(in.rules, r)
		m.rules = append(m.rules, r)
		m.byID[c.id] = r
	}
	for _, in := range m.inputs {
		m.cancel = append(m.cancel, in.tag.Subscribe(func(core.Value) {
			m.mu.Lock()
			defer m.unlockAndNotify(m.revision)
			if !m.closed {
				// Tag callbacks run unlocked and can arrive out of order.
				// Read the latest committed observation under the monitor lock.
				m.observe(in, m.clock(m.now()))
			}
		}))
	}
	return m, nil
}

func (m *alarmMonitor) clock(at time.Time) time.Time {
	if at.Before(m.lastNow) {
		return m.lastNow
	}
	m.lastNow = at
	return at
}

func (m *alarmMonitor) observe(in *alarmInput, now time.Time) {
	sample := in.tag.Value()
	quality := sample.Quality
	if quality > core.QualityGood {
		quality = core.QualityBad
	}
	var raw any
	if quality == core.QualityGood {
		if sample.Time.IsZero() || in.stale > 0 && sample.Time.Before(now.Add(-in.stale)) {
			quality = core.QualityUncertain
		} else {
			var err error
			raw, err = alarmScalar(in.kind, sample.Raw)
			if err != nil {
				quality = core.QualityBad
			}
		}
	}
	for _, r := range in.rules {
		if r.state.Quality != quality {
			m.revision++
		}
		r.state.Quality = quality
		if quality != core.QualityGood {
			r.pending = alarmIdle
			continue // unknown quality never asserts or clears an alarm
		}
		r.state.Value = raw
		m.advance(r, raw, now)
	}
}

func (m *alarmMonitor) advance(r *liveAlarmRule, raw any, now time.Time) {
	var pending alarmCandidate
	var wait time.Duration
	switch {
	case r.state.ConditionActive && r.rule.resets(raw):
		pending, wait = alarmReturning, r.rule.clear
	case !r.state.ConditionActive && r.rule.trips(raw):
		pending, wait = alarmRising, r.rule.raise
	case r.state.Active && !r.state.ConditionActive && r.state.Acknowledged && r.rule.resets(raw):
		// A previously returned alarm was acknowledged without currently
		// confirmed normal quality. Require a fresh reset dwell.
		pending, wait = alarmLatchClearing, r.rule.clear
	}
	if pending == alarmIdle {
		r.pending = alarmIdle
		return
	}
	if r.pending != pending {
		r.pending, r.due = pending, now.Add(wait)
	}
	if now.Before(r.due) {
		return
	}
	r.pending = alarmIdle
	switch pending {
	case alarmRising:
		r.state.Active, r.state.ConditionActive, r.state.Acknowledged = true, true, false
		r.state.Since = now
		r.state.Token = AlarmToken{owner: m.identity, activation: m.sequence + 1}
		m.record(AlarmRaised, r.state, now)
	case alarmReturning:
		r.state.ConditionActive = false
		if r.rule.latch && !r.state.Acknowledged {
			m.record(AlarmReturned, r.state, now)
		} else {
			m.clear(r, now)
		}
	case alarmLatchClearing:
		m.clear(r, now)
	}
}

func (m *alarmMonitor) clear(r *liveAlarmRule, now time.Time) {
	r.pending = alarmIdle
	r.state.Active = false
	m.record(AlarmCleared, r.state, now)
	r.state.Token = AlarmToken{}
}

func (m *alarmMonitor) record(kind AlarmEventKind, state AlarmState, at time.Time) {
	m.sequence++
	m.revision++
	event := AlarmEvent{Sequence: m.sequence, Kind: kind, State: state, At: at}
	if m.length == len(m.history) {
		m.history[m.head] = event
		m.head = (m.head + 1) % len(m.history)
		return
	}
	m.history[(m.head+m.length)%len(m.history)] = event
	m.length++
}

// tick uses the runtime's existing clock. Pending transitions and previously
// unknown quality are checked; stable good inputs use normal notifications.
func (m *alarmMonitor) tick(at time.Time) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.unlockAndNotify(m.revision)
	if m.closed {
		return
	}
	now := m.clock(at)
	for _, in := range m.inputs {
		if in.rules[0].state.Quality != core.QualityGood {
			m.observe(in, now) // a silent, same-value heartbeat can restore freshness
			continue
		}
		for _, r := range in.rules {
			if r.pending != alarmIdle {
				m.observe(in, now)
				break
			}
		}
	}
}

func (m *alarmMonitor) close() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.closed = true
	for listener := range m.listeners {
		listener.deliver.Store(nil)
		listener.owner.Store(nil)
	}
	m.listeners = nil
	cancel := m.cancel
	m.cancel = nil
	m.mu.Unlock()
	for _, stop := range cancel {
		stop()
	}
}

func (m *alarmMonitor) unlockAndNotify(before uint64) {
	var fan []*alarmListener
	if m.revision != before {
		for listener := range m.listeners {
			fan = append(fan, listener)
		}
	}
	m.mu.Unlock()
	for _, listener := range fan {
		if fn := listener.deliver.Load(); fn != nil {
			(*fn)()
		}
	}
}

// SubscribeAlarmChanges observes lifecycle and quality changes, not stable
// analog samples. Subscribe before reading AlarmStates for an initial view.
// Notifications are invalidations, not ordered event payloads: callbacks
// can read the current snapshots, run on the publisher/clock thread, and
// must return promptly. Do not synchronously Stop or publish from a callback.
// Cancel is idempotent; an already-admitted callback may finish.
func (r *Runtime) SubscribeAlarmChanges(fn func()) core.CancelFunc {
	m := r.alarms
	if fn == nil || m == nil {
		return func() {}
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return func() {}
	}
	if m.listeners == nil {
		m.listeners = make(map[*alarmListener]struct{})
	}
	listener := new(alarmListener)
	listener.owner.Store(m)
	listener.deliver.Store(&fn)
	m.listeners[listener] = struct{}{}
	m.mu.Unlock()
	return listener.cancel
}

// AlarmStates returns an independent snapshot in document order.
func (r *Runtime) AlarmStates() []AlarmState {
	m := r.alarms
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]AlarmState, len(m.rules))
	for i, rule := range m.rules {
		out[i] = rule.state
	}
	return out
}

// AlarmEvents returns retained events with Sequence greater than after, oldest
// first. A gap at the start means older events were evicted. History is session
// memory, independent from the point store; it remains readable after Stop.
func (r *Runtime) AlarmEvents(after uint64) []AlarmEvent {
	m := r.alarms
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []AlarmEvent
	for i := 0; i < m.length; i++ {
		e := m.history[(m.head+i)%len(m.history)]
		if e.Sequence > after {
			out = append(out, e)
		}
	}
	return out
}

// AcknowledgeAlarm records an operator acknowledgment, not a point or PLC
// write. Repeat acknowledgment of the same active occurrence is idempotent.
func (r *Runtime) AcknowledgeAlarm(id string, token AlarmToken) error {
	m := r.alarms
	if m == nil {
		return ErrAlarmNotFound
	}
	m.mu.Lock()
	defer m.unlockAndNotify(m.revision)
	if m.closed {
		return ErrAlarmsClosed
	}
	rule := m.byID[id]
	if rule == nil {
		return ErrAlarmNotFound
	}
	current := func() bool { return rule.state.Active && rule.state.Token == token }
	if !current() {
		return ErrAlarmConflict
	}
	now := m.clock(m.now())
	m.observe(rule.input, now)
	if !current() {
		return ErrAlarmConflict
	}
	if rule.state.Acknowledged {
		return nil
	}
	rule.state.Acknowledged = true
	m.record(AlarmAcknowledged, rule.state, now)
	if !rule.state.ConditionActive && rule.state.Quality == core.QualityGood && rule.rule.resets(rule.state.Value) {
		m.clear(rule, now)
	}
	return nil
}
