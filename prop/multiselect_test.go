package prop

import (
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

// TestClassifyRow drives the intersection/多值 rule directly, without a sheet:
// disjoint sets, one id declared with two types, agreeing values and
// disagreeing values, plus the single-object case that must stay as it was.
func TestClassifyRow(t *testing.T) {
	cases := []struct {
		name      string
		objCount  int
		owners    int
		mixedType bool
		values    []interface{}
		want      rowState
	}{
		{"one object keeps its row", 1, 1, false, []interface{}{"红"}, rowSame},
		{"one object keeps a row it declared twice", 1, 1, true, []interface{}{"红"}, rowSame},
		{"disjoint: only one object has it", 2, 1, false, []interface{}{"红"}, rowDrop},
		{"disjoint: none of a larger selection", 3, 2, false, []interface{}{"红", "红"}, rowDrop},
		{"two types under one id", 2, 2, true, []interface{}{"红", "红"}, rowDrop},
		{"identical values", 2, 2, false, []interface{}{"红", "红"}, rowSame},
		{"differing values", 2, 2, false, []interface{}{"红", "蓝"}, rowMulti},
		{"differing late in the selection", 3, 3, false, []interface{}{7, 7, 9}, rowMulti},
		{"identical zero values", 2, 2, false, []interface{}{0, 0}, rowSame},
	}

	for _, c := range cases {
		got := classifyRow(c.objCount, c.owners, c.mixedType, c.values)
		if got != c.want {
			t.Errorf("%s: classifyRow(%d, %d, %v, %v) = %v, want %v",
				c.name, c.objCount, c.owners, c.mixedType, c.values, got, c.want)
		}
	}
}

// TestCategoryHiddenForSelection pins which categories are single-selection
// only. Everything else must survive a multi-object bind.
func TestCategoryHiddenForSelection(t *testing.T) {
	cases := []struct {
		catKey   string
		objCount int
		want     bool
	}{
		{"events", 0, false},
		{"events", 1, false},
		{"events", 2, true},
		{"events", 9, true},
		{"layout", 2, false},
		{"appearance", 2, false},
		{"behavior", 2, false},
		{"general", 2, false},
	}

	for _, c := range cases {
		if got := categoryHiddenForSelection(c.catKey, c.objCount); got != c.want {
			t.Errorf("categoryHiddenForSelection(%q, %d) = %v, want %v",
				c.catKey, c.objCount, got, c.want)
		}
	}
}

// fakeObj stands in for a selected canvas item: it enumerates exactly the
// properties the test hands it, so a selection is just a slice of these.
type fakeObj struct {
	enum func(list IPropertyList)
}

func (o *fakeObj) EnumProperties(list IPropertyList) { o.enum(list) }

// recordingOwner is a bind owner with the PushCommand method PropertyItem looks
// for. Push applies the command exactly like gui.UndoStack.Push does (it calls
// Redo), so a test sees the same before/after state the real stack produces.
type recordingOwner struct {
	cmds []gui.ICommand
}

func (o *recordingOwner) PushCommand(cmd gui.ICommand) {
	o.cmds = append(o.cmds, cmd)
	cmd.Redo()
}

// bindSheet drives the real Bind path with an in-memory "default" config so the
// property-config lookup never reaches the resource directory.
func bindSheet(objs []interface{}, owner interface{}) *PropertySheet {
	configFileCache["default"] = &PropertyConfigFile{name: "default", doc: core.NewTDoc()}
	sheet := NewPropertySheet()
	sheet.Bind(objs, "default", owner)
	return sheet
}

// rowIDs lists the property ids the sheet kept, in row order.
func rowIDs(sheet *PropertySheet) []string {
	ids := make([]string, 0, len(sheet.rlist))
	for _, p := range sheet.rlist {
		ids = append(ids, p.Id())
	}
	return ids
}

// TestBindKeepsOnlySharedProperties covers the disjoint case: a property only
// one of the selected objects owns cannot be edited "for the selection", so it
// must not appear at all. Editing it would silently touch a single object while
// the sheet claims to speak for both.
func TestBindKeepsOnlySharedProperties(t *testing.T) {
	ax, ay, bx := 1, 2, 1
	a := &fakeObj{enum: func(l IPropertyList) {
		l.AddProperty("x", func() int { return ax }, func(v int) { ax = v })
		l.AddProperty("y", func() int { return ay }, func(v int) { ay = v })
	}}
	b := &fakeObj{enum: func(l IPropertyList) {
		l.AddProperty("x", func() int { return bx }, func(v int) { bx = v })
	}}

	sheet := bindSheet([]interface{}{a, b}, &recordingOwner{})
	if !equalStrs(rowIDs(sheet), []string{"x"}) {
		t.Errorf("rows = %v, want [x]: only the shared property may survive", rowIDs(sheet))
	}
}

// TestBindDropsMixedTypeProperty covers one id declared with two different
// value types. The merged row can only carry one type, so the other object's
// setter is dropped and an edit would reach half the selection.
func TestBindDropsMixedTypeProperty(t *testing.T) {
	an, bs := 3, "three"
	a := &fakeObj{enum: func(l IPropertyList) {
		l.AddProperty("v", func() int { return an }, func(x int) { an = x })
	}}
	b := &fakeObj{enum: func(l IPropertyList) {
		l.AddProperty("v", func() string { return bs }, func(x string) { bs = x })
	}}

	sheet := bindSheet([]interface{}{a, b}, &recordingOwner{})
	if len(rowIDs(sheet)) != 0 {
		t.Errorf("rows = %v, want []: an id with two value types is not editable for the selection", rowIDs(sheet))
	}
}

// TestBindMultiValueRowIsEmpty pins decision 2: when the selected objects hold
// different values the editor must come up empty rather than showing the first
// object's value, which reads as "already right".
func TestBindMultiValueRowIsEmpty(t *testing.T) {
	at, bt := "红", "蓝"
	a := &fakeObj{enum: func(l IPropertyList) {
		l.AddProperty("text", func() string { return at }, func(s string) { at = s })
	}}
	b := &fakeObj{enum: func(l IPropertyList) {
		l.AddProperty("text", func() string { return bt }, func(s string) { bt = s })
	}}

	sheet := bindSheet([]interface{}{a, b}, &recordingOwner{})
	item := sheet.propMap["text"]
	if item == nil {
		t.Fatal(`shared property "text" is missing from the sheet`)
	}
	if got := item.GetValueStr(); got != "" {
		t.Errorf("GetValueStr() = %q, want %q: differing values must not show one object's value", got, "")
	}
}

// TestBindUniformRowKeepsValue is the control case for the one above: agreeing
// objects still show their common value.
func TestBindUniformRowKeepsValue(t *testing.T) {
	at, bt := "红", "红"
	a := &fakeObj{enum: func(l IPropertyList) {
		l.AddProperty("text", func() string { return at }, func(s string) { at = s })
	}}
	b := &fakeObj{enum: func(l IPropertyList) {
		l.AddProperty("text", func() string { return bt }, func(s string) { bt = s })
	}}

	sheet := bindSheet([]interface{}{a, b}, &recordingOwner{})
	item := sheet.propMap["text"]
	if item == nil {
		t.Fatal(`shared property "text" is missing from the sheet`)
	}
	if got := item.GetValueStr(); got != "红" {
		t.Errorf("GetValueStr() = %q, want %q", got, "红")
	}
}

// TestEditWritesSelectionAsOneUndoStep pins decision 3 end to end: one edit is
// one command, it reaches every selected object, and undoing it puts each
// object back to *its own* previous value. The last part is what a single
// merged old value gets wrong — it stamps the first object's value over the
// rest of the selection.
func TestEditWritesSelectionAsOneUndoStep(t *testing.T) {
	at, bt := "红", "蓝"
	a := &fakeObj{enum: func(l IPropertyList) {
		l.AddProperty("text", func() string { return at }, func(s string) { at = s })
	}}
	b := &fakeObj{enum: func(l IPropertyList) {
		l.AddProperty("text", func() string { return bt }, func(s string) { bt = s })
	}}

	owner := &recordingOwner{}
	sheet := bindSheet([]interface{}{a, b}, owner)
	item := sheet.propMap["text"]
	if item == nil {
		t.Fatal(`shared property "text" is missing from the sheet`)
	}

	if err := item.SetValueStr("绿"); err != nil {
		t.Fatalf("SetValueStr: %v", err)
	}
	if at != "绿" || bt != "绿" {
		t.Fatalf("after edit at = %q, bt = %q, want both %q", at, bt, "绿")
	}
	if len(owner.cmds) != 1 {
		t.Fatalf("pushed %d commands, want 1: the selection edit is one undo step", len(owner.cmds))
	}

	owner.cmds[0].Undo()
	if at != "红" || bt != "蓝" {
		t.Errorf("after undo at = %q, bt = %q, want %q and %q: undo must restore each object's own value",
			at, bt, "红", "蓝")
	}
}

// TestTextEditKeepsMultiValueUntilTyped covers the trap the empty 多值 editor
// creates: the control commits on Deactivate, so merely visiting a row that
// came up empty would blank the value on every selected object.
func TestTextEditKeepsMultiValueUntilTyped(t *testing.T) {
	at, bt := "红", "蓝"
	a := &fakeObj{enum: func(l IPropertyList) {
		l.AddProperty("text", func() string { return at }, func(s string) { at = s })
	}}
	b := &fakeObj{enum: func(l IPropertyList) {
		l.AddProperty("text", func() string { return bt }, func(s string) { bt = s })
	}}

	sheet := bindSheet([]interface{}{a, b}, &recordingOwner{})
	item := sheet.propMap["text"]
	if item == nil {
		t.Fatal(`shared property "text" is missing from the sheet`)
	}
	edit, ok := item.Control().(*TextEdit)
	if !ok {
		t.Fatalf("control is %T, want *TextEdit", item.Control())
	}
	if got := edit.String(); got != "" {
		t.Fatalf("多值 editor shows %q, want it empty", got)
	}

	edit.Deactivate()
	if at != "红" || bt != "蓝" {
		t.Errorf("after leaving an untouched 多值 row at = %q, bt = %q, want %q and %q", at, bt, "红", "蓝")
	}

	// A typed value still commits to the whole selection.
	edit.SetText("绿")
	edit.Deactivate()
	if at != "绿" || bt != "绿" {
		t.Errorf("after typing at = %q, bt = %q, want both %q", at, bt, "绿")
	}
}

// TestCheckBoxShowsMultiValueHint covers the bool row: a checkbox has no empty
// state, so the disagreement has to be spelled out next to the box.
func TestCheckBoxShowsMultiValueHint(t *testing.T) {
	ae, be := true, false
	a := &fakeObj{enum: func(l IPropertyList) {
		l.AddProperty("enabled", func() bool { return ae }, func(v bool) { ae = v })
	}}
	b := &fakeObj{enum: func(l IPropertyList) {
		l.AddProperty("enabled", func() bool { return be }, func(v bool) { be = v })
	}}

	sheet := bindSheet([]interface{}{a, b}, &recordingOwner{})
	item := sheet.propMap["enabled"]
	if item == nil {
		t.Fatal(`shared property "enabled" is missing from the sheet`)
	}
	box, ok := item.Control().(*CheckBox)
	if !ok {
		t.Fatalf("control is %T, want *CheckBox", item.Control())
	}
	if got := box.Text(); got != multiValueText {
		t.Errorf("checkbox label = %q, want %q", got, multiValueText)
	}
	if box.IsChecked() {
		t.Error("多值 checkbox is ticked: it must not show the first object's value")
	}

	// Ticking it applies to the selection, and the row is no longer 多值.
	box.SetChecked(true)
	if !ae || !be {
		t.Errorf("after ticking ae = %v, be = %v, want both true", ae, be)
	}
	if got := box.Text(); got != "" {
		t.Errorf("checkbox label = %q after the edit, want it cleared", got)
	}
}

// TestMultiSelectHidesEventCategory pins decision 4: an event row binds one
// handler to one widget, so the 事件 category is single-selection only.
func TestMultiSelectHidesEventCategory(t *testing.T) {
	ah, bh, aw, bw := "onA", "onB", 10, 20
	a := &fakeObj{enum: func(l IPropertyList) {
		l.AddProperty("on_click", func() string { return ah }, func(s string) { ah = s })
		l.AddProperty("width", func() int { return aw }, func(v int) { aw = v })
	}}
	b := &fakeObj{enum: func(l IPropertyList) {
		l.AddProperty("on_click", func() string { return bh }, func(s string) { bh = s })
		l.AddProperty("width", func() int { return bw }, func(v int) { bw = v })
	}}

	sheet := bindSheet([]interface{}{a, b}, &recordingOwner{})
	headers, ids := layoutRows(sheet)
	for _, h := range headers {
		if h == "events" {
			t.Errorf("headers = %v: 事件 category must be hidden for a multi-object selection", headers)
		}
	}
	if !equalStrs(ids, []string{"width"}) {
		t.Errorf("rows = %v, want [width]", ids)
	}

	// A single object keeps its event rows.
	single := bindSheet([]interface{}{a}, &recordingOwner{})
	_, singleIDs := layoutRows(single)
	if !equalStrs(singleIDs, []string{"width", "on_click"}) {
		t.Errorf("single-selection rows = %v, want [width on_click]", singleIDs)
	}
}
