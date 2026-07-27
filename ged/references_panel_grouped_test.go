package ged

import (
	"testing"
)

// GL-free tests for the grouped/filterable references view. Nothing here
// creates a Frame, draws or measures a font: every assertion goes through
// the pure row builder, the panel's model state or its event handlers.

// refKindLocs is a deliberately UNSORTED reference set spanning two files
// with all three kinds represented. Fed to SetReferences it must come out
// ordered core/baz.go:7, ged/foo.go:12, ged/foo.go:30 — which is also the
// grouped row order the tests below assert against.
func refKindLocs() []ReferenceLoc {
	return []ReferenceLoc{
		{File: "ged/foo.go", Line: 30, Col: 1, Preview: "return Bar{}", Kind: RefWrite},
		{File: "core/baz.go", Line: 7, Col: 9, Preview: "_ = Bar", Kind: RefRead},
		{File: "ged/foo.go", Line: 12, Col: 5, Preview: "x := Bar()", Kind: RefDeclaration},
	}
}

// groupedRefPanel returns a sized panel holding refKindLocs pushed through
// SetReferences, i.e. sorted and in the grouped view.
func groupedRefPanel() *ReferencesPanel {
	p := NewReferencesPanel()
	p.SetSize(300, 400)
	p.SetReferences(refKindLocs())
	return p
}

// refRowMidY returns the y coordinate of the middle of visible row idx,
// with the panel unscrolled.
func refRowMidY(p *ReferencesPanel, idx int) float64 {
	return referencesHeaderH + float64(idx)*p.rowHeight + p.rowHeight/2
}

// TestSortReferencesOrder checks the comparator: path, then line, then
// column, with ties keeping input order (stable).
func TestSortReferencesOrder(t *testing.T) {
	in := []ReferenceLoc{
		{File: "b.go", Line: 1, Col: 0},
		{File: "a.go", Line: 9, Col: 2},
		{File: "a.go", Line: 2, Col: 7, Preview: "tie-first"},
		{File: "a.go", Line: 2, Col: 1},
		{File: "a.go", Line: 2, Col: 7, Preview: "tie-second"},
	}
	sortReferences(in)

	want := []struct {
		file string
		line int
		col  int
	}{
		{"a.go", 2, 1},
		{"a.go", 2, 7},
		{"a.go", 2, 7},
		{"a.go", 9, 2},
		{"b.go", 1, 0},
	}
	for i, w := range want {
		if in[i].File != w.file || in[i].Line != w.line || in[i].Col != w.col {
			t.Fatalf("row %d = %s:%d:%d, want %s:%d:%d",
				i, in[i].File, in[i].Line, in[i].Col, w.file, w.line, w.col)
		}
	}
	// Stability: the two a.go:2:7 rows keep the order they were fed in.
	if in[1].Preview != "tie-first" || in[2].Preview != "tie-second" {
		t.Errorf("tie order = %q,%q, want tie-first,tie-second", in[1].Preview, in[2].Preview)
	}
}

// TestSetReferencesDeterministicReplacement pushes the same set in two
// different orders and requires identical stored rows, then pushes a
// smaller set and requires it to REPLACE (not merge with) the old one.
func TestSetReferencesDeterministicReplacement(t *testing.T) {
	a := NewReferencesPanel()
	a.SetReferences(refKindLocs())

	shuffled := refKindLocs()
	shuffled[0], shuffled[2] = shuffled[2], shuffled[0]
	b := NewReferencesPanel()
	b.SetReferences(shuffled)

	ga, gb := a.Locations(), b.Locations()
	if len(ga) != 3 || len(gb) != 3 {
		t.Fatalf("lengths = %d,%d, want 3,3", len(ga), len(gb))
	}
	for i := range ga {
		if ga[i] != gb[i] {
			t.Fatalf("row %d differs between input orders: %+v vs %+v", i, ga[i], gb[i])
		}
	}
	want := []string{"core/baz.go", "ged/foo.go", "ged/foo.go"}
	wantLines := []int{7, 12, 30}
	for i := range want {
		if ga[i].File != want[i] || ga[i].Line != wantLines[i] {
			t.Errorf("row %d = %s:%d, want %s:%d", i, ga[i].File, ga[i].Line, want[i], wantLines[i])
		}
	}

	// Replacement, not accumulation.
	a.SetReferences([]ReferenceLoc{{File: "z.go", Line: 3, Col: 0, Preview: "Bar()"}})
	if got := a.Locations(); len(got) != 1 || got[0].File != "z.go" {
		t.Errorf("after second SetReferences: %+v, want exactly [z.go:3]", got)
	}

	// Defensive copy: mutating the caller's slice must not leak in.
	in := refKindLocs()
	c := NewReferencesPanel()
	c.SetReferences(in)
	in[0].File = "MUTATED"
	for _, loc := range c.Locations() {
		if loc.File == "MUTATED" {
			t.Error("input mutation leaked into the panel after SetReferences")
		}
	}
}

// TestSetReferencesEntersGroupedView checks the view mode: flat by
// default (so SetLocations hosts are untouched), grouped after
// SetReferences, and back to flat on SetGrouped(false).
func TestSetReferencesEntersGroupedView(t *testing.T) {
	p := NewReferencesPanel()
	if p.Grouped() {
		t.Error("a fresh panel must start in the flat view")
	}
	p.SetLocations(refKindLocs())
	if p.Grouped() {
		t.Error("SetLocations must not switch the view mode")
	}
	if got := len(p.visibleRows()); got != 3 {
		t.Errorf("flat rows = %d, want 3", got)
	}
	p.SetReferences(refKindLocs())
	if !p.Grouped() {
		t.Error("SetReferences must switch to the grouped view")
	}
	if got := len(p.visibleRows()); got != 5 {
		t.Errorf("grouped rows = %d, want 5 (2 headers + 3 refs)", got)
	}
	p.SetGrouped(false)
	if p.Grouped() {
		t.Error("SetGrouped(false) must return to the flat view")
	}
	if got := len(p.visibleRows()); got != 3 {
		t.Errorf("flat rows after SetGrouped(false) = %d, want 3", got)
	}
}

// TestBuildRefRowsGroupedOrder pins the grouped row sequence: one header
// per file in path order (post-sort), each followed by its references in
// line order, with RefIdx pointing back at the right entry.
func TestBuildRefRowsGroupedOrder(t *testing.T) {
	locs := refKindLocs()
	sortReferences(locs)
	rows := buildRefRows(locs, "", map[string]bool{}, true)

	want := []refRow{
		{Kind: refRowGroup, File: "core/baz.go", RefIdx: -1, Count: 1},
		{Kind: refRowMatch, File: "core/baz.go", RefIdx: 0},
		{Kind: refRowGroup, File: "ged/foo.go", RefIdx: -1, Count: 2},
		{Kind: refRowMatch, File: "ged/foo.go", RefIdx: 1},
		{Kind: refRowMatch, File: "ged/foo.go", RefIdx: 2},
	}
	if len(rows) != len(want) {
		t.Fatalf("rows = %d, want %d: %+v", len(rows), len(want), rows)
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, rows[i], want[i])
		}
	}

	// Same input twice → byte-identical row sequence.
	again := buildRefRows(locs, "", map[string]bool{}, true)
	for i := range rows {
		if rows[i] != again[i] {
			t.Fatalf("row %d not stable across builds: %+v vs %+v", i, rows[i], again[i])
		}
	}

	// Interleaved (unsorted) input still groups contiguously, in
	// first-appearance order.
	mixed := []ReferenceLoc{
		{File: "b.go", Line: 1},
		{File: "a.go", Line: 2},
		{File: "b.go", Line: 3},
	}
	got := buildRefRows(mixed, "", map[string]bool{}, true)
	wantMixed := []refRow{
		{Kind: refRowGroup, File: "b.go", RefIdx: -1, Count: 2},
		{Kind: refRowMatch, File: "b.go", RefIdx: 0},
		{Kind: refRowMatch, File: "b.go", RefIdx: 2},
		{Kind: refRowGroup, File: "a.go", RefIdx: -1, Count: 1},
		{Kind: refRowMatch, File: "a.go", RefIdx: 1},
	}
	if len(got) != len(wantMixed) {
		t.Fatalf("interleaved rows = %d, want %d: %+v", len(got), len(wantMixed), got)
	}
	for i := range wantMixed {
		if got[i] != wantMixed[i] {
			t.Errorf("interleaved row %d = %+v, want %+v", i, got[i], wantMixed[i])
		}
	}
}

// TestRefGroupHeaderClickToggles clicks a group header and requires the
// group to collapse (its children leave the row list) without firing the
// activation signal, then expand again on a second click.
func TestRefGroupHeaderClickToggles(t *testing.T) {
	p := groupedRefPanel()
	fired := false
	p.SigLocationActivated(func(string, int, int) { fired = true })

	// Row 2 is the "ged/foo.go (2)" header.
	p.OnLeftDown(5, refRowMidY(p, 2))
	if fired {
		t.Error("clicking a group header fired SigLocationActivated")
	}
	if !p.IsCollapsed("ged/foo.go") {
		t.Fatal("group ged/foo.go did not collapse")
	}
	rows := p.visibleRows()
	if len(rows) != 3 {
		t.Fatalf("collapsed rows = %d, want 3: %+v", len(rows), rows)
	}
	if rows[2].Kind != refRowGroup || rows[2].File != "ged/foo.go" || rows[2].Count != 2 {
		t.Errorf("collapsed header row = %+v, want the ged/foo.go header with Count 2", rows[2])
	}

	// Second click expands it again.
	p.OnLeftDown(5, refRowMidY(p, 2))
	if p.IsCollapsed("ged/foo.go") {
		t.Fatal("group ged/foo.go did not expand again")
	}
	if got := len(p.visibleRows()); got != 5 {
		t.Errorf("expanded rows = %d, want 5", got)
	}

	// A fresh push starts every group expanded again.
	p.OnLeftDown(5, refRowMidY(p, 0)) // collapse core/baz.go
	if !p.IsCollapsed("core/baz.go") {
		t.Fatal("group core/baz.go did not collapse")
	}
	p.SetReferences(refKindLocs())
	if p.IsCollapsed("core/baz.go") {
		t.Error("SetReferences must reset the collapse state")
	}
}

// TestRefGroupedRowClickActivatesRightReference drives the hit-test in
// the grouped view: the child rows sit at flat indices 1, 3 and 4, and
// each must activate its own reference (file, 1-based line, column).
func TestRefGroupedRowClickActivatesRightReference(t *testing.T) {
	p := groupedRefPanel()
	var got ReferenceLoc
	n := 0
	p.SigLocationActivated(func(file string, line, col int) {
		got = ReferenceLoc{File: file, Line: line, Col: col}
		n++
	})

	cases := []struct {
		row  int
		want ReferenceLoc
	}{
		{1, ReferenceLoc{File: "core/baz.go", Line: 7, Col: 9}},
		{3, ReferenceLoc{File: "ged/foo.go", Line: 12, Col: 5}},
		{4, ReferenceLoc{File: "ged/foo.go", Line: 30, Col: 1}},
	}
	for _, c := range cases {
		got = ReferenceLoc{}
		p.OnLeftDown(5, refRowMidY(p, c.row))
		if got != c.want {
			t.Errorf("row %d activated %+v, want %+v", c.row, got, c.want)
		}
	}
	if n != len(cases) {
		t.Errorf("activation fired %d times, want %d", n, len(cases))
	}

	// Below the last row nothing fires.
	n = 0
	p.OnLeftDown(5, refRowMidY(p, 9))
	if n != 0 {
		t.Error("a click past the last row fired SigLocationActivated")
	}
}

// TestRefFilterNarrowsAndRestores checks SetFilter hides non-matching
// rows (and the groups that lose every child), leaves the backing list
// alone, and restores everything when cleared.
func TestRefFilterNarrowsAndRestores(t *testing.T) {
	p := groupedRefPanel()

	p.SetFilter("baz")
	if got := p.Filter(); got != "baz" {
		t.Errorf("Filter() = %q, want %q", got, "baz")
	}
	rows := p.visibleRows()
	if len(rows) != 2 {
		t.Fatalf("filtered rows = %d, want 2 (1 header + 1 ref): %+v", len(rows), rows)
	}
	if rows[0].Kind != refRowGroup || rows[0].File != "core/baz.go" || rows[0].Count != 1 {
		t.Errorf("filtered header = %+v, want core/baz.go with Count 1", rows[0])
	}
	if rows[1].Kind != refRowMatch || rows[1].RefIdx != 0 {
		t.Errorf("filtered child = %+v, want the core/baz.go reference (RefIdx 0)", rows[1])
	}
	// The rows are hidden, not dropped.
	if got := len(p.Locations()); got != 3 {
		t.Errorf("Locations() under a filter = %d rows, want 3", got)
	}

	// A filter that matches nothing empties the list without touching data.
	p.SetFilter("nothing-matches-this")
	if got := len(p.visibleRows()); got != 0 {
		t.Errorf("rows for a non-matching filter = %d, want 0", got)
	}
	if got := len(p.Locations()); got != 3 {
		t.Errorf("Locations() = %d rows, want 3", got)
	}

	// Clearing restores every row, in the original order.
	p.SetFilter("")
	rows = p.visibleRows()
	if len(rows) != 5 {
		t.Fatalf("rows after clearing the filter = %d, want 5", len(rows))
	}
	if rows[3].RefIdx != 1 || rows[4].RefIdx != 2 {
		t.Errorf("restored child RefIdx = %d,%d, want 1,2", rows[3].RefIdx, rows[4].RefIdx)
	}
}

// TestRefFilterMatchesPathOrPreviewCaseInsensitively pins the matcher:
// path substring or source-line substring, either case.
func TestRefFilterMatchesPathOrPreviewCaseInsensitively(t *testing.T) {
	loc := ReferenceLoc{File: "ged/Foo.go", Line: 12, Preview: "x := Bar()"}
	cases := []struct {
		filter string
		want   bool
	}{
		{"", true},
		{"foo", true},    // path, lowercased
		{"FOO.GO", true}, // path, uppercased
		{"ged/", true},   // directory part
		{"bar", true},    // preview, lowercased
		{"X :=", true},   // preview, mixed case
		{"baz", false},
		{"y :=", false},
	}
	for _, c := range cases {
		if got := refMatchesFilter(loc, c.filter); got != c.want {
			t.Errorf("refMatchesFilter(%+v, %q) = %v, want %v", loc, c.filter, got, c.want)
		}
	}

	// The filter applies to the flat view too, keeping slice order.
	p := NewReferencesPanel()
	p.SetLocations(refKindLocs())
	p.SetFilter("return")
	rows := p.visibleRows()
	if len(rows) != 1 || rows[0].Kind != refRowMatch || rows[0].RefIdx != 0 {
		t.Fatalf("flat filtered rows = %+v, want the single 'return Bar{}' row (RefIdx 0)", rows)
	}
}

// TestRefCountsFiltered checks the header tally counts the visible rows
// and the distinct files they span.
func TestRefCountsFiltered(t *testing.T) {
	locs := refKindLocs()
	cases := []struct {
		filter  string
		matches int
		files   int
	}{
		{"", 3, 2},
		{"foo", 2, 1},
		{"baz", 1, 1},
		{"zzz", 0, 0},
	}
	for _, c := range cases {
		m, f := refCounts(locs, c.filter)
		if m != c.matches || f != c.files {
			t.Errorf("refCounts(filter=%q) = (%d,%d), want (%d,%d)", c.filter, m, f, c.matches, c.files)
		}
	}
}

// TestRefHeaderLabelQuery checks the header text: the legacy tally with
// no query, the query plus match/file counts with one, and singular
// wording at a count of 1.
func TestRefHeaderLabelQuery(t *testing.T) {
	cases := []struct {
		query   string
		matches int
		files   int
		want    string
	}{
		{"", 3, 2, "引用 / References (3)"},
		{"", 0, 0, "引用 / References (0)"},
		{"Bar", 3, 2, "Bar — 3 matches in 2 files"},
		{"Bar", 1, 1, "Bar — 1 match in 1 file"},
		{"Bar", 0, 0, "Bar — 0 matches in 0 files"},
	}
	for _, c := range cases {
		if got := refHeaderLabel(c.query, c.matches, c.files); got != c.want {
			t.Errorf("refHeaderLabel(%q,%d,%d) = %q, want %q", c.query, c.matches, c.files, got, c.want)
		}
	}

	p := NewReferencesPanel()
	p.SetQuery("Bar")
	if got := p.Query(); got != "Bar" {
		t.Errorf("Query() = %q, want %q", got, "Bar")
	}
}

// TestRefGroupLabelText checks the group header text drops the directory
// and carries the child count.
func TestRefGroupLabelText(t *testing.T) {
	cases := []struct {
		file string
		n    int
		want string
	}{
		{"ged/foo.go", 2, "foo.go (2)"},
		{"core/baz.go", 1, "baz.go (1)"},
		{"main.go", 12, "main.go (12)"},
	}
	for _, c := range cases {
		if got := refGroupLabel(c.file, c.n); got != c.want {
			t.Errorf("refGroupLabel(%q,%d) = %q, want %q", c.file, c.n, got, c.want)
		}
	}
}

// TestRefKindBadges checks the Kind → badge mapping, that the zero value
// is a read (so legacy SetLocations rows still get a badge), and that an
// unknown kind renders no badge at all.
func TestRefKindBadges(t *testing.T) {
	cases := []struct {
		kind RefKind
		want string
	}{
		{RefDeclaration, "decl"},
		{RefWrite, "write"},
		{RefRead, "read"},
		{RefKind(99), ""},
	}
	for _, c := range cases {
		if got := refKindBadge(c.kind); got != c.want {
			t.Errorf("refKindBadge(%d) = %q, want %q", c.kind, got, c.want)
		}
	}
	var zero RefKind
	if got := refKindBadge(zero); got != "read" {
		t.Errorf("refKindBadge(zero value) = %q, want %q", got, "read")
	}

	// Badges of the panel's own rows come straight off the Kind field.
	p := groupedRefPanel()
	locs := p.Locations()
	want := []string{"read", "decl", "write"} // baz.go:7, foo.go:12, foo.go:30
	for i, w := range want {
		if got := refKindBadge(locs[i].Kind); got != w {
			t.Errorf("row %d (%s:%d) badge = %q, want %q", i, locs[i].File, locs[i].Line, got, w)
		}
	}

	// The three tints have to differ or the badges are unreadable.
	d, wr, rd := refKindTint(RefDeclaration), refKindTint(RefWrite), refKindTint(RefRead)
	if d == wr || d == rd || wr == rd {
		t.Errorf("kind tints are not distinct: decl=%+v write=%+v read=%+v", d, wr, rd)
	}
}

// TestRefCancelHotspot exercises the pure cancel hit-test plus the click
// path: the button only reacts while a search is in flight, only at the
// right end of the header, and firing it clears the loading flag.
func TestRefCancelHotspot(t *testing.T) {
	const w = 300.0
	if !refCancelHit(w-5, 10, w) {
		t.Error("refCancelHit at the header's right edge = false, want true")
	}
	if refCancelHit(5, 10, w) {
		t.Error("refCancelHit on the header's left side = true, want false")
	}
	if refCancelHit(w-5, referencesHeaderH+1, w) {
		t.Error("refCancelHit below the header band = true, want false")
	}

	p := groupedRefPanel()
	cancels := 0
	activations := 0
	p.SigCancel(func() { cancels++ })
	p.SigLocationActivated(func(string, int, int) { activations++ })

	// Not loading: the button is not drawn, so nothing fires.
	p.OnLeftDown(w-10, 10)
	if cancels != 0 {
		t.Errorf("cancel fired while idle (%d times)", cancels)
	}

	p.SetLoading(true)
	if !p.Loading() {
		t.Fatal("SetLoading(true) did not take")
	}
	// Loading but on the left of the header: still nothing.
	p.OnLeftDown(5, 10)
	if cancels != 0 {
		t.Errorf("cancel fired from a left-side header click (%d times)", cancels)
	}

	p.OnLeftDown(w-10, 10)
	if cancels != 1 {
		t.Fatalf("cancel fired %d times, want 1", cancels)
	}
	if p.Loading() {
		t.Error("the panel still reports loading after cancel")
	}
	if activations != 0 {
		t.Errorf("header clicks fired SigLocationActivated %d times, want 0", activations)
	}

	// Cancelling twice needs a fresh search first.
	p.OnLeftDown(w-10, 10)
	if cancels != 1 {
		t.Errorf("cancel fired again while idle: %d times, want 1", cancels)
	}
}

// TestRefClearResetsViewState checks Clear empties the rows and resets
// the query, collapse and loading state, while leaving the user's filter
// in place.
func TestRefClearResetsViewState(t *testing.T) {
	p := groupedRefPanel()
	p.SetQuery("Bar")
	p.SetFilter("foo")
	p.SetLoading(true)
	p.OnLeftDown(5, refRowMidY(p, 0)) // collapse the one visible group

	p.Clear()
	if got := len(p.Locations()); got != 0 {
		t.Errorf("Locations() after Clear = %d rows, want 0", got)
	}
	if got := len(p.visibleRows()); got != 0 {
		t.Errorf("visibleRows() after Clear = %d, want 0", got)
	}
	if p.Query() != "" {
		t.Errorf("Query() after Clear = %q, want empty", p.Query())
	}
	if p.Loading() {
		t.Error("Loading() after Clear = true, want false")
	}
	if p.IsCollapsed("ged/foo.go") {
		t.Error("collapse state survived Clear")
	}
	if p.Filter() != "foo" {
		t.Errorf("Filter() after Clear = %q, want %q (a sticky view setting)", p.Filter(), "foo")
	}
}
