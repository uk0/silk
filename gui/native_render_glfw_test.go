//go:build !windows

package gui

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"testing"
	"unsafe"

	"github.com/go-gl/gl/v2.1/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/paint"
)

var nativeRenderErr error
var nativeRenderEvidence []string

type nativeRenderProbeWidget struct {
	Widget
	changed       bool
	requestInDraw bool
}

func (w *nativeRenderProbeWidget) Draw(p paint.Painter) {
	p.SetBrush(paint.Color{R: 10, G: 20, B: 30, A: 255})
	p.Rectangle(0, 0, w.Width(), w.Height())
	p.Fill()
	// One-device-pixel patterns reveal any extra texture resampling. A
	// translucent patch catches double multiplication of Cairo's premultiplied
	// RGB, which texture-only readback cannot detect.
	var matrix geom.Mat3x2
	p.GetMatrix(&matrix)
	for x := 40; x < 60; x++ {
		c := paint.Color{R: 255, A: 255}
		if x%2 == 0 {
			c = paint.Color{G: 255, A: 255}
		}
		p.SetBrush(c)
		p.Rectangle(float64(x)/matrix.Xx, 20, 1/matrix.Xx, 8)
		p.Fill()
	}
	p.SetOperator(paint.OpSource)
	p.SetBrush(paint.Color{R: 240, G: 120, B: 60, A: 128})
	p.Rectangle(30, 40, 12, 8)
	p.Fill()
	p.SetOperator(paint.OpOver)
	if w.changed {
		p.SetBrush(paint.Color{R: 200, G: 80, B: 40, A: 255})
		p.Rectangle(8, 8, 12, 10)
		p.Fill()
	}
	if w.requestInDraw {
		w.requestInDraw = false
		w.win.Update()
	}
}

// Runs in TestMain on the actual OS main thread, not on a testing goroutine.
// No screenshots or mock GL: read the uploaded texture back from the driver
// and compare every byte with the Cairo backbuffer after successive paints.
func runNativeRenderProbe() error {
	if !glfwReady {
		return fmt.Errorf("native render probe needs a GLFW desktop")
	}
	glfw.WindowHint(glfw.Visible, glfw.False)
	defer glfw.DefaultWindowHints()
	gw, err := createNativeRenderProbeWindow(func() (*glfw.Window, error) {
		return glfw.CreateWindow(80, 60, "silk native render test", nil, nil)
	})
	if err != nil {
		return err
	}
	if gw == nil {
		return fmt.Errorf("native render window was not created")
	}
	defer gw.Destroy()
	gw.MakeContextCurrent()
	if err := gl.Init(); err != nil {
		return err
	}
	initGL()
	savedPacer := glfwAnimationPacer
	defer func() { glfwAnimationPacer = savedPacer }()
	for _, pbo := range []bool{true, false} {
		widget := &nativeRenderProbeWidget{}
		widget.Init(widget)
		widget.SetSize(80, 60)
		win := &Window{glfwWin: gw, widget: widget, dirty: true, fullDirty: true}
		widget.win = win
		check := func(stage string) error {
			if win.glTexture == 0 || win.fullDirty {
				return fmt.Errorf("%s pbo=%v: paint failed", stage, pbo)
			}
			got := make([]byte, win.backWidth*win.backHeight*4)
			gl.BindBuffer(gl.PIXEL_PACK_BUFFER, 0)
			gl.PixelStorei(gl.PACK_ALIGNMENT, 4)
			gl.PixelStorei(gl.PACK_ROW_LENGTH, 0)
			gl.BindTexture(gl.TEXTURE_2D, win.glTexture)
			gl.GetTexImage(gl.TEXTURE_2D, 0, gl.BGRA, gl.UNSIGNED_BYTE, gl.Ptr(got))
			if err := gl.GetError(); err != gl.NO_ERROR {
				return fmt.Errorf("%s: GL readback error %d", stage, err)
			}
			win.backBuffer.Flush()
			src := unsafe.Slice((*byte)(win.backBuffer.DataPtr()), win.backBuffer.Stride()*win.backHeight)
			if !bytes.Equal(src[:4], []byte{30, 20, 10, 255}) {
				return fmt.Errorf("%s: widget background was not drawn", stage)
			}
			if widget.changed {
				sx, sy := gw.GetContentScale()
				offset := int(12*float64(sy))*win.backBuffer.Stride() + int(12*float64(sx))*4
				if !bytes.Equal(src[offset:offset+4], []byte{40, 80, 200, 255}) {
					return fmt.Errorf("%s: changed rectangle was not drawn", stage)
				}
			}
			for y := 0; y < win.backHeight; y++ {
				if !bytes.Equal(got[y*win.backWidth*4:(y+1)*win.backWidth*4], src[y*win.backBuffer.Stride():y*win.backBuffer.Stride()+win.backWidth*4]) {
					return fmt.Errorf("%s pbo=%v: stale texture row %d", stage, pbo, y)
				}
			}
			// Re-present into the backbuffer (SwapBuffers leaves its contents
			// undefined) and inspect the actual final blit, not just the upload.
			gl.DrawBuffer(gl.BACK)
			drawFullscreenQuad(win.glTexture, int32(win.backWidth), int32(win.backHeight))
			var blendSrc, blendDst int32
			gl.GetIntegerv(gl.BLEND_SRC, &blendSrc)
			gl.GetIntegerv(gl.BLEND_DST, &blendDst)
			if blendSrc != gl.SRC_ALPHA || blendDst != gl.ONE_MINUS_SRC_ALPHA {
				return fmt.Errorf("%s: blit leaked premultiplied blend state into GPU overlays", stage)
			}
			gl.ReadBuffer(gl.BACK)
			gl.ReadPixels(0, 0, int32(win.backWidth), int32(win.backHeight), gl.BGRA, gl.UNSIGNED_BYTE, gl.Ptr(got))
			if err := gl.GetError(); err != gl.NO_ERROR {
				return fmt.Errorf("%s: framebuffer readback error %d", stage, err)
			}
			for y := 0; y < win.backHeight; y++ {
				for x := 0; x < win.backWidth; x++ {
					si := y*win.backBuffer.Stride() + x*4
					di := ((win.backHeight-1-y)*win.backWidth + x) * 4
					for c := 0; c < 3; c++ {
						want := int(src[si+c]) + (51*(255-int(src[si+3]))+127)/255
						delta := int(got[di+c]) - want
						if delta < -1 || delta > 1 {
							return fmt.Errorf("%s pbo=%v: presented pixel (%d,%d) channel=%d got=%d want=%d alpha=%d", stage, pbo, x, y, c, got[di+c], want, src[si+3])
						}
					}
				}
			}
			return nil
		}
		cleanup := func() {
			if win.glTexture != 0 {
				gl.DeleteTextures(1, &win.glTexture)
			}
			if win.pbo != 0 {
				gl.DeleteBuffers(1, &win.pbo)
			}
			widget.win = nil
		}
		win.paint()
		if err := check("full"); err != nil {
			cleanup()
			return err
		}
		win.pboAvailable = pbo // exercise the no-PBO fallback on subsequent frames
		widget.changed = true
		win.MarkDirtyRect(8, 8, 12, 10)
		win.paint()
		if err := check("partial"); err != nil {
			cleanup()
			return err
		}
		if pbo {
			if !win.pboAvailable {
				cleanup()
				return fmt.Errorf("native driver failed PBO probe")
			}
			sx, sy := gw.GetContentScale()
			r := framebufferDamage(geom.Rect{X: 8, Y: 8, Width: 12, Height: 10}, false, win.backWidth, win.backHeight, float64(sx), float64(sy))
			if win.pboSize != r.Dx()*r.Dy()*4 {
				cleanup()
				return fmt.Errorf("PBO uploaded full frame for local damage")
			}
			nativeRenderEvidence = append(nativeRenderEvidence, fmt.Sprintf("frame=%dx%d full=%d bytes partial=%d bytes", win.backWidth, win.backHeight, win.backWidth*win.backHeight*4, win.pboSize))
			// Even an unchanged GPU chart must be re-enqueued after the blit.
			// Presence of the previous overlay queue forces full Cairo traversal.
			win.glOverlays = []chartOverlay{{}}
			win.MarkDirtyRect(8, 8, 12, 10)
			win.paint()
			if win.pboSize != win.backWidth*win.backHeight*4 {
				cleanup()
				return fmt.Errorf("GPU overlay queue did not force full redraw")
			}
			if err := check("overlay"); err != nil {
				cleanup()
				return err
			}
		}
		widget.requestInDraw = true
		win.MarkDirtyRect(8, 8, 12, 10)
		win.paint()
		if !win.dirty || !win.fullDirty {
			cleanup()
			return fmt.Errorf("Update issued during native Draw was lost")
		}
		win.paint()
		if err := check("reentrant"); err != nil {
			cleanup()
			return err
		}
		if win.dirty {
			cleanup()
			return fmt.Errorf("settled widget keeps repainting")
		}
		cleanup()
	}
	return nil
}

func TestGLFWNativeRenderTransfers(t *testing.T) {
	mode := os.Getenv("SILK_TEST_NATIVE_RENDER")
	if mode != "1" && mode != "auto" {
		t.Skip("opt-in real GL context: SILK_TEST_NATIVE_RENDER=1")
	}
	if nativeRenderErr != nil {
		// Hosted macOS VMs can expose Cocoa but no NSGL pixel formats.
		// Only auto mode may skip these specific capability failures. Strict
		// mode, readback mismatches and all other errors always fail.
		if mode == "auto" && nativeRenderCapabilityUnavailable(nativeRenderErr) {
			t.Skipf("runner lacks native GL capability: %v", nativeRenderErr)
		}
		t.Fatal(nativeRenderErr)
	}
	for _, evidence := range nativeRenderEvidence {
		t.Log(evidence)
	}
}

// go-gl/glfw currently panics on FormatUnavailable instead of returning it.
// Keep recovery local to probe creation and rethrow every unrelated panic.
func createNativeRenderProbeWindow(create func() (*glfw.Window, error)) (window *glfw.Window, err error) {
	defer func() {
		if p := recover(); p != nil {
			if e, ok := p.(error); ok && nativeRenderCapabilityUnavailable(e) {
				err = e
				return
			}
			panic(p)
		}
	}()
	return create()
}

func nativeRenderCapabilityUnavailable(err error) bool {
	var e *glfw.Error
	return errors.As(err, &e) && (e.Code == glfw.FormatUnavailable || e.Code == glfw.APIUnavailable || e.Code == glfw.VersionUnavailable)
}

func TestNativeRenderProbeDoesNotHideUnexpectedFailures(t *testing.T) {
	unsupported := &glfw.Error{Code: glfw.FormatUnavailable, Desc: "no test pixel format"}
	_, err := createNativeRenderProbeWindow(func() (*glfw.Window, error) { panic(unsupported) })
	if err != unsupported || !nativeRenderCapabilityUnavailable(err) {
		t.Fatalf("capability panic was not preserved: %v", err)
	}
	mismatch := errors.New("stale texture row")
	if nativeRenderCapabilityUnavailable(mismatch) {
		t.Fatal("pixel mismatch must never be a capability skip")
	}
	defer func() {
		if p := recover(); p != mismatch {
			t.Errorf("unexpected panic was hidden: %v", p)
		}
	}()
	_, _ = createNativeRenderProbeWindow(func() (*glfw.Window, error) { panic(mismatch) })
}
