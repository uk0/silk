//go:build cgo && !silk_pure_go && !windows

package gui

import (
	"errors"
	"fmt"
	"image"
	"math"
	"os"
	"runtime"
	"testing"

	"github.com/go-gl/gl/v2.1/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/paint"
	"github.com/uk0/silk/scene3d"
	"github.com/uk0/silk/scene3d/glrender"
)

var sceneDirectErr error

func TestSceneDirectGPUComposition(t *testing.T) {
	if os.Getenv("SILK_TEST_SCENE_GPU") != "1" {
		t.Skip("set SILK_TEST_SCENE_GPU=1 for actual native compositor")
	}
	if sceneDirectErr != nil {
		t.Fatal(sceneDirectErr)
	}
}

type sceneDirectProbeWidget struct {
	Widget
	layers     []sceneCompositionLayer
	complex    bool
	failure    string
	closeOwner bool
}

func (p *sceneDirectProbeWidget) Draw(g paint.Painter) {
	g.SetBrush1(paint.Color{31, 43, 57, 255})
	g.Paint()
	if p.complex {
		g.Arc(100, 85, 75, 0, 2*math.Pi)
		g.Clip()
	}
}
func (p *sceneDirectProbeWidget) DrawOverlay(g paint.Painter) {
	if p.closeOwner {
		p.win.Close()
	}
	switch p.failure {
	case "operator":
		g.SetOperator(paint.OpSource)
	case "icon":
		g.DrawIcon1(paint.LoadIcon("edit-copy"), 90, 70, 20, false)
	case "target":
		g.Target()
	case "stale":
		if c, ok := g.(*sceneCompositionPainter); ok && len(c.state.layers) > 0 {
			b := c.state.layers[0].backend.(*nativeSceneGPU)
			b.Close() // queued lease must fail safely, never touch a dead renderer
		}
	}
	// Menu-like opaque rectangle, translucent shadow, and single-pixel stripes.
	g.SetBrush1(paint.Color{233, 19, 147, 255})
	g.Rectangle(72, 48, 44, 25)
	g.Fill()
	g.SetBrush1(paint.Color{240, 120, 60, 128})
	g.Rectangle(62, 81, 110, 27)
	g.Fill()
	g.SetBrush1(paint.Color{35, 180, 245, 96})
	g.Rectangle(80, 91, 44, 25)
	g.Fill()
	var m geom.Mat3x2
	g.GetMatrix(&m)
	for i := 0; i < 8; i++ {
		c := paint.Color{R: 255, A: 255}
		if i%2 == 0 {
			c = paint.Color{G: 255, A: 255}
		}
		g.SetBrush1(c)
		g.Rectangle(126+float64(i)/m.Xx, 52, 1/m.Xx, 13)
		g.Fill()
	}
	if c, ok := g.(*sceneCompositionPainter); ok {
		p.layers = append(p.layers[:0], c.state.layers...)
	}
}

type sceneBetweenProbeWidget struct{ Widget }

func (*sceneBetweenProbeWidget) Draw(g paint.Painter) {
	g.SetBrush1(paint.Color{15, 210, 71, 190})
	g.Paint()
}

func runSceneDirectProbe() error {
	if !glfwReady {
		return fmt.Errorf("direct composition requires a desktop")
	}
	root := &sceneDirectProbeWidget{}
	root.Init(root)
	root.SetSize(250, 190)
	w := &Window{widget: root}
	root.win = w
	if err := w.create(nil, WtForm); err != nil {
		return err
	}
	defer func() {
		if w.glfwWin != nil {
			w.destroy()
		}
		flushNativeDestruction()
	}()
	glfw.SwapInterval(0)
	v := NewSceneView()
	v.SetParent(root)
	v.SetBounds(16, 20, 150, 120)
	v.SetYaw(0)
	v.SetPitch(0)
	defer v.Close()
	mid := &sceneBetweenProbeWidget{}
	mid.Init(mid)
	mid.SetParent(root)
	mid.SetBounds(90, 32, 100, 93)
	second := NewSceneView()
	second.SetParent(root)
	second.SetBounds(110, 56, 125, 110)
	second.SetYaw(.8)
	defer second.Close()
	if err := w.SetSceneGPUComposition(true); err != nil {
		return err
	}
	if w.supportsGLOverlay() {
		return fmt.Errorf("legacy chart overlay enabled over resident tree")
	}
	check := func(stage string) error {
		a, _ := v.GPUTransfers()
		b, _ := second.GPUTransfers()
		root.layers = nil
		w.paint()
		if enabled, reason := w.SceneGPUCompositionStatus(); !enabled || reason != "" {
			return fmt.Errorf("%s disabled: %s", stage, reason)
		}
		if len(root.layers) != 2 || w.dirty || w.fullDirty {
			return fmt.Errorf("%s layers=%d did not settle", stage, len(root.layers))
		}
		aa, err := v.GPUTransfers()
		if err != nil {
			return err
		}
		bb, err := second.GPUTransfers()
		if err != nil {
			return err
		}
		if aa.ColorReads != a.ColorReads || aa.IDReads != a.IDReads || bb.ColorReads != b.ColorReads || bb.IDReads != b.IDReads || aa.ColorBlits != a.ColorBlits+1 || bb.ColorBlits != b.ColorBlits+1 {
			return fmt.Errorf("%s direct path downloaded pixels: %+v -> %+v / %+v -> %+v", stage, a, aa, b, bb)
		}
		width, height := w.backWidth, w.backHeight
		// Re-present the exact queued leases and already uploaded Cairo texture into
		// BACK for deterministic pixel assertions (FRONT is undefined for hidden windows).
		w.sceneComposition.layers = root.layers
		if err := w.presentSceneComposition(width, height); err != nil {
			return err
		}
		drawFullscreenQuadLayer(w.glTexture, int32(width), int32(height), 1, 1)
		w.sceneComposition.layers = nil
		got := make([]byte, width*height*4)
		gl.ReadBuffer(gl.BACK)
		gl.BindBuffer(gl.PIXEL_PACK_BUFFER, 0)
		gl.PixelStorei(gl.PACK_ROW_LENGTH, 0)
		gl.ReadPixels(0, 0, int32(width), int32(height), gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(got))
		if e := gl.GetError(); e != gl.NO_ERROR {
			return fmt.Errorf("%s GL error %#x", stage, e)
		}
		// Same GPU raster, ordinary Cairo painter: independent ordered reference,
		// intentionally captured only after zero-readback counters were asserted.
		pm := paint.NewPixmap(width, height)
		p := pm.NewPainter()
		sx, sy := w.glfwWin.GetContentScale()
		p.Scale(float64(sx), float64(sy))
		DrawWidgetAll(root, p, 0, 0, 0, 0, root.Width(), root.Height())
		im, e := pm.Image()
		paint.ClosePainter(p)
		paint.CloseSurface(pm)
		if e != nil {
			return e
		}
		maxDelta := 0
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				r, g, b, a := im.At(x, y).RGBA()
				want := [4]uint32{r >> 8, g >> 8, b >> 8, a >> 8}
				di := ((height-1-y)*width + x) * 4
				for c := 0; c < 4; c++ {
					d := int(got[di+c]) - int(want[c])
					if d < 0 {
						d = -d
					}
					if d > maxDelta {
						maxDelta = d
					}
					if d > 1 {
						return fmt.Errorf("%s pixel %d,%d channel %d got=%d Cairo=%d", stage, x, y, c, got[di+c], want[c])
					}
				}
			}
		}
		fmt.Printf("SCENE_DIRECT_PIXELS %s %dx%d max_delta=%d no_readback ordered=2\n", stage, width, height, maxDelta)
		return nil
	}
	if err := check("initial"); err != nil {
		return err
	}
	// Pick is exactly four bytes, and must restore the visible window GL context.
	before, _ := v.GPUTransfers()
	count := v.RenderCount()
	if v.PickNode(75, 60) != "model" || glfw.GetCurrentContext() != w.glfwWin || v.RenderCount() != count {
		return fmt.Errorf("resident pick/context/cache")
	}
	after, _ := v.GPUTransfers()
	if after.ColorReads != before.ColorReads || after.IDBytes != before.IDBytes+4 {
		return fmt.Errorf("resident pick downloaded color: %+v", after)
	}
	for _, pbo := range []bool{true, false} {
		w.pboAvailable = pbo
		w.MarkDirtyRect(72, 48, 2, 2)
		if err := check(fmt.Sprintf("partial-pbo-%v", pbo)); err != nil {
			return err
		}
	}
	// Arbitrary clip masks must fall back locally, not display their bounding box.
	root.complex = true
	w.MarkFullDirty()
	w.paint()
	if len(root.layers) != 0 {
		return fmt.Errorf("complex clip accepted")
	}
	if enabled, reason := w.SceneGPUCompositionStatus(); !enabled || reason != "" {
		return fmt.Errorf("complex local fallback poisoned window")
	}
	root.complex = false
	// Fractional device placement remains ordinary Cairo, not blurred/scissored.
	second.SetBounds(110.125, 56, 125, 110)
	root.layers = nil
	w.paint()
	if len(root.layers) != 1 {
		return fmt.Errorf("fractional placement did not locally fall back: %d", len(root.layers))
	}
	second.SetBounds(110, 56, 125, 110)
	for _, size := range [][2]int{{310, 215}, {200, 165}, {250, 190}} {
		w.glfwWin.SetSize(size[0], size[1])
		glfw.PollEvents()
		w.MarkFullDirty()
		if err := check("resize"); err != nil {
			return err
		}
	}
	for _, failure := range []string{"operator", "icon", "target", "stale"} {
		root.failure = failure
		if err := w.SetSceneGPUComposition(true); err != nil {
			return err
		}
		w.paint()
		if enabled, reason := w.SceneGPUCompositionStatus(); enabled || reason == "" || !w.fullDirty {
			return fmt.Errorf("%s did not reject unsafe presentation: %v %q", failure, enabled, reason)
		}
		root.failure = ""
		w.paint()
		if w.dirty || w.fullDirty {
			return fmt.Errorf("%s fallback failed to settle", failure)
		}
	}
	// Another owner/context cannot reuse this window's renderer, even with sharing.
	other := NewForm()
	if e := other.TryAttachWindow(WtForm); e != nil {
		return e
	}
	defer other.Window().destroy()
	v.SetParent(other)
	v.SetParent(root)
	if err := w.SetSceneGPUComposition(true); err != nil {
		return err
	}
	if err := check("reparent"); err != nil {
		return err
	}
	b := v.gpu.(*nativeSceneGPU)
	snap, _ := v.scene.Snapshot()
	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		close(ready)
		for i := 0; i < 80; i++ {
			if _, e := b.renderResident(snap, scene3d.DefaultCamera(), 32, 24); !errors.Is(e, ErrUIInvalid) {
				done <- fmt.Errorf("worker Render: %v", e)
				return
			}
			if e := b.blitResident(glrender.GPUFrame{}, 32, 24, image.Rect(0, 0, 32, 24), image.Rect(0, 0, 32, 24)); !errors.Is(e, ErrUIInvalid) {
				done <- fmt.Errorf("worker Blit: %v", e)
				return
			}
			if e := w.SetSceneGPUComposition(true); !errors.Is(e, ErrUIInvalid) {
				done <- fmt.Errorf("worker API: %v", e)
				return
			}
			b.Close()
		}
		done <- nil
	}()
	<-ready
	// A close in Draw defers native teardown; all borrowed renderers retire first.
	root.closeOwner = true
	w.MarkFullDirty()
	w.paint()
	if e := <-done; e != nil {
		return e
	}
	if w.glfwWin != nil || w.backBuffer != nil || b.renderer != nil || windowSceneDevices[w] != nil {
		return fmt.Errorf("window destroyed before renderer retirement")
	}
	v.Close()
	second.Close()
	fmt.Println("SCENE_DIRECT_PASS ordered_overlap exact_clip fractional_fallback four_byte_pick no_color_readback resize pbo operator_icon_target_abort stale_lease reparent worker_close")
	return nil
}
