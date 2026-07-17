package ged

import (
	"reflect"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

// TestCaptureApplyWidgetProperties exercises captureWidgetProperties /
// applyWidgetProperties directly on a widget instance: a configured Tank's
// scalar properties are captured, then applied onto a fresh Tank, which must
// end up with the same values. The complex "颜色" (paint.Color) property is
// skipped in v1 and must not appear in the captured set.
func TestCaptureApplyWidgetProperties(t *testing.T) {
	src := gui.NewTank()
	src.SetMin(5)
	src.SetMax(250)
	src.SetLevel(0.5)
	src.SetShowLabel(false)
	src.SetTagName("PT-101")

	vals := captureWidgetProperties(src)

	for _, id := range []string{"液位", "最小值", "最大值", "显示标签", "tag"} {
		if _, ok := vals[id]; !ok {
			t.Errorf("captured set missing scalar property %q; got %v", id, vals)
		}
	}
	if _, ok := vals["颜色"]; ok {
		t.Errorf("complex color property must be skipped in v1, but was captured: %q", vals["颜色"])
	}

	dst := gui.NewTank() // defaults: min 0, max 100, level 0, showLabel true, tag ""
	applyWidgetProperties(dst, vals)

	if dst.Min() != 5 {
		t.Errorf("Min = %v, want 5", dst.Min())
	}
	if dst.Max() != 250 {
		t.Errorf("Max = %v, want 250", dst.Max())
	}
	if dst.Level() != 0.5 {
		t.Errorf("Level = %v, want 0.5", dst.Level())
	}
	if dst.ShowLabel() != false {
		t.Errorf("ShowLabel = %v, want false", dst.ShowLabel())
	}
	if dst.TagName() != "PT-101" {
		t.Errorf("TagName = %q, want %q", dst.TagName(), "PT-101")
	}
}

// TestFakeWidgetSaveLoadDesignProperties is the end-to-end regression for the
// bug: a configured widget's editable properties must survive SaveDesign ->
// LoadDesign. It first checks the in-memory TDoc round-trip, then reparses the
// design through the TDoc text form so the on-disk "props" persistence (with
// non-ASCII keys) is covered too. No GL / Draw is involved.
func TestFakeWidgetSaveLoadDesignProperties(t *testing.T) {
	fw, err := NewFakeWidgetFromFactory("gui.Tank")
	if err != nil {
		t.Fatal(err)
	}
	fw.SetWidgetName("tank1")
	fw.SetBounds(10, 10, 20, 40)

	src := fw.Widget().(*gui.Tank)
	src.SetMin(12)
	src.SetMax(888)
	src.SetLevel(0.5)
	src.SetShowLabel(false)
	src.SetTagName("LT-9")

	doc := fw.SaveDesign()

	assertRestored := func(t *testing.T, target *FakeWidget) {
		t.Helper()
		dst := target.Widget().(*gui.Tank)
		if dst.Min() != 12 {
			t.Errorf("Min = %v, want 12", dst.Min())
		}
		if dst.Max() != 888 {
			t.Errorf("Max = %v, want 888", dst.Max())
		}
		if dst.Level() != 0.5 {
			t.Errorf("Level = %v, want 0.5", dst.Level())
		}
		if dst.ShowLabel() != false {
			t.Errorf("ShowLabel = %v, want false", dst.ShowLabel())
		}
		if dst.TagName() != "LT-9" {
			t.Errorf("TagName = %q, want %q", dst.TagName(), "LT-9")
		}
		// FakeWidget's own fields still round-trip alongside the new props.
		if target.WidgetName() != "tank1" {
			t.Errorf("WidgetName = %q, want %q", target.WidgetName(), "tank1")
		}
	}

	// In-memory round-trip: load the live TDoc into a fresh FakeWidget.
	fw2, err := NewFakeWidgetFromFactory("gui.Tank")
	if err != nil {
		t.Fatal(err)
	}
	if err := fw2.LoadDesign(doc); err != nil {
		t.Fatal(err)
	}
	assertRestored(t, fw2)

	// On-disk round-trip: reparse the serialized text, proving the "props"
	// node and its (non-ASCII) keys survive TDoc text persistence.
	reparsed, err := core.LoadTDocStr(doc.String())
	if err != nil {
		t.Fatalf("reparse design text: %v", err)
	}
	fw3, err := NewFakeWidgetFromFactory("gui.Tank")
	if err != nil {
		t.Fatal(err)
	}
	if err := fw3.LoadDesign(reparsed); err != nil {
		t.Fatal(err)
	}
	assertRestored(t, fw3)
}

// TestScalarRoundTripKinds covers scalarToString / parseScalar across every
// scalar kind — including the integer/unsigned paths that Tank does not
// exercise — and confirms non-scalar types are rejected.
func TestScalarRoundTripKinds(t *testing.T) {
	cases := []interface{}{
		"hello world", // value with a space: quoted, survives round-trip
		true,
		false,
		int(-42),
		uint(7),
		float64(3.5),
		float32(1.25),
	}
	for _, want := range cases {
		s, ok := scalarToString(reflect.ValueOf(want))
		if !ok {
			t.Errorf("scalarToString(%v) reported unsupported", want)
			continue
		}
		got, ok := parseScalar(s, reflect.TypeOf(want))
		if !ok {
			t.Errorf("parseScalar(%q, %T) reported unsupported", s, want)
			continue
		}
		if got.Interface() != want {
			t.Errorf("round-trip %T: got %v, want %v (serialized %q)", want, got.Interface(), want, s)
		}
	}

	type complexT struct{ A, B int }
	if _, ok := scalarToString(reflect.ValueOf(complexT{1, 2})); ok {
		t.Error("scalarToString must reject non-scalar struct types")
	}
	if _, ok := parseScalar("whatever", reflect.TypeOf(complexT{})); ok {
		t.Error("parseScalar must reject non-scalar target types")
	}
}
