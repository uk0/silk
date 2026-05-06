package state

// State is one node in the state-machine graph. States may be atomic
// (no children) or compound (a parent that auto-enters its initial
// child on entry). A "final" state has IsFinal=true and stops the
// machine when entered at the top level — matching QFinalState.
//
// onEntry / onExit are called by the dispatcher at the right moments
// during a transition. They receive no arguments because the typical
// hook closes over its own widget references; callers needing the
// transition event can read it from Machine.LastEvent inside the
// hook.
type State struct {
	// Name is a human-readable identifier. Used in error messages and
	// the dispatcher's debug logs. Two states may share a name in
	// different parts of the tree; the machine identifies states by
	// pointer, not name.
	Name string

	// Parent is the compound state this is nested inside, or nil for
	// top-level states. Set by Machine.AddState when the AddChildState
	// path is used; ignored for top-level AddState.
	Parent *State

	// Children is the list of nested states for a compound state.
	// Empty for atomic states.
	Children []*State

	// Initial is the child to auto-enter when the dispatcher reaches
	// this compound state. Required (must be in Children) when
	// Children is non-empty. Ignored for atomic states.
	Initial *State

	// IsFinal flags this as a final / terminating state. Entering a
	// top-level final state stops the machine. A final state inside a
	// compound parent fires the parent's "finished" event.
	IsFinal bool

	// Transitions is the list of outgoing edges. Order matters — the
	// dispatcher tries each in declaration order and takes the first
	// that matches both event name AND optional Guard.
	Transitions []*Transition

	// OnEntry is called when the dispatcher enters this state. May be
	// nil. Called AFTER the parent's OnEntry if walking down a
	// hierarchy and BEFORE any nested initial child's OnEntry.
	OnEntry func()

	// OnExit is called when the dispatcher leaves this state. Symmetric
	// to OnEntry — fires BEFORE the parent's OnExit when walking up.
	OnExit func()
}

// Transition is one outgoing edge from a State. The dispatcher walks
// the active state's transition list (and its parents'), picking the
// first whose Event matches the posted event AND whose Guard (if set)
// returns true.
//
// Action runs during the transition, between source.OnExit and
// target.OnEntry. Mirrors QAbstractTransition::onTransition.
type Transition struct {
	// Event is the name the dispatcher matches against PostEvent's
	// argument. Empty Event means "match any event" — useful for
	// catch-all transitions on a parent state.
	Event string

	// Guard is an optional predicate. The transition fires only when
	// Guard returns true (or Guard is nil). Lets multiple transitions
	// share an event name and pick by condition.
	Guard func() bool

	// Target is the destination state. Cannot be nil — callers needing
	// a self-transition should set Target = source.
	Target *State

	// Action is fired during the transition, with the LCA-based
	// ordering described on Machine.PostEvent. May be nil.
	Action func()
}

// AddTransition is a convenience for the common case: define an
// event-name transition with no guard or action. Returns the new
// Transition so the caller can attach a Guard / Action afterwards.
func (s *State) AddTransition(event string, target *State) *Transition {
	t := &Transition{Event: event, Target: target}
	s.Transitions = append(s.Transitions, t)
	return t
}

// IsCompound reports whether this state has children (i.e. is a
// hierarchical compound state).
func (s *State) IsCompound() bool { return len(s.Children) > 0 }

// IsAtomic is the inverse — leaf states with no children.
func (s *State) IsAtomic() bool { return len(s.Children) == 0 }

// AncestorPath returns the chain of states from root to this state,
// inclusive. Used by the dispatcher to compute the LCA between two
// states for transition ordering.
func (s *State) AncestorPath() []*State {
	var path []*State
	for cur := s; cur != nil; cur = cur.Parent {
		path = append([]*State{cur}, path...)
	}
	return path
}
