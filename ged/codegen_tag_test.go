package ged

import (
	"strings"
	"testing"

	"github.com/uk0/silk/graph"
)

// addFakeWithTag places a designer widget of the given factory into scene,
// optionally setting its design-time "tag" property (empty tag = untagged).
func addFakeWithTag(t *testing.T, scene *GedScene, factory, name, tag string) {
	t.Helper()
	w, err := NewFakeWidgetFromFactory(factory)
	if err != nil {
		t.Fatalf("create %s: %v", factory, err)
	}
	w.SetWidgetName(name)
	w.SetBounds(5, 5, 30, 20)
	if tag != "" {
		tw, ok := w.Widget().(interface{ SetTagName(string) })
		if !ok {
			t.Fatalf("%s widget has no SetTagName", factory)
		}
		tw.SetTagName(tag)
	}
	cmd := graph.NewAddCommand()
	cmd.AddItem(w, scene)
	scene.PushCommand(cmd)
}

// TestGenerateCodeTagBinding verifies that industrial widgets carrying a
// design-time tag emit a core.TagDB plus the correct runtime BindTag wiring —
// float widgets via eased BindTagAnimated, booleans via BindTag+Post — while an
// untagged widget emits none.
func TestGenerateCodeTagBinding(t *testing.T) {
	scene := NewGedScene()
	scene.SetFormTitle("HMI")
	scene.SetSize(200, 150)
	addFakeWithTag(t, scene, "gui.Tank", "tank1", "level")  // float, needs /100
	addFakeWithTag(t, scene, "gui.Valve", "valve1", "pump") // bool
	addFakeWithTag(t, scene, "gui.Label", "lbl1", "")       // untagged control

	code := scene.GenerateCode(CodeGenOptions{PackageName: "main", TypeName: "HMIUI"})

	want := []string{
		"Tags *core.TagDB",
		"ui.Tags = core.NewTagDB()",
		"Tank1 *gui.Tank",   // factoryMap now types industrial widgets concretely
		"Valve1 *gui.Valve", // (not gui.IWidget), so the setter calls compile
		`gui.BindTagAnimated(ui.Tags.GetOrCreate("level", core.Meta{}), func(v float64) { ui.Tank1.SetLevel(v / 100) }, 300*time.Millisecond)`,
		`gui.BindTag(ui.Tags.GetOrCreate("pump", core.Meta{}), func(v interface{}) { gui.Post(func() { ui.Valve1.SetState(gui.TagBool(v)) }) })`,
	}
	for _, w := range want {
		if !strings.Contains(code, w) {
			t.Errorf("generated code missing:\n  %s", w)
		}
	}
	// Exactly the two tagged widgets bind — the untagged Label must not.
	if n := strings.Count(code, "GetOrCreate"); n != 2 {
		t.Errorf("GetOrCreate count = %d, want 2 (one per tagged widget)", n)
	}

	// Type-check the emitted BindTagAnimated / BindTag / TagDB against the real
	// gui + core API — the definitive proof the 组态 bindings compile.
	vetGeneratedCode(t, code)
}

// TestGenerateCodeNoTagsNoTagDB confirms a design with no tagged widgets emits
// no TagDB field or init, keeping ordinary UIs free of SCADA plumbing.
func TestGenerateCodeNoTagsNoTagDB(t *testing.T) {
	scene := NewGedScene()
	scene.SetFormTitle("Plain")
	scene.SetSize(100, 80)
	addFakeWithTag(t, scene, "gui.Button", "btn1", "")

	code := scene.GenerateCode(CodeGenOptions{PackageName: "main", TypeName: "PlainUI"})
	if strings.Contains(code, "core.TagDB") || strings.Contains(code, "NewTagDB") {
		t.Error("scene with no tags should not emit a TagDB")
	}
}
