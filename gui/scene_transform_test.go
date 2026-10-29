package gui

import (
	"errors"
	"github.com/uk0/silk/async"
	"github.com/uk0/silk/paint"
	"github.com/uk0/silk/scene3d"
	"math"
	"reflect"
	"testing"
)

func basisStart(t *testing.T, e *SceneDocumentEditor, mode SceneTransformMode, axis int) (x, y, nextX, nextY float64) {
	t.Helper()
	mustAuthor(t, e.SetTransformMode(mode))
	v := e.preview
	tool := v.translation
	if mode == SceneTransformRotate {
		cache := tool.rotationGeometry()
		if cache == nil {
			t.Fatal("no rings")
		}
		for i := 2; i < 50; i++ {
			if !cache.visible[axis][i] || !cache.visible[axis][i+6] {
				continue
			}
			p, next := cache.points[axis][i], cache.points[axis][i+6]
			if p.X < 0 || p.Y < 0 || p.X >= v.Width() || p.Y >= v.Height() {
				continue
			}
			v.OnLeftDown(p.X, p.Y)
			if tool.gesture != nil && tool.axis == axis {
				return p.X, p.Y, next.X, next.Y
			}
			v.OnHide()
		}
		t.Fatal("no selectable rotation ring", axis)
	}
	origin, handles, ok := tool.handles()
	if !ok {
		t.Fatal("no scale handles")
	}
	if axis == 3 {
		x, y = origin.X, origin.Y
		nextX, nextY = x+40, y-20
	} else {
		h := handles[axis]
		if !h.valid {
			t.Fatal("no axis handle")
		}
		dx, dy := h.end.X-origin.X, h.end.Y-origin.Y
		x, y = origin.X+dx*.75, origin.Y+dy*.75
		nextX, nextY = x+dx*.7, y+dy*.7
	}
	v.OnLeftDown(x, y)
	if tool.gesture == nil || tool.axis != axis || !v.HasCapture() || v.dragging {
		t.Fatal("scale did not capture", axis, tool.axis)
	}
	return
}

func TestSceneTransformRotateScaleSingleCommitAndExactUndo(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode SceneTransformMode
		axis int
	}{{"rotate", SceneTransformRotate, 2}, {"scale-x", SceneTransformScale, 0}, {"uniform-scale", SceneTransformScale, 3}} {
		t.Run(tc.name, func(t *testing.T) {
			e := translationEditor(t)
			original, _ := e.Document()
			before, _ := e.session.WorldTransform(e.selected)
			camera := e.preview.camera()
			mustAuthor(t, e.SetRotationSnapStep(math.Pi/12))
			mustAuthor(t, e.SetScaleSnapRatio(1.1))
			_, _, x, y := basisStart(t, e, tc.mode, tc.axis)
			v := e.preview
			v.OnMouseMove(x, y)
			if v.translation.gesture == nil {
				t.Fatal("gesture canceled")
			}
			op := v.translation.operation
			if op == scene3d.Identity() {
				t.Fatal("no transform preview")
			}
			during, _ := e.Document()
			if !reflect.DeepEqual(original, during) || e.Modified() || e.CanUndo() {
				t.Fatal("preview changed authored state")
			}
			if tc.mode == SceneTransformScale {
				f := op[0]
				if tc.axis == 3 && (op[5] != f || op[10] != f) {
					t.Fatal("not uniform")
				}
				q := math.Log(f) / math.Log(1.1)
				if math.Abs(q-math.Round(q)) > 1e-10 {
					t.Fatal("scale snapping", f)
				}
			}
			v.OnLeftUp(x, y)
			if v.HasCapture() || v.translation.gesture != nil || e.revision != 1 {
				t.Fatal("gesture not retired or multiple commits")
			}
			world, _ := e.session.WorldTransform(e.selected)
			if world[12] != before[12] || world[13] != before[13] || world[14] != before[14] || v.camera() != camera {
				t.Fatal("origin/camera changed")
			}
			after, _ := e.Document()
			if reflect.DeepEqual(original, after) || !reflect.DeepEqual(original.Meshes, after.Meshes) || !reflect.DeepEqual(original.Bindings, after.Bindings) {
				t.Fatal("wrong authored result")
			}
			mustAuthor(t, e.Undo())
			undo, _ := e.Document()
			if !reflect.DeepEqual(original, undo) {
				t.Fatal("undo not exact")
			}
			mustAuthor(t, e.Redo())
			redo, _ := e.Document()
			if !reflect.DeepEqual(after, redo) {
				t.Fatal("redo not exact")
			}
			rebuilt, err := scene3d.NewEditSession(t.Context(), after)
			mustAuthor(t, err)
			saved, _ := rebuilt.Document()
			rebuilt.Close()
			if !reflect.DeepEqual(after, saved) {
				t.Fatal("roundtrip changed transform")
			}
		})
	}
}

func TestSceneTransformCancellationModeAndStaleEdits(t *testing.T) {
	for _, mode := range []SceneTransformMode{SceneTransformRotate, SceneTransformScale} {
		for _, reason := range []string{"escape", "focus", "capture", "resize", "camera", "hide", "disable", "mode", "snap", "invalid-pointer", "stale"} {
			t.Run(string(rune('0'+mode))+"/"+reason, func(t *testing.T) {
				e := translationEditor(t)
				before, _ := e.Document()
				axis := 2
				if mode == SceneTransformScale {
					axis = 0
				}
				_, _, x, y := basisStart(t, e, mode, axis)
				e.preview.OnMouseMove(x, y)
				v := e.preview
				switch reason {
				case "escape":
					if !v.HandleEscape(false) {
						t.Fatal("escape not consumed")
					}
				case "focus":
					v.OnFocusChanged(NewEdit(), v)
				case "capture":
					v.OnPointerCancel()
				case "resize":
					v.SetSize(v.Width()+1, v.Height())
				case "camera":
					v.SetYaw(v.Yaw() + .2)
				case "hide":
					v.OnHide()
				case "disable":
					v.SetEnabled(false)
				case "mode":
					mustAuthor(t, e.SetTransformMode(SceneTransformMove))
				case "snap":
					mustAuthor(t, e.SetRotationSnapStep(.2))
				case "invalid-pointer":
					v.OnMouseMove(math.NaN(), 0)
				case "stale":
					n := e.session.Nodes()[0]
					n.Name = "external edit"
					mustAuthor(t, e.session.UpdateNode(t.Context(), n))
					v.OnMouseMove(x+2, y+2)
				}
				if v.translation.gesture != nil || v.HasCapture() {
					t.Fatal("gesture not canceled")
				}
				if reason != "stale" {
					after, _ := e.Document()
					if !reflect.DeepEqual(before, after) || e.CanUndo() {
						t.Fatal("canceled edit committed")
					}
					n, _ := v.scene.Node(e.selected)
					var want scene3d.Mat4
					for _, node := range before.Nodes {
						if node.ID == e.selected {
							copy(want[:], node.Transform)
						}
					}
					if n.Transform != want {
						t.Fatal("preview not restored")
					}
				}
			})
		}
	}
}

func TestSceneTransformActionsNoopAndOverlayCache(t *testing.T) {
	e := translationEditor(t)
	v := e.preview
	paintScene(t, v)
	count := v.RenderCount()
	for i, mode := range []SceneTransformMode{SceneTransformRotate, SceneTransformScale, SceneTransformMove} {
		index := []int{8, 9, 7}[i]
		e.buttons[index].Action().Trigger(nil)
		if e.TransformMode() != mode || !e.buttons[index].Action().IsChecked() || e.buttons[index].Icon() == LoadIcon("image-missing") {
			t.Fatal("mode action", mode)
		}
		for j := 7; j < 10; j++ {
			if e.buttons[j].Action().IsChecked() != (j == index) {
				t.Fatal("not exclusive")
			}
		}
		paintScene(t, v)
		if v.RenderCount() != count {
			t.Fatal("tool-only change rerasterized geometry")
		}
	}
	for _, mode := range []SceneTransformMode{SceneTransformRotate, SceneTransformScale} {
		axis := 2
		if mode == SceneTransformScale {
			axis = 3
		}
		x, y, _, _ := basisStart(t, e, mode, axis)
		v.OnLeftUp(x, y)
		if e.Modified() || e.CanUndo() {
			t.Fatal("click committed")
		}
	}
	mustAuthor(t, e.SetTransformMode(SceneTransformRotate))
	paintScene(t, v)
	builds := v.translation.rings.builds
	for range 10 {
		paintScene(t, v)
	}
	if v.translation.rings.builds != builds {
		t.Fatal("idle rebuilt ring projections")
	}
	for _, step := range []float64{-1, math.NaN(), math.Inf(1)} {
		if e.SetRotationSnapStep(step) == nil {
			t.Fatal("invalid rotation step")
		}
	}
	for _, ratio := range []float64{-1, 1, .5, math.NaN(), math.Inf(1)} {
		if e.SetScaleSnapRatio(ratio) == nil {
			t.Fatal("invalid scale ratio")
		}
	}
	if e.SetTransformMode(255) == nil || e.TransformMode() != SceneTransformRotate {
		t.Fatal("invalid mode changed state")
	}
	mustAuthor(t, e.SetTransformMode(SceneTransformScale))
	e.SetTranslationEnabled(false)
	if e.TransformMode() != SceneTransformScale {
		t.Fatal("translation disable changed other tool")
	}
	e.Close()
	for _, err := range []error{e.SetTransformMode(SceneTransformMove), e.SetRotationSnapStep(0), e.SetScaleSnapRatio(0)} {
		if !errors.Is(err, async.ErrClosed) {
			t.Fatal(err)
		}
	}
}

func TestSceneTransformHiDPIRingsAndScaleBounds(t *testing.T) {
	for _, mode := range []SceneTransformMode{SceneTransformRotate, SceneTransformScale} {
		e := translationEditor(t)
		mustAuthor(t, e.SetTransformMode(mode))
		e.preview.SetSize(180, 160)
		for _, scale := range []float64{1, 1.5, 2} {
			pm := paint.NewPixmap(440, 400)
			g := pm.NewPainter()
			g.SetBrush1(paint.Color{213, 21, 43, 255})
			g.Paint()
			g.Scale(scale, scale)
			g.Translate(20, 20)
			e.preview.Draw(g)
			im, err := pm.Image()
			mustAuthor(t, err)
			for _, p := range [][2]int{{1, 1}, {435, 395}} {
				r, green, b, _ := im.At(p[0], p[1]).RGBA()
				if r>>8 != 213 || green>>8 != 21 || b>>8 != 43 {
					t.Fatal("overlay escaped clip")
				}
			}
			paint.ClosePainter(g)
			paint.CloseSurface(pm)
		}
		e.Close()
	}
}
