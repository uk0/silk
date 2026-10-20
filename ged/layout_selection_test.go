package ged

import (
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/decl"
	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/gui"
)

func layoutFixture(t *testing.T, kind LayoutKind) (*GedView, []*FakeWidget) {
	t.Helper()
	v := NewGedView()
	s := v.GedScene()
	s.SetSize(200, 150)
	var widgets []*FakeWidget
	for i, b := range []geom.Rect{{10, 10, 30, 10}, {65, 10, 35, 10}, {10, 40, 30, 10}, {65, 40, 35, 10}} {
		factory := "gui.Button"
		if kind == LayoutForm && i%2 == 0 {
			factory = "gui.Label"
		}
		w, err := NewFakeWidgetFromFactory(factory)
		if err != nil {
			t.Fatal(err)
		}
		w.SetWidgetName([]string{"A", "B", "C", "D"}[i])
		w.SetBounds1(b)
		w.SetParent(s)
		widgets = append(widgets, w)
	}
	v.Selection().Clear()
	for _, w := range widgets {
		v.Selection().Add(w)
	}
	return v, widgets
}
func checkRuntimeLayout(t *testing.T, root gui.IWidget, children []gui.IWidget, kind LayoutKind) {
	t.Helper()
	if len(children) != 4 {
		t.Fatalf("runtime children=%d, want 4", len(children))
	}
	for _, w := range children {
		x, y, ww, hh := w.Bounds()
		if ww <= 0 || hh <= 0 || x < 0 || y < 0 || math.IsNaN(x) {
			t.Fatalf("invalid bounds %v %v %v %v", x, y, ww, hh)
		}
	}
	a, b, c := children[0], children[1], children[2]
	switch kind {
	case LayoutHorizontal:
		if b.X() < a.X()+a.Width() {
			t.Fatal("horizontal children overlap")
		}
	case LayoutVertical:
		if b.Y() < a.Y()+a.Height() {
			t.Fatal("vertical children overlap")
		}
	case LayoutGrid, LayoutForm:
		if b.X() < a.X()+a.Width() || c.Y() < a.Y()+a.Height() {
			t.Fatal("grid/form children overlap")
		}
	}
}
func TestLayoutSelectionRuntimeRoundTrip(t *testing.T) {
	for _, kind := range []LayoutKind{LayoutHorizontal, LayoutVertical, LayoutGrid, LayoutForm} {
		t.Run(string(kind), func(t *testing.T) {
			v, widgets := layoutFixture(t, kind)
			before := make([]geom.Rect, len(widgets))
			for i, w := range widgets {
				before[i] = w.Bounds1()
			}
			if !v.LayoutSelection(kind) {
				t.Fatal("layout refused")
			}
			box := v.GedScene().Children()[0].(*FakeWidget)
			if !box.layoutManaged {
				t.Fatal("layout not managed")
			}
			runtime := box.Generate()
			checkRuntimeLayout(t, runtime, runtime.Children(), kind)
			// Canvas uses shared scene coordinates; no child may jump to canvas origin.
			if widgets[0].X() < box.X() || widgets[0].Y() < box.Y() {
				t.Fatal("canvas layout ignored container origin")
			}
			v.Scene().UndoStack().Undo()
			if got := v.Selection().ItemList(); len(got) != 4 {
				t.Fatalf("undo selection=%d", len(got))
			}
			for i, w := range widgets {
				if w.Bounds1() != before[i] || w.Parent() != v.GedScene() {
					t.Fatalf("undo failed for %s: %+v", w.WidgetName(), w.Bounds1())
				}
			}
			v.Scene().UndoStack().Redo()
			if got := v.Selection().ItemList(); len(got) != 1 {
				t.Fatalf("redo selection=%d", len(got))
			}
			box = v.GedScene().Children()[0].(*FakeWidget)
			saved := v.GedScene().SaveDesign()
			loaded := NewGedScene()
			if err := loaded.LoadDesign(saved); err != nil {
				t.Fatal(err)
			}
			loadedBox := loaded.Children()[0].(*FakeWidget)
			if !loadedBox.layoutManaged {
				t.Fatal("managed layout not persisted")
			}
			runtime = loadedBox.Generate()
			checkRuntimeLayout(t, runtime, runtime.Children(), kind)
			for _, node := range []*decl.Node{buildSceneDeclNode(loaded), mustDeclFromDesign(t, saved)} {
				_, idx, err := node.BuildWithIndex()
				if err != nil {
					t.Fatal(err)
				}
				var children []gui.IWidget
				for _, id := range []string{"A", "B", "C", "D"} {
					children = append(children, idx[id].(gui.IWidget))
				}
				checkRuntimeLayout(t, children[0].Parent(), children, kind)
			}
			if !v.CanBreakLayout() {
				t.Fatal("break should be available")
			}
			v.BreakLayoutSelection()
			if len(v.GedScene().Children()) != 4 {
				t.Fatal("break lost children")
			}
			v.Undo()
			if !v.CanBreakLayout() {
				t.Fatal("undo break did not reselect restored container")
			}
			v.Redo()
			if len(v.Selection().ItemList()) != 4 {
				t.Fatal("redo break did not reselect children")
			}
		})
	}
}
func mustDeclFromDesign(t *testing.T, doc *core.TDoc) *decl.Node {
	t.Helper()
	n, err := decl.FromTDoc(doc)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestGeneratedLayoutProgramActuallyArrangesWidgets(t *testing.T) {
	if testing.Short() {
		t.Skip("generated Go integration")
	}
	for _, kind := range []LayoutKind{LayoutGrid, LayoutForm} {
		for _, mode := range []string{"wrap", "container"} {
			t.Run(string(kind)+"/"+mode, func(t *testing.T) {
				var v *GedView
				if mode == "container" {
					v, _, _ = containerLayoutFixture(t, kind, 4)
				} else {
					v, _ = layoutFixture(t, kind)
				}
				if !v.LayoutSelection(kind) {
					t.Fatal("layout refused")
				}
				dir := t.TempDir()
				src := filepath.Join(dir, "layout.go")
				testFile := filepath.Join(dir, "layout_test.go")
				code := v.GedScene().GenerateCode(CodeGenOptions{PackageName: "ui", TypeName: "LayoutUI"})
				if !strings.Contains(code, ".AddLayoutWidget(") {
					t.Fatal("no layout registration in generated Go")
				}
				if err := os.WriteFile(src, []byte(code), 0600); err != nil {
					t.Fatal(err)
				}
				testSource := `package ui
import "testing"
func TestActualLayout(t *testing.T){
 u:=NewLayoutUI()
 if u.B.X()<u.A.X()+u.A.Width() || u.C.Y()<u.A.Y()+u.A.Height(){t.Fatal("generated widgets overlap")}
 if u.B.Width()<=0 || u.D.Height()<=0 {t.Fatal("zero sized generated fields")}
 old:=u.B.X();p:=u.A.Parent();p.SetSize(p.Width()*2,p.Height()*2)
 if p.Children()[1].Width()<=0 || (p.Children()[1].X()==old && p.Children()[1].Width()==u.A.Width()){t.Fatal("runtime resize did not relayout")}
}
`
				if err := os.WriteFile(testFile, []byte(testSource), 0600); err != nil {
					t.Fatal(err)
				}
				cmd := exec.Command("go", "test", src, testFile)
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("generated program failed: %v\n%s", err, out)
				}
			})
		}
	}
}

func TestGridPlacementAndInsetsSurviveDesignerAndDecl(t *testing.T) {
	v, _ := layoutFixture(t, LayoutGrid)
	v.LayoutSelection(LayoutGrid)
	box := v.GedScene().Children()[0].(*FakeWidget)
	grid := box.Widget().(*gui.GridLayout)
	grid.SetSpacing(8)
	grid.SetPadding(gui.Padding{L: 12, R: 14, T: 16, B: 18})
	first := box.Children()[0].(*FakeWidget)
	first.layoutCell = layoutPlacement{Row: 2, Column: 3, RowSpan: 2, ColumnSpan: 2, Valid: true}
	saved := v.GedScene().SaveDesign()
	loaded := NewGedScene()
	if err := loaded.LoadDesign(saved); err != nil {
		t.Fatal(err)
	}
	restored := loaded.Children()[0].(*FakeWidget)
	if p := restored.Children()[0].(*FakeWidget).layoutCell; p != first.layoutCell {
		t.Fatalf("placement lost: %+v", p)
	}
	if p := restored.Widget().(*gui.GridLayout).Padding(); p != (gui.Padding{L: 12, R: 14, T: 16, B: 18}) {
		t.Fatalf("padding lost: %+v", p)
	}
	n := mustDeclFromDesign(t, saved)
	_, idx, err := n.BuildWithIndex()
	if err != nil {
		t.Fatal(err)
	}
	a := idx["A"].(gui.IWidget)
	if a.X() <= 12 || a.Y() <= 16 || a.Width() <= 0 {
		t.Fatalf("span registration missing: %+v", a)
	}
	src := loaded.GenerateCode(CodeGenOptions{PackageName: "ui", TypeName: "Grid"})
	for _, want := range []string{"AddLayoutWidget(ui.A, 2, 3, 2, 2)", "SetSpacing(8)", "SetPadding(gui.Padding{L: 12, R: 14, T: 16, B: 18})"} {
		if !strings.Contains(src, want) {
			t.Fatalf("missing %s", want)
		}
	}
}
