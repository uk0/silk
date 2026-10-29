//go:build cgo && !silk_pure_go

package glrender

import (
	"bytes"
	"errors"
	"fmt"
	"image/color"
	"testing"

	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/uk0/silk/scene3d"
	gl "github.com/uk0/silk/scene3d/glrender/internal/gl21"
)

var nativeColorRan bool
var nativeColorErr error

func TestNativeColorOnlyCaptureAndFullUpgrade(t *testing.T) {
	if !nativeColorRan {
		t.Skip("set SILK_TEST_SCENE_GPU=1 for actual driver/context validation")
	}
	if nativeColorErr != nil {
		t.Fatal(nativeColorErr)
	}
}

func nativeColorProbe() error {
	r, err := NewCurrent()
	if err != nil {
		return err
	}
	defer r.Close()
	reference, err := NewCurrent()
	if err != nil {
		return err
	}
	defer reference.Close()
	s := scene3d.NewScene()
	for _, n := range []scene3d.Node{
		{ID: "center", Mesh: scene3d.Box(), Color: color.RGBA{215, 32, 71, 255}},
		{ID: "asymmetric", Mesh: scene3d.Box(), Color: color.RGBA{41, 213, 135, 255}, Transform: scene3d.Translation(scene3d.Vec3{X: 1.2, Y: 1.1, Z: 1}).Mul(scene3d.Scaling(scene3d.Vec3{X: .25, Y: .4, Z: .3}))},
	} {
		if err = s.Add(n); err != nil {
			return err
		}
	}
	snap, err := s.Snapshot()
	if err != nil {
		return err
	}
	c := scene3d.DefaultCamera()
	c.Eye = scene3d.Vec3{Z: 6}
	bg := color.RGBA{20, 28, 41, 255}
	if _, err = (*Renderer)(nil).ReadColor(GPUFrame{}); !errors.Is(err, ErrClosed) {
		return fmt.Errorf("nil color: %v", err)
	}
	if _, err = r.ReadColor(GPUFrame{}); !errors.Is(err, ErrFrameStale) {
		return fmt.Errorf("zero color: %v", err)
	}
	var previous GPUFrame
	var expected TransferStats
	for i, size := range [][2]int{{64, 48}, {80, 60}, {17, 29}, {64, 48}} {
		w, h := size[0], size[1]
		f, e := r.RenderGPU(snap, c, w, h, bg)
		if e != nil {
			return e
		}
		if i > 0 {
			if _, e = r.ReadColor(previous); !errors.Is(e, ErrFrameStale) {
				return fmt.Errorf("stale color: %v", e)
			}
		}
		if _, e = reference.ReadColor(f); !errors.Is(e, ErrFrameStale) {
			return fmt.Errorf("foreign color: %v", e)
		}
		r.api.PixelStorei(gl.PACK_SKIP_ROWS, 2)
		r.api.PixelStorei(gl.PACK_SKIP_PIXELS, 3)
		capture, e := r.ReadColor(f)
		if e != nil {
			return e
		}
		expected.ColorReads++
		expected.ColorBytes += uint64(w * h * 4)
		tr, e := r.Transfers()
		if e != nil || tr != expected || capture.IsCached() || capture.Sequence != f.sequence || capture.Stats.ReadbackBytes != int64(w*h*4) || len(r.frame.pickPixels) != 0 || r.frame.Image != nil {
			return fmt.Errorf("color allocated/read IDs or incorrect counters: %+v want %+v: %v", tr, expected, e)
		}
		pixels := append([]byte(nil), capture.Image.Pix...)
		// Independent legacy renderer is the exact pixel/ID reference, not a CPU
		// renderer with different rasterization rules.
		full, e := reference.Render(snap, c, w, h, bg)
		if e != nil {
			return e
		}
		if !bytes.Equal(pixels, full.Image.Pix) {
			return fmt.Errorf("color orientation/byte mismatch case %d", i)
		}
		again, e := r.ReadColor(f)
		if e != nil || !again.IsCached() || !bytes.Equal(pixels, again.Image.Pix) {
			return fmt.Errorf("color cache: %v", e)
		}
		// Raster identity, including background, has to agree with full ID capture.
		for _, p := range [][2]int{{0, 0}, {w / 2, h / 2}, {w - 1, h - 1}, {w * 3 / 4, h / 4}} {
			id, e := r.PickGPU(f, p[0], p[1])
			if e != nil || id != full.Pick(p[0], p[1]) {
				return fmt.Errorf("color-only pick parity at %v: %q %v", p, id, e)
			}
			expected.IDReads++
			expected.IDBytes += 4
		}
		tr, e = r.Transfers()
		if e != nil || tr != expected {
			return fmt.Errorf("cached color/picks transferred extra bytes %+v", tr)
		}
		previous = f
	}
	// A different host context must not allow even the cached color path.
	current := glfw.GetCurrentContext()
	other, err := glfw.CreateWindow(16, 16, "color wrong context", nil, nil)
	if err != nil {
		return err
	}
	other.MakeContextCurrent()
	_, contextErr := r.ReadColor(previous)
	current.MakeContextCurrent()
	other.Destroy()
	if !errors.Is(contextErr, ErrContext) {
		return fmt.Errorf("wrong-context cached color: %v", contextErr)
	}
	full, err := r.Readback(previous)
	if err != nil {
		return err
	}
	expected.IDReads++
	expected.IDBytes += 64 * 48 * 4
	tr, _ := r.Transfers()
	if tr != expected || full.Stats.ReadbackBytes != 64*48*8 || full.Pick(32, 24) != "center" {
		return fmt.Errorf("upgrade repeated color or bad ID: %+v", tr)
	}
	again, err := r.ReadColor(previous)
	if err != nil || !again.IsCached() || again.Image != full.Image {
		return fmt.Errorf("full to color cache: %v", err)
	}
	_, err = r.Readback(previous)
	tr, _ = r.Transfers()
	if err != nil || tr != expected {
		return fmt.Errorf("full cache traffic %+v %v", tr, err)
	}
	// Once full capture exists, a resized color capture must not reuse its old
	// dimensions/IDs when later upgraded (equal area, different shape included).
	for _, size := range [][2]int{{96, 32}, {32, 96}} {
		w, h := size[0], size[1]
		f, e := r.RenderGPU(snap, c, w, h, bg)
		if e != nil {
			return e
		}
		co, e := r.ReadColor(f)
		if e != nil {
			return e
		}
		pixels := append([]byte(nil), co.Image.Pix...)
		fu, e := r.Readback(f)
		if e != nil {
			return e
		}
		ref, e := reference.Render(snap, c, w, h, bg)
		if e != nil {
			return e
		}
		if !bytes.Equal(pixels, fu.Image.Pix) || !bytes.Equal(fu.Image.Pix, ref.Image.Pix) || !bytes.Equal(fu.pickPixels, ref.pickPixels) {
			return fmt.Errorf("resized full/color parity %v", size)
		}
		previous = f
	}
	r.Close()
	if _, err = r.ReadColor(previous); !errors.Is(err, ErrClosed) {
		return fmt.Errorf("closed color: %v", err)
	}
	fmt.Println("GPU_COLOR_PASS exact_rgba_pick color_only_no_id_alloc cache_upgrade resize context_thread")
	return nil
}
