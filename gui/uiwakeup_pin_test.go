package gui

import (
	"os"
	"strings"
	"testing"
)

// TestWin32BackendInstallsTheUIWakeup guards a Windows-only latency bug that no
// test on another platform can otherwise see. gui.Post appends to the queue and
// nudges the wakeup hook; the GLFW backend installs glfw.PostEmptyEvent, but the
// Win32 backend installed nothing, so a task posted by a worker goroutine sat
// there until the 47ms idle WM_TIMER happened to turn the pump — and never ran
// at all if that timer failed to arm. The hook must be a THREAD-targeted post:
// a worker owns no window handle it may legally touch.
//
// Source-level, like TestWin32BackendPinsTheUIThread, because neither the
// compiler nor a test run on macOS reaches into a windows-only file.
func TestWin32BackendInstallsTheUIWakeup(t *testing.T) {
	src, err := os.ReadFile("window_windows.go")
	if err != nil {
		t.Fatalf("cannot read the Win32 backend: %v", err)
	}
	body := initFuncBody(string(src))
	if body == "" {
		t.Fatal("window_windows.go has no init() to install the wakeup in")
	}
	if !strings.Contains(body, "SetUIWakeup(") {
		t.Error("init() installs no UI wakeup; an off-thread gui.Post waits for the 47ms idle timer instead of running now")
	}
	if !strings.Contains(body, "win32.PostThreadMessage(uiThreadId") {
		t.Error("the wakeup does not post to the UI thread id; a window-targeted post would make a worker goroutine touch a window handle it does not own")
	}
	if !strings.Contains(body, "uiThreadOwner = ") {
		t.Error("init() installs no uiThreadOwner, so the off-UI-thread mutation detector stays inert on Windows")
	}
}
