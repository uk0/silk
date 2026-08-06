package prop

import (
	"testing"

	"github.com/uk0/silk/paint"
)

var (
	blue = paint.Color{R: 33, G: 150, B: 243, A: 255}
	red  = paint.Color{R: 255, G: 0, B: 0, A: 255}
	gray = paint.Color{R: 128, G: 128, B: 128, A: 255}
)

// pick performs the write gui.ColorPicker.OnLeftDown makes when a palette
// swatch is clicked. Going through the picker (rather than calling the item)
// is the point: it is the seam the editor listens on.
func pick(edit *ColorEdit, c paint.Color) {
	edit.picker.SetColor(c)
}

// TestColorPropertyGetsColorEditor pins the dispatch. paint.Color is a struct,
// so defaultControlType's kind switch handed it to TextEdit — which renders the
// value through core.VisualString but commits through core.PersistSscan, and
// fmt has no scanner for a struct. Every edit failed with
// "can't scan type: *paint.Color", i.e. colors were read-only in the designer.
func TestColorPropertyGetsColorEditor(t *testing.T) {
	sheet := newTestSheet()

	c := blue
	item, _ := sheet.AddProperty("颜色", func() paint.Color { return c }, func(v paint.Color) { c = v })
	if item == nil {
		t.Fatal(`AddProperty("颜色") returned nil item`)
	}
	if got := item.defaultControlType(); got != "ColorEdit" {
		t.Errorf("defaultControlType() = %q, want %q", got, "ColorEdit")
	}
	edit, ok := item.Control().(*ColorEdit)
	if !ok {
		t.Fatalf("color property got a %T control, want *prop.ColorEdit", item.Control())
	}
	if edit.Color() != blue {
		t.Errorf("editor shows %v, want the property's value %v", edit.Color(), blue)
	}
	if !edit.IsEnabled() {
		t.Error("writable color property got a disabled editor")
	}
}

// TestColorEditPickWritesSelection is the edit leg: a pick reaches every
// selected object, as one undo step, and undo restores each object's own color.
func TestColorEditPickWritesSelection(t *testing.T) {
	ac, bc := blue, blue
	a := &fakeObj{enum: func(l IPropertyList) {
		l.AddProperty("颜色", func() paint.Color { return ac }, func(v paint.Color) { ac = v })
	}}
	b := &fakeObj{enum: func(l IPropertyList) {
		l.AddProperty("颜色", func() paint.Color { return bc }, func(v paint.Color) { bc = v })
	}}

	owner := &recordingOwner{}
	sheet := bindSheet([]interface{}{a, b}, owner)
	item := sheet.propMap["颜色"]
	if item == nil {
		t.Fatal(`shared property "颜色" is missing from the sheet`)
	}
	edit, ok := item.Control().(*ColorEdit)
	if !ok {
		t.Fatalf("control is %T, want *ColorEdit", item.Control())
	}

	pick(edit, red)
	if ac != red || bc != red {
		t.Fatalf("after a pick ac = %v, bc = %v, want both %v", ac, bc, red)
	}
	if len(owner.cmds) != 1 {
		t.Fatalf("pushed %d commands, want 1: one pick is one undo step", len(owner.cmds))
	}

	owner.cmds[0].Undo()
	if ac != blue || bc != blue {
		t.Errorf("after undo ac = %v, bc = %v, want both %v", ac, bc, blue)
	}
}

// TestColorEditRefreshIsNotAnEdit covers the trap the picker's callback
// creates: UpdateValue pushes the property's value into the picker through the
// same SetColor a user pick arrives on. Refreshing a row must not write it.
func TestColorEditRefreshIsNotAnEdit(t *testing.T) {
	ac, bc := blue, gray
	a := &fakeObj{enum: func(l IPropertyList) {
		l.AddProperty("颜色", func() paint.Color { return ac }, func(v paint.Color) { ac = v })
	}}
	b := &fakeObj{enum: func(l IPropertyList) {
		l.AddProperty("颜色", func() paint.Color { return bc }, func(v paint.Color) { bc = v })
	}}

	owner := &recordingOwner{}
	sheet := bindSheet([]interface{}{a, b}, owner)
	item := sheet.propMap["颜色"]
	if item == nil {
		t.Fatal(`shared property "颜色" is missing from the sheet`)
	}
	edit, ok := item.Control().(*ColorEdit)
	if !ok {
		t.Fatalf("control is %T, want *ColorEdit", item.Control())
	}

	edit.UpdateValue()
	if ac != blue || bc != gray {
		t.Errorf("a refresh changed the objects: ac = %v, bc = %v, want %v and %v", ac, bc, blue, gray)
	}
	if len(owner.cmds) != 0 {
		t.Errorf("a refresh pushed %d undo commands, want 0", len(owner.cmds))
	}
	if !item.IsMultiValue() {
		t.Error("a refresh cleared the 多值 state it was rendering")
	}
}

// TestColorEditShowsMultiValueHint covers the intersection rule for the new
// kind: disagreeing objects must not show one object's color, and the row says
// 多值 until something is picked.
func TestColorEditShowsMultiValueHint(t *testing.T) {
	ac, bc := blue, gray
	a := &fakeObj{enum: func(l IPropertyList) {
		l.AddProperty("颜色", func() paint.Color { return ac }, func(v paint.Color) { ac = v })
	}}
	b := &fakeObj{enum: func(l IPropertyList) {
		l.AddProperty("颜色", func() paint.Color { return bc }, func(v paint.Color) { bc = v })
	}}

	sheet := bindSheet([]interface{}{a, b}, &recordingOwner{})
	item := sheet.propMap["颜色"]
	if item == nil {
		t.Fatal(`shared property "颜色" is missing from the sheet`)
	}
	if !item.IsMultiValue() {
		t.Fatal("differing colors did not produce a 多值 row")
	}
	edit, ok := item.Control().(*ColorEdit)
	if !ok {
		t.Fatalf("control is %T, want *ColorEdit", item.Control())
	}
	if got := item.MultiValueHint(); got != multiValueText {
		t.Errorf("hint = %q, want %q", got, multiValueText)
	}
	if edit.Color() != (paint.Color{}) {
		t.Errorf("多值 editor shows %v, want the zero color: it must not show one object's value", edit.Color())
	}

	pick(edit, red)
	if ac != red || bc != red {
		t.Errorf("after picking on a 多值 row ac = %v, bc = %v, want both %v", ac, bc, red)
	}
	if got := item.MultiValueHint(); got != "" {
		t.Errorf("hint = %q after the edit, want it cleared", got)
	}
}

// TestColorEditDropsRowNotSharedBySelection is the control case: a color only
// one selected object owns cannot speak for the selection, so the row goes.
func TestColorEditDropsRowNotSharedBySelection(t *testing.T) {
	ac := blue
	a := &fakeObj{enum: func(l IPropertyList) {
		l.AddProperty("颜色", func() paint.Color { return ac }, func(v paint.Color) { ac = v })
	}}
	b := &fakeObj{enum: func(l IPropertyList) {}}

	sheet := bindSheet([]interface{}{a, b}, &recordingOwner{})
	if len(rowIDs(sheet)) != 0 {
		t.Errorf("rows = %v, want []: a color only one object has is not editable for the selection", rowIDs(sheet))
	}
}

// TestReadOnlyColorGetsDisabledEditor mirrors the bool row's rule: the sheet
// drops writes to a read-only property, so the editor must not look openable —
// otherwise it shows a color the object never took.
func TestReadOnlyColorGetsDisabledEditor(t *testing.T) {
	sheet := newTestSheet()

	c := blue
	ro, _ := sheet.AddProperty("只读颜色", func() paint.Color { return c }, nil)
	if ro == nil {
		t.Fatal("AddProperty returned a nil item for a color property")
	}
	edit, ok := ro.Control().(*ColorEdit)
	if !ok {
		t.Fatalf("read-only color property got a %T control, want *prop.ColorEdit", ro.Control())
	}
	if edit.IsEnabled() {
		t.Error("read-only color property got an enabled editor")
	}
	pick(edit, red)
	if c != blue {
		t.Errorf("a pick reached a read-only property: value = %v, want %v", c, blue)
	}
}
