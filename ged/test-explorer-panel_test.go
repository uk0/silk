package ged

import (
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

const (
	tePkgGed  = "github.com/uk0/silk/ged"
	tePkgCore = "github.com/uk0/silk/core"
)

// testExplorerFixture returns the first run every test starts from: two
// packages, a passing test, a failing test with one passing and one
// failing subtest, and a skipped test in the second package.
//
// Fully expanded (the model's default), it flattens to 7 rows:
//
//	0 ged                    depth 0
//	1 TestAlpha              depth 1
//	2 TestBeta               depth 1
//	3 TestBeta/case_one      depth 2
//	4 TestBeta/case_two      depth 2
//	5 core                   depth 0
//	6 TestGamma              depth 1
func testExplorerFixture() []PkgNode {
	return []PkgNode{
		{
			Path:    tePkgGed,
			Status:  TestNodeFail,
			Elapsed: 1.5,
			Tests: []TestNode{
				{Name: "TestAlpha", Status: TestNodePass, Elapsed: 0.01},
				{
					Name: "TestBeta", Status: TestNodeFail, Elapsed: 0.05,
					Output: "boom", File: "ged/beta_test.go", Line: 42,
				},
				{Name: "TestBeta/case_one", Status: TestNodePass},
				{
					Name: "TestBeta/case_two", Status: TestNodeFail,
					File: "ged/beta_test.go", Line: 51,
				},
			},
		},
		{
			Path:   tePkgCore,
			Status: TestNodePass,
			Tests:  []TestNode{{Name: "TestGamma", Status: TestNodeSkip}},
		},
	}
}

// teNewModel returns a model preloaded with testExplorerFixture.
func teNewModel() *TestExplorerModel {
	m := NewTestExplorerModel()
	m.SetResults(testExplorerFixture())
	return m
}

// teRowNames maps rows to their node display labels — the readable form for
// assertions about the flattened tree.
func teRowNames(rows []TestExplorerRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Node.Name
	}
	return out
}

func teEqNames(a, b []string) bool {
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

// TestTestExplorerMergeKeepsPriorStatus verifies a targeted re-run only
// updates what it reports: TestBeta flips to pass, while TestAlpha, the
// subtests it did not mention, and the whole untouched second package
// keep their earlier verdicts.
func TestTestExplorerMergeKeepsPriorStatus(t *testing.T) {
	m := teNewModel()

	m.SetResults([]PkgNode{{
		Path:   tePkgGed,
		Status: TestNodePass,
		Tests:  []TestNode{{Name: "TestBeta", Status: TestNodePass, Elapsed: 0.03}},
	}})

	beta := m.Find(tePkgGed, "TestBeta")
	if beta == nil {
		t.Fatal("TestBeta missing after merge")
	}
	if beta.Status != TestNodePass {
		t.Errorf("TestBeta status = %v, want pass", beta.Status)
	}
	if beta.Elapsed != 0.03 {
		t.Errorf("TestBeta elapsed = %v, want 0.03", beta.Elapsed)
	}
	// Output and the locator came from the previous run; an empty
	// incoming value must not erase them.
	if beta.Output != "boom" {
		t.Errorf("TestBeta output = %q, want %q (kept from prior run)", beta.Output, "boom")
	}
	if beta.File != "ged/beta_test.go" || beta.Line != 42 {
		t.Errorf("TestBeta locator = %s:%d, want ged/beta_test.go:42", beta.File, beta.Line)
	}

	if n := m.Find(tePkgGed, "TestAlpha"); n == nil || n.Status != TestNodePass {
		t.Errorf("TestAlpha = %+v, want kept at pass", n)
	}
	if n := m.Find(tePkgGed, "TestBeta/case_two"); n == nil || n.Status != TestNodeFail {
		t.Errorf("TestBeta/case_two = %+v, want kept at fail", n)
	}
	// The second package was absent from the re-run entirely.
	if n := m.Find(tePkgCore, "TestGamma"); n == nil || n.Status != TestNodeSkip {
		t.Errorf("TestGamma = %+v, want kept at skip", n)
	}
	if got := len(m.Packages()); got != 2 {
		t.Errorf("packages after merge = %d, want 2 (no duplicates)", got)
	}
}

// TestTestExplorerMergeRunningThenFail verifies progressive pushes
// accumulate: a "running" update keeps the earlier output, and the final
// verdict overwrites only the status.
func TestTestExplorerMergeRunningThenFail(t *testing.T) {
	m := NewTestExplorerModel()
	m.SetResults([]PkgNode{{Path: tePkgGed, Tests: []TestNode{
		{Name: "TestX", Status: TestNodeFail, Output: "old failure"},
	}}})
	m.SetResults([]PkgNode{{Path: tePkgGed, Tests: []TestNode{
		{Name: "TestX", Status: TestNodeRunning},
	}}})

	n := m.Find(tePkgGed, "TestX")
	if n.Status != TestNodeRunning {
		t.Errorf("status = %v, want running", n.Status)
	}
	if n.Output != "old failure" {
		t.Errorf("output = %q, want the prior output preserved", n.Output)
	}

	m.SetResults([]PkgNode{{Path: tePkgGed, Tests: []TestNode{
		{Name: "TestX", Status: TestNodePass, Elapsed: 0.2, Output: "ok"},
	}}})
	if n.Status != TestNodePass || n.Output != "ok" || n.Elapsed != 0.2 {
		t.Errorf("after final push = %+v, want pass/ok/0.2", n)
	}
}

// TestTestExplorerSubtestNesting verifies "TestBeta/case_x" rows land
// under TestBeta instead of becoming flat children of the package, and
// that each node keeps its full test path.
func TestTestExplorerSubtestNesting(t *testing.T) {
	m := teNewModel()

	pkg := m.Find(tePkgGed, "")
	if pkg == nil {
		t.Fatal("package node missing")
	}
	if !pkg.IsPackage() {
		t.Error("package node reports IsPackage() == false")
	}
	if got := len(pkg.Children); got != 2 {
		t.Fatalf("package children = %d, want 2 (TestAlpha, TestBeta)", got)
	}

	beta := m.Find(tePkgGed, "TestBeta")
	if got := len(beta.Children); got != 2 {
		t.Fatalf("TestBeta children = %d, want 2 subtests", got)
	}
	for i, want := range []string{"case_one", "case_two"} {
		c := beta.Children[i]
		if c.Name != want {
			t.Errorf("subtest %d name = %q, want %q", i, c.Name, want)
		}
		if c.Test != "TestBeta/"+want {
			t.Errorf("subtest %d test path = %q, want %q", i, c.Test, "TestBeta/"+want)
		}
		if c.Pkg != tePkgGed {
			t.Errorf("subtest %d pkg = %q, want %q", i, c.Pkg, tePkgGed)
		}
		if c.IsPackage() {
			t.Errorf("subtest %d reports IsPackage() == true", i)
		}
	}
}

// TestTestExplorerImplicitParent verifies a subtest reported without its
// parent still nests: the parent node is materialised with an unknown
// status.
func TestTestExplorerImplicitParent(t *testing.T) {
	m := NewTestExplorerModel()
	m.SetResults([]PkgNode{{Path: tePkgGed, Tests: []TestNode{
		{Name: "TestOuter/inner", Status: TestNodeFail},
	}}})

	outer := m.Find(tePkgGed, "TestOuter")
	if outer == nil {
		t.Fatal("implicit parent TestOuter was not created")
	}
	if outer.Status != TestNodeUnknown {
		t.Errorf("implicit parent status = %v, want unknown", outer.Status)
	}
	if len(outer.Children) != 1 || outer.Children[0].Name != "inner" {
		t.Fatalf("TestOuter children = %+v, want one node named inner", outer.Children)
	}
}

// TestTestExplorerFlattenExpansion verifies Rows() honours each node's
// expansion flag, including the depth it reports.
func TestTestExplorerFlattenExpansion(t *testing.T) {
	m := teNewModel()

	rows := m.Rows()
	wantNames := []string{tePkgGed, "TestAlpha", "TestBeta", "case_one", "case_two", tePkgCore, "TestGamma"}
	if !teEqNames(teRowNames(rows), wantNames) {
		t.Fatalf("rows = %v\nwant %v", teRowNames(rows), wantNames)
	}
	wantDepth := []int{0, 1, 1, 2, 2, 0, 1}
	for i, d := range wantDepth {
		if rows[i].Depth != d {
			t.Errorf("row %d (%s) depth = %d, want %d", i, rows[i].Node.Name, rows[i].Depth, d)
		}
	}

	m.CollapseAll()
	if got := teRowNames(m.Rows()); !teEqNames(got, []string{tePkgGed, tePkgCore}) {
		t.Fatalf("collapsed rows = %v, want the two package rows", got)
	}

	// Expanding just one package reveals its tests but not the subtests
	// of the still-collapsed TestBeta.
	m.Find(tePkgGed, "").Expanded = true
	if got := teRowNames(m.Rows()); !teEqNames(got, []string{tePkgGed, "TestAlpha", "TestBeta", tePkgCore}) {
		t.Fatalf("one-package rows = %v", got)
	}

	m.ExpandAll()
	if got := teRowNames(m.Rows()); !teEqNames(got, wantNames) {
		t.Fatalf("re-expanded rows = %v\nwant %v", got, wantNames)
	}
}

// TestTestExplorerFailedOnly verifies the failed-only toggle keeps
// failures plus the ancestors that lead to them, and drops passing and
// skipped siblings.
func TestTestExplorerFailedOnly(t *testing.T) {
	m := teNewModel()
	m.SetFailedOnly(true)
	if !m.FailedOnly() {
		t.Fatal("FailedOnly() == false after SetFailedOnly(true)")
	}

	want := []string{tePkgGed, "TestBeta", "case_two"}
	if got := teRowNames(m.Rows()); !teEqNames(got, want) {
		t.Fatalf("failed-only rows = %v\nwant %v", got, want)
	}

	m.SetFailedOnly(false)
	if got := len(m.Rows()); got != 7 {
		t.Fatalf("rows after clearing the toggle = %d, want 7", got)
	}
}

// TestTestExplorerFilter verifies the text filter is case-insensitive,
// keeps the ancestor path of a match on screen, and drops branches with
// no match at all.
func TestTestExplorerFilter(t *testing.T) {
	m := teNewModel()

	m.SetFilter("ALPHA")
	if got, want := m.FilterText(), "alpha"; got != want {
		t.Errorf("FilterText() = %q, want %q", got, want)
	}
	if got := teRowNames(m.Rows()); !teEqNames(got, []string{tePkgGed, "TestAlpha"}) {
		t.Fatalf("filtered rows = %v, want the ged package and TestAlpha", got)
	}

	// A subtest match keeps its parent test and package visible while
	// its passing sibling stays hidden.
	m.SetFilter("case_two")
	if got := teRowNames(m.Rows()); !teEqNames(got, []string{tePkgGed, "TestBeta", "case_two"}) {
		t.Fatalf("subtest-filtered rows = %v", got)
	}

	// A parent match reveals its whole subtree.
	m.SetFilter("TestBeta")
	if got := teRowNames(m.Rows()); !teEqNames(got, []string{tePkgGed, "TestBeta", "case_one", "case_two"}) {
		t.Fatalf("parent-filtered rows = %v", got)
	}

	m.SetFilter("")
	if got := len(m.Rows()); got != 7 {
		t.Fatalf("rows after clearing the filter = %d, want 7", got)
	}
}

// TestTestExplorerCountsAndFailedTests verifies the header tally counts
// each executed leaf exactly once, and that FailedTests reports only the
// top-most failure so a re-run command stays minimal.
func TestTestExplorerCountsAndFailedTests(t *testing.T) {
	m := teNewModel()

	pass, fail, skip := m.Counts()
	if pass != 2 || fail != 1 || skip != 1 {
		t.Errorf("Counts() = (%d,%d,%d), want (2,1,1)", pass, fail, skip)
	}

	failed := m.FailedTests()
	if len(failed) != 1 {
		t.Fatalf("FailedTests() = %+v, want only the top-most failure", failed)
	}
	if failed[0] != (TestRef{Pkg: tePkgGed, Test: "TestBeta"}) {
		t.Errorf("FailedTests()[0] = %+v, want {ged TestBeta}", failed[0])
	}

	m.Clear()
	if len(m.Packages()) != 0 || len(m.Rows()) != 0 || len(m.FailedTests()) != 0 {
		t.Error("Clear() left state behind")
	}
}

// TestTestExplorerExpanderVsRowHitTest verifies a click in the indent
// gutter toggles expansion without activating the row, while a click on
// the label activates it — row 2 is TestBeta, which has both children
// and a failure locator.
func TestTestExplorerExpanderVsRowHitTest(t *testing.T) {
	p := NewTestExplorerPanel()
	p.SetResults(testExplorerFixture())

	var (
		activated int
		gotFile   string
		gotLine   int
	)
	p.SigActivate(func(file string, line int) {
		activated++
		gotFile, gotLine = file, line
	})

	betaY := testExplorerHeaderH + 2*p.rowHeight + p.rowHeight/2

	// Expander box of a depth-1 row: [20, 32).
	p.OnLeftDown(testExplorerIndent(1)+2, betaY)
	if n := teFindTest(p, "TestBeta"); n.Expanded {
		t.Error("click in the expander box did not collapse TestBeta")
	}
	if activated != 0 {
		t.Error("expander click fired SigActivate")
	}
	if got := len(p.Rows()); got != 5 {
		t.Errorf("rows after collapsing TestBeta = %d, want 5", got)
	}

	// Same row, but past the expander: activates instead of toggling.
	p.OnLeftDown(120, betaY)
	if activated != 1 {
		t.Fatalf("label click fired SigActivate %d times, want 1", activated)
	}
	if gotFile != "ged/beta_test.go" || gotLine != 42 {
		t.Errorf("activated %s:%d, want ged/beta_test.go:42", gotFile, gotLine)
	}
	if teFindTest(p, "TestBeta").Expanded {
		t.Error("label click toggled expansion")
	}

	// A childless row has no expander at all: the same x activates it.
	if testExplorerExpanderHit(testExplorerIndent(1)+2, 1, false) {
		t.Error("testExplorerExpanderHit: childless row reported a hit")
	}
	// Header-band clicks are inert.
	before := len(p.Rows())
	p.OnLeftDown(5, 5)
	if len(p.Rows()) != before {
		t.Error("header-band click changed the rows")
	}
}

// teFindTest is a tiny lookup helper for panel tests: the fixture only has
// tests in the ged package.
func teFindTest(p *TestExplorerPanel, test string) *TestTreeNode {
	return p.Model().Find(tePkgGed, test)
}

// TestTestExplorerSigRunCarriesPkgAndTest verifies the "运行" entry hands
// the host both the package import path and the full test path, and that
// a package row asks for the whole package (empty test).
func TestTestExplorerSigRunCarriesPkgAndTest(t *testing.T) {
	p := NewTestExplorerPanel()
	p.SetResults(testExplorerFixture())

	var gotPkg, gotTest string
	fired := 0
	p.SigRun(func(pkg, test string) {
		gotPkg, gotTest = pkg, test
		fired++
	})

	// Row 4 is TestBeta/case_two.
	items := p.buildContextMenu(4)
	if len(items) == 0 {
		t.Fatal("buildContextMenu(4) returned no entries")
	}
	if items[0].Label != "运行" {
		t.Fatalf("first entry = %q, want 运行", items[0].Label)
	}
	if !items[0].Enabled {
		t.Fatal("运行 disabled although SigRun is bound")
	}
	items[0].Action()
	if fired != 1 || gotPkg != tePkgGed || gotTest != "TestBeta/case_two" {
		t.Fatalf("SigRun fired %d times with (%q, %q), want 1 with (%q, %q)",
			fired, gotPkg, gotTest, tePkgGed, "TestBeta/case_two")
	}

	// Row 0 is the package itself: no test name means "run the package".
	p.buildContextMenu(0)[0].Action()
	if gotPkg != tePkgGed || gotTest != "" {
		t.Errorf("package row ran (%q, %q), want (%q, \"\")", gotPkg, gotTest, tePkgGed)
	}

	if p.buildContextMenu(-1) != nil || p.buildContextMenu(99) != nil {
		t.Error("out-of-range rows produced a menu")
	}
}

// TestTestExplorerSigDebugAndRerunFailed verifies the debug entry carries
// the same target as run, and that rerun-failed is only offered when the
// tree actually holds a failure.
func TestTestExplorerSigDebugAndRerunFailed(t *testing.T) {
	p := NewTestExplorerPanel()
	p.SetResults(testExplorerFixture())

	var dbgPkg, dbgTest string
	p.SigDebug(func(pkg, test string) { dbgPkg, dbgTest = pkg, test })
	rerun := 0
	p.SigRerunFailed(func() { rerun++ })

	items := p.buildContextMenu(2) // TestBeta
	if items[1].Label != "调试" {
		t.Fatalf("second entry = %q, want 调试", items[1].Label)
	}
	items[1].Action()
	if dbgPkg != tePkgGed || dbgTest != "TestBeta" {
		t.Errorf("SigDebug got (%q, %q), want (%q, TestBeta)", dbgPkg, dbgTest, tePkgGed)
	}

	if items[2].Label != "重跑失败的测试" || !items[2].Enabled {
		t.Fatalf("third entry = %+v, want an enabled 重跑失败的测试", items[2])
	}
	items[2].Action()
	if rerun != 1 {
		t.Errorf("SigRerunFailed fired %d times, want 1", rerun)
	}

	// No failures in the tree -> the entry is offered but disabled.
	green := NewTestExplorerPanel()
	green.SigRerunFailed(func() {})
	green.SetResults([]PkgNode{{Path: tePkgGed, Status: TestNodePass, Tests: []TestNode{
		{Name: "TestOK", Status: TestNodePass},
	}}})
	if green.buildContextMenu(0)[2].Enabled {
		t.Error("重跑失败的测试 enabled with no failures in the tree")
	}
}

// TestTestExplorerMenuViewEntries verifies the view entries act on the
// model: expand/collapse all and the failed-only toggle.
func TestTestExplorerMenuViewEntries(t *testing.T) {
	p := NewTestExplorerPanel()
	p.SetResults(testExplorerFixture())

	items := p.buildContextMenu(0)
	byLabel := make(map[string]testExplorerMenuItem, len(items))
	for _, it := range items {
		if !it.Separator {
			byLabel[it.Label] = it
		}
	}

	byLabel["折叠全部"].Action()
	if got := len(p.Rows()); got != 2 {
		t.Errorf("rows after 折叠全部 = %d, want 2", got)
	}
	byLabel["展开全部"].Action()
	if got := len(p.Rows()); got != 7 {
		t.Errorf("rows after 展开全部 = %d, want 7", got)
	}
	byLabel["仅显示失败"].Action()
	if !p.Model().FailedOnly() {
		t.Error("仅显示失败 did not turn the toggle on")
	}
	if got := len(p.Rows()); got != 3 {
		t.Errorf("rows with 仅显示失败 = %d, want 3", got)
	}
}

// TestTestExplorerPanelFilterAndClear verifies the panel's thin wrappers
// reach the model and reset the view state.
func TestTestExplorerPanelFilterAndClear(t *testing.T) {
	p := NewTestExplorerPanel()
	p.SetResults(testExplorerFixture())

	p.SetFilter("Gamma")
	if got := teRowNames(p.Rows()); !teEqNames(got, []string{tePkgCore, "TestGamma"}) {
		t.Fatalf("panel filtered rows = %v", got)
	}
	p.SetFilter("")
	p.SetFailedOnly(true)
	if got := len(p.Rows()); got != 3 {
		t.Errorf("panel failed-only rows = %d, want 3", got)
	}
	p.Clear()
	if got := len(p.Rows()); got != 0 {
		t.Errorf("rows after Clear() = %d, want 0", got)
	}
}

// TestTestExplorerLabelHelpers pins the pure formatting helpers used by
// the draw path.
func TestTestExplorerLabelHelpers(t *testing.T) {
	cases := []struct {
		st    TestNodeStatus
		glyph string
		name  string
	}{
		{TestNodeUnknown, "·", "unknown"},
		{TestNodeRunning, "◌", "running"},
		{TestNodePass, "✓", "pass"},
		{TestNodeFail, "✕", "fail"},
		{TestNodeSkip, "⊝", "skip"},
	}
	for _, c := range cases {
		if got := testNodeGlyph(c.st); got != c.glyph {
			t.Errorf("testNodeGlyph(%v) = %q, want %q", c.st, got, c.glyph)
		}
		if got := c.st.String(); got != c.name {
			t.Errorf("%d.String() = %q, want %q", int(c.st), got, c.name)
		}
	}

	if got := testElapsedLabel(0); got != "" {
		t.Errorf("testElapsedLabel(0) = %q, want empty", got)
	}
	if got := testElapsedLabel(0.05); got != "(0.05s)" {
		t.Errorf("testElapsedLabel(0.05) = %q, want (0.05s)", got)
	}
	if got := testElapsedLabel(1.5); got != "(1.50s)" {
		t.Errorf("testElapsedLabel(1.5) = %q, want (1.50s)", got)
	}

	if got, want := testExplorerIndent(0), testExplorerGutterX; got != want {
		t.Errorf("testExplorerIndent(0) = %v, want %v", got, want)
	}
	if got, want := testExplorerIndent(2), testExplorerGutterX+2*testExplorerIndentW; got != want {
		t.Errorf("testExplorerIndent(2) = %v, want %v", got, want)
	}
}

// TestTestExplorerFactoryRegistered verifies the panel is reachable
// through the object factory and listed as a tool view, which is what
// lets silkide restore it from a saved layout.
func TestTestExplorerFactoryRegistered(t *testing.T) {
	f := core.FindFactory("ged.TestExplorer")
	if f == nil {
		t.Fatal(`factory "ged.TestExplorer" not registered`)
	}
	obj := f.New()
	p, ok := obj.(*TestExplorerPanel)
	if !ok {
		t.Fatalf("factory produced %T, want *TestExplorerPanel", obj)
	}
	if p.Model() == nil {
		t.Error("factory-built panel has a nil model (Init did not run)")
	}

	def, ok := gui.GetToolViewDef("ged.TestExplorer")
	if !ok {
		t.Fatal(`tool view "ged.TestExplorer" not registered`)
	}
	if def.Name == "" {
		t.Error("tool view has an empty Name")
	}
}
