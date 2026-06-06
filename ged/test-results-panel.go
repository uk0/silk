package ged

import (
	"silk/core"
	"silk/gui"
	"silk/paint"
	"strconv"
	"strings"
)

func init() {
	core.RegisterFactory("ged.TestResults", gui.TypeOf(TestResultsPanel{}))
	gui.RegisterToolView(gui.ToolViewDef{
		Id:   "ged.TestResults",
		Name: "测试",
		Icon: "run",
		Desc: "Go 测试结果（结构化视图）",
	})
}

// TestStatus is the outcome bucket of a single `go test -v` line.
// BuildOutput knows nothing about test outcomes — it just flags lines
// as error/not-error; ProblemsPanel only models error vs warning. A
// test row is a three-state thing (passed / failed / skipped), so it
// gets its own little enum.
type TestStatus int

const (
	TestPassed TestStatus = iota
	TestFailed
	TestSkipped
)

// TestResult is one parsed row from a `go test -v` capture: a test or
// subtest name, the package it ran under (when the summary line was
// seen first), the outcome bucket, the raw duration string the runner
// printed, and — for FAIL rows only — the captured failure output and
// the first `file:line:` locator extracted from it for jump-to.
//
// Where Problem keeps a Message field per row, TestResult keeps the
// whole Output blob: a failing test typically spans many lines (the
// helper trace, the want/got diff, the stack), and the panel needs all
// of it to render a useful preview.
type TestResult struct {
	Name     string     // e.g. "TestParseFoo" or "TestFoo/subtest"
	Package  string     // e.g. "silk/ged" (may be empty for tabular subtests)
	Status   TestStatus // pass / fail / skip
	Duration string     // raw, as printed by the runner, e.g. "0.02s"
	File     string     // file path from the FAIL message, "" if not present
	Line     int        // 1-based line from the FAIL message, 0 if not present
	Output   string     // captured failure output between --- FAIL and the next --- or summary
}

// parseTestOutput parses the standard `go test -v` format into a list
// of TestResult rows in the order the runner emitted them. Recognised
// shapes:
//
//	=== RUN   TestName
//	    --- PASS: TestName (0.02s)
//	    --- FAIL: TestName (0.05s)
//	    --- SKIP: TestName (0.00s)
//	    --- PASS: TestX/sub (0.00s)
//	ok    silk/ged    0.5s
//	FAIL  silk/ged    0.5s
//
// For FAIL rows, every captured line after the `--- FAIL:` header and
// before the next `--- ` (PASS/FAIL/SKIP) or `ok |FAIL  pkg` summary
// becomes Output, and the first line matching `<file>:<line>:` is
// extracted into File / Line so a click can jump there. The package
// for the row defaults to the most recent `ok |FAIL  pkg` line; this
// matches how `go test -v` interleaves runs and summaries.
//
// Free function — no widget, no GL — so it can be unit-tested directly.
func parseTestOutput(output string) []TestResult {
	var results []TestResult
	// idxByName lets the FAIL-output collector find the row it should
	// append captured lines to without rescanning the slice each line.
	idxByName := make(map[string]int)
	curFailIdx := -1 // index of the result whose Output we are filling

	// pkg is the current package, set by `ok|FAIL  pkg  time` lines.
	// It is attached to subsequent rows; this matches how go test -v
	// prints the summary after the per-test lines for that package.
	pkg := ""

	for _, raw := range strings.Split(output, "\n") {
		raw = strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(raw)

		// A `--- PASS|FAIL|SKIP:` header ends the previous failure
		// capture and starts the next row.
		if status, name, dur, ok := parseTestStatusLine(trimmed); ok {
			r := TestResult{
				Name:     name,
				Status:   status,
				Duration: dur,
				Package:  pkg,
			}
			results = append(results, r)
			idxByName[name] = len(results) - 1
			if status == TestFailed {
				curFailIdx = len(results) - 1
			} else {
				curFailIdx = -1
			}
			continue
		}

		// `ok|FAIL  pkg  time` summary lines: capture the package
		// label for following rows and stop the failure-output collector
		// since the package's per-test stream has ended.
		if p, ok := parsePackageSummaryLine(trimmed); ok {
			pkg = p
			// Backfill the package on rows that ran in this package
			// but didn't have it yet (the summary always comes after
			// the test lines in `go test -v` output).
			for i := range results {
				if results[i].Package == "" {
					results[i].Package = p
				}
			}
			curFailIdx = -1
			continue
		}

		// `=== RUN   TestName` lines are informational only — the
		// status line is what creates the row. Skip without ending
		// any in-flight capture.
		if strings.HasPrefix(trimmed, "=== RUN") ||
			strings.HasPrefix(trimmed, "=== PAUSE") ||
			strings.HasPrefix(trimmed, "=== CONT") {
			continue
		}

		// Anything else while we are inside a FAIL block belongs to
		// that test's Output. We keep the raw line (with original
		// indentation) so the panel can show the runner's formatting.
		if curFailIdx >= 0 {
			r := &results[curFailIdx]
			if r.Output == "" {
				r.Output = raw
			} else {
				r.Output += "\n" + raw
			}
			// First `<file>:<line>:` locator wins for File/Line.
			if r.File == "" {
				if file, line, ok := parseTestFailLocator(trimmed); ok {
					r.File = file
					r.Line = line
				}
			}
		}
	}

	return results
}

// parseTestStatusLine tries to read a `--- PASS|FAIL|SKIP: name (dur)`
// header. The header is indented by the runner; the caller hands us a
// pre-trimmed line. Returns the status, the test (or subtest) name,
// and the raw duration string. ok is false when the line is not a
// status header.
func parseTestStatusLine(trimmed string) (TestStatus, string, string, bool) {
	const passPrefix = "--- PASS: "
	const failPrefix = "--- FAIL: "
	const skipPrefix = "--- SKIP: "
	var status TestStatus
	var rest string
	switch {
	case strings.HasPrefix(trimmed, passPrefix):
		status = TestPassed
		rest = trimmed[len(passPrefix):]
	case strings.HasPrefix(trimmed, failPrefix):
		status = TestFailed
		rest = trimmed[len(failPrefix):]
	case strings.HasPrefix(trimmed, skipPrefix):
		status = TestSkipped
		rest = trimmed[len(skipPrefix):]
	default:
		return 0, "", "", false
	}
	// rest is now "TestName (0.02s)" — split on the last "(" to be safe
	// against names that contain spaces (subtests can).
	open := strings.LastIndex(rest, "(")
	close := strings.LastIndex(rest, ")")
	if open < 0 || close < 0 || close < open {
		// No duration parens; treat the whole thing as the name.
		return status, strings.TrimSpace(rest), "", true
	}
	name := strings.TrimSpace(rest[:open])
	dur := rest[open+1 : close]
	return status, name, dur, true
}

// parsePackageSummaryLine reads an `ok  pkg  time` or `FAIL  pkg  time`
// summary line and returns the package label. ok is false when the
// line is not a summary. The runner separates the columns with at
// least one tab or run of spaces; Fields() handles both.
func parsePackageSummaryLine(trimmed string) (string, bool) {
	fields := strings.Fields(trimmed)
	if len(fields) < 2 {
		return "", false
	}
	switch fields[0] {
	case "ok", "FAIL", "?":
		// `?` lines (no test files) carry a package but no duration;
		// `ok|FAIL` carry both. In both cases the second field is the
		// package, which is all we need.
		return fields[1], true
	}
	return "", false
}

// parseTestFailLocator extracts the first `<file>:<line>:` locator
// from a failure-output line. Test helpers and t.Errorf emit lines like
// "    parse_test.go:42: ...", possibly prefixed with whitespace; the
// caller hands us a pre-trimmed line. ok is false when no locator was
// found.
func parseTestFailLocator(trimmed string) (string, int, bool) {
	parts := strings.SplitN(trimmed, ":", 3)
	if len(parts) < 3 {
		return "", 0, false
	}
	file := strings.TrimSpace(parts[0])
	line, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || file == "" || line <= 0 {
		return "", 0, false
	}
	return file, line, true
}

// TestResultsPanel is a structured view of `go test -v` output. Unlike
// BuildOutput — a free-text log that highlights lines — and unlike
// ProblemsPanel — a flat list of compiler diagnostics — this pane is
// one row per test with a status glyph, the test name, the raw
// duration, and, for FAILs, the file:line preview. Clicking a FAIL row
// whose locator was recoverable fires SigResultActivated for the host
// IDE to jump to the failing line.
type TestResultsPanel struct {
	gui.Widget
	results          []TestResult
	scrollY          float64
	hoverIdx         int
	rowHeight        float64
	cbActivate       func(r TestResult)
	cbRunTestRequest func(name string)
	// clipboardFn is an indirection over gui.Clipboard so the right-click
	// "复制名称 / 复制输出" entries can be unit-tested headlessly. When nil,
	// the panel falls back to gui.Clipboard.SetData.
	clipboardFn func(s string)
}

// NewTestResultsPanel creates an empty test-results panel.
func NewTestResultsPanel() *TestResultsPanel {
	p := new(TestResultsPanel)
	p.Init(p)
	return p
}

func (this *TestResultsPanel) Init(self gui.IWidget) {
	this.Widget.Init(self)
	this.rowHeight = 20
	this.hoverIdx = -1
}

// SetOutput parses raw `go test -v` output and replaces the result
// list. Resets the scroll and hover state so the new run starts at
// the top.
func (this *TestResultsPanel) SetOutput(output string) {
	this.results = parseTestOutput(output)
	this.scrollY = 0
	this.hoverIdx = -1
	this.Self().Update()
}

// Results returns the parsed test rows in their emitted order.
func (this *TestResultsPanel) Results() []TestResult {
	return this.results
}

// Clear empties the result list.
func (this *TestResultsPanel) Clear() {
	this.results = nil
	this.scrollY = 0
	this.hoverIdx = -1
	this.Self().Update()
}

// SigResultActivated registers the callback fired when the user clicks
// a FAIL row that has a recoverable File. PASS and SKIP rows are inert
// — there is nothing to jump to.
func (this *TestResultsPanel) SigResultActivated(fn func(r TestResult)) {
	this.cbActivate = fn
}

// Counts returns the tally of passed, failed, and skipped rows. Used
// by the panel header and by callers that want a quick green/red badge
// (e.g. the dock tab can colour itself based on Counts).
func (this *TestResultsPanel) Counts() (passed, failed, skipped int) {
	for _, r := range this.results {
		switch r.Status {
		case TestPassed:
			passed++
		case TestFailed:
			failed++
		case TestSkipped:
			skipped++
		}
	}
	return
}

// --- Drawing ---

const testResultsHeaderH = 22.0

// Draw renders the header tally then one row per test result.
func (this *TestResultsPanel) Draw(g paint.Painter) {
	w, h := this.Size()

	// Dark background, matching the sibling panes.
	g.SetBrush1(paint.Color{R: 25, G: 25, B: 30, A: 255})
	g.Rectangle(0, 0, w, h)
	g.Fill()

	font := paint.NewFont("Menlo", 12, false, false)
	g.SetFont(font)
	fe := font.FontExtents()

	// Header band: "✓ N  ✕ M  ⊝ K".
	g.SetBrush1(paint.Color{R: 35, G: 35, B: 42, A: 255})
	g.Rectangle(0, 0, w, testResultsHeaderH)
	g.Fill()
	passed, failed, skipped := this.Counts()
	g.SetBrush1(paint.Color{R: 200, G: 200, B: 210, A: 255})
	header := "✓ " + strconv.Itoa(passed) +
		"  ✕ " + strconv.Itoa(failed) +
		"  ⊝ " + strconv.Itoa(skipped)
	g.DrawText1(8, fe.Ascent+4, header)

	if len(this.results) == 0 {
		return
	}

	rh := this.rowHeight
	areaTop := testResultsHeaderH
	startIdx := int(this.scrollY / rh)
	if startIdx < 0 {
		startIdx = 0
	}
	visibleCount := int((h-areaTop)/rh) + 2

	for i := startIdx; i < startIdx+visibleCount && i < len(this.results); i++ {
		y := areaTop + float64(i)*rh - this.scrollY
		r := this.results[i]

		// Hover wins over the alternating stripe.
		if i == this.hoverIdx {
			g.SetBrush1(paint.Color{R: 50, G: 50, B: 62, A: 255})
			g.Rectangle(0, y, w, rh)
			g.Fill()
		} else if i%2 == 1 {
			g.SetBrush1(paint.Color{R: 32, G: 32, B: 38, A: 255})
			g.Rectangle(0, y, w, rh)
			g.Fill()
		}

		this.drawStatusGlyph(g, r.Status, y, rh)

		// Test name in light grey.
		g.SetBrush1(paint.Color{R: 200, G: 200, B: 210, A: 255})
		g.DrawText1(24, y+fe.Ascent+2, r.Name)
		nameExt := font.TextExtents(r.Name)

		// Duration in muted blue-grey, right after the name.
		if r.Duration != "" {
			g.SetBrush1(paint.Color{R: 130, G: 145, B: 165, A: 255})
			g.DrawText1(24+nameExt.Width+12, y+fe.Ascent+2, "("+r.Duration+")")
		}

		// For FAILs, show the file:line locator after the duration.
		if r.Status == TestFailed && r.File != "" {
			g.SetBrush1(paint.Color{R: 120, G: 160, B: 210, A: 255})
			loc := r.File + ":" + strconv.Itoa(r.Line)
			// Approximate offset: name + " (dur) " + a gap. Cheap to
			// compute since extents on a short string are O(string).
			dur := ""
			if r.Duration != "" {
				dur = " (" + r.Duration + ")"
			}
			off := font.TextExtents(r.Name + dur)
			g.DrawText1(24+off.Width+18, y+fe.Ascent+2, loc)
		}
	}
}

// drawStatusGlyph paints a small outcome marker at the row's left
// gutter: a green check for passes, a red cross for fails, an amber
// dash for skips.
func (this *TestResultsPanel) drawStatusGlyph(g paint.Painter, st TestStatus, y, rh float64) {
	cx := 12.0
	cy := y + rh/2
	d := 4.0
	switch st {
	case TestPassed:
		// Green check: short stroke up-and-right.
		g.MoveTo(cx-d, cy)
		g.LineTo(cx-1, cy+d-1)
		g.LineTo(cx+d, cy-d)
		g.SetPen1(paint.Color{R: 110, G: 200, B: 110, A: 255}, 2)
		g.Stroke()
	case TestFailed:
		// Red cross.
		g.MoveTo(cx-d, cy-d)
		g.LineTo(cx+d, cy+d)
		g.MoveTo(cx+d, cy-d)
		g.LineTo(cx-d, cy+d)
		g.SetPen1(paint.Color{R: 230, G: 80, B: 80, A: 255}, 2)
		g.Stroke()
	case TestSkipped:
		// Amber dash.
		g.MoveTo(cx-d, cy)
		g.LineTo(cx+d, cy)
		g.SetPen1(paint.Color{R: 230, G: 180, B: 60, A: 255}, 2)
		g.Stroke()
	}
}

// --- Events ---

// OnLeftDown activates a FAIL row with a recoverable File via the
// SigResultActivated callback. Passes and skips are inert clicks.
func (this *TestResultsPanel) OnLeftDown(x, y float64) {
	this.SetFocus()
	idx := this.rowAt(y)
	if idx < 0 || idx >= len(this.results) {
		return
	}
	r := this.results[idx]
	if r.Status == TestFailed && r.File != "" && this.cbActivate != nil {
		this.cbActivate(r)
	}
}

// OnMouseMove tracks hover state for the row highlight.
func (this *TestResultsPanel) OnMouseMove(x, y float64) {
	idx := this.rowAt(y)
	if idx < 0 || idx >= len(this.results) {
		idx = -1
	}
	if idx != this.hoverIdx {
		this.hoverIdx = idx
		this.Self().Update()
	}
}

// OnMouseLeave clears the hover highlight.
func (this *TestResultsPanel) OnMouseLeave() {
	if this.hoverIdx != -1 {
		this.hoverIdx = -1
		this.Self().Update()
	}
}

// OnMouseWheel scrolls the row list vertically.
func (this *TestResultsPanel) OnMouseWheel(x, y, z float64) {
	this.scrollY -= z * 3 * this.rowHeight
	if this.scrollY < 0 {
		this.scrollY = 0
	}
	_, h := this.Size()
	maxScroll := float64(len(this.results))*this.rowHeight - (h - testResultsHeaderH)
	if maxScroll < 0 {
		maxScroll = 0
	}
	if this.scrollY > maxScroll {
		this.scrollY = maxScroll
	}
	this.Self().Update()
}

// rowAt maps a y coordinate (below the header) to a result index, or
// -1 when y lands on the header band.
func (this *TestResultsPanel) rowAt(y float64) int {
	if y < testResultsHeaderH {
		return -1
	}
	return int((y - testResultsHeaderH + this.scrollY) / this.rowHeight)
}

func (this *TestResultsPanel) SizeHints() gui.SizeHints {
	return gui.SizeHints{MinWidth: 200, MinHeight: 80}
}

// SigRunTestRequested registers the callback fired when the user picks
// "运行此测试" from a row's context menu. The host (silkide) is expected
// to translate the test name into `go test -run ^<name>$ ./...` and
// stream the new output back through SetOutput.
func (this *TestResultsPanel) SigRunTestRequested(fn func(name string)) {
	this.cbRunTestRequest = fn
}

// testResultsMenuItem is one row of the context menu produced by
// buildContextMenu. Splitting the menu out of OnRightDown keeps the
// per-entry wiring (label, enablement, action) directly testable:
// tests can call buildContextMenu and invoke Action without standing
// up a Popup, a Window, or a GLFW context.
type testResultsMenuItem struct {
	Label     string
	Enabled   bool
	Separator bool   // true when this entry is just a visual separator
	Action    func() // nil when Separator or Enabled is false
}

// buildContextMenu produces the right-click menu entries for the row
// at index `row`. Out-of-range rows yield nil (no menu). The three
// canonical entries are always present: "运行此测试", "复制名称",
// "复制输出"; the Output entry is disabled when the row carries no
// captured output (PASS / SKIP, or a FAIL with empty Output). A fourth
// "跳转" entry appears for FAIL rows that have a recoverable File.
func (this *TestResultsPanel) buildContextMenu(row int) []testResultsMenuItem {
	if row < 0 || row >= len(this.results) {
		return nil
	}
	r := this.results[row]

	items := []testResultsMenuItem{
		{
			Label:   "运行此测试",
			Enabled: r.Name != "",
			Action: func() {
				if this.cbRunTestRequest != nil {
					this.cbRunTestRequest(r.Name)
				}
			},
		},
		{
			Label:   "复制名称",
			Enabled: r.Name != "",
			Action:  func() { this.clipboardWrite(r.Name) },
		},
		{
			Label:   "复制输出",
			Enabled: r.Output != "",
			Action:  func() { this.clipboardWrite(r.Output) },
		},
	}

	// Jump-to-file:line mirrors the OnLeftDown behaviour, reused via the
	// SigResultActivated callback. Only meaningful for FAIL rows that
	// captured a locator.
	if r.Status == TestFailed && r.File != "" {
		items = append(items,
			testResultsMenuItem{Separator: true},
			testResultsMenuItem{
				Label:   "跳转",
				Enabled: this.cbActivate != nil,
				Action:  func() { this.cbActivate(r) },
			},
		)
	}
	return items
}

// clipboardWrite copies s to the framework clipboard. The default path
// is gui.Clipboard.SetData; tests can swap this.clipboardFn for an
// in-memory recorder to assert what would have been copied without a
// live GLFW window. A clipboard error is downgraded to a core.Warn so a
// transient copy failure never crashes the IDE.
func (this *TestResultsPanel) clipboardWrite(s string) {
	if this.clipboardFn != nil {
		this.clipboardFn(s)
		return
	}
	if _, err := gui.Clipboard.SetData(s); err != nil {
		core.Warn("TestResultsPanel: clipboard write failed: ", err)
	}
}

// OnRightDown opens the row's context menu. A click outside any row
// (header band or empty space) is inert — there is nothing to act on.
func (this *TestResultsPanel) OnRightDown(x, y float64) {
	this.SetFocus()
	idx := this.rowAt(y)
	if idx < 0 || idx >= len(this.results) {
		return
	}
	items := this.buildContextMenu(idx)
	if len(items) == 0 {
		return
	}
	gui.ShowContextMenu(this.Self(), x, y, func(m *gui.Menu) {
		for _, it := range items {
			if it.Separator {
				m.AddSeparator()
				continue
			}
			btn := m.AddButton1(it.Label, nil)
			if !it.Enabled {
				btn.SetEnabled(false)
				continue
			}
			action := it.Action
			btn.Action().BindFunc0(func() { action() })
		}
	})
}
