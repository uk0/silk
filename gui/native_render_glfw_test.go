//go:build !windows

package gui

import (
	"bytes"
	"fmt"
	"os"
	"testing"
	"unsafe"

	"github.com/go-gl/gl/v2.1/gl"
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
	if probeWindow == nil {
		return fmt.Errorf("native render probe needs a GLFW desktop")
	}
	gw := probeWindow
	gw.SetSize(80, 60)
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
	if os.Getenv("SILK_TEST_NATIVE_RENDER") != "1" {
		t.Skip("opt-in real GL context: SILK_TEST_NATIVE_RENDER=1")
	}
	if nativeRenderErr != nil {
		t.Fatal(nativeRenderErr)
	}
	for _, evidence := range nativeRenderEvidence {
		t.Log(evidence)
	}
}
