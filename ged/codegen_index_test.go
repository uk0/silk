package ged

import (
	"strings"
	"testing"

	"github.com/uk0/silk/graph"
)

// addFakeTo places a widget of any factory under parent, so the index tests can
// build nested designs and designs holding a factory codegen has no mapping for.
func addFakeTo(t *testing.T, parent graph.IItem, factory, name string) *FakeWidget {
	t.Helper()
	fw, err := NewFakeWidgetFromFactory(factory)
	if err != nil {
		t.Fatalf("create %s: %v", factory, err)
	}
	fw.SetWidgetName(name)
	fw.SetBounds(5, 5, 30, 8)
	fw.SetParent(parent)
	return fw
}

// lineOf returns line n of src, or "" when there is no such line.
func lineOf(src string, n int) string {
	lines := strings.Split(src, "\n")
	if n < 0 || n >= len(lines) {
		return ""
	}
	return lines[n]
}

// TestGenerateCodeIndexedLinesPointAtConstructors: every widget in the design —
// nested ones included — maps to the line that actually builds it in the
// finished, gofmt'd source. This is what the 生成代码 view scrolls to when a
// widget is selected on the canvas.
func TestGenerateCodeIndexedLinesPointAtConstructors(t *testing.T) {
	scene := NewGedScene()
	scene.SetFormTitle("Indexed")
	scene.SetSize(120, 90)

	btn := addFakeTo(t, scene, "gui.Button", "btnOK")
	box := addFakeTo(t, scene, "gui.VBox", "sidebar")
	inner := addFakeTo(t, box, "gui.Label", "lblInner")
	edit := addFakeTo(t, scene, "gui.Edit", "editName")

	gen := scene.GenerateCodeIndexed(CodeGenOptions{PackageName: "main", TypeName: "IndexedUI"})
	if gen.Err != nil {
		t.Fatalf("unexpected error: %v", gen.Err)
	}

	want := map[*FakeWidget]string{
		btn:   "ui.BtnOK = ",
		box:   "ui.Sidebar = ",
		inner: "ui.LblInner = ",
		edit:  "ui.EditName = ",
	}
	for fake, anchor := range want {
		line, ok := gen.Line(fake)
		if !ok {
			t.Errorf("%s: no line recorded", fake.WidgetName())
			continue
		}
		got := strings.TrimLeft(lineOf(gen.Code, line), " \t")
		if !strings.HasPrefix(got, anchor) {
			t.Errorf("%s mapped to line %d = %q, want a line starting %q\n----\n%s",
				fake.WidgetName(), line, got, anchor, gen.Code)
		}
	}
}

// TestGenerateCodeIndexedMatchesGenerateCode: the indexed pass is the same
// generator, not a second one. GenerateCode is defined as its Code, and a
// divergence here would mean the designer previews one file and exports another.
func TestGenerateCodeIndexedMatchesGenerateCode(t *testing.T) {
	scene := NewGedScene()
	scene.SetFormTitle("Same")
	scene.SetSize(100, 70)
	btn := addFakeTo(t, scene, "gui.Button", "btnGo")
	btn.SetCode("func onBtnGoClick() { fmt.Println(\"go\") }")
	addFakeTo(t, scene, "gui.Slider", "vol")

	opts := CodeGenOptions{PackageName: "main", TypeName: "SameUI"}
	if got, want := scene.GenerateCodeIndexed(opts).Code, scene.GenerateCode(opts); got != want {
		t.Errorf("GenerateCodeIndexed().Code != GenerateCode()\n--- indexed ---\n%s\n--- plain ---\n%s", got, want)
	}
}

// TestIndexAnchorsForwardOnly: the scan never goes backwards, so a field name
// that reappears in an appended handler body cannot claim the line of a widget
// declared after it. A text search for the widget name would take the bait.
func TestIndexAnchorsForwardOnly(t *testing.T) {
	src := strings.Join([]string{
		"func NewUI() *UI {",
		"\tui.Btn = gui.NewButton1(\"\", nil)",
		"\tui.Lbl = gui.NewLabel(\"\")",
		"}",
		"",
		"func onBtnClick() {",
		"\tui.Btn = nil",
		"}",
	}, "\n")

	got := indexAnchors(src, []string{constructorAnchor("Btn"), constructorAnchor("Lbl")})
	want := []int{1, 2}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("indexAnchors = %v, want %v", got, want)
	}
}

// TestIndexAnchorsMissing: an anchor that is not in the source resolves to -1
// rather than to some other widget's line.
func TestIndexAnchorsMissing(t *testing.T) {
	src := "\tui.Btn = gui.NewButton1(\"\", nil)\n"
	got := indexAnchors(src, []string{constructorAnchor("Nope")})
	if len(got) != 1 || got[0] != -1 {
		t.Errorf("indexAnchors = %v, want [-1]", got)
	}
}

// TestIndexAnchorsPrefixNotConfused: "Btn" must not match the "Btn_2" line.
// The dedup suffix in the collect walk makes near-identical field names normal.
func TestIndexAnchorsPrefixNotConfused(t *testing.T) {
	src := strings.Join([]string{
		"\tui.Btn_2 = gui.NewButton1(\"\", nil)",
		"\tui.Btn = gui.NewButton1(\"\", nil)",
	}, "\n")
	got := indexAnchors(src, []string{constructorAnchor("Btn_2"), constructorAnchor("Btn")})
	if len(got) != 2 || got[0] != 0 || got[1] != 1 {
		t.Errorf("indexAnchors = %v, want [0 1]", got)
	}
}

// TestGenerateCodeIndexedReportsUnmappedFactory: a widget whose factory has no
// entry in factoryMap degrades to a gui.IWidget field holding core.New(...) —
// output that builds but is not the design. The indexed pass names the widget
// so the view can show that instead of the misleading listing.
func TestGenerateCodeIndexedReportsUnmappedFactory(t *testing.T) {
	scene := NewGedScene()
	scene.SetFormTitle("Unmapped")
	scene.SetSize(100, 70)
	addFakeTo(t, scene, "gui.Button", "btnOK")
	addFakeTo(t, scene, "gui.Separator", "sep1")

	gen := scene.GenerateCodeIndexed(CodeGenOptions{PackageName: "main", TypeName: "UnmappedUI"})
	if gen.Err == nil {
		t.Fatalf("no error for an unmapped factory\n----\n%s", gen.Code)
	}
	msg := gen.Err.Error()
	if !strings.Contains(msg, "sep1") {
		t.Errorf("error does not name the widget: %q", msg)
	}
	if !strings.Contains(msg, "gui.Separator") {
		t.Errorf("error does not name the factory: %q", msg)
	}
	if strings.Contains(msg, "btnOK") {
		t.Errorf("error names a widget that generated fine: %q", msg)
	}

	// The degraded code is still produced, unchanged: GenerateCode's contract
	// does not move, only the extra diagnosis alongside it is new.
	if !strings.Contains(gen.Code, `core.New("gui.Separator")`) {
		t.Errorf("degraded output missing, GenerateCode behaviour changed\n----\n%s", gen.Code)
	}
}

// TestGenerateCodeIndexedCleanDesignHasNoError guards the other direction: a
// design made only of mapped factories must not raise an error, or the view
// would refuse to show anything.
func TestGenerateCodeIndexedCleanDesignHasNoError(t *testing.T) {
	scene := NewGedScene()
	scene.SetFormTitle("Clean")
	scene.SetSize(100, 70)
	addFakeTo(t, scene, "gui.Button", "btnOK")
	addFakeTo(t, scene, "gui.Label", "lbl")

	if err := scene.GenerateCodeIndexed(CodeGenOptions{PackageName: "main"}).Err; err != nil {
		t.Errorf("unexpected error for a fully mapped design: %v", err)
	}
}
