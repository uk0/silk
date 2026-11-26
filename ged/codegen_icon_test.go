package ged

import (
	"strings"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
)

// The button icon travels design → file → code: the property sheet writes a
// resource name onto the widget, SaveDesign/LoadDesign carry it through the
// "props" block, and codegen emits the SetIconName call that reproduces it.
// This file covers the two ends persistence and generation own.

// unmappedIconFactory is a Button registered under a name ged's factoryMap does
// not know. A widget like that degrades to a gui.IWidget field (see
// interfaceGoType), an interface with no SetIconName, so the icon emission has
// to skip it however loudly the widget answers IconName(). No shipped widget is
// both icon-bearing and unmapped today — this fixture is what keeps the first
// one that is from generating a file the user cannot build.
const unmappedIconFactory = "gedtest.IconWidget"

type unmappedIconWidget struct {
	gui.Button
}

func init() {
	core.RegisterFactory(unmappedIconFactory, core.TypeOf((*unmappedIconWidget)(nil)))
}

// newIconScene builds the one-widget scene the codegen assertions generate
// from. iconName "" leaves the widget as dragged.
func newIconScene(t *testing.T, factory, widgetName, iconName string) *GedScene {
	t.Helper()
	scene := NewGedScene()
	scene.SetFormTitle("IconForm")
	scene.SetSize(100, 80)

	fake, err := NewFakeWidgetFromFactory(factory)
	if err != nil {
		t.Fatalf("failed to create %s: %v", factory, err)
	}
	fake.SetWidgetName(widgetName)
	fake.SetBounds(5, 5, 25, 7)
	if iconName != "" {
		named, ok := fake.Widget().(interface{ SetIconName(string) })
		if !ok {
			t.Fatalf("factory %s produced %T, which has no SetIconName", factory, fake.Widget())
		}
		named.SetIconName(iconName)
	}
	cmd := graph.NewAddCommand()
	cmd.AddItem(fake, scene)
	scene.PushCommand(cmd)
	return scene
}

// TestButtonIconNameRoundTrip: the icon name a design sets survives a save and
// a load into a fresh widget. The assertion is deliberately on the reload and
// not on the getter — the name rides the "props" block, which is built from
// Button.EnumProperties, so dropping the "图标" row loses the icon on save while
// a getter-only test would stay green.
func TestButtonIconNameRoundTrip(t *testing.T) {
	saved, err := NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatal("failed to create button:", err)
	}
	btn, ok := saved.Widget().(*gui.Button)
	if !ok {
		t.Fatalf("factory gui.Button produced %T, want *gui.Button", saved.Widget())
	}
	btn.SetIconName("save")

	doc := saved.SaveDesign()

	loaded, err := NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatal("failed to create button:", err)
	}
	if err := loaded.LoadDesign(doc); err != nil {
		t.Fatal("LoadDesign:", err)
	}
	got, ok := loaded.Widget().(*gui.Button)
	if !ok {
		t.Fatalf("reloaded widget is %T, want *gui.Button", loaded.Widget())
	}
	if name := got.IconName(); name != "save" {
		t.Errorf(`reloaded IconName() = %q, want "save" (the design did not persist the icon)`, name)
	}
}

// Exercise the authored option through design persistence and the generated UI,
// not just the text getter. A saved caption can exist but not be drawn.
func TestButtonTextWithIconRoundTripAndCodegen(t *testing.T) {
	for _, tt := range []struct {
		name, factory          string
		authored, legacy, want bool
		call                   string
	}{
		{"ordinary", "gui.Button", true, false, true, ".SetTextVisible(true)"},
		{"legacy icon-only", "gui.Button", false, true, false, ""},
		{"print", "gui.PrintButton", true, false, true, ".SetTextVisible(true)"},
		{"HTTP icon-only", "gui.HTTPButton", false, false, false, ".SetTextVisible(false)"},
		{"legacy HTTP", "gui.HTTPButton", false, true, true, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			type buttonLike interface {
				Text() string
				IsTextVisible() bool
				SetTextVisible(bool)
			}
			scene := newIconScene(t, tt.factory, "btnLabel", "build")
			source := scene.Children()[0].(*FakeWidget)
			source.Widget().(buttonLike).SetTextVisible(tt.authored)
			doc := source.SaveDesign()
			if tt.legacy {
				props := doc.ChildByKey("props", false)
				children := props.Childdren()
				props.Clear()
				for _, child := range children {
					if child.Key() != "有图标时显示文字" {
						props.AddChild(child.Clone())
					}
				}
			}

			doc, err := core.LoadTDocStr(doc.String())
			if err != nil {
				t.Fatal(err)
			}
			loadedScene := NewGedScene()
			loaded, err := NewFakeWidgetFromFactory(tt.factory)
			if err != nil {
				t.Fatal(err)
			}
			if err := loaded.LoadDesign(doc); err != nil {
				t.Fatal(err)
			}
			b := loaded.Widget().(buttonLike)
			if b.Text() == "" || b.IsTextVisible() != tt.want {
				t.Fatalf("reloaded caption: text=%q shown=%v, want shown=%v", b.Text(), b.IsTextVisible(), tt.want)
			}
			cmd := graph.NewAddCommand()
			cmd.AddItem(loaded, loadedScene)
			loadedScene.PushCommand(cmd)
			code := loadedScene.GenerateCode(CodeGenOptions{})
			if tt.call == "" {
				if strings.Contains(code, "ui.BtnLabel.SetTextVisible(") {
					t.Fatalf("legacy factory default unexpectedly overridden:\n%s", code)
				}
			} else if !strings.Contains(code, "ui.BtnLabel"+tt.call) {
				t.Fatalf("generated UI does not preserve caption visibility:\n%s", code)
			}
		})
	}
}

// TestGenerateCodeButtonIconName: a designed icon comes out as the one call
// that reproduces it.
func TestGenerateCodeButtonIconName(t *testing.T) {
	scene := newIconScene(t, "gui.Button", "btnSave", "save")
	code := scene.GenerateCode(CodeGenOptions{PackageName: "main", TypeName: "IconFormUI"})

	want := "\tui.BtnSave.SetIconName(\"save\")\n"
	if !strings.Contains(code, want) {
		t.Errorf("generated code is missing %q\n---\n%s", want, code)
	}
}

// buttonNoIconGolden is the full output of newIconScene(t, "btnPlain", "")
// captured before icon emission (geometry uses exact 96 / 25.4 conversion).
// A button that names no icon must
// generate exactly this, so an unconditional SetIconName("") — or a stray blank
// line left behind by the new loop — fails here.
const buttonNoIconGolden = `// Code generated by Silk Designer Editor. DO NOT EDIT.
package main

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

type IconFormUI struct {
	Form     *gui.Form
	BtnPlain *gui.Button
}

func NewIconFormUI() *IconFormUI {
	ui := new(IconFormUI)

	ui.Form = gui.NewForm()
	ui.Form.SetTitle("IconForm")
	ui.Form.SetSize(378, 302)

	ui.BtnPlain = gui.NewButton1("", nil)
	ui.BtnPlain.SetParent(ui.Form)
	ui.BtnPlain.SetBounds(19, 19, 94, 26)

	ui.BtnPlain.SetText("Button")

	return ui
}

func main() {
	ui := NewIconFormUI()
	ui.Form.AttachWindow(gui.WtForm)
	ui.Form.Window().SetIcon(nil)
	ui.Form.Window().MoveToCenter()
	ui.Form.Show()
	core.EventLoop()
}
`

func TestGenerateCodeButtonWithoutIconName(t *testing.T) {
	scene := newIconScene(t, "gui.Button", "btnPlain", "")
	code := scene.GenerateCode(CodeGenOptions{PackageName: "main", TypeName: "IconFormUI"})

	if code != buttonNoIconGolden {
		t.Errorf("a button with no icon changed the generated output\n--- got ---\n%s\n--- want ---\n%s", code, buttonNoIconGolden)
	}
}

// TestGenerateCodeUnmappedWidgetKeepsItsIconUnemitted: an icon-bearing widget
// whose factory codegen has no mapping for holds its name just as a Button
// does, but its field is declared gui.IWidget — the emission has to stay
// silent, because gui.IWidget has no SetIconName and the call would not
// compile. This is the same shape as the CodeEditor event binding that used to
// be emitted against a degraded field (see codegen_compile_test.go).
func TestGenerateCodeUnmappedWidgetKeepsItsIconUnemitted(t *testing.T) {
	scene := newIconScene(t, unmappedIconFactory, "iconBtn", "build")
	code := scene.GenerateCode(CodeGenOptions{PackageName: "main", TypeName: "IconFormUI"})

	if !hasStructField(code, "IconBtn", interfaceGoType) {
		t.Fatalf("fixture is not exercising the degraded path: IconBtn is not declared %s\n----\n%s", interfaceGoType, code)
	}
	if strings.Contains(code, "ui.IconBtn.SetIconName(") {
		t.Errorf("icon emitted against the %s field IconBtn; %s has no SetIconName, so the generated file does not compile\n----\n%s",
			interfaceGoType, interfaceGoType, code)
	}
}

// TestCodeGenUnmappedIconWidgetCompiles is the assertion above stated as the
// consequence: go vet type-checks the generated module, so an icon call emitted
// against a gui.IWidget field is reported by the compiler's own message rather
// than by a string match this file has to keep in sync.
func TestCodeGenUnmappedIconWidgetCompiles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compile test in short mode")
	}

	scene := newIconScene(t, unmappedIconFactory, "iconBtn", "build")
	vetGeneratedCode(t, scene.GenerateCode(CodeGenOptions{PackageName: "main", TypeName: "IconFormUI"}))
}
