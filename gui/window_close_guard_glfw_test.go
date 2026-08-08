//go:build !windows

package gui

import (
	"os"
	"testing"

	"github.com/go-gl/glfw/v3.3/glfw"
)

// probeWindow is one real, hidden GLFW window, kept for the whole test binary
// so onWindowClose can be driven with the argument it is written for. Nothing
// else in the package's tests needs a window: they exercise widgets, not the
// window procedure.
var probeWindow *glfw.Window

// TestMain exists to create that window. glfwCreateWindow makes an NSWindow,
// which AppKit permits only on the main thread — from a test goroutine the
// binary dies in Cocoa before the first assertion. init() (window_glfw.go) has
// already pinned the main goroutine to that thread and initialised GLFW, and
// TestMain is the one test entry point that still runs on it.
func TestMain(m *testing.M) {
	glfw.WindowHint(glfw.Visible, glfw.False)
	gw, err := glfw.CreateWindow(1, 1, "silk test", nil, nil)
	glfw.DefaultWindowHints() // the hints are global; leave them as found
	if err == nil {
		probeWindow = gw
	}
	code := m.Run()
	// Destroying is main-thread-only for the same reason, so it happens here
	// rather than in a t.Cleanup.
	if probeWindow != nil {
		probeWindow.Destroy()
	}
	os.Exit(code)
}

// TestOnWindowCloseObeysTheFrameVeto drives the GLFW close handler itself, with
// the window manager's own argument, and asks what became of the frame.
//
// The polarity is the whole guard: read as f.CanClose() instead of
// !f.CanClose(), the handler tears down precisely the frames that said no and
// keeps precisely the ones that said yes. Both halves still call CanClose
// before PromptSaveClose, so a test that reads the source for the call and its
// order — the only one this had — passes over the inversion while the user
// loses every unsaved buffer to a Cancel.
func TestOnWindowCloseObeysTheFrameVeto(t *testing.T) {
	t.Run("a veto keeps the window and the work", func(t *testing.T) {
		closed, quit := driveWindowClose(t, false)
		if closed != 0 {
			t.Errorf("the frame was closed %d time(s) after its guard refused; the unsaved work is gone and Cancel meant nothing", closed)
		}
		if quit {
			t.Error("the application was asked to quit after the close was refused")
		}
	})

	t.Run("a yes closes it", func(t *testing.T) {
		closed, quit := driveWindowClose(t, true)
		if closed != 1 {
			t.Errorf("the frame was closed %d time(s) after its guard allowed the close, want 1; the red button does nothing", closed)
		}
		if !quit {
			t.Error("the last form closed without the application quitting")
		}
	})
}

// driveWindowClose hands onWindowClose a frame whose guard answers `allow`, and
// reports how many times that frame was actually closed and whether the event
// loop was asked to end.
//
// The Window is built by hand rather than through AttachWindow because
// AttachWindow creates its own NSWindow, which this goroutine may not do; every
// field onWindowClose and the teardown below it read is filled in.
func driveWindowClose(t *testing.T, allow bool) (closed int, quit bool) {
	t.Helper()
	if probeWindow == nil {
		t.Fatal("no GLFW window to close; TestMain could not create one")
	}
	gw := probeWindow

	frame := NewFrame()
	frame.SetCanCloseCallback(func(*Frame) bool { return allow })
	win := &Window{glfwWin: gw, wt: WtForm, widget: frame, enabled: true}
	winMap[gw] = win

	frame.SetClosingCallback(func(*Frame) {
		closed++
		// Window.destroy is a few frames further down and calls
		// glfwDestroyWindow, which is main-thread-only like the creation
		// above. Unhooking the OS window here — the frame has just reported
		// that its teardown has begun — is what a real destroy would have
		// done to winMap anyway, and destroy() skips the C call once glfwWin
		// is nil.
		delete(winMap, gw)
		win.glfwWin = nil
	})

	// Frame.Close() rewrites both of these, and the rest of the package's
	// tests share them.
	savedDefault := defaultFrame
	savedQuit := shouldQuit
	shouldQuit = false
	t.Cleanup(func() {
		delete(winMap, gw)
		defaultFrame = savedDefault
		shouldQuit = savedQuit
	})

	onWindowClose(gw)
	// QuitLoop is what core.Quit reaches through core.SetMainLoop, so this is
	// the event loop being told to stop — the process ending.
	return closed, shouldQuit
}
