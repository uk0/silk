package ged

import (
	"os/exec"
	"silk/core"
	"silk/gui"
	"silk/paint"
	"strings"
)

func init() {
	core.RegisterFactory("ged.QuickHelp", gui.TypeOf(QuickHelpPanel{}))
	gui.RegisterToolView(gui.ToolViewDef{
		Id:   "ged.QuickHelp",
		Name: "快速帮助",
		Icon: "edit",
		Desc: "Qt Creator 风格的 Quick Help：调用 go doc 查看符号文档",
	})
}

// runGoDoc shells out to `go doc <symbol>` in the supplied working
// directory and returns the combined stdout+stderr as the rendered text.
// The bool return is wrapped inside err: a non-nil err is returned for
// non-zero exits, but the captured output is still useful — `go doc`
// prints a helpful "no such package/symbol" message on stderr in that
// case, and the caller (and tests) want to show it to the user.
//
// Kept as a small free function so the panel API is testable without
// dragging the GL widget into the smoke test.
func runGoDoc(symbol string, cwd string) (output string, err error) {
	if strings.TrimSpace(symbol) == "" {
		return "", nil
	}
	cmd := exec.Command("go", "doc", symbol)
	if cwd != "" {
		cmd.Dir = cwd
	}
	out, runErr := cmd.CombinedOutput()
	return string(out), runErr
}

// QuickHelpPanel is a Qt Creator-style "Quick Help" pane that renders
// Go documentation for a single symbol. The mechanism is a synchronous
// subprocess to `go doc <symbol>` whose combined stdout+stderr becomes
// the panel's text body.
//
// Synchronous because `go doc` is fast in practice (~50ms on a warm
// module cache); an async / cancellation path is a follow-up if it ever
// shows up as a perceptible stall. The host wires the lookup to "show
// doc for the word under cursor" — the widget itself does not know
// about editors.
type QuickHelpPanel struct {
	gui.Widget
	symbol    string
	doc       string
	lines     []string
	scrollY   float64
	rowHeight float64
}

// NewQuickHelpPanel creates an empty quick-help panel.
func NewQuickHelpPanel() *QuickHelpPanel {
	p := new(QuickHelpPanel)
	p.Init(p)
	return p
}

func (this *QuickHelpPanel) Init(self gui.IWidget) {
	this.Widget.Init(self)
	this.rowHeight = 16
}

// Lookup spawns `go doc <symbol>` and populates the panel with its
// output. An empty symbol is a no-op so callers can bind the panel
// directly to a "word under cursor" hook without guarding the call. On
// non-zero exit the captured error output is shown — `go doc` writes a
// useful "no such symbol" message in that case.
func (this *QuickHelpPanel) Lookup(symbol string) {
	symbol = strings.TrimSpace(symbol)
	if symbol == "" {
		return
	}
	out, _ := runGoDoc(symbol, "")
	this.symbol = symbol
	this.setDocText(out)
}

// SetDoc sets the panel's text body directly, without running a
// subprocess. Useful for tests, for hosts that already have the doc in
// hand (e.g. cached), and for clearing the panel via SetDoc("").
// Symbol() returns "" after this call — there is no associated symbol.
func (this *QuickHelpPanel) SetDoc(text string) {
	this.symbol = ""
	this.setDocText(text)
}

// Doc returns the current doc body, exactly as it was set (no header
// prefix, no trimming).
func (this *QuickHelpPanel) Doc() string {
	return this.doc
}

// Symbol returns the last symbol passed to Lookup, or "" when the panel
// is empty or was populated via SetDoc directly.
func (this *QuickHelpPanel) Symbol() string {
	return this.symbol
}

// setDocText is the shared "replace body + reset scroll + repaint" path
// used by both Lookup and SetDoc. Splitting the text into a `[]string`
// up front keeps Draw's per-frame work to an index-and-blit loop.
func (this *QuickHelpPanel) setDocText(text string) {
	this.doc = text
	if text == "" {
		this.lines = nil
	} else {
		this.lines = strings.Split(strings.TrimRight(text, "\n"), "\n")
	}
	this.scrollY = 0
	this.Self().Update()
}

// --- Drawing ---

const quickHelpHeaderH = 22.0

// Draw renders a header band showing the symbol (or "(no symbol)") and
// then one row per line of the doc body. Monospace, dark background to
// match the sibling output panes.
func (this *QuickHelpPanel) Draw(g paint.Painter) {
	w, h := this.Size()

	// Dark background, matching BuildOutput / ProblemsPanel.
	g.SetBrush1(paint.Color{R: 25, G: 25, B: 30, A: 255})
	g.Rectangle(0, 0, w, h)
	g.Fill()

	font := paint.NewFont("Menlo", 12, false, false)
	g.SetFont(font)
	fe := font.FontExtents()

	// Header band with the symbol name.
	g.SetBrush1(paint.Color{R: 35, G: 35, B: 42, A: 255})
	g.Rectangle(0, 0, w, quickHelpHeaderH)
	g.Fill()
	g.SetBrush1(paint.Color{R: 200, G: 200, B: 210, A: 255})
	header := this.symbol
	if header == "" {
		header = "(no symbol)"
	}
	g.DrawText1(8, fe.Ascent+4, header)

	if len(this.lines) == 0 {
		return
	}

	rh := this.rowHeight
	areaTop := quickHelpHeaderH
	startIdx := int(this.scrollY / rh)
	if startIdx < 0 {
		startIdx = 0
	}
	visibleCount := int((h-areaTop)/rh) + 2

	g.SetBrush1(paint.Color{R: 200, G: 200, B: 210, A: 255})
	for i := startIdx; i < startIdx+visibleCount && i < len(this.lines); i++ {
		y := areaTop + float64(i)*rh - this.scrollY
		g.DrawText1(8, y+fe.Ascent+2, this.lines[i])
	}
}

// --- Events ---

// OnMouseWheel scrolls the doc body vertically. Mirrors the wheel
// handling in BuildOutput / ProblemsPanel so the feel is consistent
// across the dockable panes.
func (this *QuickHelpPanel) OnMouseWheel(x, y, z float64) {
	this.scrollY -= z * 3 * this.rowHeight
	if this.scrollY < 0 {
		this.scrollY = 0
	}
	_, h := this.Size()
	maxScroll := float64(len(this.lines))*this.rowHeight - (h - quickHelpHeaderH)
	if maxScroll < 0 {
		maxScroll = 0
	}
	if this.scrollY > maxScroll {
		this.scrollY = maxScroll
	}
	this.Self().Update()
}

// OnLeftDown only grabs focus; there is nothing clickable in the body
// (yet — a future pass may make symbol references jump-to-able).
func (this *QuickHelpPanel) OnLeftDown(x, y float64) {
	this.SetFocus()
}

func (this *QuickHelpPanel) SizeHints() gui.SizeHints {
	return gui.SizeHints{MinWidth: 200, MinHeight: 80}
}
