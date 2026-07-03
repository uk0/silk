package ged

import (
	"fmt"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
	"strings"
	"time"
)

func init() {
	core.RegisterFactory("ged.ConsolePanel", gui.TypeOf(ConsolePanel{}))
	gui.RegisterToolView(gui.ToolViewDef{
		Id:   "ged.ConsolePanel",
		Name: "控制台",
		Icon: "edit",
		Desc: "运行时控制台输出",
	})
}

// ConsoleLine represents a single line of console output.
type ConsoleLine struct {
	Text      string
	Level     int // 0=stdout, 1=stderr, 2=system
	Timestamp string
}

// ConsolePanel is a terminal-like output panel for showing runtime output
// from F5-launched applications, similar to Qt Creator's Application Output.
type ConsolePanel struct {
	gui.Widget
	lines      []ConsoleLine
	scrollY    float64
	maxLines   int
	rowHeight  float64
	autoScroll bool
}

// NewConsolePanel creates a new console output panel.
func NewConsolePanel() *ConsolePanel {
	p := new(ConsolePanel)
	p.Init(p)
	return p
}

func (this *ConsolePanel) Init(self gui.IWidget) {
	this.Widget.Init(self)
	this.maxLines = 1000
	this.rowHeight = 18
	this.autoScroll = true
}

// AppendLine adds a single output line with a timestamp and level.
func (this *ConsolePanel) AppendLine(text string, level int) {
	ts := time.Now().Format("15:04:05.000")
	this.lines = append(this.lines, ConsoleLine{
		Text:      text,
		Level:     level,
		Timestamp: ts,
	})
	// Trim excess lines
	if len(this.lines) > this.maxLines {
		excess := len(this.lines) - this.maxLines
		this.lines = this.lines[excess:]
	}
	// Auto-scroll to bottom
	if this.autoScroll {
		_, h := this.Size()
		totalH := float64(len(this.lines)) * this.rowHeight
		if totalH > h {
			this.scrollY = totalH - h
		}
	}
	this.Self().Update()
}

// AppendOutput parses multi-line text and adds each line as stdout (level 0).
func (this *ConsolePanel) AppendOutput(text string) {
	parts := strings.Split(text, "\n")
	for _, part := range parts {
		if part == "" {
			continue
		}
		this.AppendLine(part, 0)
	}
}

// AppendError adds text as stderr output (level 1, displayed in red).
func (this *ConsolePanel) AppendError(text string) {
	parts := strings.Split(text, "\n")
	for _, part := range parts {
		if part == "" {
			continue
		}
		this.AppendLine(part, 1)
	}
}

// AppendSystem adds a system message (level 2, displayed in blue/italic).
func (this *ConsolePanel) AppendSystem(text string) {
	this.AppendLine(text, 2)
}

// Clear removes all console output.
func (this *ConsolePanel) Clear() {
	this.lines = nil
	this.scrollY = 0
	this.Self().Update()
}

// Draw renders the console panel.
func (this *ConsolePanel) Draw(g paint.Painter) {
	w, h := this.Size()

	// Dark terminal background
	g.SetBrush1(paint.Color{R: 30, G: 30, B: 35, A: 255})
	g.Rectangle(0, 0, w, h)
	g.Fill()

	if len(this.lines) == 0 {
		// Show placeholder
		font := paint.NewFont("Menlo", 12, false, false)
		g.SetFont(font)
		fe := font.FontExtents()
		g.SetBrush1(paint.Color{R: 80, G: 80, B: 90, A: 255})
		g.DrawText1(10, h/2+fe.Ascent/2, "控制台就绪 — 按 F5 运行应用程序")
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

	timestampW := 100.0 // width reserved for timestamp column
	lineNumW := 45.0    // width reserved for line numbers

	for i := startIdx; i < startIdx+visibleCount && i < len(this.lines); i++ {
		y := float64(i)*rh - this.scrollY
		line := this.lines[i]

		// Alternate row background for readability
		if i%2 == 1 {
			g.SetBrush1(paint.Color{R: 33, G: 33, B: 38, A: 255})
			g.Rectangle(0, y, w, rh)
			g.Fill()
		}

		// Error lines get subtle red background
		if line.Level == 1 {
			g.SetBrush1(paint.Color{R: 60, G: 25, B: 25, A: 180})
			g.Rectangle(0, y, w, rh)
			g.Fill()
		}

		textY := y + fe.Ascent + (rh-fe.Height)/2

		// Draw line number (dim)
		g.SetBrush1(paint.Color{R: 70, G: 70, B: 80, A: 255})
		numStr := fmt.Sprintf("%d", i+1)
		numExt := font.TextExtents(numStr)
		g.DrawText1(lineNumW-numExt.XAdvance-4, textY, numStr)

		// Draw timestamp (dim gray)
		g.SetBrush1(paint.Color{R: 90, G: 90, B: 100, A: 255})
		g.DrawText1(lineNumW+4, textY, line.Timestamp)

		// Draw text with level-based color
		textX := lineNumW + timestampW + 4
		switch line.Level {
		case 0: // stdout: light gray
			g.SetBrush1(paint.Color{R: 190, G: 190, B: 200, A: 255})
		case 1: // stderr: red
			g.SetBrush1(paint.Color{R: 230, G: 80, B: 80, A: 255})
		case 2: // system: blue
			g.SetBrush1(paint.Color{R: 80, G: 150, B: 230, A: 255})
		}

		// For system messages, use italic font
		if line.Level == 2 {
			italicFont := paint.NewFont("Menlo", 12, false, true)
			g.SetFont(italicFont)
			g.DrawText1(textX, textY, line.Text)
			g.SetFont(font) // restore
		} else {
			g.DrawText1(textX, textY, line.Text)
		}
	}

	// Draw separator line between line-numbers and timestamps
	g.SetPen1(paint.Color{R: 50, G: 50, B: 60, A: 255}, 1)
	g.Line(lineNumW, 0, lineNumW, h)
	g.Stroke()

	// Draw separator between timestamps and text
	g.SetPen1(paint.Color{R: 50, G: 50, B: 60, A: 255}, 1)
	g.Line(lineNumW+timestampW, 0, lineNumW+timestampW, h)
	g.Stroke()
}

// OnMouseWheel handles vertical scrolling.
func (this *ConsolePanel) OnMouseWheel(x, y, z float64) {
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
	// Disable auto-scroll if user scrolls up manually
	_, viewH := this.Size()
	totalH := float64(len(this.lines)) * this.rowHeight
	if totalH > viewH && this.scrollY < totalH-viewH-this.rowHeight {
		this.autoScroll = false
	} else {
		this.autoScroll = true
	}
	this.Self().Update()
}

// OnLeftDown handles focus on click.
func (this *ConsolePanel) OnLeftDown(x, y float64) {
	this.SetFocus()
}

func (this *ConsolePanel) SizeHints() gui.SizeHints {
	return gui.SizeHints{MinWidth: 200, MinHeight: 80}
}
