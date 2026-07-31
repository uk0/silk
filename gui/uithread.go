package gui

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"sync"

	"github.com/uk0/silk/core"
)

// Off-UI-thread widget mutation detector.
//
// Widgets carry no locking at all. A SetText, a resize or an Update from a
// worker goroutine races the paint pass and the layout walk, and the failure is
// not a panic but silent corruption: a string drawn half-written, a widget
// painted at a stale size, the window map written while the loop ranges it. The
// symptom shows up later, somewhere else, and never reproduces — which is why
// this is worth detecting at the moment of the mutation instead of debugging
// afterwards. gui.Post (uiqueue.go) is the way to avoid it.
//
// The check hangs off Widget.Update(), the one call every visible mutation
// funnels into, and does nothing unless SILK_DEBUG armed it.

// uiThreadOwner reports whether the caller runs on the thread that owns the
// event loop. Only the window backend knows what that means, so it installs
// this; nil in headless tests and wherever the platform has no cheap thread
// identity, and a nil hook disables the check rather than guessing.
var uiThreadOwner func() bool

// uiThreadDebug gates the detector. Indirect because the flag behind
// core.IsDebugOn is read from SILK_DEBUG once at process start and cannot be
// flipped afterwards, which would leave the check untestable.
var uiThreadDebug = core.IsDebugOn

// uiThreadReportLimit caps the reports per offending call site. One offending
// worker can call Update in a tight loop, and a log flood would bury the first
// report — the one that still has the offender's own frames on the stack. Per
// site rather than per process, because a session lasts hours: the ticker that
// misbehaves from the first second must not spend the budget the LSP callback
// or the file watcher needs when it first fires, half an hour later.
const uiThreadReportLimit = 5

// uiThreadSiteLimit bounds how many sites are remembered, so the bookkeeping
// stays finite in a process that runs for days. A program with this many
// distinct off-thread mutation sites is past the point of being told again.
const uiThreadSiteLimit = 64

var (
	uiThreadReportMu sync.Mutex
	uiThreadReports  = make(map[uint64]int) // call site -> reports already made
)

// assertUIThread reports that what is running off the UI thread. Ordered so the
// common case is a load and a nil compare: it sits on Widget.Update(), which
// runs on every repaint request.
func assertUIThread(what string) {
	owner := uiThreadOwner
	if owner == nil || !uiThreadDebug() || owner() {
		return
	}
	reportOffUIThread(what)
}

// reportOffUIThread names the offender. The stack is the whole point — without
// it the warning says a mutation happened on the wrong thread but not which
// goroutine started it.
func reportOffUIThread(what string) {
	if !claimUIThreadReport(uiThreadCallSite()) {
		return
	}
	core.Warn(fmt.Sprintf(
		"ui: %s ran off the UI thread; widgets have no locking, so this corrupts state silently — marshal it with gui.Post\n%s",
		what, debug.Stack()))
}

// claimUIThreadReport spends one report from the site's own budget.
func claimUIThreadReport(site uint64) bool {
	uiThreadReportMu.Lock()
	defer uiThreadReportMu.Unlock()
	made, tracked := uiThreadReports[site]
	if made >= uiThreadReportLimit || (!tracked && len(uiThreadReports) >= uiThreadSiteLimit) {
		return false
	}
	uiThreadReports[site] = made + 1
	return true
}

// uiThreadCallSite fingerprints the frames above the detector, which is what
// tells one offender from another: the same worker mutating from the same line
// hashes to the same site, a different one does not. FNV-1a straight over the
// raw PCs — they only ever have to compare equal, never to be resolved or
// printed, and the stack the report carries is built separately.
func uiThreadCallSite() uint64 {
	var pcs [32]uintptr
	n := runtime.Callers(3, pcs[:]) // skip runtime.Callers, this, reportOffUIThread
	h := uint64(14695981039346656037)
	for _, pc := range pcs[:n] {
		h = (h ^ uint64(pc)) * 1099511628211
	}
	return h
}
