//go:build !windows

package gui

import (
	"fmt"
	"image"
	"math"

	"github.com/go-gl/gl/v2.1/gl"
	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/paint"
	"github.com/uk0/silk/scene3d"
	"github.com/uk0/silk/scene3d/glrender"
)

type sceneResidentBackend interface {
	sceneGPUBackend
	sceneWindow() *Window
	renderResident(*scene3d.Snapshot, scene3d.Camera, int, int) (glrender.GPUFrame, error)
	blitResident(glrender.GPUFrame, int, int, image.Rectangle, image.Rectangle) error
}
type sceneCompositionLayer struct {
	backend   sceneResidentBackend
	frame     glrender.GPUFrame
	dst, clip image.Rectangle
}
type sceneCompositionWindow struct {
	enabled bool
	failed  string
	layers  []sceneCompositionLayer
}

// SetSceneGPUComposition opts a native GLFW window into experimental resident
// scene composition. Default is false. The first version uses full repaint/upload
// (not a continuous redraw loop). Exact rectangular clips and OVER are required;
// unsupported drawing aborts that presentation and schedules a full Cairo retry.
// A failure disables the path until explicitly re-enabled. Windows/GDI and pure
// Go do not support it. Call on the UI thread, after attaching, outside Draw.
func (w *Window) SetSceneGPUComposition(enabled bool) error {
	if !IsUIThread() {
		return ErrUIInvalid
	}
	if w == nil || w.glfwWin == nil {
		return glrender.ErrClosed
	}
	if w.painting {
		return fmt.Errorf("scene composition cannot change during Draw")
	}
	if enabled && !sceneCompositionAvailable() {
		return glrender.ErrUnavailable
	}
	w.sceneComposition.enabled = enabled
	w.sceneComposition.failed = ""
	w.sceneComposition.layers = nil
	w.MarkFullDirty()
	return nil
}

// SceneGPUCompositionStatus is UI-owner-only. A nonempty reason explains why
// the requested path fell back. Enabled does not imply every scene is eligible.
func (w *Window) SceneGPUCompositionStatus() (enabled bool, reason string) {
	if w == nil {
		return false, ""
	}
	return w.sceneComposition.enabled, w.sceneComposition.failed
}
func (w *Window) beginSceneComposition(p paint.Painter) *sceneCompositionPainter {
	w.sceneComposition.layers = w.sceneComposition.layers[:0]
	return &sceneCompositionPainter{Painter: p, window: w, state: &w.sceneComposition}
}
func (s *sceneCompositionWindow) reject(reason string) {
	if s.failed == "" {
		s.failed = reason
	}
}

type sceneCompositionPainter struct {
	paint.Painter
	window *Window
	state  *sceneCompositionWindow
}

func (p *sceneCompositionPainter) unsafe(reason string) {
	if len(p.state.layers) > 0 {
		p.state.reject(reason)
	}
}
func (p *sceneCompositionPainter) SetOperator(op paint.Operator) {
	if op != paint.OpOver {
		p.unsafe("destination-dependent painter operator")
	}
	p.Painter.SetOperator(op)
}
func (p *sceneCompositionPainter) checkOperator() {
	if op, ok := paint.PainterOperator(p.Painter); !ok || op != paint.OpOver {
		p.unsafe("restored destination-dependent painter operator")
	}
}
func (p *sceneCompositionPainter) Restore() int {
	n := p.Painter.Restore()
	p.checkOperator()
	return n
}
func (p *sceneCompositionPainter) RestoreTo(n int) bool {
	ok := p.Painter.RestoreTo(n)
	p.checkOperator()
	return ok
}
func (p *sceneCompositionPainter) Target() paint.Surface {
	p.unsafe("painter target inspection requires Cairo composition")
	return p.Painter.Target()
}

// Procedural icons receive Cairo directly and may change operators behind the
// wrapper. Until their isolation contract is strengthened, never guess safety.
func (p *sceneCompositionPainter) DrawIcon(i paint.Icon, size float64, grayed bool) {
	if i != nil {
		p.unsafe("icon callback requires Cairo composition")
	}
	p.Painter.DrawIcon(i, size, grayed)
}
func (p *sceneCompositionPainter) DrawIcon1(i paint.Icon, x, y, size float64, grayed bool) {
	if i != nil {
		p.unsafe("icon callback requires Cairo composition")
	}
	p.Painter.DrawIcon1(i, x, y, size, grayed)
}
func sceneDeviceDestination(m geom.Mat3x2, w, h float64) (image.Rectangle, bool) {
	if m.Xy != 0 || m.Yx != 0 || m.Xx <= 0 || m.Yy <= 0 {
		return image.Rectangle{}, false
	}
	edges := [4]float64{m.X0, m.Y0, m.X0 + w*m.Xx, m.Y0 + h*m.Yy}
	for _, x := range edges {
		if math.IsNaN(x) || math.IsInf(x, 0) || x != math.Trunc(x) || x < -(1<<29) || x > 1<<29 {
			return image.Rectangle{}, false
		}
	}
	r := image.Rect(int(edges[0]), int(edges[1]), int(edges[2]), int(edges[3]))
	return r, !r.Empty()
}
func (p *sceneCompositionPainter) drawScene(v *SceneView, s *scene3d.Snapshot, c scene3d.Camera, rw, rh int) (bool, error) {
	if p.state.failed != "" || v.backendMode == "cpu" || v.OwnerWindow() != p.window {
		return false, nil
	}
	if op, ok := paint.PainterOperator(p.Painter); !ok || op != paint.OpOver {
		return false, nil
	}
	var m geom.Mat3x2
	p.GetMatrix(&m)
	w, h := v.Size()
	dst, ok := sceneDeviceDestination(m, w, h)
	if !ok {
		return false, nil
	}
	clip, ok := paint.DeviceClipRectangle(p.Painter)
	if !ok || clip.Empty() || !clip.In(dst) {
		return false, nil
	}
	b, ok := v.gpu.(sceneResidentBackend)
	if !ok || b.sceneWindow() != p.window {
		v.releaseGPU()
		v.gpuAttempted = true
		v.gpu, v.gpuError = newWindowSceneGPUBackend(p.window)
		b, ok = v.gpu.(sceneResidentBackend)
		if !ok {
			p.state.reject("resident context unavailable")
			v.gpuAttempted = false // retry the ordinary hidden-context GPU/Cairo path
			return false, nil
		}
	}
	frame, err := b.renderResident(s, c, rw, rh)
	if err != nil {
		v.gpuError = err
		v.releaseGPU()
		p.state.reject(err.Error())
		return false, nil
	}
	if !frame.Statistics().Cached {
		v.renderCount++
	}
	// Each opaque later scene removes earlier CPU pixels in its exact clip. The
	// ordered GPU underlays plus final premultiplied Cairo layer therefore retain
	// sibling, menu and gizmo ordering. No rectangular approximation is permitted.
	if !paint.ClearDeviceClip(p.Painter) {
		p.state.reject("could not clear exact device clip")
		return false, nil
	}
	p.state.layers = append(p.state.layers, sceneCompositionLayer{b, frame, dst, clip})
	return true, nil
}
func (w *Window) presentSceneComposition(width, height int) error {
	// RenderGPU left no shader bound; each native backend also resets FBO/state.
	initGL()
	gl.Viewport(0, 0, int32(width), int32(height))
	if w.transparentFramebuffer {
		gl.ClearColor(0, 0, 0, 0)
	} else {
		gl.ClearColor(.2, .2, .2, 1)
	}
	gl.Clear(gl.COLOR_BUFFER_BIT)
	for _, l := range w.sceneComposition.layers {
		if err := l.backend.blitResident(l.frame, width, height, l.dst, l.clip); err != nil {
			return err
		}
	}
	initGL()
	gl.Viewport(0, 0, int32(width), int32(height))
	return nil
}
