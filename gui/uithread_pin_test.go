package gui

import (
	"os"
	"strings"
	"testing"
)

// TestWin32BackendPinsTheUIThread guards the fix for a hang that only shows up
// on Windows: the client area stays black, the title bar gains
// "(Not Responding)" and CPU sits flat, because the message pump has drifted
// onto an OS thread that owns no windows and blocks forever on an empty queue.
//
// Win32 gives windows, SetTimer(NULL, ...) timers and message queues hard
// thread affinity, and Go's main goroutine migrates between OS threads on its
// own — so the backend has to pin it. The GLFW backend already did; the Win32
// one did not, and nothing caught that because neither the compiler nor a test
// on macOS can see into a windows-only file. Hence this source-level check: it
// is the only form of the invariant that runs on every platform.
func TestWin32BackendPinsTheUIThread(t *testing.T) {
	src, err := os.ReadFile("window_windows.go")
	if err != nil {
		t.Fatalf("cannot read the Win32 backend: %v", err)
	}
	body := initFuncBody(string(src))
	if body == "" {
		t.Fatal("window_windows.go has no init() to pin the thread in")
	}
	if !strings.Contains(body, "runtime.LockOSThread()") {
		t.Error("init() does not call runtime.LockOSThread(); the message pump can migrate off the thread that owns the windows and will hang there")
	}
	if !strings.Contains(body, "uiThreadId = win32.GetCurrentThreadId()") {
		t.Error("init() does not record the UI thread id, so MainLoop cannot tell it is on the wrong thread")
	}
}

// initFuncBody returns the text between the braces of the first top-level
// init() in src, or "" when there is none.
func initFuncBody(src string) string {
	start := strings.Index(src, "\nfunc init() {")
	if start < 0 {
		return ""
	}
	rest := src[start+len("\nfunc init() {"):]
	if end := strings.Index(rest, "\n}"); end >= 0 {
		return rest[:end]
	}
	return rest
}
