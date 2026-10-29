package gui

import (
	"context"
	"image/color"
	"math"
	"path/filepath"
	"strings"
	"sync"

	"github.com/uk0/silk/async"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/paint"
	"github.com/uk0/silk/scene3d"
)

// SceneView composites a 3D scene inside the ordinary widget tree, not a native
// child/overlay window. All methods except returned binding cancellation funcs
// are UI-thread-only. Call Close when its host is disposed. Reparent/detach
// cancels model work and tag subscriptions. No timer or animation runs at rest.
//
// Auto uses an isolated offscreen OpenGL backend when available, with explicit
// CPU fallback diagnostics. OBJ/glTF/GLB static scenes are supported; PBR and
// full material/animation rendering remain separate milestones.
// SceneDocumentEditor adds inert, undoable authoring and translation handles.
type SceneView struct {
	Widget
	deviceDropActive              bool
	deviceDropPosition            scene3d.Vec3
	translation                   *sceneTranslationTool
	renderCount                   uint64
	renderError                   error
	uploadedSequence              uint64
	renderScaleX, renderScaleY    float64
	scene                         *scene3d.Scene
	renderer                      scene3d.Renderer
	gpu                           sceneGPUBackend
	backendMode                   string
	gpuAttempted                  bool
	gpuError                      error
	pixmap                        paint.Pixmap
	target                        scene3d.Vec3
	yaw, pitch, distance          float64
	modelSource                   string
	warnings                      []string
	lastError                     error
	selected                      string
	selectedFn                    func(string)
	loadedFn                      func()
	errorFn                       func(string)
	busy, closed, dragging, moved bool
	lastX, lastY, startX, startY  float64
	generation                    uint64
	cancel                        context.CancelFunc
	bindingMu                     sync.Mutex
	bindings                      map[uint64]func()
	nextBinding                   uint64
	bindingProgram                *scene3d.BindingSet
	bindingSpecs                  []scene3d.Binding
}

func init()                    { core.RegisterFactory("gui.SceneView", core.TypeOf((*SceneView)(nil))) }
func NewSceneView() *SceneView { v := new(SceneView); v.Init(v); return v }
func (v *SceneView) Init(self IWidget) {
	v.Widget.Init(self)
	v.backendMode = "auto"
	v.yaw = .65
	v.pitch = .35
	v.distance = 6
	v.scene = scene3d.NewScene()
	_ = v.scene.Add(scene3d.Node{ID: "model", Name: "Preview", Mesh: scene3d.Box()})
}
func (v *SceneView) Scene() *scene3d.Scene { return v.scene }

// RefreshScene requests a frame after a direct Scene edit. Prefer the node
// setters below when editing from a binding or other UI callback.
func (v *SceneView) RefreshScene() {
	if !v.closed {
		v.Self().Update()
	}
}
func (v *SceneView) SetScene(s *scene3d.Scene) error {
	if v.closed {
		return async.ErrClosed
	}
	if s == nil {
		return scene3d.ErrNode
	}
	if _, err := s.Snapshot(); err != nil {
		return err
	}
	v.OnHide()
	v.CancelLoad()
	v.unbindAll()
	v.scene = s
	v.bindingProgram = nil
	v.bindingSpecs = nil
	v.selected = ""
	v.warnings = nil
	v.target = scene3d.Vec3{}
	v.lastError = nil
	v.RefreshScene()
	return nil
}
func (v *SceneView) SetNodeTransform(id string, m scene3d.Mat4) error {
	if v.closed {
		return async.ErrClosed
	}
	before := v.scene.Revision()
	if e := v.scene.SetTransform(id, m); e != nil {
		return e
	}
	if before != v.scene.Revision() {
		v.RefreshScene()
	}
	return nil
}
func (v *SceneView) SetNodeVisible(id string, visible bool) error {
	if v.closed {
		return async.ErrClosed
	}
	before := v.scene.Revision()
	if e := v.scene.SetVisible(id, visible); e != nil {
		return e
	}
	if before != v.scene.Revision() {
		v.RefreshScene()
	}
	return nil
}
func (v *SceneView) SetNodeColor(id string, c color.RGBA) error {
	if v.closed {
		return async.ErrClosed
	}
	before := v.scene.Revision()
	if e := v.scene.SetColor(id, c); e != nil {
		return e
	}
	if before != v.scene.Revision() {
		v.RefreshScene()
	}
	return nil
}
func (v *SceneView) ModelSource() string { return v.modelSource }

// SetModelSource only stores configuration; construction/deserialization never
// opens a path. Call LoadModel explicitly on an operator/application action.
func (v *SceneView) SetModelSource(path string) {
	if path != v.modelSource {
		v.CancelLoad()
		v.modelSource = path
	}
}
func (v *SceneView) Yaw() float64      { return v.yaw }
func (v *SceneView) Pitch() float64    { return v.pitch }
func (v *SceneView) Distance() float64 { return v.distance }
func finiteSceneValue(x float64) bool  { return !math.IsNaN(x) && !math.IsInf(x, 0) }
func (v *SceneView) SetYaw(x float64) {
	if finiteSceneValue(x) {
		x = math.Remainder(x, 2*math.Pi)
		if x != v.yaw {
			if v.translation != nil {
				v.translation.cancel()
			}
			v.yaw = x
			v.RefreshScene()
		}
	}
}
func (v *SceneView) SetPitch(x float64) {
	if finiteSceneValue(x) {
		x = max(-1.5, min(1.5, x))
		if x != v.pitch {
			if v.translation != nil {
				v.translation.cancel()
			}
			v.pitch = x
			v.RefreshScene()
		}
	}
}
func (v *SceneView) SetDistance(x float64) {
	if finiteSceneValue(x) {
		x = max(.001, min(1e12, x))
		if x != v.distance {
			if v.translation != nil {
				v.translation.cancel()
			}
			v.distance = x
			v.RefreshScene()
		}
	}
}
func (v *SceneView) camera() scene3d.Camera {
	sy, cy := math.Sincos(v.yaw)
	sp, cp := math.Sincos(v.pitch)
	return scene3d.Camera{Eye: v.target.Add(scene3d.Vec3{v.distance * cp * sy, v.distance * sp, v.distance * cp * cy}), Target: v.target, Up: scene3d.Vec3{Y: 1}, FOVY: math.Pi / 4, Near: v.distance / 1000, Far: v.distance * 1000}
}

// FitScene reframes the current visible world-space bounds after node edits or
// tag updates. It is explicit, not an automatic camera-follow/render loop.
func (v *SceneView) FitScene() error {
	if v.closed {
		return async.ErrClosed
	}
	v.OnHide()
	snapshot, err := v.scene.Snapshot()
	if err != nil {
		return err
	}
	lo, hi, err := snapshot.Bounds()
	if err != nil {
		return err
	}
	v.target = lo.Add(hi.Sub(lo).Mul(.5))
	v.SetDistance(max(.001, hi.Sub(lo).Length()*1.5))
	v.RefreshScene()
	return nil
}

func (v *SceneView) IsLoading() bool { return v.busy }
func (v *SceneView) LastError() error {
	if v.lastError != nil {
		return v.lastError
	}
	return v.renderError
}
func (v *SceneView) ModelWarnings() []string     { return append([]string(nil), v.warnings...) }
func (v *SceneView) SelectedNode() string        { return v.selected }
func (v *SceneView) RenderCount() uint64         { return v.renderCount }
func (v *SceneView) SigSelected(fn func(string)) { v.selectedFn = fn }
func (v *SceneView) SigModelLoaded(fn func())    { v.loadedFn = fn }
func (v *SceneView) SigError(fn func(string))    { v.errorFn = fn }
func (v *SceneView) fail(err error) {
	v.lastError = err
	v.RefreshScene()
	if err != nil && v.errorFn != nil {
		v.errorFn(err.Error())
	}
}

// LoadModel parses OBJ/glTF/GLB/.silkscene off-thread using the bounded executor. Relative paths
// resolve against the process working directory, not a project asset resolver.
// Completion applies only to this generation. Failed loads keep the old scene.
func (v *SceneView) LoadModel(ctx context.Context) error {
	if ctx == nil {
		return ErrUIInvalid
	}
	if v.closed {
		return async.ErrClosed
	}
	if v.busy {
		return async.ErrBusy
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	path := v.modelSource
	extension := strings.ToLower(filepath.Ext(path))
	if path == "" || (extension != ".obj" && extension != ".gltf" && extension != ".glb" && extension != ".silkscene") {
		return scene3d.ErrUnsupported
	}
	child, cancel := context.WithCancel(ctx)
	v.generation++
	generation := v.generation
	v.cancel = cancel
	v.busy = true
	v.lastError = nil
	_, err := RunAsync(child, nil, func(ctx context.Context) (*scene3d.LoadedModel, error) {
		return scene3d.LoadModelFile(ctx, path, scene3d.LoadLimits{})
	}, func(model *scene3d.LoadedModel, err error) {
		if v.closed || generation != v.generation {
			return
		}
		v.busy = false
		v.cancel = nil
		cancel()
		if err != nil {
			v.fail(err)
			return
		}
		snapshot, err := model.Scene.Snapshot()
		if err != nil {
			v.fail(err)
			return
		}
		lo, hi, err := snapshot.Bounds()
		if err != nil && !(extension == ".silkscene" && snapshot.InstanceCount() == 0) {
			v.fail(err)
			return
		}
		v.unbindAll()
		v.scene = model.Scene
		v.bindingProgram = nil
		v.bindingSpecs = append([]scene3d.Binding(nil), model.Bindings...)
		v.selected = ""
		v.warnings = append([]string(nil), model.Warnings...)
		v.target = lo.Add(hi.Sub(lo).Mul(.5))
		if snapshot.InstanceCount() > 0 {
			v.SetDistance(max(.001, hi.Sub(lo).Length()*1.5))
		}
		v.RefreshScene()
		if v.loadedFn != nil {
			v.loadedFn()
		}
	})
	if err != nil {
		cancel()
		v.busy = false
		v.cancel = nil
		v.fail(err)
		return err
	}
	v.RefreshScene()
	return nil
}
func (v *SceneView) CancelLoad() {
	v.generation++
	if v.cancel != nil {
		v.cancel()
		v.cancel = nil
	}
	v.busy = false
	v.RefreshScene()
}
func (v *SceneView) Close() {
	if v.closed {
		return
	}
	v.closed = true
	v.OnHide()
	v.translation = nil
	v.CancelLoad()
	v.unbindAll()
	paint.CloseSurface(v.pixmap)
	v.pixmap = nil
	v.releaseGPU()
	v.renderer.Reset()
	v.scene = nil
	v.bindingProgram = nil
	v.bindingSpecs = nil
	v.selectedFn = nil
	v.loadedFn = nil
	v.errorFn = nil
}
func (v *SceneView) SetParent(parent IWidget) {
	if parent != v.Parent() {
		v.releaseGPU()
		v.gpuAttempted = false
		v.gpuError = nil
		v.OnHide()
		v.CancelLoad()
		v.unbindAll()
	}
	v.Widget.SetParent(parent)
}
func (v *SceneView) Detach() { v.SetParent(nil) }
func (v *SceneView) Draw(g paint.Painter) {
	w, h := v.Size()
	if v.closed || w <= 0 || h <= 0 || !finiteSceneValue(w) || !finiteSceneValue(h) {
		return
	}
	g.Save()
	defer g.Restore()
	g.Rectangle(0, 0, w, h)
	g.Clip()
	g.SetBrush1(paint.Color{20, 28, 41, 255})
	g.Paint()
	var m geom.Mat3x2
	g.GetMatrix(&m)
	sx, sy := math.Hypot(m.Xx, m.Yx), math.Hypot(m.Xy, m.Yy)
	rw, rh := math.Ceil(w*sx), math.Ceil(h*sy)
	if !finiteSceneValue(rw) || !finiteSceneValue(rh) || rw < 1 || rh < 1 || rw > scene3d.MaxDimension || rh > scene3d.MaxDimension || rw*rh > scene3d.MaxPixels {
		v.renderError = scene3d.ErrLimit
		v.drawDiagnostic(g, w)
		return
	}
	v.renderScaleX, v.renderScaleY = sx, sy
	snap, err := v.scene.Snapshot()
	if err == nil {
		var f scene3d.FrameView
		f, err = v.renderScene(snap, v.camera(), int(rw), int(rh))
		if err == nil {
			changed := v.uploadedSequence != v.renderCount

			if v.pixmap == nil || v.pixmap.Width() != int(rw) || v.pixmap.Height() != int(rh) {
				paint.CloseSurface(v.pixmap)
				v.pixmap = paint.NewPixmap(int(rw), int(rh))
				changed = true
			}
			if changed {
				err = v.pixmap.SetImage(f.RGBA())
				if err == nil {
					v.uploadedSequence = v.renderCount
				}
			}
			if err == nil {
				g.DrawPixmap5(0, 0, w, h, v.pixmap)
			}
		}
	}
	if v.translation != nil {
		v.translation.draw(g)
	}
	v.drawDeviceDrop(g)
	// Paint diagnostics without callbacks or I/O during Draw.
	v.renderError = err
	v.drawDiagnostic(g, w)
}
func (v *SceneView) drawDiagnostic(g paint.Painter, w float64) {
	if err := v.LastError(); err != nil {
		g.SetFont(Theme().Font)
		g.SetBrush1(paint.Color{255, 160, 130, 255})
		g.DrawText1(8, 20, elideToWidth(Theme().Font, err.Error(), max(0, w-16)))
	}
}

func (v *SceneView) SizeHints() SizeHints {
	return SizeHints{Width: 320, Height: 240, Policy: GrowHorizontal | GrowVertical}
}
func (v *SceneView) EnumProperties(list core.IPropertyList) {
	list.AddProperty("Render Backend", v.RenderBackend, v.SetRenderBackend)
	list.AddProperty("Model Source", v.ModelSource, v.SetModelSource)
	list.AddProperty("Camera Yaw", v.Yaw, v.SetYaw)
	list.AddProperty("Camera Pitch", v.Pitch, v.SetPitch)
	list.AddProperty("Camera Distance", v.Distance, v.SetDistance)
}
func (v *SceneView) OnLeftDown(x, y float64) {
	if v.closed || !v.IsEnabled() || !finiteSceneValue(x) || !finiteSceneValue(y) {
		return
	}
	v.OnHide()
	v.SetFocus()
	if v.translation != nil && v.translation.begin(x, y) {
		return
	}
	v.dragging = true
	v.moved = false
	v.lastX, v.lastY, v.startX, v.startY = x, y, x, y
	v.PushCapture()
}
func (v *SceneView) OnMouseMove(x, y float64) {
	if v.translation != nil && v.translation.gesture != nil {
		v.translation.move(x, y)
		return
	}
	if !v.dragging || v.closed || !finiteSceneValue(x) || !finiteSceneValue(y) {
		return
	}
	if !v.IsEnabled() {
		v.OnHide()
		return
	}
	if math.Hypot(x-v.startX, y-v.startY) > 3 {
		v.moved = true
	}
	if v.moved {
		v.SetYaw(v.yaw + (x-v.lastX)*.01)
		v.SetPitch(v.pitch + (y-v.lastY)*.01)
	}
	v.lastX, v.lastY = x, y
}
func (v *SceneView) OnLeftUp(x, y float64) {
	if v.translation != nil && v.translation.gesture != nil {
		v.translation.finish(x, y)
		return
	}
	was, moved := v.dragging, v.moved
	v.dragging = false
	v.PopCapture()
	if !was || moved || v.closed || !v.IsEnabled() {
		return
	}
	id := v.PickNode(x, y)
	if id != v.selected {
		v.selected = id
		v.RefreshScene()
		if v.selectedFn != nil {
			v.selectedFn(id)
		}
	}
}

// PickNode uses the current scene/camera, not a stale last-painted ID buffer.
func (v *SceneView) PickNode(x, y float64) string {
	w, h := v.Size()
	if v.closed || w <= 0 || h <= 0 || x < 0 || y < 0 || x >= w || y >= h || !finiteSceneValue(x) || !finiteSceneValue(y) || !finiteSceneValue(w) || !finiteSceneValue(h) || w > scene3d.MaxDimension || h > scene3d.MaxDimension {
		return ""
	}
	snap, e := v.scene.Snapshot()
	if e != nil {
		return ""
	}
	// Reuse the physical-pixel resolution last used by Draw. Picking at logical
	// resolution would replace the HiDPI framebuffer and rerasterize twice.
	sx, sy := v.renderScaleX, v.renderScaleY
	if sx <= 0 || sy <= 0 {
		sx, sy = 1, 1
	}
	rw, rh := math.Ceil(w*sx), math.Ceil(h*sy)
	if !finiteSceneValue(rw) || !finiteSceneValue(rh) || rw > scene3d.MaxDimension || rh > scene3d.MaxDimension || rw*rh > scene3d.MaxPixels {
		return ""
	}
	f, e := v.renderScene(snap, v.camera(), int(rw), int(rh))
	if e != nil {
		return ""
	}
	return f.Pick(min(int(rw)-1, int(x*rw/w)), min(int(rh)-1, int(y*rh/h)))
}
func (v *SceneView) OnMouseWheel(x, y, z float64) {
	if v.closed || !v.IsEnabled() || !finiteSceneValue(z) {
		return
	}
	if v.translation != nil && v.translation.gesture != nil {
		v.translation.cancel()
	}
	// GLFW/macOS can deliver large smooth-scroll/page deltas, unlike a Win32
	// wheel notch. Cap one event to one notch while preserving fractional input;
	// a page event must not zoom by exp(5) and make the model disappear.
	v.SetDistance(v.distance * math.Exp(-max(-1, min(1, z))*.1))
}
func (v *SceneView) OnHide() {
	v.OnDragLeave()
	v.dragging = false
	if v.translation != nil {
		v.translation.cancel()
	}
	v.PopCapture()
}
