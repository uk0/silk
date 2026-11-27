//go:build !windows

package gui

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/go-gl/glfw/v3.3/glfw"
)

var nativeFirstFrameErr error

func TestGLFWNativeFirstFrameUsesClientSize(t *testing.T) {
	if os.Getenv("SILK_TEST_NATIVE_FIRST_FRAME") != "1" {
		t.Skip("opt-in desktop probe: SILK_TEST_NATIVE_FIRST_FRAME=1")
	}
	if nativeFirstFrameErr != nil {
		t.Fatal(nativeFirstFrameErr)
	}
}

// Like TestMain's other GLFW probes, use the OS main thread.
func runNativeFirstFrameProbe() error {
	if !glfwReady {
		return fmt.Errorf("native first-frame probe needs a desktop")
	}
	form := NewForm()
	form.SetVisible(false)
	form.SetSize(480, 320)
	if err := form.TryAttachWindow(WtForm); err != nil {
		return err
	}
	defer form.DetachWindow()
	win := form.Window().NakedWindow()
	// GLFW may defer callbacks for an unmapped native toplevel. A resize
	// after attachment still has to reach the widget before Show, so callers
	// can lay out the content without reading stale client dimensions.
	form.SetSize(520, 360)
	if form.Width() != 520 || form.Height() != 360 || win.canRenderFrame() {
		return fmt.Errorf("hidden resize: logical=%gx%g, expected 520x360 without drawing", form.Width(), form.Height())
	}
	form.Show()
	defer form.Hide()
	deadline := time.Now().Add(2 * time.Second)
	for !win.IsVisible() && time.Now().Before(deadline) {
		glfw.WaitEventsTimeout(.02)
	}
	if !win.IsVisible() {
		return fmt.Errorf("native form never became visible")
	}
	// Leave the real drawable intact but reproduce the absence of a native
	// resize callback that would have initialized the logical widget.
	form.NakedWidget().setSize(0, 0)
	wantW, wantH := win.glfwWin.GetSize()
	if !validRenderSize(float64(wantW), float64(wantH)) || form.Width() != 0 || form.Height() != 0 {
		return fmt.Errorf("probe needs native drawable and zero logical bounds; native=%dx%d logical=%gx%g",
			wantW, wantH, form.Width(), form.Height())
	}
	if !win.canRenderFrame() {
		return fmt.Errorf("ready native window %dx%d never received its first frame", wantW, wantH)
	}
	win.MarkFullDirty()
	win.paint()
	if form.Width() != float64(wantW) || form.Height() != float64(wantH) || win.glTexture == 0 {
		return fmt.Errorf("first paint failed to establish geometry/texture")
	}
	form.Hide()
	glfw.PollEvents()
	if win.canRenderFrame() {
		return fmt.Errorf("hidden form became eligible to paint")
	}
	return nil
}
