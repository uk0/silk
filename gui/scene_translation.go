package gui

import (
	"context"
	"math"

	"github.com/uk0/silk/async"
	"github.com/uk0/silk/paint"
	"github.com/uk0/silk/scene3d"
)

// TranslationEnabled enables world XYZ handles in this editor's embedded
// preview. It does not start live bindings or change the authored document.
func (e *SceneDocumentEditor) TranslationEnabled() bool { return e.transformMode == SceneTransformMove }
func (e *SceneDocumentEditor) SetTranslationEnabled(enabled bool) {
	if enabled {
		_ = e.SetTransformMode(SceneTransformMove)
	} else if e.TranslationEnabled() {
		_ = e.SetTransformMode(SceneTransformNone)
	}
}
func (e *SceneDocumentEditor) TranslationSnapStep() float64 { return e.translationSnapStep }

// SetTranslationSnapStep configures world-displacement snapping (0 = free).
// Holding Shift during a pointer gesture temporarily uses a one-unit grid.
func (e *SceneDocumentEditor) SetTranslationSnapStep(step float64) error {
	if e.closed {
		return async.ErrClosed
	}
	if !finiteSceneValue(step) || step < 0 {
		return scene3d.ErrGeometry
	}
	if step != e.translationSnapStep {
		e.preview.OnHide()
		e.translationSnapStep = step
	}
	return nil
}

type sceneTranslationTool struct {
	editor                        *SceneDocumentEditor
	origin                        scene3d.Vec3 // cached authored origin, updated on selection/edit only
	valid                         bool
	gesture                       interface{ Cancel() }
	solver                        scene3d.AxisDrag
	id                            string
	original                      scene3d.Mat4
	previewScene                  *scene3d.Scene
	delta                         scene3d.Vec3
	camera                        scene3d.Camera
	width, height, startX, startY float64
	axis                          int
	moved                         bool
	operation                     scene3d.Mat4
	rotationSolver                scene3d.RotationDrag
	rawAngle, handleLength        float64
	rings                         sceneRotationCache
}

type sceneTranslationHandle struct {
	end   scene3d.Vec3
	valid bool
}

var sceneTranslationAxes = [3]scene3d.Vec3{{X: 1}, {Y: 1}, {Z: 1}}
var sceneTranslationColors = [3]paint.Color{{245, 91, 97, 255}, {85, 207, 141, 255}, {84, 160, 255, 255}}

func (t *sceneTranslationTool) sync() {
	t.valid = false
	if t.editor.closed || t.editor.selected == "" {
		return
	}
	m, err := t.editor.session.WorldTransform(t.editor.selected)
	if err == nil {
		t.origin = m.Point(scene3d.Vec3{})
		t.valid = true
	}
}
func (t *sceneTranslationTool) handles() (scene3d.Vec3, [3]sceneTranslationHandle, bool) {
	var handles [3]sceneTranslationHandle
	v := t.editor.preview
	if !t.valid || t.editor.transformMode == SceneTransformNone || t.editor.closed || v.closed || !v.IsEnabled() {
		return scene3d.Vec3{}, handles, false
	}
	w, h := v.Size()
	camera := v.camera()
	origin := t.origin.Add(t.delta)
	pixel, visible, err := camera.Project(origin, w, h)
	if err != nil || !visible {
		return pixel, handles, false
	}
	depth := origin.Sub(camera.Eye).Dot(camera.Target.Sub(camera.Eye).Unit())
	length := depth * 2 * math.Tan(camera.FOVY/2) * 64 / h
	for i, axis := range sceneTranslationAxes {
		end, _, err := camera.Project(origin.Add(axis.Mul(length)), w, h)
		if err == nil && end.Z >= 0 && end.Z <= 1 && math.Hypot(end.X-pixel.X, end.Y-pixel.Y) >= 12 {
			handles[i] = sceneTranslationHandle{end: end, valid: true}
		}
	}
	return pixel, handles, true
}
func (t *sceneTranslationTool) draw(g paint.Painter) {
	if t.editor.transformMode == SceneTransformRotate {
		t.drawRotation(g)
		return
	}
	origin, handles, ok := t.handles()
	if !ok {
		return
	}
	g.SetFont(Theme().Font)
	for i, handle := range handles {
		if !handle.valid {
			continue
		}
		end := handle.end
		g.SetPen1(paint.Color{12, 18, 27, 255}, 5)
		g.Line(origin.X, origin.Y, end.X, end.Y)
		g.Stroke()
		color := sceneTranslationColors[i]
		if t.gesture != nil && t.axis == i {
			color = paint.Color{255, 224, 120, 255}
		}
		g.SetPen1(color, 2)
		g.Line(origin.X, origin.Y, end.X, end.Y)
		g.Stroke()
		g.SetBrush1(color)
		if t.editor.transformMode == SceneTransformScale {
			g.Rectangle(end.X-4, end.Y-4, 8, 8)
		} else {
			g.Arc(end.X, end.Y, 4, 0, 2*math.Pi)
		}
		g.Fill()
		g.DrawText1(end.X+7, end.Y-5, []string{"X", "Y", "Z"}[i])
	}
	g.SetBrush1(paint.Color{235, 242, 250, 255})
	if t.editor.transformMode == SceneTransformScale {
		g.Rectangle(origin.X-5, origin.Y-5, 10, 10)
	} else {
		g.Arc(origin.X, origin.Y, 3, 0, 2*math.Pi)
	}
	g.Fill()
}
func (t *sceneTranslationTool) begin(x, y float64) bool {
	if t.editor.transformMode == SceneTransformRotate || t.editor.transformMode == SceneTransformScale {
		return t.beginBasis(x, y)
	}
	origin, handles, ok := t.handles()
	v := t.editor.preview
	if !ok || x < 0 || y < 0 || x >= v.Width() || y >= v.Height() || math.Hypot(x-origin.X, y-origin.Y) < 9 {
		return false
	}
	best, axis := 7.0, -1
	for i, handle := range handles {
		if !handle.valid {
			continue
		}
		dx, dy := handle.end.X-origin.X, handle.end.Y-origin.Y
		u := max(0, min(1, ((x-origin.X)*dx+(y-origin.Y)*dy)/(dx*dx+dy*dy)))
		distance := math.Hypot(x-(origin.X+u*dx), y-(origin.Y+u*dy))
		if distance < best {
			best, axis = distance, i
		}
	}
	if axis < 0 {
		return false
	}
	camera := v.camera()
	ray, err := camera.Ray(x, y, v.Width(), v.Height())
	if err != nil {
		return true // Hit a handle, never fall through to orbit on an error.
	}
	solver, err := scene3d.NewAxisDrag(ray, t.origin, sceneTranslationAxes[axis])
	if err != nil {
		return true
	}
	gesture, err := t.editor.session.BeginTranslation(context.Background(), t.editor.selected)
	if err != nil {
		t.editor.report(err)
		return true
	}
	node, _ := v.scene.Node(t.editor.selected)
	t.gesture, t.solver, t.axis = gesture, solver, axis
	t.id, t.original, t.previewScene = t.editor.selected, node.Transform, v.scene
	t.camera, t.width, t.height = camera, v.Width(), v.Height()
	t.startX, t.startY, t.moved = x, y, false
	t.delta = scene3d.Vec3{}
	v.PushCapture()
	v.RefreshScene()
	return true
}
func (t *sceneTranslationTool) move(x, y float64) {
	v := t.editor.preview
	if t.gesture == nil {
		return
	}
	if !v.HasCapture() || !v.IsEnabled() || v.closed || v.scene != t.previewScene || v.camera() != t.camera || v.Width() != t.width || v.Height() != t.height || !finiteSceneValue(x) || !finiteSceneValue(y) {
		t.cancel()
		return
	}
	if math.Hypot(x-t.startX, y-t.startY) > 3 {
		t.moved = true
	}
	if !t.moved {
		return
	}
	if _, ok := t.gesture.(*scene3d.TransformEdit); ok {
		t.moveBasis(x, y)
		return
	}
	ray, err := t.camera.Ray(x, y, t.width, t.height)
	step := t.editor.translationSnapStep
	if IsKeyDown(KeyShift) {
		step = 1
	}
	var delta scene3d.Vec3
	if err == nil {
		delta, err = t.solver.Offset(ray, step)
	}
	var matrix scene3d.Mat4
	if err == nil {
		matrix, err = t.gesture.(*scene3d.TranslationEdit).Transform(delta)
	}
	if err == nil {
		err = v.SetNodeTransform(t.id, matrix)
	}
	if err != nil {
		t.cancel()
		return
	}
	t.delta = delta
}
func (t *sceneTranslationTool) finish(x, y float64) {
	t.move(x, y) // Release may arrive without a final motion event.
	if t.gesture == nil {
		return
	}
	v := t.editor.preview
	gesture, delta, id, original := t.gesture, t.delta, t.id, t.original
	t.gesture, t.previewScene = nil, nil // Retire before capture/scene callbacks.
	t.delta = scene3d.Vec3{}
	v.PopCapture()
	_ = v.SetNodeTransform(id, original)
	var err error
	switch g := gesture.(type) {
	case *scene3d.TranslationEdit:
		err = g.Commit(context.Background(), delta)
	case *scene3d.TransformEdit:
		err = g.Commit(context.Background(), t.operation)
	}
	gesture.Cancel()
	if err != nil {
		t.editor.report(err)
	}
	t.editor.syncSession(id)
	t.sync()
	v.RefreshScene()
}
func (t *sceneTranslationTool) cancel() {
	if t.gesture == nil {
		return
	}
	v := t.editor.preview
	gesture, scene, id, original := t.gesture, t.previewScene, t.id, t.original
	t.gesture, t.previewScene = nil, nil
	t.delta = scene3d.Vec3{}
	gesture.Cancel()
	v.PopCapture()
	if !v.closed && v.scene == scene {
		_ = v.SetNodeTransform(id, original)
		v.RefreshScene()
	}
}

// HandleEscape lets a captured drag consume one Escape before its parent modal
// closes. Without an active gesture the ordinary popup/dialog policy is retained.
func (v *SceneView) HandleEscape(repeat bool) bool {
	if v.translation == nil || v.translation.gesture == nil {
		return false
	}
	if !repeat {
		v.translation.cancel()
	}
	return true
}
func (v *SceneView) OnFocusChanged(newFocus, oldFocus IWidget) {
	if newFocus != v.Self() {
		v.OnHide()
	}
	v.Widget.OnFocusChanged(newFocus, oldFocus)
}
func (v *SceneView) SetEnabled(enabled bool) {
	if !enabled {
		v.OnHide()
	}
	v.Widget.SetEnabled(enabled)
}

// Resizing changes the projection used by a gesture; cancel rather than jump.
func (v *SceneView) OnResize() {
	v.OnHide()
	v.Widget.OnResize()
}
