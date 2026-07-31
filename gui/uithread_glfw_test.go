//go:build !windows

package gui

import (
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/uk0/silk/core"
)

// TestGLFWBackendInstallsTheUIThreadOwner covers the whole detector on macOS and
// Linux. assertUIThread returns on a nil hook, so with no owner installed here
// the check hanging off Widget.Update is dead code on the platform silk is
// developed on, and the Windows-only tests cannot notice.
func TestGLFWBackendInstallsTheUIThreadOwner(t *testing.T) {
	if uiThreadOwner == nil {
		t.Fatal("the GLFW backend installed no uiThreadOwner; every off-thread widget mutation goes unreported on macOS and Linux")
	}
}

// TestGLFWDetectorCatchesAWorkerGoroutine drives the real chain the detector
// exists for — a background goroutine calling SetText, which is what a data
// binding, a file watcher or a build worker does — against whatever hook the
// backend installed, not a stand-in. A hook that answers a constant, or one
// built on something that is not thread identity, passes the nil check above
// and still fails here.
func TestGLFWDetectorCatchesAWorkerGoroutine(t *testing.T) {
	var mu sync.Mutex
	var warnings []string
	unregister := core.RegisterLogSink(func(level core.LogLevel, message string) {
		if level != core.LevelWarn {
			return
		}
		mu.Lock()
		warnings = append(warnings, message)
		mu.Unlock()
	})
	defer unregister()

	oldDebug := uiThreadDebug
	uiThreadDebug = func() bool { return true }
	defer func() { uiThreadDebug = oldDebug }()

	label := NewLabel("x")

	// Building the label already ran through Update on this goroutine, which is
	// not the UI thread either; only the worker's report is under test.
	resetUIThreadReports()
	mu.Lock()
	warnings = nil
	mu.Unlock()

	done := make(chan struct{})
	go func() {
		defer close(done)
		// A locked goroutine owns its thread outright, so it cannot be the one
		// init() pinned — the report below is then unambiguous.
		runtime.LockOSThread()
		label.SetText("off the UI thread")
	}()
	<-done

	mu.Lock()
	defer mu.Unlock()
	if len(warnings) == 0 {
		t.Fatal("a worker goroutine mutated a Label and the detector said nothing")
	}
	if got := warnings[0]; !strings.Contains(got, "TestGLFWDetectorCatchesAWorkerGoroutine") {
		t.Errorf("the report does not name the offending goroutine: %q", got)
	}
}

// TestGLFWUIThreadOwnerAcceptsItsOwnThread is the other half: a hook that
// always answered "not the UI thread" would satisfy the test above and then
// warn on every legitimate Update, which is worse than no detector at all.
// Nothing else can see this — the real UI thread is the main one, and no test
// runs there.
func TestGLFWUIThreadOwnerAcceptsItsOwnThread(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	saved := uiThreadId
	uiThreadId = currentThreadId() // stand in for the thread init() pinned
	defer func() { uiThreadId = saved }()

	if !uiThreadOwner() {
		t.Fatal("the owner hook disowns the very thread it was given; every mutation on the UI thread would be reported")
	}
}
