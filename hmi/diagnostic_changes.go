package hmi

import (
	"sync/atomic"

	"github.com/uk0/silk/core"
)

type diagnosticObserver struct {
	fn atomic.Pointer[func()]
}

// SubscribeDiagnosticChanges observes changes to source or retention faults.
// Subscribe before reading Diagnostics for the initial state. Notifications
// are coalescible invalidations, not ordered payloads. Callbacks may run
// concurrently on acquisition/maintenance workers, without the runtime lock.
// Return promptly; do not synchronously stop the runtime from a callback.
// Cancel is idempotent; an already-admitted callback may still finish.
func (r *Runtime) SubscribeDiagnosticChanges(fn func()) core.CancelFunc {
	if fn == nil {
		return func() {}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return func() {}
	}
	o := new(diagnosticObserver)
	o.fn.Store(&fn)
	if r.diagnosticObservers == nil {
		r.diagnosticObservers = make(map[*diagnosticObserver]struct{})
	}
	r.diagnosticObservers[o] = struct{}{}
	return func() {
		if o.fn.Swap(nil) == nil {
			return
		}
		r.mu.Lock()
		delete(r.diagnosticObservers, o)
		r.mu.Unlock()
	}
}

func (r *Runtime) diagnosticFanoutLocked() []*diagnosticObserver {
	if len(r.diagnosticObservers) == 0 {
		return nil
	}
	fan := make([]*diagnosticObserver, 0, len(r.diagnosticObservers))
	for o := range r.diagnosticObservers {
		fan = append(fan, o)
	}
	return fan
}

func notifyDiagnosticChanges(fan []*diagnosticObserver) {
	for _, o := range fan {
		if f := o.fn.Load(); f != nil {
			(*f)()
		}
	}
}

func (r *Runtime) setSourceDiagnostic(id string, err error) {
	// Do not invoke implementation-provided Error methods while holding mu.
	var message string
	if err != nil {
		message = err.Error()
	}
	r.mu.Lock()
	old, existed := r.sourceErrors[id]
	if r.closed || (existed == (err != nil) && old == message) {
		r.mu.Unlock()
		return
	}
	if err == nil {
		delete(r.sourceErrors, id)
	} else {
		r.sourceErrors[id] = message
	}
	fan := r.diagnosticFanoutLocked()
	r.mu.Unlock()
	notifyDiagnosticChanges(fan)
}

func (r *Runtime) setRetentionDiagnostic(err error) {
	var message string
	if err != nil {
		message = err.Error()
		if message == "" {
			message = "storage maintenance failed"
		}
	}
	r.mu.Lock()
	if r.closed || r.retentionError == message {
		r.mu.Unlock()
		return
	}
	r.retentionError = message
	fan := r.diagnosticFanoutLocked()
	r.mu.Unlock()
	notifyDiagnosticChanges(fan)
}
