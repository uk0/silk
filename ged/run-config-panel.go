package ged

import (
	"math"
	"strconv"
	"strings"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

func init() {
	core.RegisterFactory("ged.RunConfig", gui.TypeOf(RunConfigPanel{}))
	gui.RegisterToolView(gui.ToolViewDef{
		Id:   "ged.RunConfig",
		Name: "运行配置",
		Icon: "run",
		Desc: "运行配置 (命令行参数 / 工作目录 / 环境变量)",
	})
}

// RunConfig is the structured run configuration the panel edits.
// Env entries are "KEY=value" strings; blank entries are dropped on
// parse/serialize round-trips (see parseEnvLines / joinEnvLines).
type RunConfig struct {
	Args       string
	WorkingDir string
	Env        []string
}

// envRow is one editable environment variable line in the panel. We
// keep the raw KEY=VALUE text so the user can edit either side freely;
// the host reads the flushed entries through Config().
type envRow struct {
	Text string
}

// parseEnvLines splits a multi-line raw string into env entries,
// trimming surrounding whitespace and dropping blank lines.
// Order is preserved.
func parseEnvLines(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, "\n")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		s := strings.TrimSpace(p)
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	return out
}

// joinEnvLines joins env entries with newlines. Empty input returns "".
func joinEnvLines(env []string) string {
	if len(env) == 0 {
		return ""
	}
	return strings.Join(env, "\n")
}

// runConfigEqual compares two RunConfigs by value, including env order.
// Used so SetConfig does not fire SigChanged for a no-op update.
func runConfigEqual(a, b RunConfig) bool {
	if a.Args != b.Args || a.WorkingDir != b.WorkingDir {
		return false
	}
	if len(a.Env) != len(b.Env) {
		return false
	}
	for i := range a.Env {
		if a.Env[i] != b.Env[i] {
			return false
		}
	}
	return true
}

// RunConfigPanel is a Qt Creator-style "Run Configuration" form: a
// structured editor for the project's command-line args, working
// directory, and environment variables.
//
// The host pushes the current config in via SetConfig and registers a
// SigChanged callback. The panel does not read or write preferences
// itself — it is a pure view + editor over a RunConfig value.
type RunConfigPanel struct {
	gui.Widget

	cfg     RunConfig
	envRows []envRow

	hoverIdx  int // rcHoverNone / rcHoverArgs / rcHoverWD / rcHoverAdd or env idx >= 0
	scrollY   float64
	cbChanged func(cfg RunConfig)
}

// Synthetic row codes used by hoverIdx / hitRow. Env rows use real
// indices >= 0.
const (
	rcHoverNone = -1
	rcHoverArgs = -2
	rcHoverWD   = -3
	rcHoverAdd  = -4
)

// NewRunConfigPanel creates an empty run-config panel.
func NewRunConfigPanel() *RunConfigPanel {
	p := new(RunConfigPanel)
	p.Init(p)
	return p
}

func (this *RunConfigPanel) Init(self gui.IWidget) {
	this.Widget.Init(self)
	this.hoverIdx = rcHoverNone
}

// SetConfig replaces the current configuration. If the normalised new
// value equals the current one, SigChanged is NOT fired — this keeps
// idempotent host pushes cheap.
func (this *RunConfigPanel) SetConfig(cfg RunConfig) {
	// Normalise the Env slice through parse/join so the round-trip
	// behaviour (drop blanks, trim) is the panel's canonical form.
	normalised := RunConfig{
		Args:       cfg.Args,
		WorkingDir: cfg.WorkingDir,
		Env:        parseEnvLines(joinEnvLines(cfg.Env)),
	}
	if runConfigEqual(this.cfg, normalised) {
		// Still resync envRows so the editor view tracks the model.
		this.rebuildEnvRows()
		return
	}
	this.cfg = normalised
	this.rebuildEnvRows()
	this.fireChanged()
	this.Self().Update()
}

// Config returns the current configuration. The returned Env slice is a
// fresh copy; mutating it does not affect the panel.
func (this *RunConfigPanel) Config() RunConfig {
	out := RunConfig{Args: this.cfg.Args, WorkingDir: this.cfg.WorkingDir}
	if len(this.cfg.Env) > 0 {
		out.Env = make([]string, len(this.cfg.Env))
		copy(out.Env, this.cfg.Env)
	}
	return out
}

// SigChanged registers the callback fired whenever the configuration
// changes — through SetConfig with a different value, or through an
// in-panel edit (args / working dir / env add / env edit / env remove).
func (this *RunConfigPanel) SigChanged(fn func(cfg RunConfig)) {
	this.cbChanged = fn
}

// rebuildEnvRows resyncs envRows from cfg.Env.
func (this *RunConfigPanel) rebuildEnvRows() {
	this.envRows = make([]envRow, len(this.cfg.Env))
	for i, e := range this.cfg.Env {
		this.envRows[i] = envRow{Text: e}
	}
}

// flushEnvRowsToCfg rebuilds cfg.Env from envRows, dropping blank rows.
// Returns true if cfg.Env changed.
func (this *RunConfigPanel) flushEnvRowsToCfg() bool {
	next := make([]string, 0, len(this.envRows))
	for _, r := range this.envRows {
		s := strings.TrimSpace(r.Text)
		if s == "" {
			continue
		}
		next = append(next, s)
	}
	if len(next) == len(this.cfg.Env) {
		same := true
		for i := range next {
			if next[i] != this.cfg.Env[i] {
				same = false
				break
			}
		}
		if same {
			return false
		}
	}
	this.cfg.Env = next
	return true
}

func (this *RunConfigPanel) fireChanged() {
	if this.cbChanged != nil {
		this.cbChanged(this.Config())
	}
}

// ---------------------------------------------------------------------------
// Layout constants
// ---------------------------------------------------------------------------

const (
	rcHeaderH  = 26.0
	rcRowH     = 32.0
	rcEnvRowH  = 26.0
	rcLabelW   = 110.0
	rcPadLeft  = 10.0
	rcPadRight = 10.0
	rcBtnW     = 22.0
	rcAddBtnW  = 70.0
	rcAddBtnH  = 22.0
	rcGapY     = 6.0
)

// Layout helpers — kept in one place so Draw and the hit-tester agree.
func (this *RunConfigPanel) rowYArgs() float64 { return rcHeaderH + rcGapY - this.scrollY }
func (this *RunConfigPanel) rowYWD() float64   { return this.rowYArgs() + rcRowH }
func (this *RunConfigPanel) envBlockY() float64 {
	return this.rowYWD() + rcRowH + rcGapY*2
}
func (this *RunConfigPanel) envBlockEnd() float64 {
	return this.envBlockY() + float64(len(this.envRows))*rcEnvRowH
}
func (this *RunConfigPanel) addBtnY() float64 {
	return this.envBlockEnd() + rcGapY
}

// ---------------------------------------------------------------------------
// Drawing
// ---------------------------------------------------------------------------

func (this *RunConfigPanel) Draw(g paint.Painter) {
	t := gui.Theme()
	w, h := this.Size()

	// Background
	g.SetBrush1(t.ViewBGColor)
	g.Rectangle(0, 0, w, h)
	g.Fill()

	// Header strip
	g.SetBrush1(paint.Color{R: 235, G: 238, B: 245, A: 255})
	g.Rectangle(0, 0, w, rcHeaderH)
	g.Fill()
	g.SetPen1(paint.Color{R: 200, G: 200, B: 210, A: 255}, 1)
	g.MoveTo(0, rcHeaderH)
	g.LineTo(w, rcHeaderH)
	g.Stroke()

	headerFont := paint.NewFont(t.Font.Family(), 12, true, false)
	g.SetFont(headerFont)
	g.SetBrush1(t.TextColor)
	g.DrawText1(8, rcHeaderH-7, "运行配置 (Run Configuration)")

	labelFont := paint.NewFont(t.Font.Family(), 11, true, false)
	valueFont := paint.NewFont(t.Font.Family(), 11, false, false)

	// Args row
	this.drawScalarRow(g, w, this.rowYArgs(), "命令行参数", this.cfg.Args,
		this.hoverIdx == rcHoverArgs, t.TextColor, labelFont, valueFont)
	// Working dir row
	this.drawScalarRow(g, w, this.rowYWD(), "工作目录", this.cfg.WorkingDir,
		this.hoverIdx == rcHoverWD, t.TextColor, labelFont, valueFont)

	// Env section header
	envHeaderY := this.envBlockY() - rcGapY
	g.SetFont(labelFont)
	g.SetBrush1(t.TextColor)
	g.DrawText1(rcPadLeft, envHeaderY, "环境变量")

	// Env rows
	for i, row := range this.envRows {
		ry := this.envBlockY() + float64(i)*rcEnvRowH
		if ry+rcEnvRowH < rcHeaderH || ry > h {
			continue
		}
		if this.hoverIdx == i {
			g.SetBrush1(paint.Color{R: 230, G: 235, B: 245, A: 255})
			g.Rectangle(0, ry, w, rcEnvRowH)
			g.Fill()
		}

		// Value box
		valX := rcPadLeft
		valW := w - rcPadLeft - rcPadRight - rcBtnW - 6
		if valW < 20 {
			valW = 20
		}
		g.SetBrush1(paint.Color{R: 255, G: 255, B: 255, A: 255})
		g.Rectangle(valX, ry+3, valW, rcEnvRowH-6)
		g.FillPreserve()
		g.SetPen1(paint.Color{R: 210, G: 210, B: 220, A: 255}, 1)
		g.Stroke()

		g.SetFont(valueFont)
		display := row.Text
		if display == "" {
			display = "(KEY=value)"
			g.SetBrush1(paint.Color{R: 150, G: 150, B: 160, A: 255})
		} else {
			g.SetBrush1(t.TextColor)
		}
		g.DrawText1(valX+6, ry+rcEnvRowH*0.65, display)

		// Remove button
		btnX := w - rcPadRight - rcBtnW
		g.SetBrush1(paint.Color{R: 230, G: 90, B: 90, A: 255})
		g.Rectangle(btnX, ry+3, rcBtnW, rcEnvRowH-6)
		g.Fill()
		g.SetFont(labelFont)
		g.SetBrush1(paint.Color{R: 255, G: 255, B: 255, A: 255})
		g.DrawText1(btnX+7, ry+rcEnvRowH*0.65, "-")
	}

	// Add button + count hint
	addY := this.addBtnY()
	if addY > rcHeaderH && addY < h {
		btnX := rcPadLeft
		g.SetBrush1(t.HighLightColor)
		g.Rectangle(btnX, addY, rcAddBtnW, rcAddBtnH)
		g.Fill()
		g.SetFont(labelFont)
		g.SetBrush1(paint.Color{R: 255, G: 255, B: 255, A: 255})
		g.DrawText1(btnX+10, addY+rcAddBtnH*0.65, "+ 添加")

		hint := strconv.Itoa(len(this.envRows)) + " 项"
		g.SetFont(valueFont)
		g.SetBrush1(paint.Color{R: 120, G: 120, B: 130, A: 255})
		g.DrawText1(btnX+rcAddBtnW+10, addY+rcAddBtnH*0.65, hint)
	}
}

// drawScalarRow renders the Args / Working Dir labelled row.
func (this *RunConfigPanel) drawScalarRow(g paint.Painter, w, ry float64,
	label, value string, hover bool, textColor paint.Color,
	labelFont, valueFont paint.Font) {
	if hover {
		g.SetBrush1(paint.Color{R: 230, G: 235, B: 245, A: 255})
		g.Rectangle(0, ry, w, rcRowH)
		g.Fill()
	}
	g.SetFont(labelFont)
	g.SetBrush1(textColor)
	g.DrawText1(rcPadLeft, ry+rcRowH*0.6, label)

	valX := rcPadLeft + rcLabelW
	valW := w - valX - rcPadRight
	if valW < 20 {
		valW = 20
	}
	g.SetBrush1(paint.Color{R: 255, G: 255, B: 255, A: 255})
	g.Rectangle(valX, ry+5, valW, rcRowH-10)
	g.FillPreserve()
	g.SetPen1(paint.Color{R: 210, G: 210, B: 220, A: 255}, 1)
	g.Stroke()

	g.SetFont(valueFont)
	display := value
	if display == "" {
		display = "(点击编辑)"
		g.SetBrush1(paint.Color{R: 150, G: 150, B: 160, A: 255})
	} else {
		g.SetBrush1(textColor)
	}
	g.DrawText1(valX+6, ry+rcRowH*0.6, display)
}

// ---------------------------------------------------------------------------
// Interaction
// ---------------------------------------------------------------------------

// hitRow maps (x, y) to one of the synthetic row codes or an env idx.
func (this *RunConfigPanel) hitRow(x, y float64) int {
	if y < rcHeaderH {
		return rcHoverNone
	}
	ay := this.rowYArgs()
	if y >= ay && y < ay+rcRowH {
		return rcHoverArgs
	}
	wy := this.rowYWD()
	if y >= wy && y < wy+rcRowH {
		return rcHoverWD
	}
	ey := this.envBlockY()
	if y >= ey && y < ey+float64(len(this.envRows))*rcEnvRowH {
		idx := int(math.Floor((y - ey) / rcEnvRowH))
		if idx >= 0 && idx < len(this.envRows) {
			return idx
		}
	}
	addY := this.addBtnY()
	if y >= addY && y < addY+rcAddBtnH && x >= rcPadLeft && x <= rcPadLeft+rcAddBtnW {
		return rcHoverAdd
	}
	return rcHoverNone
}

// isRemoveButtonHit reports whether (x, y) lies in an env row's [-] button.
func (this *RunConfigPanel) isRemoveButtonHit(envIdx int, x, y float64) bool {
	if envIdx < 0 || envIdx >= len(this.envRows) {
		return false
	}
	w, _ := this.Size()
	ry := this.envBlockY() + float64(envIdx)*rcEnvRowH
	btnX := w - rcPadRight - rcBtnW
	return x >= btnX && x <= btnX+rcBtnW && y >= ry+3 && y <= ry+rcEnvRowH-3
}

func (this *RunConfigPanel) OnMouseMove(x, y float64) {
	idx := this.hitRow(x, y)
	if idx != this.hoverIdx {
		this.hoverIdx = idx
		this.Self().Update()
	}
}

func (this *RunConfigPanel) OnMouseLeave() {
	if this.hoverIdx != rcHoverNone {
		this.hoverIdx = rcHoverNone
		this.Self().Update()
	}
}

func (this *RunConfigPanel) OnLeftDown(x, y float64) {
	this.SetFocus()
	hit := this.hitRow(x, y)
	switch hit {
	case rcHoverArgs:
		val, ok := gui.ShowInputBox(this, nil, "运行配置", "命令行参数:", this.cfg.Args)
		if ok {
			this.SetArgs(val)
		}
	case rcHoverWD:
		val, ok := gui.ShowInputBox(this, nil, "运行配置", "工作目录:", this.cfg.WorkingDir)
		if ok {
			this.SetWorkingDir(val)
		}
	case rcHoverAdd:
		this.AddEnv("")
	case rcHoverNone:
		return
	default:
		if hit >= 0 && hit < len(this.envRows) {
			if this.isRemoveButtonHit(hit, x, y) {
				this.RemoveEnv(hit)
				return
			}
			val, ok := gui.ShowInputBox(this, nil, "环境变量",
				"KEY=value:", this.envRows[hit].Text)
			if ok {
				this.SetEnvAt(hit, val)
			}
		}
	}
}

// AddEnv appends a new env row. Exposed publicly so tests and host code
// can drive edits without going through ShowInputBox.
func (this *RunConfigPanel) AddEnv(text string) {
	this.envRows = append(this.envRows, envRow{Text: text})
	if this.flushEnvRowsToCfg() {
		this.fireChanged()
	}
	this.Self().Update()
}

// RemoveEnv deletes the env row at idx. No-op on out-of-range idx.
func (this *RunConfigPanel) RemoveEnv(idx int) {
	if idx < 0 || idx >= len(this.envRows) {
		return
	}
	this.envRows = append(this.envRows[:idx], this.envRows[idx+1:]...)
	if this.flushEnvRowsToCfg() {
		this.fireChanged()
	}
	this.Self().Update()
}

// SetEnvAt replaces the env row at idx. No-op on out-of-range idx.
func (this *RunConfigPanel) SetEnvAt(idx int, text string) {
	if idx < 0 || idx >= len(this.envRows) {
		return
	}
	this.envRows[idx].Text = text
	if this.flushEnvRowsToCfg() {
		this.fireChanged()
	}
	this.Self().Update()
}

// SetArgs replaces the args string.
func (this *RunConfigPanel) SetArgs(args string) {
	if args == this.cfg.Args {
		return
	}
	this.cfg.Args = args
	this.fireChanged()
	this.Self().Update()
}

// SetWorkingDir replaces the working directory.
func (this *RunConfigPanel) SetWorkingDir(dir string) {
	if dir == this.cfg.WorkingDir {
		return
	}
	this.cfg.WorkingDir = dir
	this.fireChanged()
	this.Self().Update()
}

func (this *RunConfigPanel) OnMouseWheel(x, y, delta float64) {
	this.scrollY -= delta * 15
	if this.scrollY < 0 {
		this.scrollY = 0
	}
	maxScroll := this.contentHeight() - this.Height()
	if maxScroll < 0 {
		maxScroll = 0
	}
	if this.scrollY > maxScroll {
		this.scrollY = maxScroll
	}
	this.Self().Update()
}

func (this *RunConfigPanel) contentHeight() float64 {
	return rcHeaderH + 2*rcRowH + rcGapY*3 + float64(len(this.envRows))*rcEnvRowH + rcAddBtnH + 12
}

func (this *RunConfigPanel) SizeHints() gui.SizeHints {
	return gui.SizeHints{Width: 300, Height: 200, MinWidth: 240, MinHeight: 140}
}
