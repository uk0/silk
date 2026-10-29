package gui

import (
	"context"
	"github.com/uk0/silk/async"
	"github.com/uk0/silk/paint"
	"github.com/uk0/silk/scene3d"
	"math"
)

// SceneTransformMode selects one mutually exclusive embedded manipulator.
type SceneTransformMode uint8

const (
	SceneTransformNone SceneTransformMode = iota
	SceneTransformMove
	SceneTransformRotate
	SceneTransformScale
)

func (e *SceneDocumentEditor) TransformMode() SceneTransformMode { return e.transformMode }
func (e *SceneDocumentEditor) SetTransformMode(mode SceneTransformMode) error {
	if e.closed {
		return async.ErrClosed
	}
	if mode > SceneTransformScale {
		return scene3d.ErrGeometry
	}
	if mode == e.transformMode {
		return nil
	}
	e.preview.OnHide()
	e.transformMode = mode
	e.preview.translation.sync()
	e.syncActions()
	e.preview.Update() // overlay only; selecting a tool must not rerasterize geometry
	return nil
}
func (e *SceneDocumentEditor) toggleTransformMode(mode SceneTransformMode) {
	if e.transformMode == mode {
		mode = SceneTransformNone
	}
	e.report(e.SetTransformMode(mode))
}

// RotationSnapStep uses radians (0 = free). Shift temporarily uses pi/12.
func (e *SceneDocumentEditor) RotationSnapStep() float64 { return e.rotationSnapStep }
func (e *SceneDocumentEditor) SetRotationSnapStep(step float64) error {
	if e.closed {
		return async.ErrClosed
	}
	if !finiteSceneValue(step) || step < 0 {
		return scene3d.ErrGeometry
	}
	if step != e.rotationSnapStep {
		e.preview.OnHide()
		e.rotationSnapStep = step
	}
	return nil
}

// ScaleSnapRatio is multiplicative (0 = free, otherwise > 1). Shift uses 1.1.
func (e *SceneDocumentEditor) ScaleSnapRatio() float64 { return e.scaleSnapRatio }
func (e *SceneDocumentEditor) SetScaleSnapRatio(ratio float64) error {
	if e.closed {
		return async.ErrClosed
	}
	if !finiteSceneValue(ratio) || (ratio != 0 && ratio <= 1) {
		return scene3d.ErrGeometry
	}
	if ratio != e.scaleSnapRatio {
		e.preview.OnHide()
		e.scaleSnapRatio = ratio
	}
	return nil
}

type sceneRotationCache struct {
	camera        scene3d.Camera
	origin        scene3d.Vec3
	width, height float64
	points        [3][65]scene3d.Vec3
	visible       [3][65]bool
	ready         bool
	builds        uint64
}

func (t *sceneTranslationTool) rotationGeometry() *sceneRotationCache {
	pixel, _, ok := t.handles()
	if !ok {
		return nil
	}
	v := t.editor.preview
	c := v.camera()
	w, h := v.Size()
	cache := &t.rings
	if cache.ready && cache.camera == c && cache.origin == t.origin && cache.width == w && cache.height == h {
		return cache
	}
	cache.ready = true
	cache.camera, cache.origin, cache.width, cache.height = c, t.origin, w, h
	cache.builds++
	cache.visible = [3][65]bool{}
	depth := t.origin.Sub(c.Eye).Dot(c.Target.Sub(c.Eye).Unit())
	radius := depth * 2 * math.Tan(c.FOVY/2) * 60 / h
	ray, err := c.Ray(pixel.X, pixel.Y, w, h)
	if err != nil {
		return nil
	}
	for axis, n := range sceneTranslationAxes {
		// An edge-on ring has no well-conditioned plane intersection.
		if math.Abs(ray.Direction.Dot(n)) < .08 {
			continue
		}
		u := sceneTranslationAxes[(axis+1)%3]
		b := n.Cross(u)
		for i := 0; i <= 64; i++ {
			a := float64(i) * 2 * math.Pi / 64
			p := t.origin.Add(u.Mul(math.Cos(a) * radius)).Add(b.Mul(math.Sin(a) * radius))
			screen, _, err := c.Project(p, w, h)
			cache.points[axis][i] = screen
			cache.visible[axis][i] = err == nil && screen.Z >= 0 && screen.Z <= 1 && p.Sub(c.Eye).Dot(c.Target.Sub(c.Eye)) > 0
		}
	}
	return cache
}
func sceneSegmentDistance(x, y float64, a, b scene3d.Vec3) float64 {
	dx, dy := b.X-a.X, b.Y-a.Y
	den := dx*dx + dy*dy
	if den == 0 {
		return math.Hypot(x-a.X, y-a.Y)
	}
	u := max(0, min(1, ((x-a.X)*dx+(y-a.Y)*dy)/den))
	return math.Hypot(x-a.X-u*dx, y-a.Y-u*dy)
}
func (t *sceneTranslationTool) drawRotation(g paint.Painter) {
	cache := t.rotationGeometry()
	if cache == nil {
		return
	}
	g.SetFont(Theme().Font)
	for axis, points := range cache.points {
		color := sceneTranslationColors[axis]
		if t.gesture != nil && t.axis == axis {
			color = paint.Color{255, 224, 120, 255}
		}
		for _, width := range []float64{5, 2} {
			c := color
			if width == 5 {
				c = paint.Color{12, 18, 27, 255}
			}
			g.SetPen1(c, width)
			for i := 1; i < len(points); i++ {
				if cache.visible[axis][i-1] && cache.visible[axis][i] {
					g.Line(points[i-1].X, points[i-1].Y, points[i].X, points[i].Y)
				}
			}
			g.Stroke()
		}
		if cache.visible[axis][8] {
			g.SetBrush1(color)
			g.DrawText1(points[8].X+5, points[8].Y-5, []string{"X", "Y", "Z"}[axis])
		}
	}
}
func (t *sceneTranslationTool) beginBasis(x, y float64) bool {
	v := t.editor.preview
	origin, handles, ok := t.handles()
	if !ok || x < 0 || y < 0 || x >= v.Width() || y >= v.Height() {
		return false
	}
	axis := -1
	best := 7.0
	mode := t.editor.transformMode
	if mode == SceneTransformRotate {
		cache := t.rotationGeometry()
		if cache == nil {
			return false
		}
		for a, points := range cache.points {
			for i := 1; i < len(points); i++ {
				if cache.visible[a][i-1] && cache.visible[a][i] {
					d := sceneSegmentDistance(x, y, points[i-1], points[i])
					if d < best {
						best, axis = d, a
					}
				}
			}
		}
	} else {
		if math.Abs(x-origin.X) <= 7 && math.Abs(y-origin.Y) <= 7 {
			axis = 3
		} else {
			for a, h := range handles {
				if h.valid {
					d := sceneSegmentDistance(x, y, origin, h.end)
					if d < best {
						best, axis = d, a
					}
				}
			}
		}
	}
	if axis < 0 {
		return false
	}
	camera := v.camera()
	ray, err := camera.Ray(x, y, v.Width(), v.Height())
	if err != nil {
		return true
	}
	if mode == SceneTransformRotate {
		t.rotationSolver, err = scene3d.NewRotationDrag(ray, t.origin, sceneTranslationAxes[axis])
	} else if axis < 3 {
		t.solver, err = scene3d.NewAxisDrag(ray, t.origin, sceneTranslationAxes[axis])
	}
	if err != nil {
		return true
	}
	gesture, err := t.editor.session.BeginTransform(context.Background(), t.editor.selected)
	if err != nil {
		t.editor.report(err)
		return true
	}
	node, _ := v.scene.Node(t.editor.selected)
	t.gesture, t.axis = gesture, axis
	t.id, t.original, t.previewScene = t.editor.selected, node.Transform, v.scene
	t.camera, t.width, t.height = camera, v.Width(), v.Height()
	t.startX, t.startY, t.moved = x, y, false
	t.operation = scene3d.Identity()
	t.rawAngle = 0
	t.delta = scene3d.Vec3{}
	depth := t.origin.Sub(camera.Eye).Dot(camera.Target.Sub(camera.Eye).Unit())
	t.handleLength = depth * 2 * math.Tan(camera.FOVY/2) * 64 / t.height
	v.PushCapture()
	v.Update()
	return true
}
func (t *sceneTranslationTool) moveBasis(x, y float64) {
	var op scene3d.Mat4
	var err error
	if t.editor.transformMode == SceneTransformRotate {
		var ray scene3d.Ray
		ray, err = t.camera.Ray(x, y, t.width, t.height)
		angle := t.rawAngle
		if err == nil {
			angle, err = t.rotationSolver.Angle(ray, t.rawAngle)
		}
		if err == nil {
			t.rawAngle = angle
			step := t.editor.rotationSnapStep
			if IsKeyDown(KeyShift) {
				step = math.Pi / 12
			}
			if step > 0 {
				angle = math.Round(angle/step) * step
			}
			op, err = scene3d.RotationAxis(sceneTranslationAxes[t.axis], angle)
		}
	} else {
		exponent := (x - t.startX - (y - t.startY)) / 80
		if t.axis < 3 {
			var ray scene3d.Ray
			ray, err = t.camera.Ray(x, y, t.width, t.height)
			if err == nil {
				var offset scene3d.Vec3
				offset, err = t.solver.Offset(ray, 0)
				exponent = offset.Dot(sceneTranslationAxes[t.axis]) / t.handleLength
			}
		}
		// A single drag spans 1/1024..1024; repeat gestures for larger authored scales.
		factor := math.Exp2(max(-10, min(10, exponent)))
		ratio := t.editor.scaleSnapRatio
		if IsKeyDown(KeyShift) {
			ratio = 1.1
		}
		if ratio > 1 {
			factor = math.Pow(ratio, math.Round(math.Log(factor)/math.Log(ratio)))
		}
		f := scene3d.Vec3{X: 1, Y: 1, Z: 1}
		switch t.axis {
		case 0:
			f.X = factor
		case 1:
			f.Y = factor
		case 2:
			f.Z = factor
		case 3:
			f = scene3d.Vec3{X: factor, Y: factor, Z: factor}
		}
		op = scene3d.Scaling(f)
	}
	if err == nil {
		var m scene3d.Mat4
		m, err = t.gesture.(*scene3d.TransformEdit).Transform(op)
		if err == nil {
			err = t.editor.preview.SetNodeTransform(t.id, m)
		}
	}
	if err != nil {
		t.cancel()
		return
	}
	t.operation = op
}
