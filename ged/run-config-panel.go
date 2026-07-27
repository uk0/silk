package ged

import (
	"errors"
	"math"
	"strconv"
	"strings"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/ide/runconfig"
	"github.com/uk0/silk/paint"
)

func init() {
	core.RegisterFactory("ged.RunConfig", gui.TypeOf(RunConfigPanel{}))
	gui.RegisterToolView(gui.ToolViewDef{
		Id:   "ged.RunConfig",
		Name: "运行配置",
		Icon: "run",
		Desc: "运行配置 (命名配置列表 / 模式 / 目标 / 参数 / 工作目录 / 环境变量)",
	})
}

// errNoRunConfigStore is returned by the named-configuration operations
// when no store has been installed with SetStore — the panel is then a
// plain single-config editor and has nothing to add to or apply into.
var errNoRunConfigStore = errors.New("ged: run config panel has no store")

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

// splitRunArgs splits the panel's single args line into the argv slice a
// runconfig.Config stores. Whitespace-separated with no quote handling:
// the panel edits one plain text line, and a host that needs argv with
// embedded spaces writes it into the store directly.
func splitRunArgs(s string) []string {
	f := strings.Fields(s)
	if len(f) == 0 {
		return nil
	}
	return f
}

// nextRunMode returns the launch mode after mode in runconfig.Modes()
// order, wrapping around. Backs the click-to-cycle mode row.
func nextRunMode(mode string) string {
	modes := runconfig.Modes()
	for i, m := range modes {
		if m == mode {
			return modes[(i+1)%len(modes)]
		}
	}
	return modes[0]
}

// RunConfigPanel is a Qt Creator-style "Run Configuration" form: the
// project's list of named launch configurations on top, and a structured
// editor for the selected one below — target, launch mode, kit,
// command-line args, working directory and environment variables.
//
// Two layers, deliberately:
//
//   - The single-config layer (RunConfig / SetConfig / Config /
//     SigChanged) is the original API and still works standalone, with
//     no store attached: it edits args, working dir and env only.
//   - The named layer (SetStore / SelectConfig / AddConfig /
//     DuplicateSelected / RemoveSelected / Apply, reported through
//     SigSelect and SigApply) is backed by a runconfig.Store. Selecting
//     a configuration loads its fields into the editor; Apply writes the
//     edited fields back into the store under the selected name.
//
// The panel never launches anything and never persists the store itself
// — the host owns both.
type RunConfigPanel struct {
	gui.Widget

	cfg     RunConfig
	envRows []envRow

	store   *runconfig.Store   // nil when the panel runs as a plain single-config editor
	cfgList []runconfig.Config // cached store.List() snapshot for Draw / hit-test
	selName string             // name of the selected configuration, "" when none
	editCfg runconfig.Config   // working copy of the selected configuration (identity + target/mode/kit)

	hoverIdx  int // rcHoverNone / rcHover* / a list-row code / an env idx >= 0
	scrollY   float64
	cbChanged func(cfg RunConfig)
	cbSelect  func(cfg runconfig.Config)
	cbApply   func(cfg runconfig.Config)
}

// Synthetic row codes used by hoverIdx / hitRow. Env rows use real
// indices >= 0; configuration-list rows encode as rcListRowBase-idx, so
// one int carries the synthetic codes plus two independent row families.
const (
	rcHoverNone     = -1
	rcHoverArgs     = -2
	rcHoverWD       = -3
	rcHoverAdd      = -4 // env "+ 添加" button
	rcHoverTarget   = -5
	rcHoverMode     = -6
	rcHoverKit      = -7
	rcHoverCfgNew   = -8
	rcHoverCfgDup   = -9
	rcHoverCfgDel   = -10
	rcHoverCfgApply = -11
	rcListRowBase   = -100
)

// listRowOf decodes a hit code back into a configuration-list row index,
// reporting false for anything that is not a list row.
func listRowOf(code int) (int, bool) {
	if code > rcListRowBase {
		return 0, false
	}
	return rcListRowBase - code, true
}

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

// SigSelect registers the callback fired when a different configuration
// becomes selected — by a list click, SelectConfig / SelectIndex, or the
// implicit selection SetStore makes.
func (this *RunConfigPanel) SigSelect(fn func(cfg runconfig.Config)) {
	this.cbSelect = fn
}

// SigApply registers the callback fired after Apply has committed the
// edited fields into the store. It carries the stored configuration.
func (this *RunConfigPanel) SigApply(fn func(cfg runconfig.Config)) {
	this.cbApply = fn
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
// Named configurations (store-backed)
// ---------------------------------------------------------------------------

// SetStore installs the store backing the configuration list and selects
// its default configuration (firing SigSelect). Passing nil detaches the
// store, leaving the panel a plain single-config editor.
func (this *RunConfigPanel) SetStore(s *runconfig.Store) {
	this.store = s
	this.selName = ""
	this.editCfg = runconfig.Config{}
	this.refreshList()
	if s != nil {
		if d, ok := s.Default(); ok {
			this.SelectConfig(d.Name)
		}
	}
	this.Self().Update()
}

// Store returns the installed store, or nil.
func (this *RunConfigPanel) Store() *runconfig.Store { return this.store }

// Configs returns the panel's snapshot of the store's configurations, in
// list order.
func (this *RunConfigPanel) Configs() []runconfig.Config {
	out := make([]runconfig.Config, 0, len(this.cfgList))
	for _, c := range this.cfgList {
		out = append(out, c.Clone())
	}
	return out
}

// Selected returns the name of the selected configuration, "" when none.
func (this *RunConfigPanel) Selected() string { return this.selName }

// EditedConfig returns the selected configuration as currently edited:
// its stored identity (name, kit, pre-run tasks, default flag) merged
// with the live target, mode, args, working directory and env rows.
func (this *RunConfigPanel) EditedConfig() runconfig.Config {
	c := this.editCfg.Clone()
	c.Args = splitRunArgs(this.cfg.Args)
	c.WorkDir = this.cfg.WorkingDir
	c.Env = runconfig.ParseEnv(this.cfg.Env)
	return c
}

// SelectConfig selects the configuration named name, loading its fields
// into the editor and firing SigSelect (plus SigChanged when the loaded
// args / working dir / env differ from what was on screen). Re-selecting
// the already selected name is a no-op. Reports whether name was found.
func (this *RunConfigPanel) SelectConfig(name string) bool {
	if this.store == nil {
		return false
	}
	c, ok := this.store.Get(name)
	if !ok {
		return false
	}
	if this.selName == c.Name {
		return true
	}
	this.selName = c.Name
	this.editCfg = c
	this.loadEditFields(c)
	if this.cbSelect != nil {
		this.cbSelect(this.editCfg.Clone())
	}
	this.Self().Update()
	return true
}

// SelectIndex selects the configuration at idx in list order.
func (this *RunConfigPanel) SelectIndex(idx int) bool {
	if idx < 0 || idx >= len(this.cfgList) {
		return false
	}
	return this.SelectConfig(this.cfgList[idx].Name)
}

// AddConfig appends a new configuration named name — launch mode "run"
// against the project's own package — and selects it. The store's
// validation applies, so a blank name is rejected.
func (this *RunConfigPanel) AddConfig(name string) error {
	if this.store == nil {
		return errNoRunConfigStore
	}
	c := runconfig.Config{
		Name:   strings.TrimSpace(name),
		Target: ".",
		Mode:   runconfig.ModeRun,
	}
	if err := this.store.Add(c); err != nil {
		return err
	}
	this.refreshList()
	this.SelectConfig(c.Name)
	this.Self().Update()
	return nil
}

// DuplicateSelected copies the *stored* selected configuration under
// newName (never the default) and selects the copy. Unapplied in-panel
// edits are not carried over — Apply first to include them.
func (this *RunConfigPanel) DuplicateSelected(newName string) error {
	if this.store == nil {
		return errNoRunConfigStore
	}
	src, ok := this.store.Get(this.selName)
	if !ok {
		return runconfig.ErrNotFound
	}
	c := src.Clone()
	c.Name = strings.TrimSpace(newName)
	c.Default = false
	if err := this.store.Add(c); err != nil {
		return err
	}
	this.refreshList()
	this.SelectConfig(c.Name)
	this.Self().Update()
	return nil
}

// RemoveSelected deletes the selected configuration and selects the
// first remaining one, or clears the editor when none is left. Reports
// whether something was removed.
func (this *RunConfigPanel) RemoveSelected() bool {
	if this.store == nil || !this.store.Remove(this.selName) {
		return false
	}
	this.selName = ""
	this.editCfg = runconfig.Config{}
	this.refreshList()
	if len(this.cfgList) > 0 {
		this.SelectIndex(0)
	} else {
		this.loadEditFields(runconfig.Config{})
	}
	this.Self().Update()
	return true
}

// Apply commits EditedConfig into the store under the selected name and
// fires SigApply. Validation errors (a blank target, say) are returned
// unchanged and leave the store untouched.
func (this *RunConfigPanel) Apply() error {
	if this.store == nil {
		return errNoRunConfigStore
	}
	if this.selName == "" {
		return runconfig.ErrNotFound
	}
	c := this.EditedConfig()
	if err := this.store.Update(this.selName, c); err != nil {
		return err
	}
	this.selName = strings.TrimSpace(c.Name)
	this.editCfg = c
	this.refreshList()
	if this.cbApply != nil {
		this.cbApply(c.Clone())
	}
	this.Self().Update()
	return nil
}

// Target returns the edited target (package path or file).
func (this *RunConfigPanel) Target() string { return this.editCfg.Target }

// Mode returns the edited launch mode, defaulting to "run" when unset.
func (this *RunConfigPanel) Mode() string { return this.editCfg.EffectiveMode() }

// Kit returns the edited kit / toolchain name.
func (this *RunConfigPanel) Kit() string { return this.editCfg.KitName }

// SetTarget replaces the edited target.
func (this *RunConfigPanel) SetTarget(target string) {
	if target == this.editCfg.Target {
		return
	}
	this.editCfg.Target = target
	this.Self().Update()
}

// SetMode replaces the edited launch mode. Unknown modes are ignored.
func (this *RunConfigPanel) SetMode(mode string) {
	if !runconfig.ValidMode(mode) || mode == this.editCfg.Mode {
		return
	}
	this.editCfg.Mode = mode
	this.Self().Update()
}

// SetKit replaces the edited kit / toolchain name.
func (this *RunConfigPanel) SetKit(kit string) {
	if kit == this.editCfg.KitName {
		return
	}
	this.editCfg.KitName = kit
	this.Self().Update()
}

// refreshList resyncs the cached list snapshot from the store.
func (this *RunConfigPanel) refreshList() {
	if this.store == nil {
		this.cfgList = nil
		return
	}
	this.cfgList = this.store.List()
}

// loadEditFields pushes c's args / working dir / env into the
// single-config editor, firing SigChanged when the value actually
// changes so single-config hosts keep tracking the panel. Env order
// becomes sorted-by-key: a Config stores env as a map.
func (this *RunConfigPanel) loadEditFields(c runconfig.Config) {
	next := RunConfig{
		Args:       strings.Join(c.Args, " "),
		WorkingDir: c.WorkDir,
		Env:        c.EnvSlice(),
	}
	changed := !runConfigEqual(this.cfg, next)
	this.cfg = next
	this.rebuildEnvRows()
	if changed {
		this.fireChanged()
	}
}

// ---------------------------------------------------------------------------
// Layout constants
// ---------------------------------------------------------------------------

const (
	rcHeaderH   = 26.0
	rcRowH      = 32.0
	rcEnvRowH   = 26.0
	rcLabelW    = 110.0
	rcPadLeft   = 10.0
	rcPadRight  = 10.0
	rcBtnW      = 22.0
	rcAddBtnW   = 70.0
	rcAddBtnH   = 22.0
	rcGapY      = 6.0
	rcListHeadH = 20.0 // caption line above the configuration list
	rcListRowH  = 24.0
	rcCfgBtnW   = 52.0 // one configuration-list toolbar button
	rcCfgBtnGap = 6.0
)

// rcCfgBtnCount is the number of configuration-list toolbar buttons
// (新建 / 复制 / 删除 / 应用).
const rcCfgBtnCount = 4

// Layout helpers — kept in one place so Draw and the hit-tester agree.

// listY is the top of the configuration list's first row.
func (this *RunConfigPanel) listY() float64 {
	return rcHeaderH + rcListHeadH - this.scrollY
}

// listRowCount is the number of list rows drawn: the configuration count,
// or one reserved row for the "no configurations" placeholder.
func (this *RunConfigPanel) listRowCount() int {
	if len(this.cfgList) == 0 {
		return 1
	}
	return len(this.cfgList)
}

func (this *RunConfigPanel) listEndY() float64 {
	return this.listY() + float64(this.listRowCount())*rcListRowH
}

// cfgBtnY is the top of the configuration toolbar row.
func (this *RunConfigPanel) cfgBtnY() float64 { return this.listEndY() + rcGapY }

func (this *RunConfigPanel) rowYTarget() float64 {
	return this.cfgBtnY() + rcAddBtnH + rcGapY*2
}
func (this *RunConfigPanel) rowYMode() float64 { return this.rowYTarget() + rcRowH }
func (this *RunConfigPanel) rowYKit() float64  { return this.rowYMode() + rcRowH }
func (this *RunConfigPanel) rowYArgs() float64 { return this.rowYKit() + rcRowH }
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

// cfgBtnX returns the left edge of toolbar button i.
func cfgBtnX(i int) float64 {
	return rcPadLeft + float64(i)*(rcCfgBtnW+rcCfgBtnGap)
}

// cfgBtnIndexAt maps an x coordinate to a toolbar button index, or -1.
func cfgBtnIndexAt(x float64) int {
	for i := 0; i < rcCfgBtnCount; i++ {
		bx := cfgBtnX(i)
		if x >= bx && x <= bx+rcCfgBtnW {
			return i
		}
	}
	return -1
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

	// Configuration list caption + rows + toolbar
	g.SetFont(labelFont)
	g.SetBrush1(t.TextColor)
	g.DrawText1(rcPadLeft, rcHeaderH+rcListHeadH-6-this.scrollY, "配置列表")
	this.drawConfigList(g, w, h, labelFont, valueFont, t.TextColor, t.HighLightColor)
	this.drawConfigButtons(g, h, labelFont, t.HighLightColor)

	// Selected configuration's fields
	this.drawScalarRow(g, w, this.rowYTarget(), "目标 (包/文件)", this.editCfg.Target,
		this.hoverIdx == rcHoverTarget, t.TextColor, labelFont, valueFont)
	this.drawScalarRow(g, w, this.rowYMode(), "运行模式", this.Mode(),
		this.hoverIdx == rcHoverMode, t.TextColor, labelFont, valueFont)
	this.drawScalarRow(g, w, this.rowYKit(), "工具链 (Kit)", this.editCfg.KitName,
		this.hoverIdx == rcHoverKit, t.TextColor, labelFont, valueFont)
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

// drawConfigList renders one row per named configuration: the selected
// row is tinted and bold, the default one carries a leading marker, and
// the launch mode is shown on the right.
func (this *RunConfigPanel) drawConfigList(g paint.Painter, w, h float64,
	labelFont, valueFont paint.Font, textColor, hiColor paint.Color) {
	ly := this.listY()

	if len(this.cfgList) == 0 {
		if ly+rcListRowH > rcHeaderH && ly < h {
			g.SetFont(valueFont)
			g.SetBrush1(paint.Color{R: 150, G: 150, B: 160, A: 255})
			g.DrawText1(rcPadLeft+4, ly+rcListRowH*0.7, "(暂无配置，点击 新建)")
		}
		return
	}

	selFont := paint.NewFont(labelFont.Family(), 11, true, false)
	for i, c := range this.cfgList {
		ry := ly + float64(i)*rcListRowH
		if ry+rcListRowH < rcHeaderH || ry > h {
			continue
		}
		selected := c.Name == this.selName
		if selected {
			g.SetBrush1(hiColor)
			g.Rectangle(0, ry, w, rcListRowH)
			g.Fill()
		} else if hoverIdx, ok := listRowOf(this.hoverIdx); ok && hoverIdx == i {
			g.SetBrush1(paint.Color{R: 230, G: 235, B: 245, A: 255})
			g.Rectangle(0, ry, w, rcListRowH)
			g.Fill()
		}

		name := c.Name
		if c.Default {
			name = "* " + name
		}
		if selected {
			g.SetFont(selFont)
			g.SetBrush1(paint.Color{R: 255, G: 255, B: 255, A: 255})
		} else {
			g.SetFont(valueFont)
			g.SetBrush1(textColor)
		}
		g.DrawText1(rcPadLeft+4, ry+rcListRowH*0.7, name)

		modeX := w - rcPadRight - 52
		if modeX > rcPadLeft+80 {
			if !selected {
				g.SetBrush1(paint.Color{R: 120, G: 120, B: 130, A: 255})
			}
			g.SetFont(valueFont)
			g.DrawText1(modeX, ry+rcListRowH*0.7, c.EffectiveMode())
		}
	}
}

// drawConfigButtons renders the 新建 / 复制 / 删除 / 应用 toolbar under the
// configuration list.
func (this *RunConfigPanel) drawConfigButtons(g paint.Painter, h float64,
	labelFont paint.Font, hiColor paint.Color) {
	by := this.cfgBtnY()
	if by+rcAddBtnH < rcHeaderH || by > h {
		return
	}
	labels := [rcCfgBtnCount]string{"新建", "复制", "删除", "应用"}
	codes := [rcCfgBtnCount]int{rcHoverCfgNew, rcHoverCfgDup, rcHoverCfgDel, rcHoverCfgApply}
	for i, label := range labels {
		bx := cfgBtnX(i)
		fill := paint.Color{R: 240, G: 242, B: 248, A: 255}
		if this.hoverIdx == codes[i] {
			fill = hiColor
		}
		g.SetBrush1(fill)
		g.Rectangle(bx, by, rcCfgBtnW, rcAddBtnH)
		g.FillPreserve()
		g.SetPen1(paint.Color{R: 205, G: 208, B: 218, A: 255}, 1)
		g.Stroke()

		g.SetFont(labelFont)
		if this.hoverIdx == codes[i] {
			g.SetBrush1(paint.Color{R: 255, G: 255, B: 255, A: 255})
		} else {
			g.SetBrush1(paint.Color{R: 70, G: 74, B: 84, A: 255})
		}
		g.DrawText1(bx+8, by+rcAddBtnH*0.7, label)
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

// hitRow maps (x, y) to one of the synthetic row codes, a list-row code
// or an env idx.
func (this *RunConfigPanel) hitRow(x, y float64) int {
	if y < rcHeaderH {
		return rcHoverNone
	}
	ly := this.listY()
	if y >= ly && y < ly+float64(this.listRowCount())*rcListRowH {
		idx := int(math.Floor((y - ly) / rcListRowH))
		if idx >= 0 && idx < len(this.cfgList) {
			return rcListRowBase - idx
		}
		return rcHoverNone
	}
	by := this.cfgBtnY()
	if y >= by && y < by+rcAddBtnH {
		switch cfgBtnIndexAt(x) {
		case 0:
			return rcHoverCfgNew
		case 1:
			return rcHoverCfgDup
		case 2:
			return rcHoverCfgDel
		case 3:
			return rcHoverCfgApply
		}
		return rcHoverNone
	}
	ty := this.rowYTarget()
	if y >= ty && y < ty+rcRowH {
		return rcHoverTarget
	}
	my := this.rowYMode()
	if y >= my && y < my+rcRowH {
		return rcHoverMode
	}
	ky := this.rowYKit()
	if y >= ky && y < ky+rcRowH {
		return rcHoverKit
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
	if idx, ok := listRowOf(hit); ok {
		this.SelectIndex(idx)
		return
	}
	switch hit {
	case rcHoverCfgNew:
		val, ok := gui.ShowInputBox(this, nil, "新建运行配置", "配置名称:", "")
		if ok {
			this.reportRunConfigErr(this.AddConfig(val))
		}
	case rcHoverCfgDup:
		val, ok := gui.ShowInputBox(this, nil, "复制运行配置", "配置名称:",
			this.selName+" 副本")
		if ok {
			this.reportRunConfigErr(this.DuplicateSelected(val))
		}
	case rcHoverCfgDel:
		this.RemoveSelected()
	case rcHoverCfgApply:
		this.reportRunConfigErr(this.Apply())
	case rcHoverTarget:
		val, ok := gui.ShowInputBox(this, nil, "运行配置", "目标 (包路径或文件):",
			this.editCfg.Target)
		if ok {
			this.SetTarget(val)
		}
	case rcHoverMode:
		this.SetMode(nextRunMode(this.Mode()))
	case rcHoverKit:
		val, ok := gui.ShowInputBox(this, nil, "运行配置", "工具链 (Kit):",
			this.editCfg.KitName)
		if ok {
			this.SetKit(val)
		}
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

// reportRunConfigErr surfaces a store rejection (duplicate name, blank
// name / target) to the user. Nil is ignored.
func (this *RunConfigPanel) reportRunConfigErr(err error) {
	if err == nil {
		return
	}
	gui.ShowMessageBox(this, nil, "运行配置", err.Error(), []string{"确定"})
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
	return rcHeaderH + rcListHeadH + float64(this.listRowCount())*rcListRowH +
		rcGapY + rcAddBtnH + rcGapY*2 + 5*rcRowH + rcGapY*2 +
		float64(len(this.envRows))*rcEnvRowH + rcAddBtnH + 12
}

func (this *RunConfigPanel) SizeHints() gui.SizeHints {
	return gui.SizeHints{Width: 320, Height: 420, MinWidth: 260, MinHeight: 200}
}
