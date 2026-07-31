package prop

import "testing"

// TestPropMatchesFilter pins the filter-box predicate down: a blank query keeps
// every row, a query that matches nothing keeps none, matching is
// case-insensitive on both sides, and a relabelled property is matched by its
// label as well as by its id. Without the label arm, typing the text the sheet
// actually shows ("颜色") would hide the row it names.
func TestPropMatchesFilter(t *testing.T) {
	cases := []struct {
		name   string
		label  string
		id     string
		filter string
		want   bool
	}{
		{"empty query keeps the row", "宽度", "width", "", true},
		{"blank query keeps the row", "宽度", "width", "   ", true},
		{"no match hides the row", "宽度", "width", "color", false},
		{"row case folded", "Width", "Width", "wid", true},
		{"query case folded", "width", "width", "WID", true},
		{"substring, not prefix", "最大宽度", "maxWidth", "width", true},
		{"label matched when it differs from the id", "颜色", "fg", "颜色", true},
		{"query trimmed before matching", "宽度", "width", "  wid  ", true},
	}

	for _, c := range cases {
		if got := propMatchesFilter(c.label, c.id, c.filter); got != c.want {
			t.Errorf("%s: propMatchesFilter(%q, %q, %q) = %v, want %v",
				c.name, c.label, c.id, c.filter, got, c.want)
		}
	}
}

// addTestProp registers a property and appends it to the sheet's row list —
// the step Bind normally performs. Bind is avoided here because it resolves the
// property config through the resource directory, which would touch disk.
func addTestProp(sheet *PropertySheet, id string, get, set interface{}) *PropertyItem {
	item, _ := sheet.AddProperty(id, get, set)
	if item == nil {
		panic("AddProperty rejected " + id)
	}
	sheet.rlist = append(sheet.rlist, item)
	return item
}

// layoutRows splits the laid-out sheet into the category headers and the
// property ids that are actually on screen, in draw order.
func layoutRows(sheet *PropertySheet) (headers, ids []string) {
	for _, entry := range sheet.categoryLayout {
		if entry.isHeader {
			headers = append(headers, entry.category)
		} else if entry.item != nil {
			ids = append(ids, entry.item.Id())
		}
	}
	return
}

func equalStrs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestSheetFilterNarrowsRowsAndHeaders drives the filter through the layout:
// a category keeps its header while any child matches, loses it when none do,
// and an empty query restores every header and row. It also pins the trap that
// comes with hiding rows in a sheet whose editors are absolutely positioned
// child widgets — a filtered-out control that stays visible keeps painting over
// the rows that survived.
func TestSheetFilterNarrowsRowsAndHeaders(t *testing.T) {
	sheet := newTestSheet()

	x := 0
	color := "red"
	xItem := addTestProp(sheet, "x", func() int { return x }, func(v int) { x = v })
	addTestProp(sheet, "color", func() string { return color }, func(v string) { color = v })

	sheet.Layout()
	headers, ids := layoutRows(sheet)
	if !equalStrs(headers, []string{"layout", "appearance"}) {
		t.Fatalf("unfiltered headers = %v, want [layout appearance]", headers)
	}
	if !equalStrs(ids, []string{"x", "color"}) {
		t.Fatalf("unfiltered rows = %v, want [x color]", ids)
	}

	// A query matching only the appearance child drops the layout header too.
	sheet.SetFilter("COL")
	headers, ids = layoutRows(sheet)
	if !equalStrs(headers, []string{"appearance"}) {
		t.Errorf(`after SetFilter("COL") headers = %v, want [appearance]`, headers)
	}
	if !equalStrs(ids, []string{"color"}) {
		t.Errorf(`after SetFilter("COL") rows = %v, want [color]`, ids)
	}
	if xItem.Control().IsVisible() {
		t.Error(`filtered-out row "x" kept a visible editor control`)
	}

	// No match at all: no rows and no headers left standing.
	sheet.SetFilter("zzz-nothing")
	headers, ids = layoutRows(sheet)
	if len(headers) != 0 || len(ids) != 0 {
		t.Errorf("non-matching filter left headers = %v, rows = %v, want both empty", headers, ids)
	}

	// Clearing restores everything, controls included.
	sheet.SetFilter("")
	headers, ids = layoutRows(sheet)
	if !equalStrs(headers, []string{"layout", "appearance"}) {
		t.Errorf(`after SetFilter("") headers = %v, want [layout appearance]`, headers)
	}
	if !equalStrs(ids, []string{"x", "color"}) {
		t.Errorf(`after SetFilter("") rows = %v, want [x color]`, ids)
	}
	if !xItem.Control().IsVisible() {
		t.Error(`clearing the filter left row "x" hidden`)
	}
}

// TestSheetFilterFollowsSearchBox checks the two directions stay in step: text
// typed into the box reaches the filter, and a programmatic SetFilter shows up
// in the box instead of leaving it stale.
func TestSheetFilterFollowsSearchBox(t *testing.T) {
	sheet := newTestSheet()
	v := 0
	addTestProp(sheet, "x", func() int { return v }, func(n int) { v = n })

	sheet.searchBox.SetText("x")
	if sheet.Filter() != "x" {
		t.Errorf("typing into the search box gave Filter() = %q, want %q", sheet.Filter(), "x")
	}

	sheet.SetFilter("")
	if got := sheet.searchBox.Text(); got != "" {
		t.Errorf(`SetFilter("") left the search box showing %q`, got)
	}
}
