// Package state is Silk's hierarchical finite state machine — the
// equivalent of Qt's QStateMachine framework. Apps wire UI flow,
// animation sequences, and dialog modal stacks as a graph of named
// states + event-triggered transitions; the Machine drives the
// active-state cursor and fires onEntry / onExit hooks at the right
// moments.
//
// Why a state machine and not a sea of bool flags? Two reasons. First,
// transitions become explicit — every change of mode is a named edge
// you can reason about, instead of a tangle of "if showing && not
// loading && hasUser". Second, the hierarchy lets a parent group
// share entry / exit logic across cousin states (e.g. "every state
// inside the Editor compound state shares the same status-bar setup").
//
// Typical usage:
//
//	m := state.NewMachine()
//
//	idle := m.AddState("idle")
//	loading := m.AddState("loading")
//	ready := m.AddState("ready")
//
//	idle.OnEntry = func() { ui.SetStatus("idle") }
//	loading.OnEntry = func() { ui.ShowSpinner() }
//	loading.OnExit = func() { ui.HideSpinner() }
//
//	idle.AddTransition("startLoad", loading)
//	loading.AddTransition("loaded", ready)
//
//	m.SetInitialState(idle)
//	m.Start()
//
//	// Later, in response to user action:
//	m.PostEvent("startLoad")
//	m.PostEvent("loaded")
//
// Hierarchical states (compound states): a parent state has children
// and an initial-child; entering the parent automatically descends
// into that child. PostEvent walks from the active leaf state up
// toward the root looking for a matching transition — children
// override their parents.
//
// Goal lined up against QStateMachine: feature-parity for the dominant
// 80% (atomic + compound states, event transitions, entry/exit hooks,
// final states, initial state). The Qt-specific assignProperty,
// signal-bound transitions, and SCXML import/export are out of scope —
// Silk's signal-slot already covers the property-binding territory and
// SCXML is a niche.
package state
