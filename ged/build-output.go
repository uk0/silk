package ged

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
	"strconv"
	"strings"
)

func init() {
	core.RegisterFactory("ged.BuildOutput", gui.TypeOf(BuildOutput{}))
	gui.RegisterToolView(gui.ToolViewDef{
		Id:   "ged.BuildOutput",
		Name: "输出",
		Icon: "edit",
		Desc: "编译输出及错误导航",
	})
}

// BuildOutputLine represents a single line in the build output.
type BuildOutputLine struct {
	Text    string
	IsError bool
	File    string
	Line    int
	Col     int
}

// BuildOutput is a panel that displays compiler output with clickable
// error navigation, similar to Qt Creator's build output pane.
type BuildOutput struct {
	gui.Widget
	lines        []BuildOutputLine
	scrollY      float64
	hoverIdx     int
	rowHeight    float64
	cbErrorClick func(file string, line, col int)
}

// NewBuildOutput creates a new build output panel.
func NewBuildOutput() *BuildOutput {
	p := new(BuildOutput)
	p.Init(p)
	return p
}

func (this *BuildOutput) Init(self gui.IWidget) {
	this.Widget.Init(self)
	this.rowHeight = 20
	this.hoverIdx = -1
}

// SetOutput parses compiler output text and populates the line list.
// It recognizes Go compiler error format: filename.go:LINE:COL: message
func (this *BuildOutput) SetOutput(text string) {
	this.lines = nil
	this.scrollY = 0
	this.hoverIdx = -1

	rawLines := strings.Split(text, "\n")
	for _, raw := range rawLines {
		raw = strings.TrimRight(raw, "\r")
		if raw == "" {
			continue
		}
		bol := BuildOutputLine{Text: raw}

		// Try to parse "file.go:LINE:COL: ..." error format.
		// Be careful: split on ":" but file path could be relative with no drive letter.
		parts := strings.SplitN(raw, ":", 4)
		if len(parts) >= 4 {
			file := strings.TrimSpace(parts[0])
			lineNum, errL := strconv.Atoi(strings.TrimSpace(parts[1]))
			colNum, errC := strconv.Atoi(strings.TrimSpace(parts[2]))
			if errL == nil && errC == nil && file != "" {
				bol.IsError = true
				bol.File = file
				bol.Line = lineNum
				bol.Col = colNum
			}
		}
		// Also catch "file.go:LINE: ..." (no column) format
		if !bol.IsError && len(parts) >= 3 {
			file := strings.TrimSpace(parts[0])
			lineNum, errL := strconv.Atoi(strings.TrimSpace(parts[1]))
			if errL == nil && file != "" {
				bol.IsError = true
				bol.File = file
				bol.Line = lineNum
				bol.Col = 1
			}
		}

		this.lines = append(this.lines, bol)
	}
	this.Self().Update()
}

// Clear removes all output lines.
func (this *BuildOutput) Clear() {
	this.lines = nil
	this.scrollY = 0
	this.hoverIdx = -1
	this.Self().Update()
}

// SigErrorClick registers a callback invoked when the user clicks an error line.
func (this *BuildOutput) SigErrorClick(fn func(file string, line, col int)) {
	this.cbErrorClick = fn
}

// HasErrors returns true if any parsed lines are errors.
func (this *BuildOutput) HasErrors() bool {
	for _, l := range this.lines {
		if l.IsError {
			return true
		}
	}
	return false
}

// ErrorMap returns a map of 0-based line numbers to error messages,
// suitable for passing to CodeEditor.SetErrors().
func (this *BuildOutput) ErrorMap() map[int]string {
	errs := make(map[int]string)
	for _, l := range this.lines {
		if l.IsError && l.Line > 0 {
			lineIdx := l.Line - 1 // convert to 0-based
			// Append if multiple errors on same line
			if existing, ok := errs[lineIdx]; ok {
				errs[lineIdx] = existing + "; " + l.Text
			} else {
				errs[lineIdx] = l.Text
			}
		}
	}
	return errs
}

// Draw renders the build output panel.
func (this *BuildOutput) Draw(g paint.Painter) {
	w, h := this.Size()

	// Dark background
	g.SetBrush1(paint.Color{R: 25, G: 25, B: 30, A: 255})
	g.Rectangle(0, 0, w, h)
	g.Fill()

	if len(this.lines) == 0 {
		return
	}

	font := paint.NewFont("Menlo", 12, false, false)
	g.SetFont(font)
	fe := font.FontExtents()

	rh := this.rowHeight
	startIdx := int(this.scrollY / rh)
	if startIdx < 0 {
		startIdx = 0
	}
	visibleCount := int(h/rh) + 2

	for i := startIdx; i < startIdx+visibleCount && i < len(this.lines); i++ {
		y := float64(i)*rh - this.scrollY
		line := this.lines[i]

		// Hover highlight
		if i == this.hoverIdx {
			g.SetBrush1(paint.Color{R: 45, G: 45, B: 55, A: 255})
			g.Rectangle(0, y, w, rh)
			g.Fill()
		}

		// Error lines in red, normal in white/gray
		if line.IsError {
			g.SetBrush1(paint.Color{R: 230, G: 80, B: 80, A: 255})
		} else {
			g.SetBrush1(paint.Color{R: 180, G: 180, B: 190, A: 255})
		}
		g.DrawText1(8, y+fe.Ascent+2, line.Text)
	}
}

// OnLeftDown handles click on an error line to navigate to file:line:col.
func (this *BuildOutput) OnLeftDown(x, y float64) {
	this.SetFocus()
	idx := int((y + this.scrollY) / this.rowHeight)
	if idx < 0 || idx >= len(this.lines) {
		return
	}
	line := this.lines[idx]
	if line.IsError && this.cbErrorClick != nil {
		this.cbErrorClick(line.File, line.Line, line.Col)
	}
}

// OnMouseMove tracks hover state for visual feedback.
func (this *BuildOutput) OnMouseMove(x, y float64) {
	idx := int((y + this.scrollY) / this.rowHeight)
	if idx < 0 || idx >= len(this.lines) {
		idx = -1
	}
	if idx != this.hoverIdx {
		this.hoverIdx = idx
		this.Self().Update()
	}
}

// OnMouseWheel handles vertical scrolling of the output.
func (this *BuildOutput) OnMouseWheel(x, y, z float64) {
	this.scrollY -= z * 3 * this.rowHeight
	if this.scrollY < 0 {
		this.scrollY = 0
	}
	_, h := this.Size()
	maxScroll := float64(len(this.lines))*this.rowHeight - h
	if maxScroll < 0 {
		maxScroll = 0
	}
	if this.scrollY > maxScroll {
		this.scrollY = maxScroll
	}
	this.Self().Update()
}

func (this *BuildOutput) SizeHints() gui.SizeHints {
	return gui.SizeHints{MinWidth: 200, MinHeight: 80}
}
