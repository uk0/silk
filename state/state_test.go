package state

import "testing"

// TestStartFailsWithoutInitial: explicit failure mode for the
// dominant misuse — calling Start before SetInitialState.
func TestStartFailsWithoutInitial(t *testing.T) {
	m := NewMachine()
	m.AddState("a")
	if err := m.Start(); err == nil {
		t.Errorf("Start without initial should error")
	}
}

// TestStartEntersInitialState fires the initial state's OnEntry on
// Start and exposes it via Active().
func TestStartEntersInitialState(t *testing.T) {
	m := NewMachine()
	a := m.AddState("a")
	entered := false
	a.OnEntry = func() { entered = true }
	m.SetInitialState(a)
	if err := m.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !entered {
		t.Errorf("initial state OnEntry did not fire")
	}
	if m.Active() != a {
		t.Errorf("Active = %v, want %v", m.Active(), a)
	}
}

// TestSimpleTransition: post a matching event, expect the
// destination state to become active and OnEntry/OnExit to fire
// in the right order.
func TestSimpleTransition(t *testing.T) {
	m := NewMachine()
	a := m.AddState("a")
	b := m.AddState("b")
	a.AddTransition("go", b)

	var trace []string
	a.OnEntry = func() { trace = append(trace, "a-enter") }
	a.OnExit = func() { trace = append(trace, "a-exit") }
	b.OnEntry = func() { trace = append(trace, "b-enter") }
	b.OnExit = func() { trace = append(trace, "b-exit") }

	m.SetInitialState(a)
	m.Start()
	if !m.PostEvent("go") {
		t.Fatalf("PostEvent(go) returned false")
	}
	want := []string{"a-enter", "a-exit", "b-enter"}
	if !sliceEq(trace, want) {
		t.Errorf("trace = %v, want %v", trace, want)
	}
	if m.Active() != b {
		t.Errorf("after transition, Active = %v, want b", m.Active())
	}
}

// TestEventNoMatchKeepsActive: posting an unmatched event leaves
// the active state and trace unchanged.
func TestEventNoMatchKeepsActive(t *testing.T) {
	m := NewMachine()
	a := m.AddState("a")
	b := m.AddState("b")
	a.AddTransition("go", b)
	m.SetInitialState(a)
	m.Start()

	if m.PostEvent("nope") {
		t.Errorf("unmatched event should return false")
	}
	if m.Active() != a {
		t.Errorf("after no-match, Active = %v, want a", m.Active())
	}
}

// TestGuardedTransition: with a Guard returning false, the
// transition does not fire.
func TestGuardedTransition(t *testing.T) {
	m := NewMachine()
	a := m.AddState("a")
	b := m.AddState("b")
	t1 := a.AddTransition("go", b)
	t1.Guard = func() bool { return false }
	m.SetInitialState(a)
	m.Start()

	if m.PostEvent("go") {
		t.Errorf("guard=false transition fired")
	}
}

// TestGuardedTransitionPicksFirstMatch: with two transitions on
// the same event, the dispatcher takes the first whose Guard is
// satisfied — declaration order matters.
func TestGuardedTransitionPicksFirstMatch(t *testing.T) {
	m := NewMachine()
	a := m.AddState("a")
	b := m.AddState("b")
	c := m.AddState("c")
	cond := false
	t1 := a.AddTransition("go", b)
	t1.Guard = func() bool { return cond }
	a.AddTransition("go", c)
	m.SetInitialState(a)
	m.Start()

	// First post: cond=false → second transition fires (target c).
	m.PostEvent("go")
	if m.Active() != c {
		t.Errorf("with cond=false: Active = %v, want c", m.Active())
	}
}

// TestTransitionAction: Action runs between OnExit and OnEntry.
func TestTransitionAction(t *testing.T) {
	m := NewMachine()
	a := m.AddState("a")
	b := m.AddState("b")
	t1 := a.AddTransition("go", b)
	var trace []string
	a.OnExit = func() { trace = append(trace, "exit") }
	t1.Action = func() { trace = append(trace, "action") }
	b.OnEntry = func() { trace = append(trace, "entry") }
	m.SetInitialState(a)
	m.Start()
	m.PostEvent("go")
	want := []string{"exit", "action", "entry"}
	if !sliceEq(trace, want) {
		t.Errorf("trace = %v, want %v", trace, want)
	}
}

// TestEmptyEventCatchAll: an empty Event filter matches every
// event ("catch-all" semantics).
func TestEmptyEventCatchAll(t *testing.T) {
	m := NewMachine()
	a := m.AddState("a")
	b := m.AddState("b")
	a.AddTransition("", b) // empty Event
	m.SetInitialState(a)
	m.Start()
	if !m.PostEvent("anything") {
		t.Errorf("catch-all transition did not fire")
	}
	if m.Active() != b {
		t.Errorf("Active = %v, want b", m.Active())
	}
}

// TestCompoundStateAutoEntersInitial: entering a compound state
// auto-descends into its Initial child.
func TestCompoundStateAutoEntersInitial(t *testing.T) {
	m := NewMachine()
	parent := m.AddState("parent")
	c1 := m.AddChildState(parent, "c1")
	c2 := m.AddChildState(parent, "c2")
	_ = c2

	var trace []string
	parent.OnEntry = func() { trace = append(trace, "parent-in") }
	c1.OnEntry = func() { trace = append(trace, "c1-in") }

	m.SetInitialState(parent)
	m.Start()

	want := []string{"parent-in", "c1-in"}
	if !sliceEq(trace, want) {
		t.Errorf("trace = %v, want %v", trace, want)
	}
	if m.Active() != c1 {
		t.Errorf("Active = %v, want c1 (auto-descended)", m.Active())
	}
}

// TestCompoundParentTransitionFires: a transition on the parent
// fires when the child has no matching transition. Child overrides
// parent — this test verifies the parent fallback.
func TestCompoundParentTransitionFires(t *testing.T) {
	m := NewMachine()
	parent := m.AddState("parent")
	c1 := m.AddChildState(parent, "c1")
	other := m.AddState("other")
	parent.AddTransition("escape", other)
	_ = c1

	m.SetInitialState(parent)
	m.Start()
	if !m.PostEvent("escape") {
		t.Errorf("parent transition didn't fire from child")
	}
	if m.Active() != other {
		t.Errorf("Active = %v, want other", m.Active())
	}
}

// TestCompoundChildOverridesParent: child transition on the same
// event takes priority.
func TestCompoundChildOverridesParent(t *testing.T) {
	m := NewMachine()
	parent := m.AddState("parent")
	c1 := m.AddChildState(parent, "c1")
	c2 := m.AddChildState(parent, "c2")
	other := m.AddState("other")
	parent.AddTransition("go", other)
	c1.AddTransition("go", c2)

	m.SetInitialState(parent)
	m.Start()
	m.PostEvent("go")
	if m.Active() != c2 {
		t.Errorf("Active = %v, want c2 (child wins)", m.Active())
	}
}

// TestExitChainLeafToParent: when transitioning out of a child
// to a sibling-of-parent, OnExit fires leaf-first then parent.
func TestExitChainLeafToParent(t *testing.T) {
	m := NewMachine()
	parent := m.AddState("parent")
	c1 := m.AddChildState(parent, "c1")
	other := m.AddState("other")
	parent.AddTransition("escape", other)

	var trace []string
	c1.OnExit = func() { trace = append(trace, "c1-exit") }
	parent.OnExit = func() { trace = append(trace, "parent-exit") }
	other.OnEntry = func() { trace = append(trace, "other-in") }

	m.SetInitialState(parent)
	m.Start()
	m.PostEvent("escape")
	want := []string{"c1-exit", "parent-exit", "other-in"}
	if !sliceEq(trace, want) {
		t.Errorf("trace = %v, want %v", trace, want)
	}
}

// TestFinalStateStopsMachine: entering a top-level final state
// halts the machine and fires onFinished.
func TestFinalStateStopsMachine(t *testing.T) {
	m := NewMachine()
	a := m.AddState("a")
	end := m.AddState("end")
	end.IsFinal = true
	a.AddTransition("done", end)

	finished := false
	m.SetOnFinished(func() { finished = true })

	m.SetInitialState(a)
	m.Start()
	m.PostEvent("done")
	if !finished {
		t.Errorf("onFinished did not fire on top-level final state")
	}
	if m.IsRunning() {
		t.Errorf("machine still running after final state")
	}
}

// TestStopFiresExitChain: explicit Stop calls OnExit on every
// state from the leaf up.
func TestStopFiresExitChain(t *testing.T) {
	m := NewMachine()
	parent := m.AddState("p")
	child := m.AddChildState(parent, "c")
	var trace []string
	child.OnExit = func() { trace = append(trace, "c-exit") }
	parent.OnExit = func() { trace = append(trace, "p-exit") }
	m.SetInitialState(parent)
	m.Start()
	m.Stop()
	want := []string{"c-exit", "p-exit"}
	if !sliceEq(trace, want) {
		t.Errorf("trace = %v, want %v", trace, want)
	}
	if m.IsRunning() {
		t.Errorf("running after Stop")
	}
}

// TestPostEventBeforeStartIsNoOp.
func TestPostEventBeforeStartIsNoOp(t *testing.T) {
	m := NewMachine()
	a := m.AddState("a")
	b := m.AddState("b")
	a.AddTransition("go", b)
	if m.PostEvent("go") {
		t.Errorf("PostEvent before Start should return false")
	}
}

// TestLastEventReadInsideHook is a cross-test of the LastEvent
// accessor: it must be set correctly when an OnEntry hook runs.
func TestLastEventReadInsideHook(t *testing.T) {
	m := NewMachine()
	a := m.AddState("a")
	b := m.AddState("b")
	a.AddTransition("go", b)
	var seen string
	b.OnEntry = func() { seen = m.LastEvent() }
	m.SetInitialState(a)
	m.Start()
	m.PostEvent("go")
	if seen != "go" {
		t.Errorf("LastEvent inside b.OnEntry = %q, want go", seen)
	}
}

// TestSelfTransition: source == target, OnExit and OnEntry both
// fire on the same state.
func TestSelfTransition(t *testing.T) {
	m := NewMachine()
	a := m.AddState("a")
	a.AddTransition("loop", a)

	var trace []string
	a.OnEntry = func() { trace = append(trace, "in") }
	a.OnExit = func() { trace = append(trace, "out") }
	m.SetInitialState(a)
	m.Start() // initial entry: trace = [in]
	m.PostEvent("loop")
	want := []string{"in", "out", "in"}
	if !sliceEq(trace, want) {
		t.Errorf("self-transition trace = %v, want %v", trace, want)
	}
}

func sliceEq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
