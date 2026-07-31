package gui

import (
	"os"
	"strings"
	"testing"
)

// TestWin32DoubleClickReachesTheButtonHandlers guards the second click of a
// fast double click. Both window classes are registered with CS_DBLCLKS, so
// Windows replaces the second WM_xBUTTONDOWN of a double click with
// WM_xBUTTONDBLCLK; a wndProcFunc that has no case for it hands the click to
// DefWindowProc and the widget under the cursor never sees the press, while the
// matching WM_xBUTTONUP still arrives. Closing several tabs with rapid middle
// clicks in the same spot then lost every second click on Windows and closed
// them all under GLFW, which synthesises no double click at all.
//
// The dispatch happens inside a window procedure driven by the OS, which a test
// process has no window to drive, so this reads the source — the same approach
// as the WM_KEYDOWN layer test, and untagged because the invariant is about
// Windows/GLFW parity and has to be able to fail on either host.
func TestWin32DoubleClickReachesTheButtonHandlers(t *testing.T) {
	src := wndProcSource(t)
	if !strings.Contains(src, "CS_DBLCLKS") {
		t.Fatal("the window classes no longer ask for CS_DBLCLKS; update this test to match")
	}

	for _, c := range []struct{ msg, handler string }{
		{"WM_LBUTTONDBLCLK", "on_WM_LBUTTONDOWN("},
		{"WM_MBUTTONDBLCLK", "on_WM_MBUTTONDOWN("},
	} {
		body, ok := wndProcCase(src, c.msg)
		if !ok {
			t.Errorf("wndProcFunc has no case for win32.%s; the second click of a double click is dropped", c.msg)
			continue
		}
		if !strings.Contains(body, c.handler) {
			t.Errorf("win32.%s does not reach %s; the second click of a double click is dropped", c.msg, c.handler)
		}
	}
}

// wndProcSource returns the whole Win32 backend source.
func wndProcSource(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("window_windows.go")
	if err != nil {
		t.Fatalf("cannot read the Win32 backend: %v", err)
	}
	return string(raw)
}

// wndProcCase returns the text of the "case win32.<msg>:" arm, up to the next
// case label.
func wndProcCase(src, msg string) (string, bool) {
	start := strings.Index(src, "case win32."+msg+":")
	if start < 0 {
		return "", false
	}
	rest := src[start:]
	if end := strings.Index(rest[1:], "\n\tcase "); end >= 0 {
		rest = rest[:end+1]
	}
	return rest, true
}
