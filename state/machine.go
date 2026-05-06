package state

import (
	"errors"
	"sync"
)

// Machine drives the active-state cursor and dispatches events.
// Add states + transitions, set initial state, Start. PostEvent
// from any goroutine; the machine serialises events through an
// internal mutex.
//
// One Machine per logical flow. A widget tree may use several —
// each dialog or wizard could have its own. Machines do not share
// state with each other; PostEvent is local to the receiver.
type Machine struct {
	mu sync.Mutex

	// states is every state ever AddState'd, in the order added.
	// Hierarchical insertion is captured via State.Parent / Children
	// rather than a separate tree representation.
	states []*State

	// initial is the top-level state to enter when Start is called.
	// Must be set before Start.
	initial *State

	// active is the current leaf state (or nil before Start / after
	// Stop). For hierarchical machines this points to the deepest
	// compound child; the parent chain is reachable via active.Parent.
	active *State

	// running flags whether Start has been called and the machine
	// hasn't reached a top-level final state.
	running bool

	// lastEvent holds the most recently dispatched event name. Hooks
	// (OnEntry / OnExit) and Action functions can read this to
	// dispatch on the trigger.
	lastEvent string

	// onFinished is fired when the machine enters a top-level final
	// state and stops. May be nil. Mirrors QStateMachine::finished.
	onFinished func()
}

// NewMachine constructs an empty machine with no states.
func NewMachine() *Machine {
	return &Machine{}
}

// AddState adds a top-level state. The returned *State can be
// mutated (set OnEntry, AddTransition, etc.) freely until Start.
//
// Adding states after Start is allowed but the machine won't be
// notified — use only during initial wiring.
func (m *Machine) AddState(name string) *State {
	s := &State{Name: name}
	m.states = append(m.states, s)
	return s
}

// AddChildState adds a nested state under a compound parent. The
// child's Parent is set; the parent's Children list grows.
//
// If parent.Initial is nil, the first child added becomes the
// initial — convenient default for the dominant single-initial
// pattern. Override by setting parent.Initial directly.
func (m *Machine) AddChildState(parent *State, name string) *State {
	s := &State{Name: name, Parent: parent}
	parent.Children = append(parent.Children, s)
	if parent.Initial == nil {
		parent.Initial = s
	}
	m.states = append(m.states, s)
	return s
}

// SetInitialState pins the top-level entry point. Required before
// Start; calling Start without an initial returns an error.
func (m *Machine) SetInitialState(s *State) {
	m.initial = s
}

// SetOnFinished registers a callback fired when the machine reaches
// a top-level final state. Mirrors QStateMachine::finished signal.
func (m *Machine) SetOnFinished(fn func()) {
	m.onFinished = fn
}

// Start enters the initial state (descending into compound children
// as needed) and marks the machine running. Returns an error when
// no initial state was set.
func (m *Machine) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.initial == nil {
		return errors.New("state.Machine: no initial state set")
	}
	if m.running {
		return errors.New("state.Machine: already running")
	}
	m.running = true
	m.enterState(m.initial)
	return nil
}

// Stop halts the machine and clears the active state. Safe to call
// when not running.
func (m *Machine) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.running {
		return
	}
	m.exitToRoot(m.active)
	m.active = nil
	m.running = false
}

// IsRunning reports whether the machine is currently driving events.
func (m *Machine) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// Active returns the current leaf state, or nil before Start.
//
// Read without locking — the dominant call site is inside an OnEntry
// hook running on the dispatcher's goroutine while m.mu is already
// held. Locking here would deadlock. Callers from other goroutines
// should coordinate via PostEvent's return value or callbacks; the
// pointer read itself is atomic on every supported platform.
func (m *Machine) Active() *State {
	return m.active
}

// LastEvent returns the most recently dispatched event name. Useful
// inside OnEntry / OnExit / Action hooks to dispatch on the trigger.
//
// Same lock-free policy as Active — hooks fire while the dispatcher
// holds m.mu, so re-entering the lock would deadlock. The string read
// is single-word and atomic on all supported architectures.
func (m *Machine) LastEvent() string {
	return m.lastEvent
}

// PostEvent dispatches a named event. The dispatcher walks the
// active state's transition list, then up the parent chain — first
// transition whose Event matches AND whose Guard (if any) returns
// true wins. Returns true when a transition fired.
//
// Transition ordering during a fire (matches the QStateMachine
// spec):
//
//  1. Compute the LCA (lowest common ancestor) of source and target.
//  2. Walk from source up to the LCA's child, calling OnExit on each.
//  3. Run Transition.Action (after exits, before entries).
//  4. Walk from the LCA's child down to target, calling OnEntry on
//     each.
//  5. If target is a compound state, recursively descend into its
//     Initial child, calling OnEntry on each level.
//
// A self-transition (target == source) exits then re-enters source.
func (m *Machine) PostEvent(event string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.running || m.active == nil {
		return false
	}
	m.lastEvent = event

	// Walk from the active state up to root looking for a matching
	// transition.
	for s := m.active; s != nil; s = s.Parent {
		for _, t := range s.Transitions {
			if !matches(t.Event, event) {
				continue
			}
			if t.Guard != nil && !t.Guard() {
				continue
			}
			m.fireTransition(s, t)
			return true
		}
	}
	return false
}

// matches compares the event filter against the posted event. An
// empty filter matches every event (catch-all transitions).
func matches(filter, event string) bool {
	return filter == "" || filter == event
}

// fireTransition runs the LCA-based exit / action / entry sequence
// described on PostEvent. Caller must hold m.mu.
func (m *Machine) fireTransition(source *State, t *Transition) {
	if t.Target == nil {
		return
	}
	target := t.Target

	// Self-transition shortcut: source == target. Fire OnExit, Action,
	// OnEntry on the same state without descending the LCA path. The
	// LCA-based logic below would treat lca == source == target and
	// skip the exit pass entirely (the loop excludes lca).
	if source == target && source == m.active {
		if source.OnExit != nil {
			source.OnExit()
		}
		if t.Action != nil {
			t.Action()
		}
		if source.OnEntry != nil {
			source.OnEntry()
		}
		// Active stays on source; no descent needed for atomic state.
		// For a compound source the standard semantic is to re-enter
		// the initial child too — descend.
		m.descendInto(source)
		return
	}

	lca := lowestCommonAncestor(source, target)

	// Exit pass: walk from m.active up to (but not including) lca,
	// calling OnExit on each. We exit from the leaf — m.active —
	// rather than source to handle cases where the matching
	// transition was on a parent of the leaf (compound states).
	for s := m.active; s != nil && s != lca; s = s.Parent {
		if s.OnExit != nil {
			s.OnExit()
		}
	}

	// Action runs after exits, before entries.
	if t.Action != nil {
		t.Action()
	}

	// Entry pass: walk from the lca's child down to target. We need
	// to call OnEntry top-down, so collect the path first then
	// iterate.
	if target == lca {
		// Transition target IS the LCA (e.g. compound parent re-entry
		// from a child). Re-enter target by treating it as fresh.
		m.enterState(target)
		return
	}
	path := pathFromAncestorTo(lca, target)
	for _, s := range path {
		if s.OnEntry != nil {
			s.OnEntry()
		}
	}
	// If target is compound, descend into its initial child.
	m.descendInto(target)
}

// enterState handles the initial state entry path: call OnEntry on
// every state from the (no parent) root chain. Hierarchical entries
// auto-descend into Initial children. Triggers final-state stop
// when the entered state is top-level final.
func (m *Machine) enterState(s *State) {
	path := s.AncestorPath()
	// Determine which prefix to skip: we may already be inside some
	// of these ancestors (during a transition). For an initial Start
	// the active is nil so we enter every ancestor.
	commonDepth := 0
	if m.active != nil {
		// Find the LCA's depth and skip common prefix.
		lca := lowestCommonAncestor(m.active, s)
		commonDepth = depth(lca) + 1
	}
	for i := commonDepth; i < len(path); i++ {
		st := path[i]
		if st.OnEntry != nil {
			st.OnEntry()
		}
	}
	m.descendInto(s)
}

// descendInto walks down the Initial chain from s. Sets m.active to
// the deepest leaf reached. Fires the final-state callback when the
// landed leaf is top-level final.
func (m *Machine) descendInto(s *State) {
	cur := s
	for cur.IsCompound() {
		next := cur.Initial
		if next == nil {
			break
		}
		if next.OnEntry != nil {
			next.OnEntry()
		}
		cur = next
	}
	m.active = cur
	if cur.IsFinal && cur.Parent == nil {
		// Top-level final state: stop the machine.
		m.running = false
		fn := m.onFinished
		// Call after releasing the lock to avoid deadlock if the
		// callback re-enters PostEvent. We flag intent and the
		// caller of PostEvent / Start is expected to fire after
		// returning. Since fireTransition is the only caller during
		// running, we drop the lock briefly.
		if fn != nil {
			m.mu.Unlock()
			fn()
			m.mu.Lock()
		}
	}
}

// exitToRoot calls OnExit on every state from leaf up to root.
// Used by Stop.
func (m *Machine) exitToRoot(leaf *State) {
	for s := leaf; s != nil; s = s.Parent {
		if s.OnExit != nil {
			s.OnExit()
		}
	}
}

// lowestCommonAncestor finds the deepest state that is an ancestor
// of both a and b, or nil when they share no parent (separate trees).
func lowestCommonAncestor(a, b *State) *State {
	pathA := a.AncestorPath()
	pathB := b.AncestorPath()
	var lca *State
	n := len(pathA)
	if len(pathB) < n {
		n = len(pathB)
	}
	for i := 0; i < n; i++ {
		if pathA[i] != pathB[i] {
			break
		}
		lca = pathA[i]
	}
	return lca
}

// pathFromAncestorTo returns the path from ancestor's child down to
// target, inclusive of target but excluding ancestor itself.
func pathFromAncestorTo(ancestor, target *State) []*State {
	full := target.AncestorPath()
	// Find ancestor in full and slice.
	for i, s := range full {
		if s == ancestor {
			return full[i+1:]
		}
	}
	return full
}

// depth returns the level of s in the tree (root = 0). nil → -1.
func depth(s *State) int {
	if s == nil {
		return -1
	}
	d := 0
	for cur := s.Parent; cur != nil; cur = cur.Parent {
		d++
	}
	return d
}
