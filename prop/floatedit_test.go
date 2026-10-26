package prop

import (
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

func TestFloatControlKeepsExactValueAndNoOpHistory(t *testing.T) {
	if _, ok := core.New("prop.control.FloatEdit").(*FloatEdit); !ok {
		t.Fatal("missing factory")
	}
	for _, v := range []float64{65.08749999999999, 0.1, math.Pi, math.SmallestNonzeroFloat64, math.MaxFloat64, math.Copysign(0, -1)} {
		t.Run(strconv.FormatFloat(v, 'g', -1, 64), func(t *testing.T) {
			original := v
			obj := &fakeObj{enum: func(l IPropertyList) { l.AddProperty("width", func() float64 { return v }, func(n float64) { v = n }) }}
			owner := &recordingOwner{}
			sheet := bindSheet([]interface{}{obj}, owner)
			item := sheet.propMap["width"]
			e, ok := item.Control().(*FloatEdit)
			if !ok {
				t.Fatalf("control=%T", item.Control())
			}
			exact := strconv.FormatFloat(original, 'g', -1, 64)
			if e.Text() != exact || !strings.Contains(gui.GetToolTip(e), exact) {
				t.Fatal("editable text/tooltip lost exact digits")
			}
			e.Deactivate()
			e.SetFocus()
			gui.NewEdit().SetFocus()
			sheet.RefreshValues()
			if math.Float64bits(v) != math.Float64bits(original) || len(owner.cmds) != 0 {
				t.Fatal("view/focus/blur rounded data or pushed history")
			}
		})
	}
	if got := compactPropertyFloat(65.08749999999999, 64); got != "65.0875" {
		t.Fatalf("compact=%q", got)
	}
	if got := compactPropertyFloat(float64(float32(.1)), 32); got != "0.1" {
		t.Fatalf("float32 compact=%q", got)
	}
}

func TestFloatControlValidatesEntireInputAndRestoresOnBlur(t *testing.T) {
	v := 12.5
	sheet := newTestSheet()
	item, _ := sheet.AddProperty("V", func() float64 { return v }, func(n float64) { v = n })
	e := item.Control().(*FloatEdit)
	for _, bad := range []string{"", "-", ".", "1e", "1e-", "NaN", "+Inf", "-Inf", "1e999", "12 junk", "1.2.3"} {
		e.SetText(bad)
		e.OnEdited(bad)
		if v != 12.5 {
			t.Fatalf("accepted %q", bad)
		}
		e.SetFocus()
		gui.NewEdit().SetFocus()
		if e.Text() != "12.5" {
			t.Fatalf("blur kept invalid %q", e.Text())
		}
	}
	e.SetText("")
	e.OnTextInput("2")
	e.OnTextInput(".")
	e.OnTextInput("7")
	e.OnTextInput("5")
	if v != 2.75 {
		t.Fatalf("native text signal=%v", v)
	}
	e.OnEdited(" 1.25e-4 ")
	e.UpdateValue()
	if v != .000125 {
		t.Fatal("scientific notation rejected")
	}
	e.OnKeyDown(gui.KeyUp, false)
	if v != 1.000125 {
		t.Fatal("step did not use exact value")
	}
	e.SetReadOnly(true)
	item.SetCfgReadOnly(true)
	e.OnEdited("99")
	e.OnKeyDown(gui.KeyUp, false)
	if v != 1.000125 {
		t.Fatal("read-only edit changed value")
	}
}

type float32Property float32

func TestFloatControlNamedFloat32AndBoundaries(t *testing.T) {
	v := float32Property(.1)
	item, _ := newTestSheet().AddProperty("V", func() float32Property { return v }, func(n float32Property) { v = n })
	e := item.Control().(*FloatEdit)
	if e.Text() != "0.1" {
		t.Fatal("float32 expanded through float64")
	}
	e.OnEdited("1.25")
	if v != 1.25 || reflect.TypeOf(item.GetValue()) != reflect.TypeOf(v) {
		t.Fatal("named float type lost")
	}
	e.OnEdited("3.5e38")
	if v != 1.25 {
		t.Fatal("float32 overflow accepted")
	}
}

func TestFloatMixedSelectionUndoAndRefresh(t *testing.T) {
	a, b := 65.08749999999999, 18.25
	obj := func(v *float64) *fakeObj {
		return &fakeObj{enum: func(l IPropertyList) {
			l.AddProperty("width", func() float64 { return *v }, func(n float64) { *v = n })
		}}
	}
	owner := &recordingOwner{}
	sheet := bindSheet([]interface{}{obj(&a), obj(&b)}, owner)
	e := sheet.propMap["width"].Control().(*FloatEdit)
	e.SetFocus()
	gui.NewEdit().SetFocus()
	e.Deactivate()
	if e.Text() != "" || len(owner.cmds) != 0 {
		t.Fatal("visiting mixed row committed a value")
	}
	e.OnEdited("42.125")
	e.UpdateValue()
	if a != 42.125 || b != 42.125 || len(owner.cmds) != 1 {
		t.Fatal("mixed write is not one shared command")
	}
	owner.cmds[0].Undo()
	sheet.RefreshValues()
	if a != 65.08749999999999 || b != 18.25 || e.Text() != "" {
		t.Fatal("undo lost individual precision")
	}
	e.OnKeyDown(gui.KeyUp, false)
	if a != 66.08749999999999 || b != a || len(owner.cmds) != 2 {
		t.Fatal("step lost mixed-selection semantics")
	}
	before := len(owner.cmds)
	e.Deactivate()
	sheet.RefreshValues()
	if len(owner.cmds) != before {
		t.Fatal("refresh pushed history")
	}
}

func TestExplicitFloatPropertyControlMappingStillWins(t *testing.T) {
	v := 1.5
	item, _ := newTestSheet().AddProperty("V", func() float64 { return v }, func(n float64) { v = n })
	item.SetCfgWidgetType("TextEdit")
	if _, ok := item.Control().(*TextEdit); !ok {
		t.Fatalf("custom mapping ignored: %T", item.Control())
	}
}
