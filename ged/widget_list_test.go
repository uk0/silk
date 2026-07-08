package ged

import (
	"testing"
)

// TestWidgetListSCADACategory verifies the industrial/SCADA widgets are bucketed
// into their own palette category (组态 (SCADA)) rather than falling into the
// generic "其他 (Other)" group, so an HMI builder finds them grouped together.
func TestWidgetListSCADACategory(t *testing.T) {
	const catName = "组态 (SCADA)"
	want := []string{"gui.Tank", "gui.Indicator", "gui.DigitalDisplay", "gui.Valve", "gui.Pipe"}

	var members []string
	found := false
	for _, cd := range categoryDefs {
		if cd.name == catName {
			found = true
			members = cd.names
			break
		}
	}
	if !found {
		t.Fatalf("palette category %q not found in categoryDefs", catName)
	}

	member := map[string]bool{}
	for _, n := range members {
		member[n] = true
	}
	for _, w := range want {
		if !member[w] {
			t.Errorf("widget %q not mapped to category %q", w, catName)
		}
	}
}
