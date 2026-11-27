package ged

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/gui"
)

func newContainerLayoutFixture(t *testing.T, factory string, children int) (*GedView, *FakeWidget) {
	t.Helper()
	v := NewGedView()
	scene := v.GedScene()
	scene.SetSize(240, 150)
	owner := sceneWidget(t, scene, factory, "Equipment", 20, 20, 150, 90)
	for i := 0; i < children; i++ {
		label := []string{"Start", "Stop"}[i]
		child := sceneWidget(t, scene, "gui.Button", label, 37+float64(i)*51, 50, 24, 8)
		child.Widget().(*gui.Button).SetText(label)
		child.SetParent(owner)
	}
	v.Selection().Clear()
	v.Selection().Add(owner)
	return v, owner
}

func checkContentDesignBounds(t *testing.T, owner *FakeWidget) *FakeWidget {
	t.Helper()
	box := owner.activeContentLayout()
	if box == nil || !managedGeometryItem(box) || !owner.ManagesChildGeometry() {
		t.Fatal("layout is not owned by the selected container")
	}
	c := owner.Widget().(contentGeometry).ContentBounds()
	want := geom.Rect{
		X: gui.PixelToMm(c.X), Y: gui.PixelToMm(c.Y),
		Width: gui.PixelToMm(c.Width), Height: gui.PixelToMm(c.Height),
	}
	if !owner.HasLocalCoord() {
		want.X += owner.X()
		want.Y += owner.Y()
	}
	if box.Bounds1() != want || want.Y <= owner.Y() || want.Height >= owner.Height() {
		t.Fatalf("layout crosses the title or border: got %v want %v", box.Bounds1(), want)
	}
	return box
}

func checkContentRuntime(t *testing.T, form gui.IWidget, children int) {
	t.Helper()
	owner := form.Children()[0]
	slot := owner.(interface {
		gui.IWidget
		Content() gui.IWidget
		ContentBounds() geom.Rect
	})
	root := slot.Content()
	if root == nil || root.Parent() != owner || root.Bounds1() != slot.ContentBounds() || len(root.Children()) != children {
		t.Fatalf("%T did not own its managed content", owner)
	}
	w0 := root.Width()
	owner.SetSize(owner.Width()+90, owner.Height()+50)
	if root.Width() <= w0 || root.Bounds1() != slot.ContentBounds() || len(root.Children()) != children {
		t.Fatalf("%T content layout did not follow resize", owner)
	}
}

func TestContentLayoutDirectOnTitledContainers(t *testing.T) {
	for _, factory := range []string{"gui.Card", "gui.GroupBox"} {
		for _, kind := range []LayoutKind{LayoutHorizontal, LayoutVertical, LayoutGrid, LayoutForm} {
			t.Run(factory+"/"+string(kind), func(t *testing.T) {
				v, owner := newContainerLayoutFixture(t, factory, 2)
				children := owner.Children()
				before := []geom.Rect{children[0].Bounds1(), children[1].Bounds1()}
				if !v.CanLayoutSelection(kind) || !v.LayoutSelection(kind) {
					t.Fatal("valid container refused layout")
				}
				box := checkContentDesignBounds(t, owner)
				if !reflect.DeepEqual(box.Children(), children) {
					t.Fatal("layout lost children or changed order")
				}
				v.Undo()
				if owner.activeContentLayout() != nil || !reflect.DeepEqual(owner.Children(), children) ||
					children[0].Bounds1() != before[0] || children[1].Bounds1() != before[1] || !v.Selection().Contains(owner) {
					t.Fatal("undo did not restore layout-free container, geometry and selection")
				}
				v.Redo()
				if box != checkContentDesignBounds(t, owner) {
					t.Fatal("redo did not restore the owned layout")
				}
				owner.SetBounds(28, 29, 173, 98)
				checkContentDesignBounds(t, owner)
				switch w := owner.Widget().(type) {
				case *gui.Card:
					w.SetPadding(16)
				case *gui.GroupBox:
					w.SetCheckable(true)
				}
				owner.designPropertyChanged()
				checkContentDesignBounds(t, owner)
				s := v.GedScene()
				doc := s.SaveDesign()
				reload := NewGedScene()
				if err := reload.LoadDesign(doc); err != nil {
					t.Fatal(err)
				}
				checkContentDesignBounds(t, reload.Children()[0].(*FakeWidget))
				checkContentRuntime(t, s.Generate().Form(), 2)
				form, err := gui.LoadFormFromDoc(doc)
				if err != nil {
					t.Fatal(err)
				}
				checkContentRuntime(t, form, 2)
				raw, err := buildSceneDeclNode(s).Build()
				if err != nil {
					t.Fatal(err)
				}
				checkContentRuntime(t, raw.(gui.IWidget), 2)
			})
		}
	}
}

func TestContentLayoutBreakKeepsSingleFreeChild(t *testing.T) {
	for _, factory := range []string{"gui.Card", "gui.GroupBox"} {
		t.Run(factory, func(t *testing.T) {
			v, owner := newContainerLayoutFixture(t, factory, 1)
			child := owner.Children()[0]
			if !v.LayoutSelection(LayoutVertical) {
				t.Fatal("can't apply layout")
			}
			kept := child.Bounds1()
			v.Selection().Clear()
			v.Selection().Add(owner)
			if !v.CanBreakLayout() || !v.BreakLayout() || child.Parent() != owner || child.Bounds1() != kept || owner.activeContentLayout() != nil {
				t.Fatal("break from the owner did not keep the child in place inside that owner")
			}
			owner.SetSize(195, 85)
			if child.Bounds1() != kept {
				t.Fatal("free child still followed its old layout")
			}
			s := v.GedScene()
			doc := s.SaveDesign()
			loaded, err := gui.LoadFormFromDoc(doc)
			if err != nil {
				t.Fatal(err)
			}
			for _, form := range []gui.IWidget{s.Generate().Form(), loaded} {
				host := form.Children()[0].(interface {
					gui.IWidget
					Content() gui.IWidget
				})
				if host.Content() != nil || len(host.Children()) != 1 {
					t.Fatal("preview/loader promoted the remaining free child back into the slot")
				}
				old := host.Children()[0].Bounds1()
				host.SetSize(host.Width()+18, host.Height()+18)
				if host.Children()[0].Bounds1() != old {
					t.Fatal("runtime free bounds changed with owner")
				}
			}
			v.Undo()
			checkContentDesignBounds(t, owner)
			v.Redo()
			if owner.activeContentLayout() != nil || child.Parent() != owner {
				t.Fatal("redo lost free placement")
			}
		})
	}
}

func TestContentLayoutEmptyAndDropOnlyInContentArea(t *testing.T) {
	v, owner := newContainerLayoutFixture(t, "gui.Card", 0)
	if !v.LayoutSelection(LayoutHorizontal) {
		t.Fatal("empty card refused layout")
	}
	box := checkContentDesignBounds(t, owner)
	x, y := v.MapFromScene(owner.X()+6, owner.Y()+1)
	ctx := &dropCtx{pa: gui.DndCopy, payload: "gui.Button"}
	v.OnDragEnter(x, y, ctx)
	v.OnDrop(x, y, ctx)
	if ctx.Action() != gui.DndIgnore || box.HasChildren() || len(owner.Children()) != 1 {
		t.Fatal("dropping onto the chrome bypassed the owned content area")
	}
	button := placeFromPalette(t, v, "gui.Button", box.X()+10, box.Y()+14)
	if button.Parent() != box || len(owner.Children()) != 1 {
		t.Fatal("drop did not use the existing content layout")
	}
	v.Undo()
	if !v.BreakLayout() || owner.HasChildren() {
		t.Fatal("empty layout could not be removed")
	}
	v.Undo()
	checkContentDesignBounds(t, owner)
}

func TestContentLayoutRespectsLocks(t *testing.T) {
	v, owner := newContainerLayoutFixture(t, "gui.GroupBox", 2)
	child := owner.Children()[0].(*FakeWidget)
	child.SetLockSize(true)
	if v.CanLayoutSelection(LayoutGrid) || v.LayoutSelection(LayoutGrid) || owner.activeContentLayout() != nil ||
		v.Scene().UndoStack().CanUndo() {
		t.Fatal("locked child allowed container to be rearranged")
	}
}

func TestContentLayoutSDKResaveDoesNotPromoteFreeSoleChild(t *testing.T) {
	for _, factory := range []string{"gui.Card", "gui.GroupBox"} {
		t.Run(factory, func(t *testing.T) {
			v, owner := newContainerLayoutFixture(t, factory, 1)
			if !v.LayoutSelection(LayoutVertical) {
				t.Fatal("layout refused")
			}
			saveRead := func() *gui.Form {
				t.Helper()
				file := filepath.Join(t.TempDir(), "held.silkui")
				if err := gui.SaveForm(v.GedScene().Generate().Form(), file); err != nil {
					t.Fatal(err)
				}
				form, err := gui.LoadForm(file)
				if err != nil {
					t.Fatal(err)
				}
				return form
			}
			checkContentRuntime(t, saveRead(), 1)
			v.Selection().Clear()
			v.Selection().Add(owner)
			if !v.BreakLayout() {
				t.Fatal("break refused")
			}
			panel := saveRead().Children()[0].(interface {
				gui.IWidget
				Content() gui.IWidget
			})
			if panel.Content() != nil || len(panel.Children()) != 1 {
				t.Fatal("SDK round trip assigned free geometry to the content slot")
			}
		})
	}
}

func TestContentLayoutGeneratedGoFollowsCardClientSize(t *testing.T) {
	if testing.Short() {
		t.Skip("generated Go integration")
	}
	v, _ := newContainerLayoutFixture(t, "gui.Card", 2)
	if !v.LayoutSelection(LayoutForm) {
		t.Fatal("layout refused")
	}
	s := v.GedScene()
	opts := CodeGenOptions{PackageName: "ui", TypeName: "ContentLayoutUI"}
	d := t.TempDir()
	inputs := []struct{ name, source string }{
		{"imperative.go", s.GenerateCode(opts)},
		{"declarative.go", s.GenerateDeclCode(opts)},
		{"content_test.go", `package ui
import (
 "testing"
 "github.com/uk0/silk/gui"
)
func TestContentLayout(t *testing.T) {
 c:=NewContentLayoutUI()
 root,idx,err:=BuildContentLayoutUI().BuildWithIndex()
 if root==nil || err!=nil{t.Fatalf("builder: %v",err)}
 for _,card:=range []*gui.Card{c.Equipment,idx["Equipment"].(*gui.Card)}{
  body:=card.Content()
  if body==nil || body.Parent()!=card || body.Bounds1()!=card.ContentBounds() || len(body.Children())!=2{
   t.Fatal("responsive content missing in generated builder")
  }
  card.SetSize(card.Width()+60,card.Height()+40)
  if body.Bounds1()!=card.ContentBounds(){t.Fatal("content no longer follows resize")}
 }
}
`},
	}
	args := []string{"test", "-count=1"}
	for _, in := range inputs {
		file := filepath.Join(d, in.name)
		if err := os.WriteFile(file, []byte(in.source), 0600); err != nil {
			t.Fatal(err)
		}
		args = append(args, file)
	}
	if out, err := exec.Command("go", args...).CombinedOutput(); err != nil {
		t.Fatalf("generated build: %v\n%s", err, out)
	}
}
