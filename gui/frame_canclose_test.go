package gui

import (
	"os"
	"strings"
	"testing"
)

// TestFrameCanCloseVetoesTheWindowManagerClose: the close button is the one
// close an application cannot intercept anywhere else. Close() runs
// CloseAllViews before either of its callbacks fires, so "unsaved work — really
// quit?" answered from those is answered over a dismantled frame; CanClose is
// where the answer still means something.
func TestFrameCanCloseVetoesTheWindowManagerClose(t *testing.T) {
	frame := NewFrame()
	if !frame.CanClose() {
		t.Error("a frame with no guard refused to close")
	}

	asked := 0
	frame.SetCanCloseCallback(func(f *Frame) bool {
		asked++
		if f != frame {
			t.Errorf("guard was handed %v, want the frame it was installed on", f)
		}
		return false
	})
	if frame.CanClose() {
		t.Error("CanClose ignored a guard that said no")
	}
	if asked != 1 {
		t.Errorf("guard consulted %d times, want 1", asked)
	}

	frame.SetCanCloseCallback(func(*Frame) bool { return true })
	if !frame.CanClose() {
		t.Error("CanClose ignored a guard that said yes")
	}
}

// TestWindowCloseAsksTheFrameFirst: the guard only protects anything if the
// close paths consult it, and consult it before they start closing. Both
// backends run inside a window procedure the OS drives, which a test process
// has no window to drive, so this reads the source — crude, but it is the only
// form of the invariant that can fail when someone removes the call. The Win32
// backend matters most: it is verified on a real machine long after this runs.
func TestWindowCloseAsksTheFrameFirst(t *testing.T) {
	cases := []struct {
		file  string
		start string
		end   string
	}{
		{"window_glfw.go", "func onWindowClose(", "\nfunc onMouseButton("},
		{"window_windows.go", "case win32.WM_CLOSE:", "\n\tdefault:"},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			body := sourceBlock(t, c.file, c.start, c.end)
			guard := strings.Index(body, "CanClose()")
			teardown := strings.Index(body, "PromptSaveClose(")
			if guard < 0 {
				t.Fatal("the close path does not consult Frame.CanClose; unsaved work is quit over silently here")
			}
			if teardown < 0 {
				t.Fatal("the close path no longer calls PromptSaveClose; update this test to match")
			}
			if guard > teardown {
				t.Error("CanClose is consulted after the close has started; a veto arrives too late")
			}
		})
	}
}

// sourceBlock returns the text of file between the start marker and the first
// end marker after it.
func sourceBlock(t *testing.T, file, start, end string) string {
	t.Helper()
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("cannot read %s: %v", file, err)
	}
	src := string(raw)
	from := strings.Index(src, start)
	if from < 0 {
		t.Fatalf("%s has no %q", file, start)
	}
	rest := src[from:]
	to := strings.Index(rest, end)
	if to < 0 {
		t.Fatalf("%s: %q is not terminated by %q", file, start, end)
	}
	return rest[:to]
}
