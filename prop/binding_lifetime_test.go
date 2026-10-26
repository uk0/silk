package prop

import (
	"github.com/uk0/silk/gui"
	"testing"
)

func TestRetiredPropertyCannotWriteAfterRebind(t *testing.T) {
	for _, rebind := range []bool{false, true} {
		a, b := 12.5, 40.5
		first := &fakeObj{enum: func(l IPropertyList) { l.AddProperty("value", func() float64 { return a }, func(v float64) { a = v }) }}
		second := &fakeObj{enum: func(l IPropertyList) { l.AddProperty("value", func() float64 { return b }, func(v float64) { b = v }) }}
		owner, nextOwner := &recordingOwner{}, &recordingOwner{}
		sheet := bindSheet([]interface{}{first}, owner)
		item := sheet.propMap["value"]
		edit := item.Control().(*FloatEdit)
		edit.SetFocus()
		edit.SetText("99.125") // stale buffer, e.g. model changed during history
		if rebind {
			sheet.Bind([]interface{}{second}, "", nextOwner)
		} else {
			sheet.Clear(owner)
		}
		gui.NewEdit().SetFocus() // late blur must not commit the stale value
		edit.OnEdited("88.5")
		item.SetValue(77.5)
		if err := item.SetValueStr("66.5"); err == nil {
			t.Fatal("retired string write did not report stale binding")
		}
		if a != 12.5 || b != 40.5 || len(owner.cmds) != 0 || len(nextOwner.cmds) != 0 {
			t.Fatal("old control changed model or routed command to a new owner")
		}
		if rebind {
			live := sheet.propMap["value"]
			if live == item || live.retired {
				t.Fatal("new binding incorrectly retired")
			}
			live.SetValue(41.5)
			if len(nextOwner.cmds) != 1 {
				t.Fatal("new binding lost command dispatch")
			}
		}
	}
}

func TestClearWithForeignOwnerDoesNotRetireLiveProperties(t *testing.T) {
	v := 1.5
	owner := &recordingOwner{}
	obj := &fakeObj{enum: func(l IPropertyList) { l.AddProperty("value", func() float64 { return v }, func(n float64) { v = n }) }}
	sheet := bindSheet([]interface{}{obj}, owner)
	item := sheet.propMap["value"]
	sheet.Clear(&recordingOwner{})
	if item.retired || sheet.propMap["value"] != item {
		t.Fatal("foreign owner cleared live row")
	}
	item.SetValue(2.5)
	if len(owner.cmds) != 1 {
		t.Fatal("live row no longer dispatches")
	}
}
