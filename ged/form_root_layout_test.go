package ged

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/uk0/silk/decl"
	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
)

func TestFormRootLayoutFillsCanvasAndRuntime(t *testing.T) {
	for _, kind := range []LayoutKind{LayoutHorizontal, LayoutVertical, LayoutGrid, LayoutForm} {
		t.Run(string(kind), func(t *testing.T) {
			v, _ := layoutFixture(t, kind)
			scene := v.GedScene()
			scene.SetSize(200, 140)
			v.Selection().Clear()
			v.Selection().Add(scene)
			if !v.LayoutSelection(kind) {
				t.Fatal("layout action must accept the selected form itself")
			}
			one := scene.Children()
			if len(one) != 1 {
				t.Fatalf("form layout owns all initial controls; roots = %d", len(one))
			}
			box := one[0].(*FakeWidget)
			if box.Parent() != scene {
				t.Fatal("form lost its layout")
			}
			scene.SetSize(240, 180)
			if want := (geom.Rect{Width: 240, Height: 180}); box.Bounds1() != want {
				t.Fatalf("canvas layout = %v, want %v", box.Bounds1(), want)
			}
			doc := scene.SaveDesign()
			loaded := NewGedScene()
			if err := loaded.LoadDesign(doc); err != nil {
				t.Fatal(err)
			}
			loaded.SetSize(210, 160)
			if got := loaded.Children()[0].Bounds1(); got != (geom.Rect{Width: 210, Height: 160}) {
				t.Fatalf("reopened managed layout = %v", got)
			}
			sdk, err := gui.LoadFormFromDoc(doc)
			if err != nil {
				t.Fatal(err)
			}
			for _, f := range []*gui.Form{scene.Generate().Form(), sdk} {
				f.SetSize(850, 610)
				r := f.Children()[0]
				if r.Parent() != f || r.Bounds1() != (geom.Rect{Width: 850, Height: 610}) {
					t.Fatalf("runtime container did not fill form after resize: %v", r.Bounds1())
				}
				f.SetSize(650, 405)
				if r.Bounds1() != (geom.Rect{Width: 650, Height: 405}) {
					t.Fatalf("runtime container failed to shrink: %v", r.Bounds1())
				}
			}
		})
	}
}

func TestFormRootLayoutUndoRestoresFreeGeometryAndLocks(t *testing.T) {
	v, _ := layoutFixture(t, LayoutVertical)
	scene := v.GedScene()
	outer := sceneWidget(t, scene, "gui.VBox", "nested", 28, 25, 40, 30)
	inner, err := NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatal(err)
	}
	inner.SetParent(outer)
	inner.SetBounds(33, 28, 18, 9)
	top := append([]graph.IItem(nil), scene.Children()...)
	beforeOuter, beforeInner := outer.Bounds1(), inner.Bounds1()
	v.Selection().Clear()
	v.Selection().Add(scene)
	inner.SetLockSize(true)
	if v.CanLayoutSelection(LayoutVertical) || v.LayoutSelection(LayoutVertical) || scene.UndoStack().CanUndo() {
		t.Fatal("applying form layout must reject a locked descendant without history")
	}
	inner.SetLockSize(false)
	if !v.LayoutSelection(LayoutVertical) {
		t.Fatal("form layout refused")
	}
	v.Undo()
	after := scene.Children()
	if len(after) != len(top) {
		t.Fatal("undo did not restore former form children")
	}
	for i, w := range after {
		if w != top[i] {
			t.Fatalf("undo changed stacking at %d", i)
		}
	}
	if outer.Bounds1() != beforeOuter || inner.Bounds1() != beforeInner || !v.Selection().Contains(scene) {
		t.Fatalf("undo lost free geometry or form selection: %v %v", outer.Bounds1(), inner.Bounds1())
	}
	if doc := scene.SaveDesign(); doc.ChildByKey("children", false).Childdren()[0].ChildByKey("layout_fill_form", false) != nil {
		t.Fatal("undo left form layout marker behind")
	}
	v.Redo()
	box := scene.Children()[0].(*FakeWidget)
	if !managedGeometryItem(box) || !v.CanBreakLayout() {
		t.Fatal("form-managed bounds must be read-only but layout must remain breakable")
	}
	if !v.BreakLayout() {
		t.Fatal("cannot break form layout")
	}
	if len(scene.Children()) != len(top) || managedGeometryItem(outer) {
		t.Fatal("break layout did not restore free ownership")
	}
	v.Undo()
	scene.SetSize(210, 153)
	if box.Parent() != scene || box.Width() != 210 || box.Height() != 153 {
		t.Fatal("undo break did not restore responsive layout")
	}
}

func TestFormRootLayoutEmptyAndDoesNotSerializeClipboardMarker(t *testing.T) {
	v := NewGedView()
	scene := v.GedScene()
	v.Selection().Add(scene)
	if !v.LayoutSelection(LayoutGrid) {
		t.Fatal("empty form can't start with a managed layout")
	}
	box := scene.Children()[0].(*FakeWidget)
	if box.SaveDesign().ChildByKey(decl.FormLayoutFillProp, false) != nil {
		t.Fatal("copying a layout widget must not copy the form's fill policy")
	}
	if doc := scene.SaveDesign(); doc.ChildByKey("children", false).Childdren()[0].ChildByKey(decl.FormLayoutFillProp, false) == nil {
		t.Fatal("full design save omitted root marker")
	}
	v.Selection().Clear()
	v.Selection().Add(scene)
	if !v.BreakLayout() || len(scene.Children()) != 0 {
		t.Fatal("could not break empty layout directly from the form")
	}
	v.Undo()
	if scene.activeFormLayout() != box || !v.Selection().Contains(scene) {
		t.Fatal("undo lost the former form attachment")
	}
}

func TestFormRootLayoutIgnoresUnsupportedFillMarkers(t *testing.T) {
	v := NewGedView()
	scene := v.GedScene()
	plain := sceneWidget(t, scene, "gui.Button", "plain", 10, 8, 20, 10)
	free := sceneWidget(t, scene, "gui.VBox", "free", 40, 8, 20, 17)
	doc := scene.SaveDesign()
	kids := doc.ChildByKey("children", false).Childdren()
	for _, doc := range kids {
		doc.WriteAttr(decl.FormLayoutFillProp, true)
	}
	loaded := NewGedScene()
	if err := loaded.LoadDesign(doc); err != nil {
		t.Fatal(err)
	}
	loaded.SetSize(220, 160)
	if loaded.activeFormLayout() != nil || loaded.Children()[0].Bounds1() != plain.Bounds1() || loaded.Children()[1].Bounds1() != free.Bounds1() {
		t.Fatal("marked free geometry was unexpectedly managed")
	}
	f, err := gui.LoadFormFromDoc(doc)
	if err != nil {
		t.Fatal(err)
	}
	bounds := f.Children()[0].Bounds1()
	f.SetSize(880, 620)
	if f.LayoutRoot() != nil || f.Children()[0].Bounds1() != bounds {
		t.Fatal("SDK took over ordinary or disabled geometry")
	}
}

func TestFormRootLayoutGeneratedGoRetainsClientResize(t *testing.T) {
	if testing.Short() {
		t.Skip("generated Go integration")
	}
	v, _ := layoutFixture(t, LayoutGrid)
	v.Selection().Clear()
	v.Selection().Add(v.GedScene())
	if !v.LayoutSelection(LayoutGrid) {
		t.Fatal("form layout refused")
	}
	d := t.TempDir()
	opts := CodeGenOptions{PackageName: "ui", TypeName: "RootUI"}
	files := map[string]string{
		"imperative.go":  v.GedScene().GenerateCode(opts),
		"declarative.go": v.GedScene().GenerateDeclCode(opts),
		"runtime_test.go": `package ui
import (
	"testing"
	"github.com/uk0/silk/gui"
)
func TestClientResize(t *testing.T) {
	imperative := NewRootUI()
	obj, index, err := BuildRootUI().BuildWithIndex()
	if err != nil { t.Fatal(err) }
	for i, form := range []*gui.Form{imperative.Form, obj.(*gui.Form)} {
		var child gui.IWidget = imperative.A
		if i == 1 { child = index["A"].(gui.IWidget) }
		size := child.Width()
		form.SetSize(1500,800)
		root := form.LayoutRoot()
		if root == nil || root.Width()!=1500 || root.Height()!=800 || child.Parent()!=root || child.Width() <= size {
			t.Fatalf("builder %d lost the layout hierarchy or responsive client geometry", i)
		}
	}
}
`,
	}
	var args = []string{"test", "-count=1"}
	for _, name := range []string{"imperative.go", "declarative.go", "runtime_test.go"} {
		path := filepath.Join(d, name)
		if err := os.WriteFile(path, []byte(files[name]), 0600); err != nil {
			t.Fatal(err)
		}
		args = append(args, path)
	}
	if out, err := exec.Command("go", args...).CombinedOutput(); err != nil {
		t.Fatalf("generated program failed: %v\n%s", err, out)
	}
}

func TestFormRootLayoutFollowsThePreviewWindow(t *testing.T) {
	v, _ := layoutFixture(t, LayoutGrid)
	v.Selection().Clear()
	v.Selection().Add(v.GedScene())
	if !v.LayoutSelection(LayoutGrid) {
		t.Fatal("form layout refused")
	}
	p, blocked := BuildPreview(v.GedScene())
	if p == nil || len(blocked) != 0 || len(p.Children()) != 1 {
		t.Fatalf("preview could not create layout: %v", blocked)
	}
	f := p.Children()[0].(*gui.Form)
	owner := f.LayoutRoot()
	if owner == nil {
		t.Fatal("preview lost the form layout")
	}
	w, h := p.Size()
	p.SetSize(w+300, h+120)
	if f.Width() != p.Width() || f.Height() != p.Height() || owner.Width() != p.Width() || owner.Height() != p.Height() {
		t.Fatal("resizing the preview did not resize the embedded responsive form")
	}
	p.SetSize(w, h)
	if owner.Width() != w || owner.Height() != h {
		t.Fatal("shrinking the preview left the managed root at its old size")
	}
}

func TestFormRootLayoutDoesNotScaleLegacyFreePreview(t *testing.T) {
	v := NewGedView()
	sceneWidget(t, v.GedScene(), "gui.Button", "edge", 20, 10, 20, 10)
	p, blocked := BuildPreview(v.GedScene())
	if p == nil || len(blocked) != 0 {
		t.Fatalf("can't make free preview: %v", blocked)
	}
	form := p.Children()[0].(*gui.Form)
	fixed, child := form.Bounds1(), form.Children()[0].Bounds1()
	p.SetSize(p.Width()+300, p.Height()+120)
	if form.Bounds1() != fixed || form.Children()[0].Bounds1() != child {
		t.Fatal("resizing preview took over a free-positioned design")
	}
}

func TestFormRootLayoutSelectableFromBlankCanvasAndObjects(t *testing.T) {
	v := NewGedView()
	s := v.GedScene()
	tree := NewObjectInspector()
	tree.SetScene(s)
	sceneWidget(t, s, "gui.Button", "OpenedWidget", 35, 30, 25, 12)
	// Document install notifies the scene even when nothing was selected.
	s.NotifyDesignChanged()
	rows := tree.RowNames()
	if len(rows) != 2 || rows[1] != "OpenedWidget" {
		t.Errorf("Objects tree still displays the pre-load scene: %v", rows)
	}
	tree.selectRow(0)
	if !v.Selection().Contains(s) {
		t.Error("selecting the Objects root did not select the form")
	}
	v.Selection().Clear()
	if !v.CanLayoutSelection(LayoutVertical) || !v.LayoutSelection(LayoutVertical) {
		t.Fatal("with no selected control, the layout action must target the form")
	}
	v.Undo()
	if v.Selection().Count() != 0 {
		t.Fatal("undo invented a selection")
	}
	v.Redo()
	v.Selection().Clear()
	if !v.CanBreakLayout() || !v.BreakLayout() {
		t.Fatal("break did not target the whole form with no selected control")
	}
	v.Undo()
	if v.Selection().Count() != 0 || s.activeFormLayout() == nil {
		t.Fatal("undo break did not restore the prior form context")
	}
}
