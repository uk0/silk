package ged

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/uk0/silk/gui"
)

func contentOwnershipFixture(t *testing.T) *GedScene {
	t.Helper()
	v := NewGedView()
	s := v.GedScene()
	s.SetSize(240, 180)
	for i, kind := range []string{"gui.Card", "gui.GroupBox"} {
		name := []string{"PumpCard", "ValveGroup"}[i]
		x := float64(10 + 105*i)
		box := sceneWidget(t, s, kind, name, x, 10, 95, 65)
		if group, ok := box.Widget().(*gui.GroupBox); ok {
			group.SetCheckable(true)
			group.SetChecked(false)
		}
		for j, label := range []string{"Start", "Stop"} {
			child := sceneWidget(t, s, "gui.Button", name+label, x+8+float64(j*40), 32, 25, 8)
			child.Widget().(*gui.Button).SetText(label)
			child.SetParent(box)
		}
	}
	return s
}

func TestDesignerContentContainersKeepFreeSiblings(t *testing.T) {
	s := contentOwnershipFixture(t)
	for _, path := range []struct {
		name   string
		build  func(t *testing.T) gui.IWidget
		pixels func(float64) float64
	}{
		{"preview", func(t *testing.T) gui.IWidget { return s.Generate().Form() }, gui.MmToPixelZ},
		{"document", func(t *testing.T) gui.IWidget {
			form, err := gui.LoadFormFromDoc(s.SaveDesign())
			if err != nil {
				t.Fatal(err)
			}
			return form
		}, roundPx},
		{"declarative", func(t *testing.T) gui.IWidget {
			form, err := buildSceneDeclNode(s).Build()
			if err != nil {
				t.Fatal(err)
			}
			return form.(gui.IWidget)
		}, roundPx},
	} {
		t.Run(path.name, func(t *testing.T) {
			boxes := path.build(t).Children()
			if len(boxes) != 2 {
				t.Fatalf("want two content containers, got %d", len(boxes))
			}
			for i, box := range boxes {
				kids := box.Children()
				if len(kids) != 2 {
					t.Errorf("%T lost a child: want 2, got %d", box, len(kids))
					continue
				}
				for j, label := range []string{"Start", "Stop"} {
					b, ok := kids[j].(*gui.Button)
					if !ok || b.Parent() != box || b.Text() != label || b.X() != path.pixels(float64(8+40*j)) || b.Y() != path.pixels(22) {
						t.Fatalf("child %d of %T lost its authored position, parent or text: %v", j, box, kids[j].Bounds1())
					}
					if i == 1 && b.IsEnabled() {
						t.Error("unchecked GroupBox did not gate a free child")
					}
				}
				if g, ok := box.(*gui.GroupBox); ok {
					kids[0].SetEnabled(false)
					g.SetChecked(true)
					if kids[0].IsEnabled() || !kids[1].IsEnabled() {
						t.Error("opening the group gate did not preserve the disabled child's own state")
					}
				}
			}
		})
	}
}

func TestPaletteDropRetainsCardAndGroupBoxSiblingsOnRedo(t *testing.T) {
	for _, kind := range []string{"gui.Card", "gui.GroupBox"} {
		t.Run(kind, func(t *testing.T) {
			v := NewGedView()
			s := v.GedScene()
			s.SetSize(170, 120)
			owner := sceneWidget(t, s, kind, "Controls", 10, 10, 140, 90)
			first := placeFromPalette(t, v, "gui.Button", 28, 36)
			second := placeFromPalette(t, v, "gui.Button", 94, 52)
			if first.Parent() != owner || second.Parent() != owner {
				t.Fatal("palette did not drop either control into its selected container")
			}
			check := func(want int) {
				t.Helper()
				preview := s.Generate().Form().Children()[0]
				if len(owner.Children()) != want || len(preview.Children()) != want {
					t.Fatalf("canvas and preview did not both retain %d controls after the drag", want)
				}
			}
			check(2)
			v.Undo()
			check(1)
			v.Redo()
			check(2)
		})
	}
}

func TestGeneratedGoKeepsFreeContentSiblings(t *testing.T) {
	if testing.Short() {
		t.Skip("generated Go integration")
	}
	s := contentOwnershipFixture(t)
	opts := defaultCodeGenOptions(s, CodeGenOptions{PackageName: "ui", TypeName: "ContentUI"})
	d := t.TempDir()
	files := []struct{ name, body string }{
		{"form.go", s.GenerateCode(opts)},
		{"form_test.go", `package ui
import (
 "testing"
 "github.com/uk0/silk/gui"
)
func TestContent(t *testing.T) {
 c:=NewContentUI()
 for _, w := range []gui.IWidget{c.PumpCard,c.ValveGroup} {
  kids:=w.Children()
  if len(kids)!=2 || kids[0].X()!=30 || kids[1].X()!=181 {
   t.Fatalf("generated %T lost siblings or moved them: %v",w,kids)
  }
 }
 if c.ValveGroupStart.IsEnabled() {
  t.Fatal("generated checkable group did not gate its free children")
 }
}
`},
	}
	args := []string{"test", "-count=1"}
	for _, f := range files {
		name := filepath.Join(d, f.name)
		if err := os.WriteFile(name, []byte(f.body), 0600); err != nil {
			t.Fatal(err)
		}
		args = append(args, name)
	}
	if out, err := exec.Command("go", args...).CombinedOutput(); err != nil {
		t.Fatalf("generated Go failed: %v\n%s", err, out)
	}
}
