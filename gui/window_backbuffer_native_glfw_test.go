//go:build !windows

package gui

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"testing"
	"unsafe"

	"github.com/go-gl/gl/v2.1/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/uk0/silk/paint"
)

var nativeBackbufferErr error
var nativeBackbufferEvidence []string

type backbufferProbeWidget struct {
	nativeRenderProbeWidget
	nestPaint, closeInDraw bool
	closeInResize          bool
	finishedDraws          int
}

func (widget *backbufferProbeWidget) OnResize() {
	widget.Widget.OnResize()
	if widget.closeInResize {
		widget.closeInResize = false
		widget.win.Close()
	}
}

func (widget *backbufferProbeWidget) Draw(p paint.Painter) {
	if widget.nestPaint {
		widget.nestPaint = false
		widget.win.paint()
	}
	if widget.closeInDraw {
		widget.closeInDraw = false
		widget.win.Close()
	}
	// Painting after a close request proves the current native painter was
	// not retired underneath the callback. Teardown follows frame unwinding.
	widget.nativeRenderProbeWidget.Draw(p)
	widget.finishedDraws++
}

// The read is deliberately on the retained Go wrapper, not on a released C
// handle. This native-Cairo test must not allow finalizers to mask a leak.
func backbufferSurfaceRetired(p paint.Pixmap) bool {
	v := reflect.ValueOf(p)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return false
	}
	field := v.Elem().FieldByName("Surface")
	return field.IsValid() && field.Kind() == reflect.Pointer && field.IsNil()
}

// TestMain runs this on the real Cocoa/GLFW OS thread. Actual framebuffers are
// resized; each retired surface stays reachable until every assertion ends.
func runNativeBackbufferProbe() error {
	if !glfwReady {
		return fmt.Errorf("native backbuffer probe needs a desktop")
	}
	defer glfw.DefaultWindowHints()
	savedPacer := glfwAnimationPacer
	defer func() { glfwAnimationPacer = savedPacer }()
	widget := &backbufferProbeWidget{}
	widget.Init(widget)
	widget.SetSize(320, 240)
	win := &Window{widget: widget}
	widget.win = win
	if err := win.create(nil, WtForm); err != nil {
		return err
	}
	defer func() {
		if win.glfwWin != nil {
			win.destroy()
		}
		flushNativeDestruction()
	}()
	glfw.SwapInterval(0) // probe throughput, not application frame pacing
	var retired []paint.Pixmap
	var retiredPainters []paint.Painter
	checkPixels := func() error {
		w, h := win.glfwWin.GetFramebufferSize()
		if w != win.backWidth || h != win.backHeight || win.backBuffer.Width() != w || win.backBuffer.Height() != h {
			return fmt.Errorf("buffer dimensions differ from actual framebuffer")
		}
		win.backBuffer.Flush()
		got := make([]byte, w*h*4)
		gl.BindBuffer(gl.PIXEL_PACK_BUFFER, 0)
		gl.PixelStorei(gl.PACK_ROW_LENGTH, 0)
		gl.PixelStorei(gl.PACK_ALIGNMENT, 4)
		gl.BindTexture(gl.TEXTURE_2D, win.glTexture)
		gl.GetTexImage(gl.TEXTURE_2D, 0, gl.BGRA, gl.UNSIGNED_BYTE, gl.Ptr(got))
		if err := gl.GetError(); err != gl.NO_ERROR {
			return fmt.Errorf("resize GL readback error %d", err)
		}
		data := unsafe.Slice((*byte)(win.backBuffer.DataPtr()), win.backBuffer.Stride()*h)
		for y := range h {
			if !bytes.Equal(got[y*w*4:(y+1)*w*4], data[y*win.backBuffer.Stride():y*win.backBuffer.Stride()+w*4]) {
				return fmt.Errorf("resized texture differs from Cairo at row %d", y)
			}
		}
		if !bytes.Equal(got[len(got)-4:], []byte{30, 20, 10, 255}) {
			return fmt.Errorf("new bottom-right pixel %v was not fully repainted (framebuffer=%dx%d widget=%vx%v)", got[len(got)-4:], w, h, widget.Width(), widget.Height())
		}
		return nil
	}
	win.paint()
	if err := checkPixels(); err != nil {
		return err
	}
	for _, pbo := range []bool{true, false} {
		win.pboAvailable = pbo
		for i := range 24 {
			oldSurface, oldPainter := win.backBuffer, win.backPainter
			oldPainter.Save() // explicit close also retires its saved stack
			retired = append(retired, oldSurface)
			retiredPainters = append(retiredPainters, oldPainter)
			width, height := 330+(i%3)*77, 245+(i%4)*23
			win.glfwWin.SetSize(width, height)
			glfw.PollEvents()
			ww, hh := win.glfwWin.GetSize()
			widget.SetSize(float64(ww), float64(hh))
			// Simulate a delayed native resize callback on every platform. The
			// frame must reconcile the root before drawing, not leave blank edges.
			widget.w, widget.h = 320, 240
			win.MarkDirtyRect(1, 1, 2, 2) // resize must still force a full paint
			win.paint()
			if err := checkPixels(); err != nil {
				return fmt.Errorf("pbo=%v resize=%d: %w", pbo, i, err)
			}
			if !backbufferSurfaceRetired(oldSurface) || oldPainter.CurrentState() != 0 {
				return fmt.Errorf("pbo=%v resize=%d: retired Cairo resources still live", pbo, i)
			}
			if pbo && !win.pboAvailable {
				return fmt.Errorf("driver failed PBO path during resize")
			}
			// A same-size partial paint must retain both objects and settle.
			s, p := win.backBuffer, win.backPainter
			win.MarkDirtyRect(8, 8, 12, 10)
			win.paint()
			if win.backBuffer != s || win.backPainter != p || win.dirty || win.fullDirty {
				return fmt.Errorf("same-size frame replaced resources or failed to settle")
			}
		}
		nativeBackbufferEvidence = append(nativeBackbufferEvidence, fmt.Sprintf("pbo=%v: 24 real resizes, exact pixel readback, immediate retirement, stable-frame reuse", pbo))
	}
	widget.nestPaint = true
	win.MarkFullDirty()
	win.paint()
	if !win.dirty || !win.fullDirty || win.painting {
		return fmt.Errorf("nested paint was not deferred to a later full frame")
	}
	win.paint()
	if win.dirty || win.fullDirty {
		return fmt.Errorf("nested paint never settled")
	}
	if err := checkPixels(); err != nil {
		return err
	}
	surface, painter := win.backBuffer, win.backPainter
	// Keep the native pointer only until the deferred destruction queue drains.
	// A modal caller cannot drain that queue yet, but its closed child must
	// immediately disappear instead of becoming a visible, input-dead window.
	native := win.glfwWin
	native.Show()
	if native.GetAttrib(glfw.Visible) == 0 {
		return fmt.Errorf("native retirement probe was never visible")
	}
	beforeCloseDraws := widget.finishedDraws
	widget.closeInDraw = true
	win.MarkFullDirty()
	win.paint()
	if widget.finishedDraws != beforeCloseDraws+1 || win.glfwWin != nil || win.backBuffer != nil || win.backPainter != nil || win.painting || win.destroyAfterPaint ||
		!backbufferSurfaceRetired(surface) || painter.CurrentState() != 0 {
		return fmt.Errorf("Draw-triggered close did not retire resources after frame exit")
	}
	if native.GetAttrib(glfw.Visible) != 0 {
		return fmt.Errorf("detached native window remained visible before deferred destruction")
	}
	nativeBackbufferEvidence = append(nativeBackbufferEvidence, "closed native window hidden before outer-loop destruction; no inert modal remnant")
	for i, p := range retired {
		if !backbufferSurfaceRetired(p) || retiredPainters[i].CurrentState() != 0 {
			return fmt.Errorf("retained wrapper %d still owns native resources", i)
		}
	}
	nativeBackbufferEvidence = append(nativeBackbufferEvidence, "48 retired surface/painter pairs remain reachable but own no native resources; nested paint and Draw-triggered close passed")
	// Layout invoked by pre-paint size reconciliation can close the owner.
	// It must retire the old surface without allocating or drawing another one.
	closing := &backbufferProbeWidget{}
	closing.Init(closing)
	closing.SetSize(320, 240)
	cw := &Window{widget: closing}
	closing.win = cw
	if err := cw.create(nil, WtForm); err != nil {
		return err
	}
	defer func() { cw.destroy(); flushNativeDestruction() }()
	cw.paint()
	old, draws := cw.backBuffer, closing.finishedDraws
	closing.w, closing.h, closing.closeInResize = 100, 100, true
	cw.MarkFullDirty()
	cw.paint()
	if cw.glfwWin != nil || cw.backBuffer != nil || cw.painting || closing.finishedDraws != draws || !backbufferSurfaceRetired(old) {
		return fmt.Errorf("resize-layout close drew or retained a retired window")
	}
	nativeBackbufferEvidence = append(nativeBackbufferEvidence, "delayed resize callback reconciled before draw; layout-triggered close retired without another draw")
	return nil
}

func TestGLFWNativeBackbufferLifecycle(t *testing.T) {
	if os.Getenv("SILK_TEST_NATIVE_BACKBUFFER") != "1" {
		t.Skip("opt-in native resize/readback: SILK_TEST_NATIVE_BACKBUFFER=1")
	}
	if nativeBackbufferErr != nil {
		t.Fatal(nativeBackbufferErr)
	}
	for _, evidence := range nativeBackbufferEvidence {
		t.Log(evidence)
	}
}
