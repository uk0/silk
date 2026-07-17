package prop

import (
	"testing"

	"github.com/uk0/silk/core"
)

// TestIsValidPropId locks in the permissive rule: property ids double as
// display labels, so unicode/CJK, mixed case, spaces and punctuation are all
// valid; only empty/blank strings and control characters are rejected.
func TestIsValidPropId(t *testing.T) {
	valid := []string{"液位", "最小值", "协议", "地址", "点表", "tag", "Show Label", "GPU加速"}
	for _, id := range valid {
		if !IsValidPropId(id) {
			t.Errorf("IsValidPropId(%q) = false, want true", id)
		}
	}

	invalid := []string{"", "   ", "bad\x00id"}
	for _, id := range invalid {
		if IsValidPropId(id) {
			t.Errorf("IsValidPropId(%q) = true, want false", id)
		}
	}
}

// newTestSheet returns a fresh PropertySheet with an empty in-memory config so
// AddProperty's loadConfig step has a config to read. Named "default" so the
// inherit lookup short-circuits and never touches disk. Keeps the test GL-free.
func newTestSheet() *PropertySheet {
	sheet := NewPropertySheet()
	sheet.configFile = &PropertyConfigFile{name: "default", doc: core.NewTDoc()}
	return sheet
}

// TestAddPropertyPreservesUnicodeID guards the regression: before the fix,
// AddProperty lowercased the id and IsValidPropId rejected anything outside
// [a-z0-9_], so every Chinese/labeled property was silently dropped.
func TestAddPropertyPreservesUnicodeID(t *testing.T) {
	sheet := newTestSheet()

	value := "low"
	get := func() string { return value }
	set := func(s string) { value = s }

	item, first := sheet.AddProperty("液位", get, set)
	if item == nil {
		t.Fatal(`AddProperty("液位") returned nil item: unicode id was dropped`)
	}
	if !first {
		t.Fatal(`AddProperty("液位") first = false on a fresh sheet`)
	}
	if item.Id() != "液位" {
		t.Fatalf("id not preserved verbatim: got %q, want %q", item.Id(), "液位")
	}

	// getter round-trips
	if got := item.GetValue(); got != "low" {
		t.Fatalf("GetValue() = %v, want %q", got, "low")
	}
	// setter round-trips back through the backing variable
	item.SetValue("high")
	if value != "high" {
		t.Fatalf("SetValue did not reach backing store: value = %q, want %q", value, "high")
	}
	if got := item.GetValue(); got != "high" {
		t.Fatalf("round-trip GetValue() = %v, want %q", got, "high")
	}
}

// TestAddPropertyDoesNotLowercaseID proves the lowercasing was removed: a
// mixed-case id must be stored and returned exactly as given.
func TestAddPropertyDoesNotLowercaseID(t *testing.T) {
	sheet := newTestSheet()

	v := 0.0
	item, _ := sheet.AddProperty("GPU加速", func() float64 { return v }, func(x float64) { v = x })
	if item == nil {
		t.Fatal(`AddProperty("GPU加速") returned nil item`)
	}
	if item.Id() != "GPU加速" {
		t.Fatalf("id was altered: got %q, want %q", item.Id(), "GPU加速")
	}
}
