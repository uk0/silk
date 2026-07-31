package prop

import "testing"

// TestSetValueStrKeepsWholeText guards the commit path of every text row in the
// sheet. The sheet renders values with core.VisualString (plain fmt.Sprint, no
// quotes) but committed them with core.PersistSscan, whose string scanner reads
// a quoted Go literal and otherwise stops at the first space: typing
// "Hello World" into a text property stored "Hello", and emptying the box
// failed with a scan error so the property could never be cleared.
func TestSetValueStrKeepsWholeText(t *testing.T) {
	sheet := newTestSheet()

	value := "start"
	item, _ := sheet.AddProperty("文本", func() string { return value }, func(s string) { value = s })
	if item == nil {
		t.Fatal(`AddProperty("文本") returned nil item`)
	}

	if err := item.SetValueStr("Hello World"); err != nil {
		t.Fatalf(`SetValueStr("Hello World") = %v, want nil`, err)
	}
	if value != "Hello World" {
		t.Errorf("text was truncated at the space: value = %q, want %q", value, "Hello World")
	}

	if err := item.SetValueStr(""); err != nil {
		t.Fatalf(`SetValueStr("") = %v, want nil`, err)
	}
	if value != "" {
		t.Errorf("clearing the box did not clear the property: value = %q", value)
	}

	// Quotes are now content, not syntax: what the sheet shows is what it stores.
	if err := item.SetValueStr(`say "hi"`); err != nil {
		t.Fatalf(`SetValueStr("say \"hi\"") = %v, want nil`, err)
	}
	if value != `say "hi"` {
		t.Errorf("quoted text round-trip: value = %q, want %q", value, `say "hi"`)
	}

	// Property.SetValueStr is the same commit path one level down; it carried
	// the identical bug and must not be left behind.
	value = "start"
	if err := item.prop.SetValueStr("Hello World"); err != nil {
		t.Fatalf(`Property.SetValueStr("Hello World") = %v, want nil`, err)
	}
	if value != "Hello World" {
		t.Errorf("Property.SetValueStr truncated: value = %q, want %q", value, "Hello World")
	}
}

// TestSetValueStrRejectsNonNumericText keeps the string fix from spilling into
// the other types: a numeric row must still refuse garbage instead of storing
// it, so the sheet reports an error and leaves the old value in place.
func TestSetValueStrRejectsNonNumericText(t *testing.T) {
	sheet := newTestSheet()

	n := 42
	item, _ := sheet.AddProperty("宽度", func() int { return n }, func(v int) { n = v })
	if item == nil {
		t.Fatal(`AddProperty("宽度") returned nil item`)
	}

	if err := item.SetValueStr("abc"); err == nil {
		t.Error(`SetValueStr("abc") on an int property = nil, want an error`)
	}
	if n != 42 {
		t.Errorf("garbage reached an int property: value = %d, want 42", n)
	}

	if err := item.SetValueStr("7"); err != nil {
		t.Fatalf(`SetValueStr("7") = %v, want nil`, err)
	}
	if n != 7 {
		t.Errorf("valid number did not commit: value = %d, want 7", n)
	}
}

// TestReadOnlyBoolGetsDisabledCheckBox pins the read-only bool row. The check
// box used to be created enabled whatever the property said, so a click flipped
// the tick while PropertyItem.SetValue silently dropped the write — the sheet
// then showed a value the object never took, and nothing refreshed it back.
func TestReadOnlyBoolGetsDisabledCheckBox(t *testing.T) {
	sheet := newTestSheet()

	on := true
	ro, _ := sheet.AddProperty("只读开关", func() bool { return on }, nil)
	rw, _ := sheet.AddProperty("开关", func() bool { return on }, func(b bool) { on = b })
	if ro == nil || rw == nil {
		t.Fatal("AddProperty returned a nil item for a bool property")
	}

	roBox, ok := ro.Control().(*CheckBox)
	if !ok {
		t.Fatalf("read-only bool property got a %T control, want *prop.CheckBox", ro.Control())
	}
	if roBox.IsEnabled() {
		t.Error("read-only bool property got an enabled check box")
	}

	rwBox, ok := rw.Control().(*CheckBox)
	if !ok {
		t.Fatalf("writable bool property got a %T control, want *prop.CheckBox", rw.Control())
	}
	if !rwBox.IsEnabled() {
		t.Error("writable bool property got a disabled check box")
	}
	if !rwBox.IsChecked() {
		t.Error("check box did not take the property's current value")
	}
}
