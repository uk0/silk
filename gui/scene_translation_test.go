package gui

import (
	"context"
	"image/color"
	"math"
	"reflect"
	"testing"

	"github.com/uk0/silk/paint"
)

func translationEditor(t *testing.T) *SceneDocumentEditor {
	t.Helper()
	oldFocus, oldCapture := focusWidget, append([]IWidget(nil), captureStack...)
	t.Cleanup(func() { focusWidget, captureStack = oldFocus, oldCapture })
	e := authoringEditor(t)
	e.SetSize(960, 560)
	e.Layout()
	if !e.SelectNode("tank") {
		t.Fatal("fixture selection")
	}
	e.SetTranslationEnabled(true)
	return e
}
func translationStart(t *testing.T, e *SceneDocumentEditor) (float64, float64) {
	t.Helper()
	origin, handles, ok := e.preview.translation.handles()
	if !ok || !handles[0].valid {
		t.Fatal("missing X handle", origin, handles)
	}
	end := handles[0].end
	x, y := origin.X+(end.X-origin.X)*.8, origin.Y+(end.Y-origin.Y)*.8
	e.preview.OnLeftDown(x, y)
	if e.preview.translation.gesture == nil || !e.preview.HasCapture() || e.preview.dragging {
		t.Fatal("handle did not capture translation independently of orbit")
	}
	return x, y
}
func TestSceneTranslationSingleCommitPreviewHistoryAndBindings(t *testing.T) {
	e := translationEditor(t)
	original, _ := e.Document()
	oldWorld, _ := e.session.WorldTransform("tank")
	oldCamera := e.preview.camera()
	x, y := translationStart(t, e)
	for i := 0; i < 20; i++ {
		e.preview.OnMouseMove(x+float64(i)*2, y)
	}
	delta := e.preview.translation.delta
	if delta.X == 0 || delta.Y != 0 || delta.Z != 0 || e.preview.camera() != oldCamera {
		t.Fatal("drag changed camera/wrong axis", delta)
	}
	preview, _ := e.scene.Node("tank")
	unchanged, _ := e.Document()
	if !reflect.DeepEqual(original, unchanged) || e.Modified() || e.CanUndo() || e.revision != 0 {
		t.Fatal("mousemove changed authored document/history")
	}
	e.preview.OnLeftUp(x+38, y)
	after, _ := e.Document()
	world, _ := e.session.WorldTransform("tank")
	if math.Abs(world[12]-oldWorld[12]-delta.X) > 1e-9 || e.preview.HasCapture() || e.preview.translation.gesture != nil {
		t.Fatal("release failed to publish/retire", world, oldWorld, delta)
	}
	if count, _ := e.session.HistoryUsage(); count != 1 || e.revision != 1 {
		t.Fatal("more than one undo entry", count, e.revision)
	}
	if !reflect.DeepEqual(after.Bindings, original.Bindings) || !reflect.DeepEqual(after.Meshes, original.Meshes) {
		t.Fatal("moving activated/changed bindings or geometry")
	}
	node, _ := e.scene.Node("tank")
	if node.Transform != preview.Transform {
		t.Fatal("committed transform differs from preview")
	}
	mustAuthor(t, e.Undo())
	restored, _ := e.Document()
	if e.Modified() || !reflect.DeepEqual(restored, original) {
		t.Fatal("undo not exact")
	}
	mustAuthor(t, e.Redo())
	redone, _ := e.Document()
	if !reflect.DeepEqual(after, redone) {
		t.Fatal("redo not exact")
	}
	built, err := redone.Build(context.Background())
	mustAuthor(t, err)
	snap, err := built.Snapshot()
	mustAuthor(t, err)
	if snap.InstanceCount() != 1 {
		t.Fatal("committed authored document not rebuildable")
	}
}
func TestSceneTranslationCancelAllLifecyclePaths(t *testing.T) {
	for _, mode := range []string{"escape", "hide", "disable", "focus", "capture", "window-focus", "resize", "camera", "wheel", "invalid", "mode", "selection", "detach"} {
		t.Run(mode, func(t *testing.T) {
			e := translationEditor(t)
			original, _ := e.Document()
			base, _ := e.scene.Node("tank")
			x, y := translationStart(t, e)
			e.preview.OnMouseMove(x+40, y)
			switch mode {
			case "escape":
				if !e.preview.HandleEscape(false) {
					t.Fatal("Escape not consumed")
				}
			case "hide":
				e.preview.Hide()
			case "disable":
				e.preview.SetEnabled(false)
			case "focus":
				NewEdit().SetFocus()
			case "capture":
				NewButton().PushCapture()
			case "window-focus":
				e.preview.OnPointerCancel()
			case "resize":
				e.preview.SetSize(100, 100)
			case "camera":
				e.preview.SetYaw(e.preview.Yaw() + .2)
			case "wheel":
				e.preview.OnMouseWheel(x, y, 1)
			case "invalid":
				e.preview.OnMouseMove(math.NaN(), y)
			case "mode":
				e.SetTranslationEnabled(false)
			case "selection":
				e.SelectNode("assembly")
			case "detach":
				e.preview.Detach()
			}
			e.preview.OnLeftUp(x+40, y)
			after, _ := e.Document()
			n, _ := e.scene.Node("tank")
			if !reflect.DeepEqual(original, after) || e.CanUndo() || e.Modified() || n.Transform != base.Transform || e.preview.translation.gesture != nil {
				t.Fatal("cancel committed or retained preview", mode)
			}
		})
	}
}
func TestSceneTranslationNoopSnapNoTargetAndCachedDraw(t *testing.T) {
	e := translationEditor(t)
	original, _ := e.Document()
	x, y := translationStart(t, e)
	e.preview.OnLeftUp(x+1, y+1)
	if e.Modified() || e.CanUndo() {
		t.Fatal("click jitter committed")
	}
	mustAuthor(t, e.SetTranslationSnapStep(.5))
	x, y = translationStart(t, e)
	e.preview.OnMouseMove(x+55, y)
	delta := e.preview.translation.delta.X
	if delta == 0 || math.Abs(delta/.5-math.Round(delta/.5)) > 1e-12 {
		t.Fatal("world snapping", delta)
	}
	e.preview.HandleEscape(false)
	after, _ := e.Document()
	if !reflect.DeepEqual(original, after) {
		t.Fatal("snap cancel changed document")
	}
	before := paintScene(t, e.preview)
	_ = before
	renders := e.preview.RenderCount()
	paintScene(t, e.preview)
	if e.preview.RenderCount() != renders {
		t.Fatal("idle gizmo rerasterized scene")
	}
	e.buttons[7].Action().Trigger(nil)
	if e.TranslationEnabled() || e.buttons[7].Action().IsChecked() {
		t.Fatal("mode button not toggled")
	}
	e.buttons[7].Action().Trigger(nil)
	if !e.TranslationEnabled() || !e.buttons[7].Action().IsChecked() {
		t.Fatal("mode action state")
	}
	if e.buttons[7].Icon() == LoadIcon("image-missing") {
		t.Fatal("missing move icon")
	}
	mustAuthor(t, e.RemoveSubtree("assembly"))
	if _, _, ok := e.preview.translation.handles(); ok || e.buttons[7].IsEnabled() {
		t.Fatal("empty scene has gizmo")
	}
}
func TestSceneTranslationHiDPIClipsAndCaches(t *testing.T) {
	for _, scale := range []float64{1, 1.5, 2} {
		e := translationEditor(t)
		v := e.preview
		v.SetSize(200, 160)
		pm := paint.NewPixmap(500, 400)
		g := pm.NewPainter()
		g.SetBrush1(paint.Color{219, 13, 201, 255})
		g.Paint()
		g.Scale(scale, scale)
		g.Translate(10, 10)
		v.Draw(g)
		renders := v.RenderCount()
		v.Draw(g)
		if v.RenderCount() != renders || v.pixmap.Width() != int(200*scale) {
			t.Fatal("gizmo invalidated physical render cache")
		}
		im, err := pm.Image()
		mustAuthor(t, err)
		for _, pt := range [][2]int{{0, 0}, {499, 399}, {499, 50}, {50, 399}} {
			if got := color.RGBAModel.Convert(im.At(pt[0], pt[1])).(color.RGBA); got != (color.RGBA{219, 13, 201, 255}) {
				t.Fatal("gizmo escaped viewport", scale, pt, got)
			}
		}
		paint.ClosePainter(g)
		paint.CloseSurface(pm)
		e.Close()
	}
}
func TestSceneTranslationEscapeBeforeDialogCancelAndOwnerScope(t *testing.T) {
	e := translationEditor(t)
	dialog := NewDialog("scene", nil)
	dialog.SetContent(e)
	dialog.SetSize(1016, 700)
	dialog.setVisible(true)
	win := &Window{widget: dialog, wt: WtForm}
	dialog.win = win
	t.Cleanup(func() { dialog.win = nil })
	x, y := translationStart(t, e)
	e.preview.OnMouseMove(x+30, y)
	if !win.dispatchEscape(KeyEsc, false) || dialog.closed || e.preview.translation.gesture != nil {
		t.Fatal("first Escape closed modal or retained drag")
	}
	if !win.dispatchEscape(KeyEsc, false) || !dialog.closed {
		t.Fatal("second Escape did not cancel modal")
	}
}

func TestSceneTranslationNativeOwnerCancellationAndStaleEdits(t *testing.T) {
	e := translationEditor(t)
	owner := NewLabel("owned surface")
	ownerWindow := &Window{widget: owner, wt: WtForm}
	owner.win = ownerWindow
	e.SetParent(owner)
	e.SetSize(960, 560)
	e.Layout()
	t.Cleanup(func() { owner.win = nil })
	other := NewLabel("independent surface")
	otherWindow := &Window{widget: other, wt: WtForm}
	other.win = otherWindow
	t.Cleanup(func() { other.win = nil })
	original, _ := e.Document()
	x, y := translationStart(t, e)
	e.preview.OnMouseMove(x+35, y)
	otherWindow.cancelPointerGestures()
	if e.preview.translation.gesture == nil {
		t.Fatal("independent window canceled this drag")
	}
	ownerWindow.cancelPointerGestures()
	if e.preview.translation.gesture != nil || e.preview.HasCapture() {
		t.Fatal("native focus loss retained gesture/capture")
	}
	after, _ := e.Document()
	if !reflect.DeepEqual(original, after) || e.Modified() {
		t.Fatal("native cancellation committed")
	}
	x, y = translationStart(t, e)
	e.preview.OnMouseMove(x+35, y)
	node := e.document.Nodes[1]
	node.Name = "external authored edit"
	mustAuthor(t, e.UpdateNode(node))
	e.preview.OnLeftUp(x+60, y)
	after, _ = e.Document()
	if e.preview.translation.gesture != nil || e.revision != 1 || !reflect.DeepEqual(after.Nodes[1].Transform, original.Nodes[1].Transform) || after.Nodes[1].Name != node.Name {
		t.Fatal("stale release overwrote another edit")
	}
}
