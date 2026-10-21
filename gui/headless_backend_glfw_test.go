//go:build !windows

package gui

import (
	"context"
	"errors"
	"math"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/geom"
)

func TestGLFWDesktopProbeHandlesInitializationFailures(t *testing.T) {
	initErr := errors.New("display unavailable")
	dpi, err := probeGLFWDesktop(func() error { return initErr }, func() float64 {
		t.Fatal("monitor queried after Init returned an error")
		return 2
	})
	if dpi != 96 || !errors.Is(err, initErr) {
		t.Fatalf("failed Init = %v, %v; want 96 DPI and original error", dpi, err)
	}
	// The binding ignores glfwInit's return value for a platform error, so
	// Linux without DISPLAY reaches GetPrimaryMonitor before it fails.
	monitorErr := &glfw.Error{Code: glfw.ErrorCode(0x10001), Desc: "not initialized"}
	dpi, err = probeGLFWDesktop(func() error { return nil }, func() float64 { panic(monitorErr) })
	if dpi != 96 || !errors.Is(err, monitorErr) {
		t.Fatalf("failed first monitor query = %v, %v", dpi, err)
	}
}

func TestGLFWDesktopProbePreservesUnexpectedPanics(t *testing.T) {
	bug := errors.New("application bug, not a GLFW error")
	defer func() {
		if value := recover(); value != bug {
			t.Fatalf("unexpected panic was swallowed/replaced: %v", value)
		}
	}()
	_, _ = probeGLFWDesktop(func() error { return nil }, func() float64 { panic(bug) })
}

func TestGLFWDesktopProbeUsesValidLogicalDPI(t *testing.T) {
	for _, scale := range []float64{0, -1, math.NaN(), math.Inf(1), 1, 1.5, 2} {
		dpi, err := probeGLFWDesktop(func() error { return nil }, func() float64 { return scale })
		want := 96.0
		if scale > 0 && !math.IsNaN(scale) && !math.IsInf(scale, 0) {
			want *= scale
		}
		if err != nil || dpi != want {
			t.Fatalf("scale %v produced DPI/error %v/%v, want %v/nil", scale, dpi, err, want)
		}
	}
}

func TestUnavailableDesktopLeavesModelsAndQueueUsable(t *testing.T) {
	isolatePopupState(t)
	oldReady, oldErr, oldDPI := glfwReady, glfwInitErr, screenDpi
	oldTasks, oldWake := uiTasks, uiWakeupFn
	glfwReady, glfwInitErr, screenDpi = false, errors.New("headless fixture"), 96
	uiTasks, uiWakeupFn = nil, nil
	t.Cleanup(func() {
		glfwReady, glfwInitErr, screenDpi = oldReady, oldErr, oldDPI
		uiTasks, uiWakeupFn = oldTasks, oldWake
	})
	if !core.HasMainLoop() {
		t.Fatal("headless GUI import unregistered its backend")
	}
	form := NewForm()
	button := NewButton1("headless model", nil)
	button.SetParent(form)
	button.SetBounds(10, 20, 120, 30)
	if len(form.Children()) != 1 || form.Window() != nil || button.Text() != "headless model" {
		t.Fatal("model construction required a native window")
	}
	if x, y, w, h := DesktopArea(); x != 0 || y != 0 || w != 1920 || h != 1080 {
		t.Fatalf("headless desktop = %v/%v/%v/%v", x, y, w, h)
	}
	if got := platformPopupWorkArea(-10, 50); got != (geom.Rect{Width: 1920, Height: 1080}) {
		t.Fatalf("popup fallback queried an unavailable monitor: %v", got)
	}
	ran := false
	queued := make(chan struct{})
	go func() {
		Post(func() { ran = true })
		close(queued)
	}()
	<-queued
	markAllWindowsDirty() // must not call glfwPostEmptyEvent when uninitialized
	drainUITasks()
	if !ran {
		t.Fatal("headless Post lost queued work")
	}
	if cursor := loadSystemCursor(glfw.ArrowCursor); cursor == nil || cursor.glfwCursor != nil {
		t.Fatal("logical cursor construction requested a native cursor")
	}
	if _, err := NewCursorFromData(CursorData{}); !errors.Is(err, glfwInitErr) {
		t.Fatalf("native cursor request silently succeeded: %v", err)
	}
	win := &Window{widget: form}
	if err := win.create(nil, WtForm); !errors.Is(err, glfwInitErr) || win.glfwWin != nil {
		t.Fatalf("native window request lost backend failure: %v", err)
	}
	for name, start := range map[string]func(){"MainLoop": MainLoop, "Window.ShowModal": func() { win.ShowModal(nil) }} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				err, ok := recover().(error)
				if !ok || !errors.Is(err, glfwInitErr) || !strings.Contains(err.Error(), "requires a native desktop") {
					t.Fatalf("native loop request did not surface unavailable backend: %v", err)
				}
			}()
			start()
		})
	}
}

func TestNoDisplayInitializationKeepsLogicalDefaults(t *testing.T) {
	if glfwReady {
		t.Skip("real desktop is available; no-display path is exercised in Linux headless QA")
	}
	if glfwInitErr == nil || ScreenDpi() != 96 || uiWakeupFn != nil || !core.HasMainLoop() {
		t.Fatalf("no-display import state: error=%v DPI=%v wakeup=%t backend=%t", glfwInitErr, ScreenDpi(), uiWakeupFn != nil, core.HasMainLoop())
	}
	if DefaultCursor() == nil || LoadCursor("hand") == nil {
		t.Fatal("no-display import failed to register logical system cursors")
	}
}

// Fresh process is required: queue tests deliberately clear global hooks and
// would hide the original Cocoa SIGTRAP when a worker posts before any window.
func TestPostBeforeFirstNativeWindow(t *testing.T) {
	if os.Getenv("SILK_TEST_PREWINDOW_POST") == "1" {
		if probeWindow != nil || len(nativeRenderEvidence) != 0 {
			t.Fatal("pre-window test created a native window")
		}
		done := make(chan struct{})
		go func() { Post(func() {}); close(done) }()
		<-done
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestPostBeforeFirstNativeWindow$")
	cmd.Env = append(os.Environ(), "SILK_TEST_PREWINDOW_POST=1", "SILK_TEST_NATIVE_RENDER=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("pre-window background Post: %v\n%s", err, out)
	}
}
