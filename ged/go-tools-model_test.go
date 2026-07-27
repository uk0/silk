package ged

import (
	"reflect"
	"testing"

	"github.com/uk0/silk/core"
)

// GL-free throughout: the model helpers are exercised as free functions
// and the panel only through SetTools/SetFindings and the mouse handlers,
// never through Draw. No analyzer is ever executed — the findings below
// are fixtures, not tool output.

// goToolsAvailFixture is a PATH snapshot with the toolchain and
// staticcheck installed but govulncheck missing, which is the interesting
// mixed case for the picker.
func goToolsAvailFixture() []core.Tool {
	return []core.Tool{
		{Name: "go", Path: "/usr/local/go/bin/go", Available: true},
		{Name: "govulncheck"},
		{Name: "staticcheck", Path: "/Users/x/go/bin/staticcheck", Available: true},
	}
}

// sampleGoToolFindings mixes the three parser shapes — a vet diagnostic,
// two staticcheck diagnostics, a pprof row with no source location — and
// deliberately interleaves the tools so grouping has to reorder them.
func sampleGoToolFindings() []core.Finding {
	return []core.Finding{
		{Tool: core.ToolGoVet, File: "core/gotools.go", Line: 12, Col: 2,
			Severity: core.FindingError, Message: "unreachable code"},
		{Tool: core.ToolStaticcheck, File: "ged/go-tools-panel.go", Line: 40, Col: 5,
			Severity: core.FindingError, Message: "value never used", Code: "SA4006"},
		{Tool: core.ToolGoVet, File: "ged/codegen.go", Line: 88,
			Severity: core.FindingWarning, Message: "warning: shadowed variable"},
		{Tool: core.ToolPprofTop, Severity: core.FindingWarning, Code: "30.61%",
			Message: "runtime.mallocgc (flat 300ms 30.61%, cum 400ms 40.82%)"},
		{Tool: core.ToolStaticcheck, File: "core/git.go", Line: 9, Col: 6,
			Severity: core.FindingError, Message: "func runGit is unused", Code: "U1000"},
	}
}

// --- Picker model ---

// TestBuildGoToolRows checks the picker is built in core.GoToolWorkflows
// order and that Available follows the binary each workflow needs: the
// five go-subcommand workflows track "go", govulncheck is greyed out,
// staticcheck is not.
func TestBuildGoToolRows(t *testing.T) {
	rows := buildGoToolRows(goToolsAvailFixture())
	ids := core.GoToolWorkflows()
	if len(rows) != len(ids) {
		t.Fatalf("buildGoToolRows returned %d rows, want %d: %+v", len(rows), len(ids), rows)
	}
	for i, id := range ids {
		if rows[i].Id != id {
			t.Errorf("row[%d].Id = %q, want %q", i, rows[i].Id, id)
		}
		wantBin := core.GoToolBinary(id)
		if rows[i].Binary != wantBin {
			t.Errorf("row[%d].Binary = %q, want %q", i, rows[i].Binary, wantBin)
		}
		wantAvail := wantBin == "go" || wantBin == "staticcheck"
		if rows[i].Available != wantAvail {
			t.Errorf("row[%d] (%s via %s).Available = %v, want %v",
				i, id, wantBin, rows[i].Available, wantAvail)
		}
	}
}

// TestBuildGoToolRowsEmptySnapshot checks an empty snapshot greys out the
// whole picker rather than defaulting anything to available.
func TestBuildGoToolRowsEmptySnapshot(t *testing.T) {
	rows := buildGoToolRows(nil)
	if len(rows) != len(core.GoToolWorkflows()) {
		t.Fatalf("buildGoToolRows(nil) returned %d rows, want %d", len(rows), len(core.GoToolWorkflows()))
	}
	for i, r := range rows {
		if r.Available {
			t.Errorf("row[%d] = %+v, want Available false with no snapshot", i, r)
		}
	}
}

// --- Grouping ---

// TestGroupFindingsByTool checks findings bucket by tool in first-seen
// group order, each group keeping its findings in input order.
func TestGroupFindingsByTool(t *testing.T) {
	in := sampleGoToolFindings()
	groups := groupFindingsByTool(in)

	wantTools := []string{core.ToolGoVet, core.ToolStaticcheck, core.ToolPprofTop}
	gotTools := make([]string, len(groups))
	for i, g := range groups {
		gotTools[i] = g.Tool
	}
	if !reflect.DeepEqual(gotTools, wantTools) {
		t.Fatalf("group order = %#v, want %#v", gotTools, wantTools)
	}

	wantCounts := []int{2, 2, 1}
	for i, want := range wantCounts {
		if got := len(groups[i].Findings); got != want {
			t.Errorf("group %q has %d findings, want %d", groups[i].Tool, got, want)
		}
	}
	// Input order preserved inside a group: vet's rows are in[0] then in[2].
	if got := groups[0].Findings; !reflect.DeepEqual(got, []core.Finding{in[0], in[2]}) {
		t.Errorf("vet group = %+v, want %+v", got, []core.Finding{in[0], in[2]})
	}
	if got := groups[1].Findings; !reflect.DeepEqual(got, []core.Finding{in[1], in[4]}) {
		t.Errorf("staticcheck group = %+v, want %+v", got, []core.Finding{in[1], in[4]})
	}
}

// TestGroupFindingsByToolEmpty checks no findings yields no groups.
func TestGroupFindingsByToolEmpty(t *testing.T) {
	if got := groupFindingsByTool(nil); got != nil {
		t.Errorf("groupFindingsByTool(nil) = %+v, want nil", got)
	}
	if got := groupFindingsByTool([]core.Finding{}); got != nil {
		t.Errorf("groupFindingsByTool(empty) = %+v, want nil", got)
	}
}

// --- Flat row layout ---

// TestBuildGoToolsRows checks the flat sequence: every picker row first,
// then each group header followed by its findings.
func TestBuildGoToolsRows(t *testing.T) {
	tools := buildGoToolRows(goToolsAvailFixture())
	groups := groupFindingsByTool(sampleGoToolFindings())
	rows := buildGoToolsRows(tools, groups, nil)

	nPicker := len(tools)
	// 7 picker + (1 header + 2) + (1 header + 2) + (1 header + 1) = 15
	if want := nPicker + 3 + 5; len(rows) != want {
		t.Fatalf("buildGoToolsRows returned %d rows, want %d: %+v", len(rows), want, rows)
	}
	for i := 0; i < nPicker; i++ {
		if rows[i].Kind != goToolsRowPicker || rows[i].ToolIdx != i {
			t.Errorf("row[%d] = %+v, want picker row for tool %d", i, rows[i], i)
		}
	}
	want := []goToolsRow{
		{Kind: goToolsRowGroup, GroupIdx: 0},
		{Kind: goToolsRowFinding, GroupIdx: 0, Index: 0},
		{Kind: goToolsRowFinding, GroupIdx: 0, Index: 1},
		{Kind: goToolsRowGroup, GroupIdx: 1},
		{Kind: goToolsRowFinding, GroupIdx: 1, Index: 0},
		{Kind: goToolsRowFinding, GroupIdx: 1, Index: 1},
		{Kind: goToolsRowGroup, GroupIdx: 2},
		{Kind: goToolsRowFinding, GroupIdx: 2, Index: 0},
	}
	if got := rows[nPicker:]; !reflect.DeepEqual(got, want) {
		t.Errorf("findings rows = %+v, want %+v", got, want)
	}
}

// TestBuildGoToolsRowsCollapsed checks a collapsed group keeps its header
// but drops its finding rows, and that collapsing one group leaves the
// others alone.
func TestBuildGoToolsRowsCollapsed(t *testing.T) {
	tools := buildGoToolRows(goToolsAvailFixture())
	groups := groupFindingsByTool(sampleGoToolFindings())
	collapsed := map[string]bool{core.ToolStaticcheck: true}
	rows := buildGoToolsRows(tools, groups, collapsed)

	nPicker := len(tools)
	// staticcheck's two findings are gone; its header stays.
	if want := nPicker + 3 + 3; len(rows) != want {
		t.Fatalf("collapsed layout has %d rows, want %d: %+v", len(rows), want, rows)
	}
	for _, r := range rows[nPicker:] {
		if r.Kind == goToolsRowFinding && groups[r.GroupIdx].Tool == core.ToolStaticcheck {
			t.Errorf("collapsed group still renders finding row %+v", r)
		}
	}
}

// TestGoToolsRowAtY exercises the pure hit-test: rows start at topOffset
// with height rowH; the header band, out-of-range and degenerate inputs
// all yield -1.
func TestGoToolsRowAtY(t *testing.T) {
	const (
		top = goToolsHeaderH
		rh  = 20.0
		n   = 4
	)
	cases := []struct {
		name string
		y    float64
		want int
	}{
		{"above rows (header)", 10, -1},
		{"top of row 0", top, 0},
		{"middle of row 0", top + rh/2, 0},
		{"middle of row 3", top + 3*rh + rh/2, 3},
		{"last pixel of row 3", top + 4*rh - 0.5, 3},
		{"just past last row", top + 4*rh, -1},
		{"far below", 10000, -1},
	}
	for _, c := range cases {
		if got := goToolsRowAtY(c.y, top, rh, n); got != c.want {
			t.Errorf("%s: goToolsRowAtY(%v,%v,%v,%d) = %d, want %d",
				c.name, c.y, top, rh, n, got, c.want)
		}
	}
	if got := goToolsRowAtY(50, top, 0, n); got != -1 {
		t.Errorf("goToolsRowAtY with rowH=0 = %d, want -1", got)
	}
	if got := goToolsRowAtY(top+5, top, rh, 0); got != -1 {
		t.Errorf("goToolsRowAtY with count=0 = %d, want -1", got)
	}
}

// --- Labels ---

// TestGoToolsLabels checks the row texts: the header tally, the picker's
// missing-binary hint, the group header's glyph and count, the locator
// formatting, and the code prefix on a message.
func TestGoToolsLabels(t *testing.T) {
	if got, want := goToolsHeaderLabel(5), "Go 工具 / Go Tools (5)"; got != want {
		t.Errorf("goToolsHeaderLabel(5) = %q, want %q", got, want)
	}
	if got, want := goToolsHeaderLabel(0), "Go 工具 / Go Tools (0)"; got != want {
		t.Errorf("goToolsHeaderLabel(0) = %q, want %q", got, want)
	}

	pickerCases := []struct {
		row  GoToolRow
		want string
	}{
		{GoToolRow{Id: core.ToolGoVet, Binary: "go", Available: true}, "go vet"},
		{GoToolRow{Id: core.ToolGovulncheck, Binary: "govulncheck"}, "govulncheck (govulncheck not on PATH)"},
		{GoToolRow{Id: core.ToolGoTestRace, Binary: "go"}, "go test -race (go not on PATH)"},
		{GoToolRow{Id: "mystery"}, "mystery (unknown tool)"},
	}
	for _, c := range pickerCases {
		if got := goToolPickerLabel(c.row); got != c.want {
			t.Errorf("goToolPickerLabel(%+v) = %q, want %q", c.row, got, c.want)
		}
	}

	grp := GoToolGroup{Tool: core.ToolGoVet, Findings: make([]core.Finding, 3)}
	if got, want := goToolGroupLabel(grp, false), "▼ go vet (3)"; got != want {
		t.Errorf("goToolGroupLabel(expanded) = %q, want %q", got, want)
	}
	if got, want := goToolGroupLabel(grp, true), "▶ go vet (3)"; got != want {
		t.Errorf("goToolGroupLabel(collapsed) = %q, want %q", got, want)
	}

	locCases := []struct {
		f    core.Finding
		want string
	}{
		{core.Finding{File: "core/gotools.go", Line: 12, Col: 2}, "gotools.go:12:2"},
		{core.Finding{File: "ged/codegen.go", Line: 88}, "codegen.go:88"},
		{core.Finding{}, ""},
	}
	for _, c := range locCases {
		if got := goToolFindingLabel(c.f); got != c.want {
			t.Errorf("goToolFindingLabel(%+v) = %q, want %q", c.f, got, c.want)
		}
	}

	textCases := []struct {
		f    core.Finding
		want string
	}{
		{core.Finding{Message: "unreachable code"}, "unreachable code"},
		{core.Finding{Message: "value never used", Code: "SA4006"}, "SA4006: value never used"},
	}
	for _, c := range textCases {
		if got := goToolFindingText(c.f); got != c.want {
			t.Errorf("goToolFindingText(%+v) = %q, want %q", c.f, got, c.want)
		}
	}
}

// --- Panel data plumbing ---

// newTestGoToolsPanel builds a panel with a deterministic picker
// (goToolsAvailFixture) and the sample findings, sized so the whole row
// list is addressable.
func newTestGoToolsPanel() *GoToolsPanel {
	p := NewGoToolsPanel()
	p.SetSize(400, 600)
	p.SetTools(goToolsAvailFixture())
	p.SetFindings(sampleGoToolFindings())
	return p
}

// TestGoToolsPanelDefaultPicker checks a fresh panel already lists every
// workflow (availability comes from PATH, so only the ids are asserted).
func TestGoToolsPanelDefaultPicker(t *testing.T) {
	p := NewGoToolsPanel()
	rows := p.ToolRows()
	ids := core.GoToolWorkflows()
	if len(rows) != len(ids) {
		t.Fatalf("fresh panel has %d picker rows, want %d", len(rows), len(ids))
	}
	for i, id := range ids {
		if rows[i].Id != id {
			t.Errorf("picker row[%d].Id = %q, want %q", i, rows[i].Id, id)
		}
	}
	if got := p.Findings(); len(got) != 0 {
		t.Errorf("fresh panel Findings() = %+v, want none", got)
	}
}

// TestGoToolsPanelCopySemantics verifies the panel keeps its own copies:
// mutating the input slice after SetFindings, or the slice ToolRows /
// Findings hands back, must not reach its state.
func TestGoToolsPanelCopySemantics(t *testing.T) {
	p := NewGoToolsPanel()
	p.SetTools(goToolsAvailFixture())
	in := sampleGoToolFindings()
	p.SetFindings(in)

	in[0].Message = "MUTATED"
	if got := p.Findings(); got[0].Message != "unreachable code" {
		t.Errorf("input mutation leaked into panel: %+v", got[0])
	}
	out := p.Findings()
	out[1].Message = "MUTATED"
	if got := p.Findings(); got[1].Message != "value never used" {
		t.Errorf("returned-slice mutation leaked into panel: %+v", got[1])
	}
	rows := p.ToolRows()
	rows[0].Id = "MUTATED"
	if got := p.ToolRows(); got[0].Id != core.ToolGoVet {
		t.Errorf("ToolRows() returned an aliased slice: %q", got[0].Id)
	}
	groups := p.Groups()
	groups[0].Tool = "MUTATED"
	if got := p.Groups(); got[0].Tool != core.ToolGoVet {
		t.Errorf("Groups() returned an aliased slice: %q", got[0].Tool)
	}
}

// TestGoToolsPanelClearKeepsPicker checks Clear drops findings but leaves
// the picker in place, so the user can immediately re-run.
func TestGoToolsPanelClearKeepsPicker(t *testing.T) {
	p := newTestGoToolsPanel()
	p.Clear()
	if got := p.Findings(); len(got) != 0 {
		t.Errorf("Findings() after Clear = %+v, want none", got)
	}
	if got := p.Groups(); len(got) != 0 {
		t.Errorf("Groups() after Clear = %+v, want none", got)
	}
	if got := len(p.ToolRows()); got != len(core.GoToolWorkflows()) {
		t.Errorf("picker lost rows on Clear: %d", got)
	}
}

// --- Panel click routing ---

// goToolsRowY is the y of the middle of flat row idx, matching the
// panel's own geometry (rows start below the header, no scroll).
func goToolsRowY(p *GoToolsPanel, idx int) float64 {
	return goToolsHeaderH + float64(idx)*p.rowHeight + p.rowHeight/2
}

// TestGoToolsPanelRunClick drives a click on an available picker row and
// checks SigRun fires with that workflow id.
func TestGoToolsPanelRunClick(t *testing.T) {
	p := newTestGoToolsPanel()
	ids := core.GoToolWorkflows()

	var got string
	fired := 0
	p.SigRun(func(tool string) {
		got = tool
		fired++
	})
	p.SigActivate(func(string, int) { t.Error("SigActivate fired for a picker row") })

	// staticcheck is the last workflow and is available in the fixture.
	idx := len(ids) - 1
	p.OnLeftDown(5, goToolsRowY(p, idx))
	if fired != 1 {
		t.Fatalf("SigRun fired %d times, want 1", fired)
	}
	if want := ids[idx]; got != want {
		t.Errorf("SigRun tool = %q, want %q", got, want)
	}
}

// TestGoToolsPanelUnavailableRunClickInert checks the row for a missing
// binary does not start a run.
func TestGoToolsPanelUnavailableRunClickInert(t *testing.T) {
	p := newTestGoToolsPanel()
	ids := core.GoToolWorkflows()
	idx := -1
	for i, id := range ids {
		if id == core.ToolGovulncheck {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatal("govulncheck missing from the workflow list")
	}
	if p.ToolRows()[idx].Available {
		t.Fatalf("fixture claims govulncheck is available: %+v", p.ToolRows()[idx])
	}

	fired := false
	p.SigRun(func(string) { fired = true })
	p.OnLeftDown(5, goToolsRowY(p, idx))
	if fired {
		t.Error("clicking an unavailable picker row fired SigRun")
	}
}

// TestGoToolsPanelFindingClickActivates clicks the first finding row (just
// below the first group header) and checks SigActivate gets its file and
// 1-based line.
func TestGoToolsPanelFindingClickActivates(t *testing.T) {
	p := newTestGoToolsPanel()
	nPicker := len(p.ToolRows())

	var (
		gotFile string
		gotLine int
		fired   int
	)
	p.SigActivate(func(file string, line int) {
		gotFile = file
		gotLine = line
		fired++
	})
	p.SigRun(func(string) { t.Error("SigRun fired for a finding row") })

	// nPicker = group header of the vet group, nPicker+1 = its first finding.
	p.OnLeftDown(5, goToolsRowY(p, nPicker+1))
	if fired != 1 {
		t.Fatalf("SigActivate fired %d times, want 1", fired)
	}
	want := sampleGoToolFindings()[0]
	if gotFile != want.File || gotLine != want.Line {
		t.Errorf("SigActivate = (%q,%d), want (%q,%d)", gotFile, gotLine, want.File, want.Line)
	}
}

// TestGoToolsPanelLocationlessFindingInert checks a finding with no source
// location — the pprof row — cannot be jumped to.
func TestGoToolsPanelLocationlessFindingInert(t *testing.T) {
	p := NewGoToolsPanel()
	p.SetSize(400, 600)
	p.SetTools(goToolsAvailFixture())
	p.SetFindings([]core.Finding{{
		Tool: core.ToolPprofTop, Severity: core.FindingWarning, Code: "30.61%",
		Message: "runtime.mallocgc (flat 300ms 30.61%, cum 400ms 40.82%)",
	}})

	fired := false
	p.SigActivate(func(string, int) { fired = true })
	nPicker := len(p.ToolRows())
	p.OnLeftDown(5, goToolsRowY(p, nPicker+1)) // the single pprof finding
	if fired {
		t.Error("clicking a finding with no file fired SigActivate")
	}
}

// TestGoToolsPanelGroupClickCollapses checks a group header click folds the
// group (dropping its finding rows) and unfolds it again, without firing
// either callback.
func TestGoToolsPanelGroupClickCollapses(t *testing.T) {
	p := newTestGoToolsPanel()
	nPicker := len(p.ToolRows())
	before := len(p.rows())

	p.SigRun(func(string) { t.Error("SigRun fired for a group header") })
	p.SigActivate(func(string, int) { t.Error("SigActivate fired for a group header") })

	headerY := goToolsRowY(p, nPicker) // first group header (go vet, 2 findings)
	p.OnLeftDown(5, headerY)
	if !p.IsCollapsed(core.ToolGoVet) {
		t.Fatal("group header click did not collapse the go vet group")
	}
	if got, want := len(p.rows()), before-2; got != want {
		t.Errorf("collapsed row count = %d, want %d", got, want)
	}

	p.OnLeftDown(5, headerY)
	if p.IsCollapsed(core.ToolGoVet) {
		t.Fatal("second group header click did not expand the go vet group")
	}
	if got := len(p.rows()); got != before {
		t.Errorf("expanded row count = %d, want %d", got, before)
	}
}

// TestGoToolsPanelHeaderClickNoop checks a click in the tally header band
// fires nothing.
func TestGoToolsPanelHeaderClickNoop(t *testing.T) {
	p := newTestGoToolsPanel()
	p.SigRun(func(string) { t.Error("SigRun fired for a header click") })
	p.SigActivate(func(string, int) { t.Error("SigActivate fired for a header click") })
	p.OnLeftDown(5, 5)
}

// TestGoToolsPanelCollapseSurvivesSetFindings checks re-running a tool does
// not re-expand a group the user folded away.
func TestGoToolsPanelCollapseSurvivesSetFindings(t *testing.T) {
	p := newTestGoToolsPanel()
	nPicker := len(p.ToolRows())
	p.OnLeftDown(5, goToolsRowY(p, nPicker))
	if !p.IsCollapsed(core.ToolGoVet) {
		t.Fatal("setup: group did not collapse")
	}
	p.SetFindings(sampleGoToolFindings())
	if !p.IsCollapsed(core.ToolGoVet) {
		t.Error("SetFindings reset the collapse state")
	}
}

// TestGoToolsPanelSizeHints sanity-checks the dock's minimum size.
func TestGoToolsPanelSizeHints(t *testing.T) {
	sh := NewGoToolsPanel().SizeHints()
	if sh.MinWidth <= 0 || sh.MinHeight <= 0 {
		t.Errorf("SizeHints = %+v, want positive minimums", sh)
	}
}
