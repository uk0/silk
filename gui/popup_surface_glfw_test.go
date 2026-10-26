//go:build !windows

package gui

import (
	"fmt"
	"os"
	"testing"
	"unsafe"

	"github.com/go-gl/gl/v2.1/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/uk0/silk/geom"
)

var nativePopupSurfaceErr error
var nativePopupSurfaceEvidence []string

func TestPopupFramebufferHintDoesNotLeakToForms(t *testing.T) {
	for _, wt := range []WindowType{WtPopup, WtForm, WtChild, WtInherit} {
		want := glfw.False
		if wt == WtPopup {
			want = glfw.True
		}
		if popupFramebufferHint(wt) != want {
			t.Fatal("wrong framebuffer policy", wt)
		}
	}
}

func TestGLFWNativePopupSurfaceAlpha(t *testing.T) {
	if os.Getenv("SILK_TEST_NATIVE_POPUP_ALPHA") != "1" {
		t.Skip("opt-in native alpha readback: SILK_TEST_NATIVE_POPUP_ALPHA=1")
	}
	if nativePopupSurfaceErr != nil {
		t.Fatal(nativePopupSurfaceErr)
	}
	for _, s := range nativePopupSurfaceEvidence {
		t.Log(s)
	}
}

// Called by TestMain on the actual OS main thread. Native window capability,
// final GL framebuffer RGBA and chart-overlay alpha are all read back; a
// transparent Cairo texture by itself does not prove transparent presentation.
func runNativePopupSurfaceProbe() error {
	if !glfwReady {
		return fmt.Errorf("native popup alpha probe needs a desktop")
	}
	defer glfw.DefaultWindowHints()
	savedPacer := glfwAnimationPacer
	defer func() { glfwAnimationPacer = savedPacer }()
	create := func(wt WindowType) (*Window, func(), error) {
		m := NewPopupMenu()
		m.SetSize(120, 80)
		w := &Window{widget: m}
		m.win = w
		if err := w.create(nil, wt); err != nil {
			m.win = nil
			return nil, func() {}, err
		}
		return w, func() {
			w.glfwWin.MakeContextCurrent()
			if w.glTexture != 0 {
				gl.DeleteTextures(1, &w.glTexture)
			}
			if w.pbo != 0 {
				gl.DeleteBuffers(1, &w.pbo)
			}
			w.releaseBackBuffer()
			delete(winMap, w.glfwWin)
			w.glfwWin.Destroy()
			m.win = nil
		}, nil
	}
	w, closePopup, err := create(WtPopup)
	if err != nil {
		return err
	}
	defer closePopup()
	if !w.hasAlphaSurface() || w.glfwWin.GetAttrib(glfw.TransparentFramebuffer) != glfw.True {
		return fmt.Errorf("native popup alpha capability not enabled")
	}
	// This creation immediately follows a popup: hints must not leak globally.
	form, closeForm, err := create(WtForm)
	if err != nil {
		return err
	}
	defer closeForm()
	if form.hasAlphaSurface() || form.glfwWin.GetAttrib(glfw.TransparentFramebuffer) != glfw.False {
		return fmt.Errorf("popup alpha leaked into ordinary form")
	}
	for _, pbo := range []bool{true, false} {
		w.MarkFullDirty()
		w.paint()
		w.pboAvailable = pbo
		w.MarkDirtyRect(0, 0, 16, 16)
		w.paint()
		if w.fullDirty || w.dirty {
			return fmt.Errorf("popup did not settle after partial paint")
		}
		fbw, fbh := w.backWidth, w.backHeight
		gl.DrawBuffer(gl.BACK)
		drawFullscreenQuadSurface(w.glTexture, int32(fbw), int32(fbh), 1, 1, w.transparentFramebuffer)
		got := make([]byte, fbw*fbh*4)
		gl.BindBuffer(gl.PIXEL_PACK_BUFFER, 0)
		gl.PixelStorei(gl.PACK_ROW_LENGTH, 0)
		gl.PixelStorei(gl.PACK_ALIGNMENT, 4)
		gl.ReadBuffer(gl.BACK)
		gl.ReadPixels(0, 0, int32(fbw), int32(fbh), gl.BGRA, gl.UNSIGNED_BYTE, gl.Ptr(got))
		w.backBuffer.Flush()
		src := unsafe.Slice((*byte)(w.backBuffer.DataPtr()), w.backBuffer.Stride()*fbh)
		partial := 0
		for y := 0; y < fbh; y++ {
			for x := 0; x < fbw; x++ {
				si := y*w.backBuffer.Stride() + x*4
				di := ((fbh-1-y)*fbw + x) * 4
				if src[si+3] > 0 && src[si+3] < 255 {
					partial++
				}
				for c := 0; c < 4; c++ {
					d := int(got[di+c]) - int(src[si+c])
					if d < -1 || d > 1 {
						return fmt.Errorf("pbo=%v pixel(%d,%d) channel=%d presented=%d cairo=%d", pbo, x, y, c, got[di+c], src[si+c])
					}
				}
			}
		}
		if partial == 0 || src[3] != 0 || got[((fbh-1)*fbw)*4+3] != 0 {
			return fmt.Errorf("rounded corners or antialias alpha missing")
		}
		nativePopupSurfaceEvidence = append(nativePopupSurfaceEvidence, fmt.Sprintf("native popup %dx%d pbo=%v: every RGBA channel matches; %d edge pixels retain fractional alpha", fbw, fbh, pbo, partial))
	}
	// The following chart uses straight RGB. Alpha must be source-over rather
	// than alpha-squared, including when the destination is transparent.
	fbw, fbh := int32(w.backWidth), int32(w.backHeight)
	gl.ClearColor(0, 0, 0, 0)
	gl.Clear(gl.COLOR_BUFFER_BIT)
	w.glOverlays = []chartOverlay{{clip: geom.Rect{Width: float64(fbw), Height: float64(fbh)}, lines: []overlayPolyline{{rgba: [4]float32{240.0 / 255, 120.0 / 255, 60.0 / 255, .5}, width: 4, pts: []float32{10, 10, 60, 10}}}}}
	w.drawGLOverlays(fbw, fbh)
	var pixel [4]byte
	gl.ReadPixels(30, fbh-10, 1, 1, gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(&pixel[0]))
	for i, want := range []int{120, 60, 30, 128} {
		d := int(pixel[i]) - want
		if d < -1 || d > 1 {
			return fmt.Errorf("straight-alpha chart incorrectly composited: %v", pixel)
		}
	}
	if e := gl.GetError(); e != gl.NO_ERROR {
		return fmt.Errorf("native popup GL error=%d", e)
	}
	nativePopupSurfaceEvidence = append(nativePopupSurfaceEvidence, fmt.Sprintf("chart overlay rgba=%v; ordinary form alpha=false", pixel))
	return nil
}
