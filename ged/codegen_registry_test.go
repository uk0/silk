package ged

import (
	"go/format"
	"sort"
	"strings"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
)

// paletteNames returns every factory name the palette declares, in declaration
// order, so the drift tests below read the same list WidgetList.Init walks.
func paletteNames() []string {
	var out []string
	for _, cd := range categoryDefs {
		out = append(out, cd.names...)
	}
	return out
}

// TestPaletteEntryHasCodegenMapping is the guard that makes the next widget's
// author find the gap at `go test` instead of at a user's failed build: a name
// the palette offers but factoryMap does not know is generated as
// core.New("<name>").(gui.IWidget), which throws the concrete type away and —
// for anything outside the packages the generated program links — asserts on a
// nil interface and panics at startup.
func TestPaletteEntryHasCodegenMapping(t *testing.T) {
	for _, name := range paletteNames() {
		if _, ok := factoryMap[name]; !ok {
			t.Errorf("palette offers %s but ged/codegen.go's factoryMap has no entry for it; "+
				"add one (or drop it from categoryDefs)", name)
		}
	}
}

// TestCodegenMappingIsOfferedByPalette is the same guard in the other
// direction: a mapping nobody can reach from the palette is dead weight, so it
// must be either categorised or listed in paletteOptOut with a reason.
func TestCodegenMappingIsOfferedByPalette(t *testing.T) {
	inPalette := map[string]bool{}
	for _, name := range paletteNames() {
		inPalette[name] = true
	}
	var names []string
	for name := range factoryMap {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if _, optedOut := paletteOptOut[name]; optedOut {
			if inPalette[name] {
				t.Errorf("%s is both categorised in the palette and listed in paletteOptOut", name)
			}
			continue
		}
		if !inPalette[name] {
			t.Errorf("factoryMap can generate %s but no palette category lists it; "+
				"add it to categoryDefs or to paletteOptOut with a reason", name)
		}
	}
	for name := range paletteOptOut {
		if _, ok := factoryMap[name]; !ok {
			t.Errorf("paletteOptOut names %s, which factoryMap no longer maps", name)
		}
	}
}

// TestPaletteEntryIsARegisteredWidget catches a mistyped or renamed factory
// name: the palette silently drops names it cannot instantiate, so a typo
// costs the widget its palette row with no other symptom.
func TestPaletteEntryIsARegisteredWidget(t *testing.T) {
	seen := map[string]bool{}
	for _, name := range paletteNames() {
		if seen[name] {
			t.Errorf("%s is listed in more than one palette category", name)
		}
		seen[name] = true

		f := core.FindFactory(name)
		if f == nil {
			t.Errorf("palette lists %s but no factory is registered under that name", name)
			continue
		}
		if _, ok := f.New().(gui.IWidget); !ok {
			t.Errorf("palette lists %s but its factory does not produce a gui.IWidget", name)
		}
	}
}

// TestGeneratedCodeIsGofmtClean: the designer writes these files into the
// user's project, where an ungofmt'd file shows up in every later diff.
func TestGeneratedCodeIsGofmtClean(t *testing.T) {
	scene := NewGedScene()
	scene.SetFormTitle("Fmt")
	scene.SetSize(120, 90)
	addNamedWidget(t, scene, "gui.Button", "BtnVeryLongName", 5)
	addNamedWidget(t, scene, "gui.Edit", "E", 20)

	code := scene.GenerateCode(CodeGenOptions{PackageName: "main", TypeName: "FmtUI"})
	want, err := format.Source([]byte(code))
	if err != nil {
		t.Fatalf("generated code does not parse: %v\n----\n%s", err, code)
	}
	if string(want) != code {
		t.Errorf("generated code is not gofmt-clean\n--- got ---\n%s\n--- gofmt ---\n%s", code, want)
	}
}

// TestGeneratedCodeIsDeterministic: the import set and the per-widget event
// table are maps, and Go randomises map iteration, so regenerating an
// unchanged design used to produce a different file every time.
func TestGeneratedCodeIsDeterministic(t *testing.T) {
	build := func() string {
		scene := NewGedScene()
		scene.SetFormTitle("Det")
		scene.SetSize(120, 90)
		e := addNamedWidget(t, scene, "gui.Edit", "Ed", 5)
		e.SetEventHandler("OnChanged", "onChanged")
		e.SetEventHandler("OnAlpha", "onAlpha")
		e.SetEventHandler("OnBeta", "onBeta")
		return scene.GenerateCode(CodeGenOptions{PackageName: "main", TypeName: "DetUI"})
	}
	first := build()
	for i := 0; i < 50; i++ {
		if got := build(); got != first {
			t.Fatalf("run %d differs from run 0\n--- run 0 ---\n%s\n--- run %d ---\n%s", i+1, first, i+1, got)
		}
	}
}

// TestGeneratedCodeOmitsUnusedImports pins two ways the import set went stale.
// core is added up front for main()'s event loop, so a non-main package used to
// import it for nothing; and the stdlib scan matched a bare substring, so the
// word "runtime." in the unknown-binding guidance comment pulled in "time".
// Either one is a compile error, which go vet's type check reports.
func TestGeneratedCodeOmitsUnusedImports(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compile test in short mode")
	}

	scene := NewGedScene()
	scene.SetFormTitle("Imports")
	scene.SetSize(120, 90)
	e := addNamedWidget(t, scene, "gui.Edit", "Ed", 5)
	e.SetEventHandler("OnBogus", "onBogus")

	code := scene.GenerateCode(CodeGenOptions{PackageName: "ui", TypeName: "ImportsUI"})
	if strings.Contains(code, `"time"`) {
		t.Errorf("unused \"time\" import\n----\n%s", code)
	}
	if strings.Contains(code, `"github.com/uk0/silk/core"`) {
		t.Errorf("unused core import in a non-main package\n----\n%s", code)
	}
	vetGeneratedCode(t, code)
}

// TestGeneratedFieldNameNeverShadowsForm: the struct always declares Form (and
// Services/Tags on the SCADA path), so a widget the user names "Form" used to
// emit a second field with that identifier — a duplicate-field compile error,
// with every ui.Form call in the constructor silently retargeted.
func TestGeneratedFieldNameNeverShadowsForm(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compile test in short mode")
	}

	scene := NewGedScene()
	scene.SetFormTitle("Shadow")
	scene.SetSize(120, 90)
	addNamedWidget(t, scene, "gui.Button", "Form", 5)
	addNamedWidget(t, scene, "gui.Label", "Tags", 20)

	code := scene.GenerateCode(CodeGenOptions{PackageName: "main", TypeName: "ShadowUI"})
	if strings.Count(code, "\tForm ") > 1 || strings.Contains(code, "Form   *gui.Button") {
		t.Errorf("widget named Form collided with the struct's own Form field\n----\n%s", code)
	}
	vetGeneratedCode(t, code)
}

// TestGeneratedCodeKeepsDesignedCaptions: the caption emitter matched on the
// Go type name, so it covered six widget kinds and dropped the rest — a Card
// dragged onto a form carries the title "Card Title" in the designer and came
// out blank in the generated screen.
func TestGeneratedCodeKeepsDesignedCaptions(t *testing.T) {
	scene := NewGedScene()
	scene.SetFormTitle("Caption")
	scene.SetSize(120, 90)
	addNamedWidget(t, scene, "gui.Card", "Cd", 5)
	addNamedWidget(t, scene, "gui.Tag", "Tg", 20)
	addNamedWidget(t, scene, "gui.Button", "Btn", 35)

	code := scene.GenerateCode(CodeGenOptions{PackageName: "main", TypeName: "CaptionUI"})
	for _, want := range []string{
		`ui.Cd.SetTitle("Card Title")`,
		`ui.Tg.SetText("Tag")`,
		`ui.Btn.SetText("Button")`,
	} {
		if !strings.Contains(code, want) {
			t.Errorf("missing %s\n----\n%s", want, code)
		}
	}
}

// hasStructField reports whether the generated struct declares `name goType`.
// The generated file is gofmt'd, so the gap between the two is alignment
// padding that shifts whenever a longer field name joins the struct — matching
// on the pair rather than on one exact spelling keeps these assertions from
// breaking for a reason that has nothing to do with what they test.
func hasStructField(code, name, goType string) bool {
	for _, line := range strings.Split(code, "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && f[0] == name && f[1] == goType {
			return true
		}
	}
	return false
}

// addNamedWidget drops one designed widget on the scene at the given y (mm).
func addNamedWidget(t *testing.T, scene *GedScene, factory, name string, y float64) *FakeWidget {
	t.Helper()
	fake, err := NewFakeWidgetFromFactory(factory)
	if err != nil {
		t.Fatalf("create %s: %v", factory, err)
	}
	fake.SetWidgetName(name)
	fake.SetBounds(5, y, 40, 8)
	cmd := graph.NewAddCommand()
	cmd.AddItem(fake, scene)
	scene.PushCommand(cmd)
	return fake
}
