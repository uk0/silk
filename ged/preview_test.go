package ged

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/uk0/silk/gui"
)

// designFingerprint is a design's serialized form — the lines Save would write
// — as a sorted multiset. Sorted because FakeWidget.SaveDesign emits the "props"
// and "events" blocks in Go map order, so two saves of the SAME unmodified
// design differ in line order; every line still carries its own indentation, so
// a widget that moved, changed value or vanished still changes the multiset.
func designFingerprint(scene *GedScene) []string {
	lines := strings.Split(scene.SaveDesign().String(), "\n")
	sort.Strings(lines)
	return lines
}

// TestPreviewBuildMatchesDesign is the fidelity proof the 预览 rests on: the
// runtime tree scene.Generate hands the preview window must have the same shape
// and the same configured values as the design, and building it must leave the
// design itself untouched.
func TestPreviewBuildMatchesDesign(t *testing.T) {
	scene := NewGedScene()
	scene.SetFormTitle("面板")
	scene.SetSize(200, 150)

	box := sceneWidget(t, scene, "gui.VBox", "box", 10, 10, 60, 40)
	child, err := NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatalf("create gui.Button: %v", err)
	}
	child.SetWidgetName("ok")
	child.SetBounds(12, 14, 30, 8)
	child.SetParent(box)
	child.Widget().(*gui.Button).SetText("确定")
	child.SetEventHandler("Click", "onOkClick")

	before := designFingerprint(scene)
	design := scene.Generate()
	after := designFingerprint(scene)

	if !reflect.DeepEqual(before, after) {
		t.Errorf("building the preview mutated the design\n--- before ---\n%s\n--- after ---\n%s",
			strings.Join(before, "\n"), strings.Join(after, "\n"))
	}
	if design == nil {
		t.Fatal("scene.Generate returned nil")
	}
	if design.Form() == nil {
		t.Fatal("generated design has no form")
	}

	runtimeBox := design.Widget("box")
	if runtimeBox == nil {
		t.Fatal("container missing from the generated design index")
	}
	kids := runtimeBox.Children()
	if len(kids) != 1 {
		t.Fatalf("runtime container has %d children, want 1", len(kids))
	}
	btn, ok := kids[0].(*gui.Button)
	if !ok {
		t.Fatalf("runtime child is %T, want *gui.Button", kids[0])
	}
	if btn.Text() != "确定" {
		t.Errorf("runtime button text = %q, want %q", btn.Text(), "确定")
	}
	// The design widget must still hold its own state — the runtime widget is a
	// separate object, not a view onto it.
	if got := child.Widget().(*gui.Button).Text(); got != "确定" {
		t.Errorf("design button text = %q after generate, want %q", got, "确定")
	}
	if runtimeBox == child.Widget() || btn == child.Widget() {
		t.Error("runtime tree shares widget instances with the design")
	}
}

// TestPreviewShowsDesignTimeTagValue: a tag-bound widget previews at its
// design-time value. The preview starts no drivers, so this value is all it can
// ever show — the title marker (see below) is what stops it reading as live.
func TestPreviewShowsDesignTimeTagValue(t *testing.T) {
	scene := NewGedScene()
	scene.SetSize(200, 150)
	tank := sceneWidget(t, scene, "gui.Tank", "t1", 10, 10, 30, 40)
	designed := tank.Widget().(*gui.Tank)
	designed.SetTagName("level")
	designed.SetLevel(0.42)

	live, ok := scene.Generate().Widget("t1").(*gui.Tank)
	if !ok {
		t.Fatal("tank missing from the generated design index")
	}
	if live.Level() != designed.Level() {
		t.Errorf("runtime tank level = %v, want %v (design-time value)", live.Level(), designed.Level())
	}
	if live.TagName() != "level" {
		t.Errorf("runtime tank tag = %q, want %q", live.TagName(), "level")
	}
}

func TestPreviewTitle(t *testing.T) {
	cases := []struct {
		formTitle string
		unbound   bool
		want      string
	}{
		{"面板", false, "预览 - 面板"},
		{"面板", true, "预览 - 面板 (未连接数据)"},
		{"", false, "预览"},
		{"", true, "预览 (未连接数据)"},
	}
	for _, c := range cases {
		if got := previewTitle(c.formTitle, c.unbound); got != c.want {
			t.Errorf("previewTitle(%q, %v) = %q, want %q", c.formTitle, c.unbound, got, c.want)
		}
	}
}

// TestPreviewTitleMarksTagBoundDesign: a design whose widgets are bound to tags
// previews without a backend behind it, so the window says so. A design with no
// bindings has nothing to warn about and keeps the plain title.
func TestPreviewTitleMarksTagBoundDesign(t *testing.T) {
	bound := NewGedScene()
	bound.SetFormTitle("水箱")
	bound.SetSize(200, 150)
	tank := sceneWidget(t, bound, "gui.Tank", "t1", 10, 10, 30, 40)
	tank.Widget().(*gui.Tank).SetTagName("level")

	preview, blocked := BuildPreview(bound)
	if len(blocked) > 0 || preview == nil {
		t.Fatalf("BuildPreview(bound) = %v, %v; want a preview", preview, blocked)
	}
	if got := preview.Title(); got != "预览 - 水箱 (未连接数据)" {
		t.Errorf("bound design preview title = %q, want %q", got, "预览 - 水箱 (未连接数据)")
	}

	plain := NewGedScene()
	plain.SetFormTitle("水箱")
	plain.SetSize(200, 150)
	sceneWidget(t, plain, "gui.Tank", "t1", 10, 10, 30, 40)

	preview, blocked = BuildPreview(plain)
	if len(blocked) > 0 || preview == nil {
		t.Fatalf("BuildPreview(plain) = %v, %v; want a preview", preview, blocked)
	}
	if got := preview.Title(); got != "预览 - 水箱" {
		t.Errorf("unbound design preview title = %q, want %q", got, "预览 - 水箱")
	}
}

// unregisteredWidget stands in for a widget whose factory the running program
// does not know. FakeWidget.Generate returns nil for one of these and drops it —
// and its whole subtree — from the runtime tree without a word.
type unregisteredWidget struct {
	gui.Widget
}

// TestPreviewRefusesUnbuildableWidget: rather than open a window quietly missing
// part of the design, BuildPreview refuses and names what it could not build.
func TestPreviewRefusesUnbuildableWidget(t *testing.T) {
	scene := NewGedScene()
	scene.SetFormTitle("面板")
	scene.SetSize(200, 150)
	sceneWidget(t, scene, "gui.Button", "ok", 10, 10, 30, 8)
	broken := sceneWidget(t, scene, "gui.Button", "ghost", 10, 30, 30, 8)
	broken.SetWidget(&unregisteredWidget{})

	preview, blocked := BuildPreview(scene)
	if preview != nil {
		t.Error("BuildPreview opened a preview for a design it cannot build in full")
	}
	if len(blocked) != 1 {
		t.Fatalf("blocked = %v, want exactly the one unbuildable widget", blocked)
	}
	if blocked[0] != "ghost (未知类型)" {
		t.Errorf("blocked[0] = %q, want %q", blocked[0], "ghost (未知类型)")
	}
}

// TestPreviewRefusesUnbuildableNestedWidget: the same guard reaches into
// containers, because Generate recurses into them.
func TestPreviewRefusesUnbuildableNestedWidget(t *testing.T) {
	scene := NewGedScene()
	scene.SetSize(200, 150)
	box := sceneWidget(t, scene, "gui.VBox", "box", 10, 10, 60, 40)
	child, err := NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatalf("create gui.Button: %v", err)
	}
	child.SetWidgetName("ghost")
	child.SetParent(box)
	child.SetWidget(&unregisteredWidget{})

	preview, blocked := BuildPreview(scene)
	if preview != nil {
		t.Error("BuildPreview opened a preview whose container is quietly missing a child")
	}
	if len(blocked) != 1 || !strings.Contains(blocked[0], "ghost") {
		t.Fatalf("blocked = %v, want the nested widget named", blocked)
	}
}

// TestPreviewEscapeRestoresDesignerFocus: Esc closes the preview and hands the
// keyboard back. Key events go to one global focus widget, so a preview that
// keeps focus after closing leaves the designer unable to receive a keystroke.
func TestPreviewEscapeRestoresDesignerFocus(t *testing.T) {
	scene := NewGedScene()
	scene.SetFormTitle("面板")
	scene.SetSize(200, 150)
	sceneWidget(t, scene, "gui.Button", "ok", 10, 10, 30, 8)

	preview, blocked := BuildPreview(scene)
	if preview == nil || len(blocked) > 0 {
		t.Fatalf("BuildPreview = %v, %v; want a preview", preview, blocked)
	}

	designer := gui.NewLabel("designer")
	preview.RestoreFocusTo(designer)
	preview.SetFocus()
	if designer.HasFocus() {
		t.Fatal("designer still holds focus while the preview is up")
	}

	preview.OnKeyDown(gui.KeyEsc, false)
	if !designer.HasFocus() {
		t.Error("after Esc the designer did not get keyboard focus back")
	}
}

// TestPreviewIgnoresOtherKeys: only Esc closes the preview — every other key
// belongs to the previewed form.
func TestPreviewIgnoresOtherKeys(t *testing.T) {
	scene := NewGedScene()
	scene.SetSize(200, 150)

	preview, _ := BuildPreview(scene)
	if preview == nil {
		t.Fatal("BuildPreview returned no preview")
	}
	designer := gui.NewLabel("designer")
	preview.RestoreFocusTo(designer)
	preview.SetFocus()

	preview.OnKeyDown(gui.KeyEnter, false)
	if designer.HasFocus() {
		t.Error("Enter closed the preview; only Esc may")
	}
}

// TestPreviewSizeFloor: a form too small to carry a title bar and borders still
// opens at a usable size.
func TestPreviewSizeFloor(t *testing.T) {
	if w, h := previewSize(100, 80); w != 320 || h != 240 {
		t.Errorf("previewSize(100, 80) = %v, %v; want 320, 240", w, h)
	}
	if w, h := previewSize(800, 600); w != 800 || h != 600 {
		t.Errorf("previewSize(800, 600) = %v, %v; want it left alone", w, h)
	}
}
