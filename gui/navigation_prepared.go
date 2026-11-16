package gui

// Nonzero size gives every live generation a distinct address. Tokens keep old
// generations alive, so clear/rebuild ABA and counter wrap cannot revive them.
type navigationGeneration struct{ marker byte }

// Tokens retain this small identity, not a NavigationStack, its observers, or a
// workbench captured by an observer. There must be no back-pointer to an owner.
type navigationPreparation struct{ generation *navigationGeneration }

// NavigationVisit is an immutable, UI-thread-owned proposed history traversal.
// It contains positions and opaque identities, never editor buffers or widgets.
// A zero visit cannot commit. Copies share one admission: after a successful
// commit every copy and every sibling visit becomes stale. Drop unused visits
// to release their retained paths; there is no goroutine, timer, or subscription.
type NavigationVisit struct {
	state              *navigationPreparation
	generation         *navigationGeneration
	live, destination  NavPosition
	fromIndex, toIndex int
}

// Destination returns the captured target, not a live reference into history.
func (v NavigationVisit) Destination() NavPosition { return v.destination }

// PrepareNavigation snapshots back/forward candidates in traversal order without
// changing history, caret state, or notifying observers. It performs no I/O.
// forward=false means Back. At most 255 visits are returned (history is bounded
// to 256 entries). Positions equal to the live caret are skipped like Navigate.
//
// A host may try opening candidates asynchronously. After a real open succeeds,
// check CanCommitNavigation before touching UI, then CommitNavigation after the
// target location has been applied. Failed/canceled opens simply drop the visit.
// Preparing is not a reservation: multiple plans remain current until history
// changes. The host must separately guard window/document and latest-user-intent
// ownership. All three operations, like NavigationStack itself, are UI-only.
func (ns *NavigationStack) PrepareNavigation(live NavPosition, forward bool) []NavigationVisit {
	if ns == nil || len(ns.stack) == 0 {
		return nil
	}
	step, capacity := -1, ns.current
	if forward {
		step, capacity = 1, len(ns.stack)-ns.current-1
	}
	var visits []NavigationVisit
	for i := ns.current + step; i >= 0 && i < len(ns.stack); i += step {
		if sameNavLocation(live, ns.stack[i]) {
			continue
		}
		if visits == nil {
			visits = make([]NavigationVisit, 0, capacity)
		}
		if ns.prepared == nil {
			ns.prepared = &navigationPreparation{new(navigationGeneration)}
		}
		visits = append(visits, NavigationVisit{state: ns.prepared, generation: ns.prepared.generation, live: live, destination: ns.stack[i], fromIndex: ns.current, toIndex: i})
	}
	return visits
}

// CanCommitNavigation reports whether visit still belongs to this unchanged
// stack. It does not inspect editor state or the filesystem. A successful Push,
// RecordJump, traversal, Clear, or path mapping invalidates prepared visits;
// exact no-ops and subscription changes do not. Do not copy a used stack.
func (ns *NavigationStack) CanCommitNavigation(visit NavigationVisit) bool {
	return ns != nil && visit.state != nil && ns.prepared == visit.state &&
		visit.generation == visit.state.generation && ns.current == visit.fromIndex &&
		visit.toIndex >= 0 && visit.toIndex < len(ns.stack)
}

// CommitNavigation publishes a successfully completed prepared traversal and its
// live return position in one notification. It never opens files or moves a
// caret. A stale/foreign/zero visit returns false without changing anything.
// The caller must already have completed the actual navigation, and must not
// mutate history between its CanCommitNavigation check and this commit.
func (ns *NavigationStack) CommitNavigation(visit NavigationVisit) bool {
	if !ns.CanCommitNavigation(visit) {
		return false
	}
	ns.stack[ns.current] = visit.live
	ns.current = visit.toIndex
	ns.notifyChanged()
	return true
}

// CurrentPosition returns a copy of the last committed history position without
// moving the history cursor. UI-thread-only; useful when a host shows an inert
// loading/error page rather than an editor with a live caret.
func (ns *NavigationStack) CurrentPosition() (NavPosition, bool) {
	if ns == nil || len(ns.stack) == 0 || ns.current < 0 || ns.current >= len(ns.stack) {
		return NavPosition{}, false
	}
	return ns.stack[ns.current], true
}
