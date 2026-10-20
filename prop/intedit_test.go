package prop

import (
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

func TestIntegerControlFactoryAndTyping(t *testing.T) {
	if _, ok := core.New("prop.control.IntEdit").(*IntEdit); !ok {
		t.Fatal("integer editor factory is missing")
	}
	sheet := newTestSheet()
	v := int64(9007199254740993) // cannot round-trip through float64
	item, _ := sheet.AddProperty("Value", func() int64 { return v }, func(n int64) { v = n })
	edit, ok := item.Control().(*IntEdit)
	if !ok || edit.Text() != "9007199254740993" {
		t.Fatalf("integer property editor = %T", item.Control())
	}
	edit.OnEdited("9223372036854775807")
	edit.UpdateValue()
	edit.OnKeyDown(gui.KeyUp, false)
	if v != int64(9223372036854775807) {
		t.Fatal("signed max wrapped")
	}
	edit.OnKeyDown(gui.KeyDown, false)
	if v != int64(9223372036854775806) {
		t.Fatal("arrow key did not step exactly")
	}
	edit.SetText("-")
	edit.Deactivate()
	if edit.Text() != "9223372036854775806" || v != int64(9223372036854775806) {
		t.Fatal("invalid intermediate text changed the value or was not restored")
	}
	edit.OnEdited("-9223372036854775808")
	edit.UpdateValue()
	edit.OnKeyDown(gui.KeyDown, false)
	if v != int64(-9223372036854775808) {
		t.Fatal("signed min wrapped")
	}
}

func TestIntegerControlUnsignedAndReadOnly(t *testing.T) {
	sheet := newTestSheet()
	v := ^uint64(0)
	item, _ := sheet.AddProperty("Unsigned", func() uint64 { return v }, func(n uint64) { v = n })
	edit := item.Control().(*IntEdit)
	edit.OnKeyDown(gui.KeyUp, false)
	edit.OnEdited("-1")
	edit.OnEdited("18446744073709551616")
	if v != ^uint64(0) {
		t.Fatal("unsigned overflow or negative value was committed")
	}
	edit.OnKeyDown(gui.KeyDown, false)
	if v != ^uint64(0)-1 {
		t.Fatal("unsigned step lost precision")
	}
	edit.OnEdited("0")
	edit.OnKeyDown(gui.KeyDown, false)
	if v != 0 {
		t.Fatal("unsigned zero wrapped")
	}
	ro, _ := sheet.AddProperty("ReadOnly", func() uint64 { return v }, nil)
	readOnly := ro.Control().(*IntEdit)
	readOnly.OnKeyDown(gui.KeyUp, false)
	readOnly.OnEdited("123")
	if !readOnly.IsReadOnly() || v != 0 {
		t.Fatal("read-only integer accepted editing")
	}
}

func TestIntegerControlNativeEditSignalsAndMultiValue(t *testing.T) {
	sheet := newTestSheet()
	v := 7
	item, _ := sheet.AddProperty("Value", func() int { return v }, func(n int) { v = n })
	edit := item.Control().(*IntEdit)
	edit.SetText("")
	edit.OnTextInput("2")
	edit.OnTextInput("5")
	if v != 25 {
		t.Fatal("actual Edit signal did not reach the integer property")
	}
	edit.OnTextInput("x")
	edit.OnSubmit(edit.Text())
	if v != 25 || edit.Text() != "25" {
		t.Fatal("invalid suffix was committed")
	}
	// A blank mixed-selection hint must not commit zero on deactivation.
	av, bv := 3, 8
	a := &fakeObj{enum: func(l IPropertyList) {
		l.AddProperty("N", func() int { return av }, func(n int) { av = n })
	}}
	b := &fakeObj{enum: func(l IPropertyList) {
		l.AddProperty("N", func() int { return bv }, func(n int) { bv = n })
	}}
	owner := &recordingOwner{}
	mixed := bindSheet([]interface{}{a, b}, owner).propMap["N"].Control().(*IntEdit)
	mixed.Deactivate()
	if av != 3 || bv != 8 || len(owner.cmds) != 0 {
		t.Fatal("mixed selection was changed by deactivation")
	}
	mixed.OnKeyDown(gui.KeyUp, false)
	if av != 4 || bv != 4 || len(owner.cmds) != 1 {
		t.Fatal("mixed selection step was not one shared undo command")
	}
	owner.cmds[0].Undo()
	if av != 3 || bv != 8 {
		t.Fatal("integer edit undo did not restore individual values")
	}
}

func TestIntegerControlNoOpDoesNotPushUndo(t *testing.T) {
	v := int8(127)
	obj := &fakeObj{enum: func(l IPropertyList) {
		l.AddProperty("N", func() int8 { return v }, func(n int8) { v = n })
	}}
	owner := &recordingOwner{}
	edit := bindSheet([]interface{}{obj}, owner).propMap["N"].Control().(*IntEdit)
	edit.Deactivate()
	edit.OnKeyDown(gui.KeyUp, false)
	edit.OnEdited("128")
	if v != 127 || len(owner.cmds) != 0 {
		t.Fatal("no-op or overflowing integer edit changed undo history")
	}
}
