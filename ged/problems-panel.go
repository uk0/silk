package ged

import (
	"github.com/uk0/silk/buildissues"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
	"sort"
	"strconv"
)

func init() {
	core.RegisterFactory("ged.ProblemsPanel", gui.TypeOf(ProblemsPanel{}))
	gui.RegisterToolView(gui.ToolViewDef{
		Id:   "ged.ProblemsPanel",
		Name: "问题",
		Icon: "edit",
		Desc: "编译问题列表（可排序）",
	})
}

// Severity classifies a Problem as an error or a warning. BuildOutput
// only ever flags a line as error/not-error; the Problems pane needs
// the finer distinction so it can render the right glyph and tally the
// header counts separately.
type Severity int

const (
	SeverityError Severity = iota
	SeverityWarning
)

// Problem is one structured compiler diagnostic. Where BuildOutput
// keeps the raw text line around, a Problem is the parsed result only:
// a file, a 1-based line, an optional column (0 when the compiler did
// not give one), a severity and the human-readable message with the
// "file:line:col:" prefix stripped off.
type Problem struct {
	File     string
	Line     int
	Col      int
	Severity Severity
	Message  string
}

// parseProblems turns raw compiler output into structured Problem rows
// by delegating to the shared buildissues engine and mapping each
// buildissues.Issue onto the panel's Problem type. Routing through the
// one tested parser means the Problems pane and the Build Output pane
// agree on every corner case — "file:line:col:" and "file:line:" shapes,
// "# pkg" context headers, warning classification, and Windows
// drive-letter paths (C:\...:line:col). It is kept as a free function so
// it needs no GL context or live widget.
func parseProblems(output string) []Problem {
	var problems []Problem
	for _, is := range buildissues.Parse(output) {
		problems = append(problems, Problem{
			File:     is.File,
			Line:     is.Line,
			Col:      is.Col,
			Severity: mapSeverity(is.Severity),
			Message:  is.Message,
		})
	}
	return problems
}

// mapSeverity maps a buildissues.Severity onto the panel's two-value
// Severity. buildissues.Parse only ever yields Error or Warning, so any
// non-warning severity collapses to SeverityError.
func mapSeverity(s buildissues.Severity) Severity {
	if s == buildissues.Warning {
		return SeverityWarning
	}
	return SeverityError
}

// ProblemsPanel is a sortable, structured list of compiler diagnostics,
// modelled on Qt Creator's "Issues" pane. It differs from BuildOutput:
// BuildOutput is a free-text log that highlights error lines in place,
// whereas this pane shows one parsed row per problem with a severity
// glyph, a file:line locator and the message, plus a header tally and
// file-grouped sorting. Clicking a row jumps to (file, line, col).
type ProblemsPanel struct {
	gui.Widget
	problems   []Problem
	scrollY    float64
	hoverIdx   int
	rowHeight  float64
	cbActivate func(file string, line, col int)
}

// NewProblemsPanel creates an empty problems panel.
func NewProblemsPanel() *ProblemsPanel {
	p := new(ProblemsPanel)
	p.Init(p)
	return p
}

func (this *ProblemsPanel) Init(self gui.IWidget) {
	this.Widget.Init(self)
	this.rowHeight = 20
	this.hoverIdx = -1
}

// SetOutput parses raw compiler output and replaces the problem list.
func (this *ProblemsPanel) SetOutput(output string) {
	this.SetProblems(parseProblems(output))
}

// Problems returns the current problem rows in display order.
func (this *ProblemsPanel) Problems() []Problem {
	return this.problems
}

// SetProblems replaces the problem list directly and resets the view.
func (this *ProblemsPanel) SetProblems(problems []Problem) {
	this.problems = problems
	this.scrollY = 0
	this.hoverIdx = -1
	this.Self().Update()
}

// Clear removes all problems.
func (this *ProblemsPanel) Clear() {
	this.SetProblems(nil)
}

// SigProblemActivated registers the callback fired when the user clicks
// a problem row. It receives the target file, 1-based line, and column.
func (this *ProblemsPanel) SigProblemActivated(fn func(file string, line, col int)) {
	this.cbActivate = fn
}

// ErrorCount returns how many problems are errors.
func (this *ProblemsPanel) ErrorCount() int {
	n := 0
	for _, p := range this.problems {
		if p.Severity == SeverityError {
			n++
		}
	}
	return n
}

// WarningCount returns how many problems are warnings.
func (this *ProblemsPanel) WarningCount() int {
	n := 0
	for _, p := range this.problems {
		if p.Severity == SeverityWarning {
			n++
		}
	}
	return n
}

// SortByFile stably reorders the problems by file name, then by line
// within a file. Stability keeps same-(file,line) diagnostics in their
// original arrival order, which matches how the compiler emitted them.
func (this *ProblemsPanel) SortByFile() {
	sort.SliceStable(this.problems, func(i, j int) bool {
		a, b := this.problems[i], this.problems[j]
		if a.File != b.File {
			return a.File < b.File
		}
		return a.Line < b.Line
	})
	this.Self().Update()
}

// --- Drawing ---

const problemsHeaderH = 22.0

// Draw renders the header tally followed by one row per problem.
func (this *ProblemsPanel) Draw(g paint.Painter) {
	w, h := this.Size()

	// Dark background, matching BuildOutput.
	g.SetBrush1(paint.Color{R: 25, G: 25, B: 30, A: 255})
	g.Rectangle(0, 0, w, h)
	g.Fill()

	font := paint.NewFont("Menlo", 12, false, false)
	g.SetFont(font)
	fe := font.FontExtents()

	// Header: "N errors, M warnings".
	g.SetBrush1(paint.Color{R: 35, G: 35, B: 42, A: 255})
	g.Rectangle(0, 0, w, problemsHeaderH)
	g.Fill()
	g.SetBrush1(paint.Color{R: 200, G: 200, B: 210, A: 255})
	header := strconv.Itoa(this.ErrorCount()) + " errors, " +
		strconv.Itoa(this.WarningCount()) + " warnings"
	g.DrawText1(8, fe.Ascent+4, header)

	if len(this.problems) == 0 {
		return
	}

	rh := this.rowHeight
	areaTop := problemsHeaderH
	startIdx := int(this.scrollY / rh)
	if startIdx < 0 {
		startIdx = 0
	}
	visibleCount := int((h-areaTop)/rh) + 2

	for i := startIdx; i < startIdx+visibleCount && i < len(this.problems); i++ {
		y := areaTop + float64(i)*rh - this.scrollY
		p := this.problems[i]

		// Alternating row tint; hover wins over the stripe.
		if i == this.hoverIdx {
			g.SetBrush1(paint.Color{R: 50, G: 50, B: 62, A: 255})
			g.Rectangle(0, y, w, rh)
			g.Fill()
		} else if i%2 == 1 {
			g.SetBrush1(paint.Color{R: 32, G: 32, B: 38, A: 255})
			g.Rectangle(0, y, w, rh)
			g.Fill()
		}

		this.drawSeverityGlyph(g, p.Severity, y, rh)

		// Locator "file:line" in muted blue-grey.
		g.SetBrush1(paint.Color{R: 120, G: 160, B: 210, A: 255})
		loc := p.File + ":" + strconv.Itoa(p.Line)
		g.DrawText1(24, y+fe.Ascent+2, loc)
		locExt := font.TextExtents(loc)

		// Message in light grey.
		g.SetBrush1(paint.Color{R: 200, G: 200, B: 210, A: 255})
		g.DrawText1(24+locExt.Width+12, y+fe.Ascent+2, p.Message)
	}
}

// drawSeverityGlyph paints a small severity marker at the row's left
// gutter: a red cross for errors, an amber exclamation stroke for
// warnings.
func (this *ProblemsPanel) drawSeverityGlyph(g paint.Painter, sev Severity, y, rh float64) {
	cx := 12.0
	cy := y + rh/2
	d := 4.0
	if sev == SeverityWarning {
		// Amber exclamation: a vertical stroke plus a dot below.
		g.MoveTo(cx, cy-d)
		g.LineTo(cx, cy+d-3)
		g.SetPen1(paint.Color{R: 230, G: 180, B: 60, A: 255}, 2)
		g.Stroke()
		g.MoveTo(cx, cy+d)
		g.LineTo(cx, cy+d)
		g.SetPen1(paint.Color{R: 230, G: 180, B: 60, A: 255}, 2)
		g.Stroke()
		return
	}
	// Red cross for errors.
	g.MoveTo(cx-d, cy-d)
	g.LineTo(cx+d, cy+d)
	g.MoveTo(cx+d, cy-d)
	g.LineTo(cx-d, cy+d)
	g.SetPen1(paint.Color{R: 230, G: 80, B: 80, A: 255}, 2)
	g.Stroke()
}

// --- Events ---

// OnLeftDown fires the activated callback for the clicked problem row.
func (this *ProblemsPanel) OnLeftDown(x, y float64) {
	this.SetFocus()
	idx := this.rowAt(y)
	if idx < 0 || idx >= len(this.problems) {
		return
	}
	if this.cbActivate != nil {
		p := this.problems[idx]
		this.cbActivate(p.File, p.Line, p.Col)
	}
}

// OnMouseMove tracks hover state for the row highlight.
func (this *ProblemsPanel) OnMouseMove(x, y float64) {
	idx := this.rowAt(y)
	if idx < 0 || idx >= len(this.problems) {
		idx = -1
	}
	if idx != this.hoverIdx {
		this.hoverIdx = idx
		this.Self().Update()
	}
}

// OnMouseLeave clears the hover highlight.
func (this *ProblemsPanel) OnMouseLeave() {
	if this.hoverIdx != -1 {
		this.hoverIdx = -1
		this.Self().Update()
	}
}

// OnMouseWheel scrolls the row list vertically.
func (this *ProblemsPanel) OnMouseWheel(x, y, z float64) {
	this.scrollY -= z * 3 * this.rowHeight
	if this.scrollY < 0 {
		this.scrollY = 0
	}
	_, h := this.Size()
	maxScroll := float64(len(this.problems))*this.rowHeight - (h - problemsHeaderH)
	if maxScroll < 0 {
		maxScroll = 0
	}
	if this.scrollY > maxScroll {
		this.scrollY = maxScroll
	}
	this.Self().Update()
}

// rowAt maps a y coordinate (below the header) to a problem index, or
// -1 when y lands on the header band.
func (this *ProblemsPanel) rowAt(y float64) int {
	if y < problemsHeaderH {
		return -1
	}
	return int((y - problemsHeaderH + this.scrollY) / this.rowHeight)
}

func (this *ProblemsPanel) SizeHints() gui.SizeHints {
	return gui.SizeHints{MinWidth: 200, MinHeight: 80}
}
