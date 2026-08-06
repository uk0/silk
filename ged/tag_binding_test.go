package ged

import (
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/prop"
)

// bindScreenSink mirrors the sink type switch scada.BindScreen runs in
// bindIndustrial (scada/wire.go): a tag only reaches a widget through one of
// these six setters. Restated here rather than imported because the switch
// matches structurally — importing scada would pull the whole runtime container
// (device, report, playback, ...) into a designer test without proving more.
func bindScreenSink(w gui.IWidget) bool {
	switch w.(type) {
	case interface{ SetLevel(float64) }, // Tank
		interface{ SetValue(float64) }, // DigitalDisplay, Thermometer, ValueBar
		interface{ SetOn(bool) },       // Indicator
		interface{ SetState(bool) },    // Valve
		interface{ SetRunning(bool) },  // Pump
		interface{ SetActive(bool) }:   // Pipe
		return true
	}
	return false
}

// widgetHasBindingRow reports whether w offers the designer a tag-binding row,
// asked through the same capture the designer persists with so the row and the
// saved "props" block cannot disagree.
func widgetHasBindingRow(w gui.IWidget) bool {
	ep, ok := w.(core.IEnumProperties)
	if !ok {
		return false
	}
	_, found := captureWidgetProperties(ep)[prop.TagBindingID]
	return found
}

// TestBindingRowMatchesBindScreenSinks derives the 数据绑定 widget set instead
// of listing it: every registered widget is asked whether it carries the
// TagName() seam scada.BindScreen keys on, and the answer must agree with
// whether the designer offers it a binding row. A widget with the seam but no
// row is unbindable in the designer; a widget with a row but no seam offers a
// tag BindScreen will never read. Both halves also have to reach a real sink,
// or the tag the operator types is wired to nothing.
func TestBindingRowMatchesBindScreenSinks(t *testing.T) {
	var bindable []string

	for _, f := range core.AllFactories() {
		w, ok := f.New().(gui.IWidget)
		if !ok {
			continue
		}
		_, hasSeam := w.(interface{ TagName() string })
		hasRow := widgetHasBindingRow(w)

		if hasSeam != hasRow {
			t.Errorf("%s: TagName() seam = %v but 数据绑定 row = %v; the designer and scada.BindScreen disagree about whether this widget is bindable",
				f.Name(), hasSeam, hasRow)
		}
		if hasSeam {
			if !bindScreenSink(w) {
				t.Errorf("%s: exposes TagName() but matches no scada.BindScreen sink; its tag would bind to nothing", f.Name())
			}
			bindable = append(bindable, f.Name())
		}
	}

	if len(bindable) == 0 {
		t.Fatal("no bindable widget found: the 数据绑定 category would never appear")
	}
	t.Logf("bindable widgets derived from the TagName() seam: %v", bindable)
}

// TestBindingRowRoundTripsThroughDesign checks the storage the 数据绑定 row
// writes to is the one the generator reads: a tag typed in the designer must
// survive save/load and reach the runtime widget Generate builds, because
// scada.BindScreen resolves the tag off that widget and nothing else.
func TestBindingRowRoundTripsThroughDesign(t *testing.T) {
	src, err := NewFakeWidgetFromFactory("gui.Tank")
	if err != nil {
		t.Fatalf("create gui.Tank: %v", err)
	}
	setter, ok := src.Widget().(interface{ SetTagName(string) })
	if !ok {
		t.Fatal("gui.Tank has no SetTagName")
	}
	setter.SetTagName("PLC1.Temp")

	dst, err := NewFakeWidgetFromFactory("gui.Tank")
	if err != nil {
		t.Fatalf("create gui.Tank: %v", err)
	}
	if err := dst.LoadDesign(src.SaveDesign()); err != nil {
		t.Fatalf("LoadDesign: %v", err)
	}

	namer, ok := dst.Generate().(interface{ TagName() string })
	if !ok {
		t.Fatal("generated gui.Tank has no TagName")
	}
	if got := namer.TagName(); got != "PLC1.Temp" {
		t.Errorf("tag after save/load/Generate = %q, want %q", got, "PLC1.Temp")
	}
}
