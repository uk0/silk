//go:build !windows

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
// at all if that timer failed to arm. The hook now targets a message-only HWND:
// PostMessage is safe from workers, unlike window creation or mutation.
//
// Source-level, like TestWin32BackendPinsTheUIThread, because neither the
// compiler nor a test run on macOS reaches into a windows-only file. This is
// only a wiring check; TestNativeDispatchPostsWithoutIdleTimerAndJoinsShutdown
// proves actual native dispatch and thread ownership on Windows.
func TestWin32BackendInstallsTheUIWakeup(t *testing.T) {
	src, err := os.ReadFile("window_windows.go")
	if err != nil {
		t.Fatalf("cannot read the Win32 backend: %v", err)
	}
	body := initFuncBody(string(src))
	if body == "" {
		t.Fatal("window_windows.go has no init() to install the wakeup in")
	}
	if !strings.Contains(body, "startUIMessageWindow()") {
		t.Error("init() installs no UI wakeup; an off-thread gui.Post waits for the 47ms idle timer instead of running now")
	}
	if !strings.Contains(body, "uiThreadOwner = ") {
		t.Error("init() installs no uiThreadOwner, so the off-UI-thread mutation detector stays inert on Windows")
	}
}
