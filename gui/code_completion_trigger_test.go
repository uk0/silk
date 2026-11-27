package gui

import (
	"strings"
	"testing"
)

func TestAutomaticCodeCompletionRecognizesNumericSuffixes(t *testing.T) {
	for _, tt := range []struct {
		raw, prefix string
		yes         bool
	}{
		{"run(2", "", false}, {"0x", "", false}, {"0b10_1", "", false}, {"1e", "", false}, {"1e-2", "", false},
		{"value = 1.", "", false}, {"value = 1.e2", "", false}, {"value = 1.e", "", false}, {"0xA.p", "", false}, {"0x1.2p-3", "", false}, {"1i.", "", false}, {".5", "", false},
		{"f(items...", "", false}, {".", "", false}, {"sensor2", "sensor2", true}, {"sensor2.", "", true}, {"sensor2.Unit12", "Unit12", true},
		{"items[2].", "", true}, {"f(2).", "", true}, {"_2", "_2", true}, {"𐐀对象2", "𐐀对象2", true}, {"测点2.单位", "单位", true}, {"测点2.", "", true},
		{"module .", "", true}, {"x+", "", false}, {"x:\t", "", false},
	} {
		if got, ok := AutomaticCodeCompletionPrefix(tt.raw, len(tt.raw)); got != tt.prefix || ok != tt.yes {
			t.Errorf("%q: got %q, %v; want %q, %v", tt.raw, got, ok, tt.prefix, tt.yes)
		}
	}
	const multibyte = "𐐀member"
	for _, col := range []int{-1, 0, 1, 2, 3, len(multibyte) + 1} {
		if got, ok := AutomaticCodeCompletionPrefix(multibyte, col); got != "" || ok {
			t.Errorf("invalid UTF-8 caret %d: %q %v", col, got, ok)
		}
	}
	huge := strings.Repeat("x", 4<<20)
	if _, ok := AutomaticCodeCompletionPrefix(huge, len(huge)); ok {
		t.Fatal("oversize prefix not rejected")
	}
	end := huge + " x.sensor2"
	if got, ok := AutomaticCodeCompletionPrefix(end, len(end)); !ok || got != "sensor2" {
		t.Fatal("large line swallowed short useful prefix")
	}
	if a := testing.AllocsPerRun(60, func() { _, _ = AutomaticCodeCompletionPrefix(end, len(end)) }); a != 0 {
		t.Fatalf("suffix policy allocates: %g", a)
	}
}

func TestNumericInputClosesAutomaticChoicesButManualCompletionWorks(t *testing.T) {
	ed := NewCodeEditor()
	t.Cleanup(func() { ed.CancelCompletion(); ed.CancelSymbolIndex() })
	ed.SetText("func f(){\n call()\n}")
	ed.ScrollToLineCol(1, 6)
	ed.completion = &CompletionPopup{visible: true, open: true, maxVisible: 5, filtered: []CompletionItem{{Text: "something"}}}
	ed.OnTextInput("2")
	if ed.completion.visible || ed.completion.open || ed.Text() != "func f(){\n call(2)\n}" {
		t.Fatalf("numeric typing showed automatic suggestions: %q open=%v", ed.Text(), ed.completion.open)
	}
	// TriggerCompletion is the explicit SDK path, not the keyboard's
	// onCompletionRequested notification. The manual list can still open.
	ed.TriggerCompletion()
	if !ed.completion.open || ed.completion.prefix != "2" {
		t.Fatal("explicit SDK request no longer opens at a number")
	}
}

func BenchmarkAutomaticCodeCompletionPrefix(b *testing.B) {
	for _, tt := range []struct{ label, line string }{
		{"numeric", "sensor.Average(0x1.p"},
		{"unicode-selector", "decision = 设备2.量程"},
		{"huge-near-short-prefix", strings.Repeat("x", 4<<20) + "  sensor2."},
	} {
		b.Run(tt.label, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_, _ = AutomaticCodeCompletionPrefix(tt.line, len(tt.line))
			}
		})
	}
}
