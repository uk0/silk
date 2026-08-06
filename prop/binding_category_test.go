package prop

import "testing"

// TestBindingCategoryClassifiesTagID pins the 数据绑定 category onto the one
// property id that carries a 组态 binding. The classifier runs before the
// appearance/behavior scans, which match by substring, so this also guards the
// tag row against being swallowed by a keyword added to either list later.
func TestBindingCategoryClassifiesTagID(t *testing.T) {
	if got := categoryOfPropID(TagBindingID); got != "binding" {
		t.Errorf("categoryOfPropID(%q) = %q, want %q", TagBindingID, got, "binding")
	}
	if got := categoryNames["binding"]; got != "数据绑定" {
		t.Errorf("categoryNames[\"binding\"] = %q, want 数据绑定", got)
	}
	found := false
	for _, key := range categoryOrder {
		if key == "binding" {
			found = true
		}
	}
	if !found {
		t.Errorf("categoryOrder = %v, missing \"binding\": a category absent from the order emits no header", categoryOrder)
	}
}

// TestBindingCategoryMatchesExactly keeps 数据绑定 to the rows BindScreen can
// actually sink. The id is compared whole rather than by substring, so an
// unrelated property that merely spells "tag" inside its id stays where it was.
func TestBindingCategoryMatchesExactly(t *testing.T) {
	cases := []struct{ id, want string }{
		{"tag", "binding"},
		{"TAG", "binding"}, // ids are stored verbatim, so folding is the classifier's job
		{"tags", "general"},
		{"tagline", "general"},
		{"x", "layout"},
		{"color", "appearance"},
		{"value", "behavior"},
	}
	for _, c := range cases {
		if got := categoryOfPropID(c.id); got != c.want {
			t.Errorf("categoryOfPropID(%q) = %q, want %q", c.id, got, c.want)
		}
	}
}

// TestSheetShowsBindingCategoryAndFiltersIt drives the new rows through the
// real layout: the 数据绑定 header appears in category order alongside the
// existing ones, and the filter box keeps working over it — a query naming the
// tag row leaves that header standing alone, and a query that misses it drops
// the header with its last child.
func TestSheetShowsBindingCategoryAndFiltersIt(t *testing.T) {
	sheet := newTestSheet()

	x := 0
	color := "red"
	tag := "PLC1.Temp"
	addTestProp(sheet, "x", func() int { return x }, func(v int) { x = v })
	addTestProp(sheet, "color", func() string { return color }, func(v string) { color = v })
	tagItem := addTestProp(sheet, TagBindingID, func() string { return tag }, func(v string) { tag = v })

	sheet.Layout()
	headers, ids := layoutRows(sheet)
	if !equalStrs(headers, []string{"layout", "appearance", "binding"}) {
		t.Fatalf("unfiltered headers = %v, want [layout appearance binding]", headers)
	}
	if !equalStrs(ids, []string{"x", "color", TagBindingID}) {
		t.Fatalf("unfiltered rows = %v, want [x color %s]", ids, TagBindingID)
	}

	// A query naming the binding row keeps its header and drops the others.
	sheet.SetFilter("TAG")
	headers, ids = layoutRows(sheet)
	if !equalStrs(headers, []string{"binding"}) {
		t.Errorf(`after SetFilter("TAG") headers = %v, want [binding]`, headers)
	}
	if !equalStrs(ids, []string{TagBindingID}) {
		t.Errorf(`after SetFilter("TAG") rows = %v, want [%s]`, ids, TagBindingID)
	}

	// A query that misses it takes the header away with its only child, and the
	// editor control goes with it — the sheet positions editors absolutely, so a
	// filtered-out control left visible paints over the rows that survived.
	sheet.SetFilter("col")
	headers, ids = layoutRows(sheet)
	if !equalStrs(headers, []string{"appearance"}) {
		t.Errorf(`after SetFilter("col") headers = %v, want [appearance]`, headers)
	}
	if !equalStrs(ids, []string{"color"}) {
		t.Errorf(`after SetFilter("col") rows = %v, want [color]`, ids)
	}
	if tagItem.Control().IsVisible() {
		t.Error("filtered-out binding row kept a visible editor control")
	}

	// Clearing restores every header and row.
	sheet.SetFilter("")
	headers, _ = layoutRows(sheet)
	if !equalStrs(headers, []string{"layout", "appearance", "binding"}) {
		t.Errorf(`after SetFilter("") headers = %v, want [layout appearance binding]`, headers)
	}
	if !tagItem.Control().IsVisible() {
		t.Error("clearing the filter left the binding row hidden")
	}
}

// TestBindingCategoryCollapses covers the header the new category brings with
// it: 数据绑定 folds and unfolds like the rest, so an operator working on
// geometry can put the tag row away.
func TestBindingCategoryCollapses(t *testing.T) {
	sheet := newTestSheet()
	tag := ""
	addTestProp(sheet, TagBindingID, func() string { return tag }, func(v string) { tag = v })

	sheet.Layout()
	if _, ids := layoutRows(sheet); !equalStrs(ids, []string{TagBindingID}) {
		t.Fatalf("expanded rows = %v, want [%s]", ids, TagBindingID)
	}

	sheet.categories["binding"].expanded = false
	sheet.Layout()
	headers, ids := layoutRows(sheet)
	if !equalStrs(headers, []string{"binding"}) {
		t.Errorf("collapsed headers = %v, want [binding]", headers)
	}
	if len(ids) != 0 {
		t.Errorf("collapsed rows = %v, want none", ids)
	}
}
