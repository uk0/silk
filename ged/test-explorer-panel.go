package ged

import (
	"strconv"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

func init() {
	core.RegisterFactory("ged.TestExplorer", gui.TypeOf(TestExplorerPanel{}))
	gui.RegisterToolView(gui.ToolViewDef{
		Id:   "ged.TestExplorer",
		Name: "测试浏览器",
		Icon: "test-explorer",
		Desc: "Go 测试树 (包 / 测试 / 子测试)",
	})
}

// TestExplorerPanel is the hierarchical counterpart to TestResultsPanel:
// where that pane is a flat log of one run, this one is a persistent
// package -> test -> subtest tree that survives across runs. Rows carry a
// status glyph and the elapsed time, expandable rows carry an expander
// box in the indent gutter, and the context menu drives the host through
// four callbacks (run / debug / rerun-failed / jump).
//
// All state lives in TestExplorerModel; the panel is a renderer plus a
// hit-test, so everything interesting about it is testable without GL.
type TestExplorerPanel struct {
	gui.Widget

	model     *TestExplorerModel
	scrollY   float64
	hoverIdx  int
	selected  *TestTreeNode // selection is held by pointer so it survives filtering
	rowHeight float64

	cbRun         func(pkg, test string)
	cbDebug       func(pkg, test string)
	cbRerunFailed func()
	cbActivate    func(file string, line int)
}

// NewTestExplorerPanel creates an empty test-explorer panel.
func NewTestExplorerPanel() *TestExplorerPanel {
	p := new(TestExplorerPanel)
	p.Init(p)
	return p
}

func (this *TestExplorerPanel) Init(self gui.IWidget) {
	this.Widget.Init(self)
	this.rowHeight = 20
	this.hoverIdx = -1
	this.model = NewTestExplorerModel()
}

// Model returns the live tree model. The host uses it for the pieces the
// panel does not wrap — Find, Counts, FailedTests (to build the re-run
// command), ExpandAll / CollapseAll.
func (this *TestExplorerPanel) Model() *TestExplorerModel {
	return this.model
}

// SetResults merges a run's results into the tree (see
// TestExplorerModel.SetResults: absent tests keep their previous status).
func (this *TestExplorerPanel) SetResults(pkgs []PkgNode) {
	this.model.SetResults(pkgs)
	this.Self().Update()
}

// Clear empties the tree and resets the view.
func (this *TestExplorerPanel) Clear() {
	this.model.Clear()
	this.scrollY = 0
	this.hoverIdx = -1
	this.selected = nil
	this.Self().Update()
}

// SetFilter applies the case-insensitive name filter and scrolls back to
// the top, since the row count just changed under the viewport.
func (this *TestExplorerPanel) SetFilter(text string) {
	this.model.SetFilter(text)
	this.scrollY = 0
	this.hoverIdx = -1
	this.Self().Update()
}

// SetFailedOnly toggles the failures-only view.
func (this *TestExplorerPanel) SetFailedOnly(b bool) {
	this.model.SetFailedOnly(b)
	this.scrollY = 0
	this.hoverIdx = -1
	this.Self().Update()
}

// Rows returns the currently visible rows — the same slice the draw path
// and the hit-test walk.
func (this *TestExplorerPanel) Rows() []TestExplorerRow {
	return this.model.Rows()
}

// SigRun registers the callback for "运行". It receives the row's package
// import path and its full test path; test is "" on a package row, which
// means "run the whole package".
func (this *TestExplorerPanel) SigRun(fn func(pkg, test string)) {
	this.cbRun = fn
}

// SigDebug registers the callback for "调试", same arguments as SigRun —
// the host is expected to start the debugger on that test.
func (this *TestExplorerPanel) SigDebug(fn func(pkg, test string)) {
	this.cbDebug = fn
}

// SigRerunFailed registers the callback for "重跑失败的测试". It takes no
// arguments: the host reads Model().FailedTests() to build the command.
func (this *TestExplorerPanel) SigRerunFailed(fn func()) {
	this.cbRerunFailed = fn
}

// SigActivate registers the jump-to-source callback, fired when a row
// with a recovered failure locator is clicked.
func (this *TestExplorerPanel) SigActivate(fn func(file string, line int)) {
	this.cbActivate = fn
}

// --- Pure helpers (GL-free, unit-testable) ---

const (
	testExplorerHeaderH   = 22.0
	testExplorerGutterX   = 6.0  // x of the depth-0 expander box
	testExplorerIndentW   = 14.0 // added per depth level
	testExplorerExpanderW = 12.0 // width of the expander hit box
	testExplorerGlyphW    = 14.0 // gap between the expander box and the label
)

// testExplorerIndent returns the x where a row at the given depth starts
// — that is, the left edge of its expander box.
func testExplorerIndent(depth int) float64 {
	return testExplorerGutterX + float64(depth)*testExplorerIndentW
}

// testExplorerExpanderHit reports whether x lands on the expander box of
// a row at the given depth. Childless rows have no expander, so they
// always answer false and the click falls through to row activation.
func testExplorerExpanderHit(x float64, depth int, hasChildren bool) bool {
	if !hasChildren {
		return false
	}
	left := testExplorerIndent(depth)
	return x >= left && x < left+testExplorerExpanderW
}

// testNodeGlyph is the one-rune status marker drawn in front of a row.
func testNodeGlyph(st TestNodeStatus) string {
	switch st {
	case TestNodeRunning:
		return "◌"
	case TestNodePass:
		return "✓"
	case TestNodeFail:
		return "✕"
	case TestNodeSkip:
		return "⊝"
	}
	return "·"
}

// testNodeColor is the row colour that goes with a status.
func testNodeColor(st TestNodeStatus) paint.Color {
	switch st {
	case TestNodeRunning:
		return paint.Color{R: 90, G: 150, B: 220, A: 255}
	case TestNodePass:
		return paint.Color{R: 70, G: 160, B: 90, A: 255}
	case TestNodeFail:
		return paint.Color{R: 210, G: 70, B: 70, A: 255}
	case TestNodeSkip:
		return paint.Color{R: 200, G: 150, B: 50, A: 255}
	}
	return paint.Color{R: 140, G: 145, B: 155, A: 255}
}

// testElapsedLabel formats a node's runtime the way the runner prints it,
// e.g. "(0.02s)". Unrun nodes have no time, so they get "".
func testElapsedLabel(sec float64) string {
	if sec <= 0 {
		return ""
	}
	return "(" + strconv.FormatFloat(sec, 'f', 2, 64) + "s)"
}

// --- Drawing ---

// Draw paints the tally header then one indented row per visible node:
// the expander box (only when the node has children), the status glyph,
// the label, the elapsed time and — for rows with a recovered locator —
// the file:line jump target.
func (this *TestExplorerPanel) Draw(g paint.Painter) {
	w, h := this.Size()
	t := gui.Theme()

	g.SetBrush1(t.ViewBGColor)
	g.Rectangle(0, 0, w, h)
	g.Fill()

	// Header band: the leaf tally plus the active view modifiers.
	g.SetBrush1(paint.Color{R: 235, G: 238, B: 245, A: 255})
	g.Rectangle(0, 0, w, testExplorerHeaderH)
	g.Fill()
	g.SetPen1(paint.Color{R: 200, G: 200, B: 210, A: 255}, 1)
	g.MoveTo(0, testExplorerHeaderH)
	g.LineTo(w, testExplorerHeaderH)
	g.Stroke()

	headerFont := paint.NewFont(t.Font.Family(), 12, true, false)
	g.SetFont(headerFont)
	g.SetBrush1(t.TextColor)
	pass, fail, skip := this.model.Counts()
	header := "✓ " + strconv.Itoa(pass) + "  ✕ " + strconv.Itoa(fail) + "  ⊝ " + strconv.Itoa(skip)
	if this.model.FailedOnly() {
		header += "   [仅失败]"
	}
	if this.model.FilterText() != "" {
		header += "   [" + this.model.FilterText() + "]"
	}
	g.DrawText1(8, testExplorerHeaderH-5, header)

	rows := this.Rows()
	if len(rows) == 0 {
		emptyFont := paint.NewFont(t.Font.Family(), 11, false, false)
		g.SetFont(emptyFont)
		g.SetBrush1(paint.Color{R: 150, G: 150, B: 160, A: 200})
		g.DrawText1(8, testExplorerHeaderH+20, "No tests")
		return
	}

	rowFont := paint.NewFont(t.Font.Family(), 11, false, false)
	boldFont := paint.NewFont(t.Font.Family(), 11, true, false)
	g.SetFont(rowFont)
	fe := rowFont.FontExtents()

	rh := this.rowHeight
	startY := testExplorerHeaderH - this.scrollY

	for i, r := range rows {
		rowY := startY + float64(i)*rh
		if rowY+rh < testExplorerHeaderH || rowY > h {
			continue
		}
		n := r.Node

		if i%2 == 1 {
			g.SetBrush1(paint.Color{R: 245, G: 247, B: 250, A: 255})
			g.Rectangle(0, rowY, w, rh)
			g.Fill()
		}
		if n == this.selected {
			g.SetBrush1(paint.Color{R: 215, G: 228, B: 245, A: 255})
			g.Rectangle(0, rowY, w, rh)
			g.Fill()
		} else if i == this.hoverIdx {
			g.SetBrush1(paint.Color{R: 230, G: 235, B: 245, A: 255})
			g.Rectangle(0, rowY, w, rh)
			g.Fill()
		}

		textY := rowY + fe.Ascent + (rh-fe.Ascent-fe.Descent)/2
		x := testExplorerIndent(r.Depth)

		// Expander, only for rows that have children.
		if len(n.Children) > 0 {
			glyph := "▶"
			if n.Expanded {
				glyph = "▼"
			}
			g.SetFont(rowFont)
			g.SetBrush1(paint.Color{R: 110, G: 120, B: 140, A: 255})
			g.DrawText1(x, textY, glyph)
		}
		x += testExplorerExpanderW

		// Status glyph in the outcome colour.
		g.SetFont(rowFont)
		g.SetBrush1(testNodeColor(n.Status))
		g.DrawText1(x, textY, testNodeGlyph(n.Status))
		x += testExplorerGlyphW

		// Label: packages in the accent colour and bold, tests in body text.
		label := n.Name
		if n.IsPackage() {
			g.SetFont(boldFont)
			g.SetBrush1(t.HighLightColor)
			g.DrawText1(x, textY, label)
			x += boldFont.TextExtents(label).Width
		} else {
			g.SetFont(rowFont)
			g.SetBrush1(t.TextColor)
			g.DrawText1(x, textY, label)
			x += rowFont.TextExtents(label).Width
		}

		g.SetFont(rowFont)
		if el := testElapsedLabel(n.Elapsed); el != "" {
			x += 8
			g.SetBrush1(paint.Color{R: 130, G: 145, B: 165, A: 255})
			g.DrawText1(x, textY, el)
			x += rowFont.TextExtents(el).Width
		}
		if n.File != "" {
			x += 10
			g.SetBrush1(paint.Color{R: 120, G: 160, B: 210, A: 255})
			g.DrawText1(x, textY, n.File+":"+strconv.Itoa(n.Line))
		}
	}
}

// --- Events ---

// rowAt maps a y coordinate to an index into Rows(), or -1 when y lands
// on the header band or past the last row.
func (this *TestExplorerPanel) rowAt(y float64) int {
	if y < testExplorerHeaderH || this.rowHeight <= 0 {
		return -1
	}
	idx := int((y - testExplorerHeaderH + this.scrollY) / this.rowHeight)
	if idx < 0 || idx >= len(this.Rows()) {
		return -1
	}
	return idx
}

// OnLeftDown either toggles the row's expander (when the click landed in
// the expander box of a row that has children) or selects the row and
// jumps to its locator through SigActivate.
func (this *TestExplorerPanel) OnLeftDown(x, y float64) {
	this.SetFocus()
	idx := this.rowAt(y)
	if idx < 0 {
		return
	}
	r := this.Rows()[idx]
	n := r.Node
	if testExplorerExpanderHit(x, r.Depth, len(n.Children) > 0) {
		n.Expanded = !n.Expanded
		this.Self().Update()
		return
	}
	this.selected = n
	this.Self().Update()
	if n.File != "" && this.cbActivate != nil {
		this.cbActivate(n.File, n.Line)
	}
}

// OnMouseMove tracks hover state for the row highlight.
func (this *TestExplorerPanel) OnMouseMove(x, y float64) {
	idx := this.rowAt(y)
	if idx != this.hoverIdx {
		this.hoverIdx = idx
		this.Self().Update()
	}
}

// OnMouseLeave clears the hover highlight.
func (this *TestExplorerPanel) OnMouseLeave() {
	if this.hoverIdx != -1 {
		this.hoverIdx = -1
		this.Self().Update()
	}
}

// OnMouseWheel scrolls the tree vertically, clamped to the row extent.
func (this *TestExplorerPanel) OnMouseWheel(x, y, z float64) {
	this.scrollY -= z * 3 * this.rowHeight
	if this.scrollY < 0 {
		this.scrollY = 0
	}
	_, h := this.Size()
	maxScroll := float64(len(this.Rows()))*this.rowHeight - (h - testExplorerHeaderH)
	if maxScroll < 0 {
		maxScroll = 0
	}
	if this.scrollY > maxScroll {
		this.scrollY = maxScroll
	}
	this.Self().Update()
}

// testExplorerMenuItem is one entry of the row context menu. Same split
// as TestResultsPanel's: keeping the entries as data means the wiring
// (label, enablement, action) is unit-testable without a Popup.
type testExplorerMenuItem struct {
	Label     string
	Enabled   bool
	Separator bool
	Action    func()
}

// buildContextMenu produces the right-click entries for a visible row.
// Out-of-range rows yield nil (no menu). "运行" / "调试" carry the row's
// package and full test path; on a package row the test path is "".
func (this *TestExplorerPanel) buildContextMenu(row int) []testExplorerMenuItem {
	rows := this.Rows()
	if row < 0 || row >= len(rows) {
		return nil
	}
	n := rows[row].Node

	items := []testExplorerMenuItem{
		{
			Label:   "运行",
			Enabled: this.cbRun != nil,
			Action:  func() { this.cbRun(n.Pkg, n.Test) },
		},
		{
			Label:   "调试",
			Enabled: this.cbDebug != nil,
			Action:  func() { this.cbDebug(n.Pkg, n.Test) },
		},
		{
			Label:   "重跑失败的测试",
			Enabled: this.cbRerunFailed != nil && len(this.model.FailedTests()) > 0,
			Action:  func() { this.cbRerunFailed() },
		},
		{Separator: true},
		{
			Label:   "跳转",
			Enabled: n.File != "" && this.cbActivate != nil,
			Action:  func() { this.cbActivate(n.File, n.Line) },
		},
		{Separator: true},
		{
			Label:   "展开全部",
			Enabled: true,
			Action:  func() { this.model.ExpandAll(); this.Self().Update() },
		},
		{
			Label:   "折叠全部",
			Enabled: true,
			Action:  func() { this.model.CollapseAll(); this.Self().Update() },
		},
		{
			Label:   "仅显示失败",
			Enabled: true,
			Action:  func() { this.SetFailedOnly(!this.model.FailedOnly()) },
		},
	}
	return items
}

// OnRightDown opens the row's context menu; a click outside any row is
// inert.
func (this *TestExplorerPanel) OnRightDown(x, y float64) {
	this.SetFocus()
	idx := this.rowAt(y)
	if idx < 0 {
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

func (this *TestExplorerPanel) SizeHints() gui.SizeHints {
	return gui.SizeHints{MinWidth: 220, MinHeight: 80}
}
