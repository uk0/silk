package prop

import "testing"

func TestPropertyRefreshOwnerGuardsSharedInspector(t *testing.T) {
	value := 12.5
	obj := &fakeObj{enum: func(l IPropertyList) {
		l.AddProperty("F", func() float64 { return value }, func(v float64) { value = v })
	}}
	a, b := new(recordingOwner), new(recordingOwner)
	sheet := bindSheet([]interface{}{obj}, a)
	field := sheet.propMap["F"].Control().(*FloatEdit)
	field.SetText("draft")
	value = 21.125
	sheet.RefreshValuesForOwner(nil)
	sheet.RefreshValuesForOwner(b)
	if field.Text() != "draft" {
		t.Fatal("foreign owner discarded draft")
	}
	sheet.RefreshValuesForOwner(a)
	if field.Text() != "21.125" {
		t.Fatal("current owner did not refresh")
	}
	sheet.Clear(b)
	if sheet.objOwner != a {
		t.Fatal("foreign clear retired binding")
	}
	sheet.Clear(a)
	if sheet.objOwner != nil {
		t.Fatal("clear retained owner")
	}
	field.SetText("retired")
	sheet.RefreshValuesForOwner(a)
	if field.Text() != "retired" || len(a.cmds) != 0 {
		t.Fatal("retired owner refreshed or wrote through")
	}
}
