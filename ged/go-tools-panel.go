package ged

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

func init() {
	core.RegisterFactory("ged.GoToolsPanel", gui.TypeOf(GoToolsPanel{}))
	gui.RegisterToolView(gui.ToolViewDef{
		Id:   "ged.GoToolsPanel",
		Name: "Go 工具 / Go Tools",
		Icon: "run",
		Desc: "Go 分析工具 (vet / race / pprof / trace / govulncheck / staticcheck)",
	})
}

// GoToolsPanel is the analyzer pane: a picker listing every workflow
// core/gotools.go can build (`go vet`, `go test -race`, a profiling test
// run, `go tool pprof -top`, `go tool trace`, `govulncheck`,
// `staticcheck`) over a findings table grouped by the tool that produced
// each row.
//
// Like the sibling panes it is a pure view. It never builds a command and
// never starts a process: clicking an available picker row fires SigRun
// with the workflow id, and the host builds the ToolCommand
// (core.GoVetCommand and friends), runs it off the UI thread, parses the
// output (core.ParseToolOutput) and pushes the result back through
// SetFindings. Clicking a finding that has a source location fires
// SigActivate so the host can open file:line in the editor.
//
// Availability comes from the host too — SetTools(core.DetectGoTools()) —
// so the panel greys out what is not installed instead of letting the
// user start a run that would fail with "executable file not found".
type GoToolsPanel struct {
	gui.Widget

	tools     []GoToolRow
	findings  []core.Finding
	groups    []GoToolGroup
	collapsed map[string]bool

	scrollY   float64
	hoverIdx  int
	rowHeight float64

	cbRun      func(tool string)
	cbActivate func(file string, line int)
}

// NewGoToolsPanel creates the panel with the picker populated from the
// current PATH and no findings yet.
func NewGoToolsPanel() *GoToolsPanel {
	p := new(GoToolsPanel)
	p.Init(p)
	return p
}

func (this *GoToolsPanel) Init(self gui.IWidget) {
	this.Widget.Init(self)
	this.rowHeight = 20
	this.hoverIdx = -1
	this.collapsed = make(map[string]bool)
	// core.DetectGoTools is LookPath-only (no subprocess), so probing the
	// PATH during construction is cheap enough for the UI thread and gives
	// the picker a correct default before the host says anything.
	this.tools = buildGoToolRows(core.DetectGoTools())
}

// SetTools replaces the picker's availability snapshot, normally with
// core.DetectGoTools(). Tools absent from the snapshot become unavailable.
func (this *GoToolsPanel) SetTools(tools []core.Tool) {
	this.tools = buildGoToolRows(tools)
	this.hoverIdx = -1
	this.Self().Update()
}

// ToolRows returns a copy of the picker rows in display order.
func (this *GoToolsPanel) ToolRows() []GoToolRow {
	out := make([]GoToolRow, len(this.tools))
	copy(out, this.tools)
	return out
}

// SetFindings replaces the findings with a defensive copy and regroups
// them by tool. Collapse state survives — re-running one tool should not
// re-expand the groups the user folded away.
func (this *GoToolsPanel) SetFindings(findings []core.Finding) {
	this.findings = make([]core.Finding, len(findings))
	copy(this.findings, findings)
	this.groups = groupFindingsByTool(this.findings)
	this.scrollY = 0
	this.hoverIdx = -1
	this.Self().Update()
}

// Findings returns a copy of the findings in input order.
func (this *GoToolsPanel) Findings() []core.Finding {
	out := make([]core.Finding, len(this.findings))
	copy(out, this.findings)
	return out
}

// Groups returns the tool-grouped view of the findings, rebuilt from a
// copy so callers cannot mutate the panel's own grouping.
func (this *GoToolsPanel) Groups() []GoToolGroup {
	return groupFindingsByTool(this.Findings())
}

// Clear removes all findings, keeping the picker.
func (this *GoToolsPanel) Clear() {
	this.findings = nil
	this.groups = nil
	this.scrollY = 0
	this.hoverIdx = -1
	this.Self().Update()
}

// IsCollapsed reports whether a tool's group is folded.
func (this *GoToolsPanel) IsCollapsed(tool string) bool {
	return this.collapsed[tool]
}

// toggleCollapsed folds or unfolds one tool's group.
func (this *GoToolsPanel) toggleCollapsed(tool string) {
	if this.collapsed == nil {
		this.collapsed = make(map[string]bool)
	}
	this.collapsed[tool] = !this.collapsed[tool]
	this.Self().Update()
}

// SigRun registers the callback fired when an available picker row is
// clicked. It receives the core workflow id (core.ToolGoVet, ...); the
// host turns that into a ToolCommand and runs it.
func (this *GoToolsPanel) SigRun(fn func(tool string)) {
	this.cbRun = fn
}

// SigActivate registers the callback fired when a finding row with a
// source location is clicked. The host opens file at the 1-based line.
func (this *GoToolsPanel) SigActivate(fn func(file string, line int)) {
	this.cbActivate = fn
}

// --- Drawing ---

const goToolsHeaderH = 22.0

// rows is the flat row sequence both Draw and the hit-test walk.
func (this *GoToolsPanel) rows() []goToolsRow {
	return buildGoToolsRows(this.tools, this.groups, this.collapsed)
}

// Draw renders the tally header, then the picker rows (dimmed when their
// binary is missing), then one collapsible group header per tool followed
// by its findings: a severity-coloured locator and the message.
func (this *GoToolsPanel) Draw(g paint.Painter) {
	w, h := this.Size()

	g.SetBrush1(paint.Color{R: 25, G: 25, B: 30, A: 255})
	g.Rectangle(0, 0, w, h)
	g.Fill()

	font := paint.NewFont("Menlo", 12, false, false)
	g.SetFont(font)
	fe := font.FontExtents()

	g.SetBrush1(paint.Color{R: 35, G: 35, B: 42, A: 255})
	g.Rectangle(0, 0, w, goToolsHeaderH)
	g.Fill()
	g.SetBrush1(paint.Color{R: 200, G: 200, B: 210, A: 255})
	g.DrawText1(8, fe.Ascent+4, goToolsHeaderLabel(len(this.findings)))

	rows := this.rows()
	rh := this.rowHeight
	areaTop := goToolsHeaderH
	startIdx := int(this.scrollY / rh)
	if startIdx < 0 {
		startIdx = 0
	}
	visibleCount := int((h-areaTop)/rh) + 2

	for i := startIdx; i < startIdx+visibleCount && i < len(rows); i++ {
		y := areaTop + float64(i)*rh - this.scrollY
		r := rows[i]

		if i == this.hoverIdx {
			g.SetBrush1(paint.Color{R: 50, G: 50, B: 62, A: 255})
			g.Rectangle(0, y, w, rh)
			g.Fill()
		} else if r.Kind == goToolsRowGroup {
			g.SetBrush1(paint.Color{R: 38, G: 38, B: 46, A: 255})
			g.Rectangle(0, y, w, rh)
			g.Fill()
		}

		textY := y + fe.Ascent + 2
		switch r.Kind {
		case goToolsRowPicker:
			t := this.tools[r.ToolIdx]
			if t.Available {
				g.SetBrush1(paint.Color{R: 120, G: 200, B: 140, A: 255})
				g.DrawText1(8, textY, "▶")
				g.SetBrush1(paint.Color{R: 200, G: 200, B: 210, A: 255})
			} else {
				// Dimmed and glyph-less: the row is inert.
				g.SetBrush1(paint.Color{R: 110, G: 110, B: 120, A: 255})
			}
			g.DrawText1(24, textY, goToolPickerLabel(t))

		case goToolsRowGroup:
			grp := this.groups[r.GroupIdx]
			g.SetBrush1(paint.Color{R: 210, G: 200, B: 150, A: 255})
			g.DrawText1(8, textY, goToolGroupLabel(grp, this.collapsed[grp.Tool]))

		case goToolsRowFinding:
			f := this.groups[r.GroupIdx].Findings[r.Index]
			x := 24.0
			if loc := goToolFindingLabel(f); loc != "" {
				g.SetBrush1(goToolSeverityColor(f.Severity))
				g.DrawText1(x, textY, loc)
				x += font.TextExtents(loc).Width + 12
			}
			g.SetBrush1(paint.Color{R: 200, G: 200, B: 210, A: 255})
			g.DrawText1(x, textY, goToolFindingText(f))
		}
	}
}

// goToolSeverityColor maps a core.Finding severity to the locator colour:
// red for errors, amber for warnings, muted blue for info (profile rows).
func goToolSeverityColor(severity string) paint.Color {
	switch severity {
	case core.FindingError:
		return paint.Color{R: 230, G: 110, B: 110, A: 255}
	case core.FindingWarning:
		return paint.Color{R: 220, G: 180, B: 90, A: 255}
	default:
		return paint.Color{R: 120, G: 160, B: 210, A: 255}
	}
}

// --- Events ---

// OnLeftDown routes a click by row kind: an available picker row starts
// its workflow, an unavailable one is inert; a group header folds; a
// finding with a source location jumps, one without (a function-level
// pprof row) is inert.
func (this *GoToolsPanel) OnLeftDown(x, y float64) {
	this.SetFocus()
	rows := this.rows()
	idx := this.rowAt(y)
	if idx < 0 || idx >= len(rows) {
		return
	}
	r := rows[idx]
	switch r.Kind {
	case goToolsRowPicker:
		t := this.tools[r.ToolIdx]
		if !t.Available {
			return
		}
		if this.cbRun != nil {
			this.cbRun(t.Id)
		}
	case goToolsRowGroup:
		this.toggleCollapsed(this.groups[r.GroupIdx].Tool)
	case goToolsRowFinding:
		f := this.groups[r.GroupIdx].Findings[r.Index]
		if f.File == "" {
			return
		}
		if this.cbActivate != nil {
			this.cbActivate(f.File, f.Line)
		}
	}
}

// OnMouseMove tracks hover state for the row highlight.
func (this *GoToolsPanel) OnMouseMove(x, y float64) {
	idx := this.rowAt(y)
	if idx != this.hoverIdx {
		this.hoverIdx = idx
		this.Self().Update()
	}
}

// OnMouseLeave clears the hover highlight.
func (this *GoToolsPanel) OnMouseLeave() {
	if this.hoverIdx != -1 {
		this.hoverIdx = -1
		this.Self().Update()
	}
}

// OnMouseWheel scrolls the row list vertically, clamped to the content.
func (this *GoToolsPanel) OnMouseWheel(x, y, z float64) {
	this.scrollY -= z * 3 * this.rowHeight
	if this.scrollY < 0 {
		this.scrollY = 0
	}
	_, h := this.Size()
	maxScroll := float64(len(this.rows()))*this.rowHeight - (h - goToolsHeaderH)
	if maxScroll < 0 {
		maxScroll = 0
	}
	if this.scrollY > maxScroll {
		this.scrollY = maxScroll
	}
	this.Self().Update()
}

// rowAt maps a y coordinate to a flat-row index, folding in the scroll
// offset; -1 for the header band or past the last row.
func (this *GoToolsPanel) rowAt(y float64) int {
	return goToolsRowAtY(y+this.scrollY, goToolsHeaderH, this.rowHeight, len(this.rows()))
}

func (this *GoToolsPanel) SizeHints() gui.SizeHints {
	return gui.SizeHints{MinWidth: 260, MinHeight: 120}
}
