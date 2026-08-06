package ged

import (
	"reflect"
	"testing"

	"github.com/uk0/silk/gui"
)

// TestMorphCandidatesStayInPaletteCategory pins decision 1: the 变形为 menu
// offers the other placeable widgets of the clicked widget's own palette
// category — never the widget itself, never a widget from another category,
// and nothing for a factory the palette does not categorise.
func TestMorphCandidatesStayInPaletteCategory(t *testing.T) {
	got := morphCandidates("gui.Button")
	for _, want := range []string{"gui.Edit", "gui.CheckBox", "gui.Slider"} {
		if !containsString(got, want) {
			t.Errorf("morphCandidates(gui.Button) = %v, missing same-category %s", got, want)
		}
	}
	if containsString(got, "gui.Button") {
		t.Errorf("morphCandidates(gui.Button) offers the widget itself: %v", got)
	}
	for _, unwanted := range []string{"gui.Label", "gui.VBox", "gui.Tank"} {
		if containsString(got, unwanted) {
			t.Errorf("morphCandidates(gui.Button) leaked out-of-category %s: %v", unwanted, got)
		}
	}

	// A SCADA widget only ever offers SCADA widgets.
	scada := morphCandidates("gui.Tank")
	if !containsString(scada, "gui.Valve") || containsString(scada, "gui.Button") {
		t.Errorf("morphCandidates(gui.Tank) = %v, want SCADA siblings only", scada)
	}

	// gui.ScrollArea is placeable-opted-out, so it has no palette category and
	// no candidates; an unregistered name likewise.
	if c := morphCandidates("gui.ScrollArea"); len(c) != 0 {
		t.Errorf("morphCandidates(gui.ScrollArea) = %v, want none", c)
	}
	if c := morphCandidates("nope.Widget"); len(c) != 0 {
		t.Errorf("morphCandidates(nope.Widget) = %v, want none", c)
	}
}

// TestMorphSurvivingPropsFiltersByIdAndType pins decision 2/4 on the pure
// half: a captured "props" block survives only where both types register the
// same id at the same setter type.
func TestMorphSurvivingPropsFiltersByIdAndType(t *testing.T) {
	old := map[string]string{
		"文本": `"确定"`,
		"可见": "false",
		"数量": "7",
	}
	oldSetters := map[string]reflect.Type{
		"文本": reflect.TypeOf(""),
		"可见": reflect.TypeOf(false),
		"数量": reflect.TypeOf(0),
	}
	newSetters := map[string]reflect.Type{
		"文本": reflect.TypeOf(""),
		"数量": reflect.TypeOf(false), // same id, different type
		"选中": reflect.TypeOf(false),
	}

	kept := morphSurvivingProps(old, oldSetters, newSetters)
	if kept["文本"] != `"确定"` {
		t.Errorf("shared id 文本 = %q, want it carried over", kept["文本"])
	}
	if _, ok := kept["可见"]; ok {
		t.Errorf("id the new type does not register survived: %v", kept)
	}
	if _, ok := kept["数量"]; ok {
		t.Errorf("type-mismatched id survived: %v", kept)
	}
	if _, ok := kept["选中"]; ok {
		t.Errorf("id absent from the old props was invented: %v", kept)
	}
}

// TestPropertySetterTypesReadsRealWidget: the id→setter-type table the survival
// filter runs on comes from the widget itself, so a widget that stops
// registering a property stops accepting it on morph without any list edit.
func TestPropertySetterTypesReadsRealWidget(t *testing.T) {
	types := propertySetterTypes(gui.NewCheckBox())
	if got := types["文本"]; got == nil || got.Kind() != reflect.String {
		t.Errorf("CheckBox 文本 setter type = %v, want string", got)
	}
	if got := types["选中"]; got == nil || got.Kind() != reflect.Bool {
		t.Errorf("CheckBox 选中 setter type = %v, want bool", got)
	}
	if _, ok := types["可见"]; ok {
		t.Errorf("CheckBox does not register 可见, but it appeared: %v", types)
	}
}

// TestMorphSurvivingEventsFollowsCodegen pins decision 2's event half: a
// handler survives exactly when codegen can wire that event on the new type.
func TestMorphSurvivingEventsFollowsCodegen(t *testing.T) {
	// Slider → SpinBox: both widgets bind OnValueChanged, so the handler stays.
	kept, dropped := morphSurvivingEvents(map[string]string{"OnValueChanged": "onVal"}, "gui.SpinBox")
	if kept["OnValueChanged"] != "onVal" || dropped != 0 {
		t.Errorf("Slider→SpinBox kept=%v dropped=%d, want the handler carried over", kept, dropped)
	}

	// Button → CheckBox: a CheckBox has no OnClick binding, so it is dropped.
	kept, dropped = morphSurvivingEvents(map[string]string{"OnClick": "onClick"}, "gui.CheckBox")
	if len(kept) != 0 || dropped != 1 {
		t.Errorf("Button→CheckBox kept=%v dropped=%d, want the handler dropped", kept, dropped)
	}

	// Mixed: only the supported one survives, the count reports the rest.
	kept, dropped = morphSurvivingEvents(map[string]string{
		"OnClick":         "onClick",
		"OnValueChanged":  "onVal",
		"OnSomethingElse": "onX",
	}, "gui.Slider")
	if len(kept) != 1 || kept["OnValueChanged"] != "onVal" || dropped != 2 {
		t.Errorf("mixed→Slider kept=%v dropped=%d, want only OnValueChanged", kept, dropped)
	}
}

// TestMorphSelectionKeepsGeometryNameAndSharedProps is the end-to-end of
// decision 2: the widget keeps its place, its name and every property the new
// type also registers; a property only the old type had is gone.
func TestMorphSelectionKeepsGeometryNameAndSharedProps(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	fw := addFake(t, scene, "Btn1")
	fw.SetBounds(12, 20, 40, 9)
	btn, ok := fw.Widget().(*gui.Button)
	if !ok {
		t.Fatalf("fixture is not a gui.Button: %T", fw.Widget())
	}
	btn.SetText("确定")
	btn.SetEnabled(false)
	btn.SetVisible(false) // 可见 is a Button-only property id

	view.morphSelection(fw, "gui.CheckBox")

	if got := fw.WidgetFactoryName(); got != "gui.CheckBox" {
		t.Fatalf("factory after morph = %q, want gui.CheckBox", got)
	}
	if got := fw.WidgetName(); got != "Btn1" {
		t.Errorf("widget name after morph = %q, want Btn1", got)
	}
	x, y, w, h := fw.Bounds()
	if x != 12 || y != 20 || w != 40 || h != 9 {
		t.Errorf("bounds after morph = (%g,%g,%g,%g), want (12,20,40,9)", x, y, w, h)
	}
	cb, ok := fw.Widget().(*gui.CheckBox)
	if !ok {
		t.Fatalf("embedded widget after morph = %T, want *gui.CheckBox", fw.Widget())
	}
	if cb.Text() != "确定" {
		t.Errorf("文本 after morph = %q, want 确定 (shared id must survive)", cb.Text())
	}
	if cb.IsEnabled() {
		t.Errorf("可用 after morph = true, want false (shared id must survive)")
	}
	if !cb.IsVisible() {
		t.Errorf("可见 leaked onto a CheckBox, which does not register it")
	}
}

// TestMorphSelectionDropsUnsupportedEvents pins the event half end-to-end plus
// the drop count the status message reports.
func TestMorphSelectionDropsUnsupportedEvents(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	fw := addFake(t, scene, "Btn1")
	fw.SetEventHandler("OnClick", "onClick")
	view.morphSelection(fw, "gui.CheckBox")
	if len(fw.EventHandlers()) != 0 {
		t.Errorf("handlers after Button→CheckBox = %v, want none", fw.EventHandlers())
	}

	sl, err := NewFakeWidgetFromFactory("gui.Slider")
	if err != nil {
		t.Fatalf("create slider: %v", err)
	}
	sl.SetWidgetName("Sl")
	sl.SetBounds(0, 0, 30, 8)
	sl.SetParent(scene)
	sl.SetEventHandler("OnValueChanged", "onVal")
	view.morphSelection(sl, "gui.SpinBox")
	if sl.EventHandlers()["OnValueChanged"] != "onVal" {
		t.Errorf("handlers after Slider→SpinBox = %v, want OnValueChanged kept", sl.EventHandlers())
	}
}

// TestMorphSelectionUndoRedo pins decision 3: one undoable step, and undoing
// it restores the old factory with the full old props and event bindings —
// including the ones the morph dropped.
func TestMorphSelectionUndoRedo(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	fw := addFake(t, scene, "Btn1")
	btn := fw.Widget().(*gui.Button)
	btn.SetText("确定")
	btn.SetVisible(false)
	fw.SetEventHandler("OnClick", "onClick")

	view.morphSelection(fw, "gui.CheckBox")
	if fw.WidgetFactoryName() != "gui.CheckBox" {
		t.Fatalf("morph did not apply: %s", fw.WidgetFactoryName())
	}

	scene.UndoStack().Undo()
	if got := fw.WidgetFactoryName(); got != "gui.Button" {
		t.Fatalf("factory after Undo = %q, want gui.Button", got)
	}
	back, ok := fw.Widget().(*gui.Button)
	if !ok {
		t.Fatalf("embedded widget after Undo = %T, want *gui.Button", fw.Widget())
	}
	if back.Text() != "确定" {
		t.Errorf("文本 after Undo = %q, want 确定", back.Text())
	}
	if back.IsVisible() {
		t.Errorf("可见 after Undo = true, want the dropped property restored to false")
	}
	if fw.EventHandlers()["OnClick"] != "onClick" {
		t.Errorf("handlers after Undo = %v, want the dropped OnClick restored", fw.EventHandlers())
	}

	scene.UndoStack().Redo()
	if got := fw.WidgetFactoryName(); got != "gui.CheckBox" {
		t.Errorf("factory after Redo = %q, want gui.CheckBox", got)
	}
	if len(fw.EventHandlers()) != 0 {
		t.Errorf("handlers after Redo = %v, want none", fw.EventHandlers())
	}
}

// TestMorphSelectionRejectsNonCandidates: a morph to the same type or to a
// factory the designer cannot place must not reach the undo stack.
func TestMorphSelectionRejectsNonCandidates(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	fw := addFake(t, scene, "Btn1")

	view.morphSelection(fw, "gui.Button")
	view.morphSelection(fw, "ged.TerminalPanel")
	view.morphSelection(fw, "nope.Widget")

	if scene.UndoStack().CanUndo() {
		t.Errorf("a rejected morph pushed a command onto the undo stack")
	}
	if fw.WidgetFactoryName() != "gui.Button" {
		t.Errorf("factory = %q, want the widget untouched", fw.WidgetFactoryName())
	}
}
