package main

import (
	"errors"
	"sync"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

// Captured per launch, so a canceled launch can finish after a replacement
// without sharing its session or UI callbacks. Tests supply deterministic RPCs.
type debugLifecycleHooks struct {
	launch func(string, []string) (*core.DebugSession, error)
	close  func(*core.DebugSession)
	resume func(*core.DebugSession) (*core.StopState, error)
	post   func(func())
}

var debuggerHooks = debugLifecycleHooks{
	launch: core.LaunchDebug,
	close:  func(s *core.DebugSession) { _ = s.Close() },
	resume: func(s *core.DebugSession) (*core.StopState, error) { return s.Continue() },
	post:   gui.Post,
}

// globalDebugAttempt is UI-thread-owned. The attempt's cancellation/session
// fields are synchronized because launch and breakpoint RPCs run off-thread.
var globalDebugAttempt *debugLaunchAttempt

type debugLaunchAttempt struct {
	mu        sync.Mutex
	canceled  bool
	session   *core.DebugSession
	closeOnce sync.Once
	hooks     debugLifecycleHooks
}

func (a *debugLaunchAttempt) isCanceled() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.canceled
}

func (a *debugLaunchAttempt) adopt(s *core.DebugSession) bool {
	a.mu.Lock()
	a.session = s
	canceled := a.canceled
	a.mu.Unlock()
	if canceled {
		a.closeSession(s)
	}
	return !canceled
}

func (a *debugLaunchAttempt) closeSession(s *core.DebugSession) {
	if s != nil {
		a.closeOnce.Do(func() { go a.hooks.close(s) })
	}
}

func (a *debugLaunchAttempt) cancel() *core.DebugSession {
	a.mu.Lock()
	a.canceled = true
	s := a.session
	a.mu.Unlock()
	a.closeSession(s)
	return s
}

type debugInitialBreakpoint struct {
	path string
	line int
}

func (a *debugLaunchAttempt) launch(dir string, args []string, bps []debugInitialBreakpoint) {
	sess, err := a.hooks.launch(dir, args)
	if err == nil && sess == nil {
		err = errors.New("debugger launch returned no session")
	}
	if err != nil {
		if sess != nil {
			a.closeSession(sess)
		}
		a.hooks.post(func() {
			if globalDebugAttempt != a || a.isCanceled() {
				return
			}
			globalDebugAttempt = nil
			silkideToast(i18n.Tf("Debugger failed: %v", err), gui.ToastError)
		})
		return
	}
	if sess == nil || !a.adopt(sess) {
		return
	}
	for _, b := range bps {
		if a.isCanceled() {
			return
		}
		if _, err := sess.SetBreakpoint(b.path, b.line); err != nil {
			core.Warn("dlv: SetBreakpoint ", b.path, ":", b.line, ": ", err)
		}
	}
	// Do not Continue before the UI accepts this attempt: Stop can cancel
	// either the background launch or this queued publication.
	a.hooks.post(func() {
		if globalDebugAttempt != a || a.isCanceled() || globalDebug != nil {
			a.cancel()
			return
		}
		globalDebug = sess
		silkideToast(i18n.T("Debugger started"), gui.ToastInfo)
		logEvent(ged.LogInfo, "Debugger launched")
		go func() {
			if a.isCanceled() {
				return
			}
			st, err := a.hooks.resume(sess)
			if err != nil {
				if !errors.Is(err, core.ErrSessionClosed) {
					a.hooks.post(func() {
						if globalDebug == sess && !a.isCanceled() {
							silkideToast(i18n.Tf("Debugger error: %v", err), gui.ToastError)
						}
					})
				}
				return
			}
			debugHandleStop(sess, st)
		}()
	})
}

// stopProjectExecution is the generic toolbar Stop. Debugger-only callers
// (including application teardown) retain stopDebugger's narrow semantics.
// StopRunning deliberately leaves independently opened PTY shells alone.
func stopProjectExecution() {
	processStopped := stopIDEProcessRuntime()
	debugActive := globalDebug != nil || globalDebugAttempt != nil
	runStopped := globalTerminal != nil && globalTerminal.StopRunning()
	stopDebugger()
	if runStopped || processStopped {
		silkideToast(i18n.T("Run stopped"), gui.ToastInfo)
	} else if !debugActive {
		silkideToast(i18n.T("No running process"), gui.ToastInfo)
	}
}

func finishDebugSession(sess *core.DebugSession) {
	// A late exit from an old session must not clear a newer panel/session.
	if globalDebug != sess {
		go debuggerHooks.close(sess)
		return
	}
	globalDebug = nil
	if a := globalDebugAttempt; a != nil {
		globalDebugAttempt = nil
		a.cancel()
	} else {
		go debuggerHooks.close(sess)
	}
	silkideToast(i18n.T("Debuggee exited"), gui.ToastInfo)
	if globalDebugPanel != nil {
		globalDebugPanel.Clear()
	}
}
