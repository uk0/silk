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
// design-time tag make the design a SCADA app: the struct owns a shared
// *scada.Services (with Tags kept as a compatibility alias), each tagged widget
// is handed its design-time tag, and the actual value wiring is delegated to
// scada.BindScreen (called from the generated BindServices) rather than
// hand-emitted BindTag/TagDB. An untagged control is left untouched.
func TestGenerateCodeTagBinding(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compile test in short mode")
	}
	scene := NewGedScene()
	scene.SetFormTitle("HMI")
	scene.SetSize(200, 150)
	addFakeWithTag(t, scene, "gui.Tank", "tank1", "level")  // float
	addFakeWithTag(t, scene, "gui.Valve", "valve1", "pump") // bool
	addFakeWithTag(t, scene, "gui.Label", "lbl1", "")       // untagged control

	code := scene.GenerateCode(CodeGenOptions{PackageName: "main", TypeName: "HMIUI"})

	want := []string{
		"Services *scada.Services",
		"Tags *core.TagDB", // alias field kept for compatibility
		"Tank1 *gui.Tank",  // factoryMap types industrial widgets concretely so
		"Valve1 *gui.Valve", // the SetTagName calls compile
		"func (ui *HMIUI) BindServices(s *scada.Services) error",
		`ui.Tank1.SetTagName("level")`,
		`ui.Valve1.SetTagName("pump")`,
		"scada.BindScreen(s, ui.Form,",
		"scada.New(scada.DefaultConfig(core.LocalDataDir()))",
	}
	for _, w := range want {
		if !strings.Contains(code, w) {
			t.Errorf("generated code missing:\n  %s\n----\n%s", w, code)
		}
	}
	// The private-TagDB plumbing and hand-wired bindings are gone — scada.Services
	// owns the registry and scada.BindScreen does the value wiring.
	for _, bad := range []string{"core.NewTagDB()", "gui.BindTagAnimated("} {
		if strings.Contains(code, bad) {
			t.Errorf("generated code should no longer contain:\n  %s", bad)
		}
	}
	// Exactly the two tagged widgets get a tag name — the untagged Label must not.
	if n := strings.Count(code, "SetTagName("); n != 2 {
		t.Errorf("SetTagName count = %d, want 2 (one per tagged widget)", n)
	}

	// Type-check the emitted services wiring against the real scada + gui + core
	// API — the definitive proof the 组态 output compiles.
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
