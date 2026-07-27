package ged

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/ide/kits"
	"github.com/uk0/silk/paint"
)

func init() {
	core.RegisterFactory("ged.ProjectSettingsPanel", gui.TypeOf(ProjectSettingsPanel{}))
	gui.RegisterToolView(gui.ToolViewDef{
		Id:   "ged.ProjectSettingsPanel",
		Name: "项目",
		Icon: "propsheet",
		Desc: "项目设置面板",
	})
}

// Row keys. Rows are addressed by key, not by index, so adding a row
// cannot silently re-point an edit at the wrong field.
const (
	psKeyDir        = "dir"
	psKeyModule     = "module"
	psKeyGoVersion  = "goversion"
	psKeyToolchain  = "toolchain"
	psKeyRequires   = "requires"
	psKeyKit        = "kit"
	psKeyGOOS       = "goos"
	psKeyGOARCH     = "goarch"
	psKeyTags       = "tags"
	psKeyBuildMode  = "buildmode"
	psKeyRace       = "race"
	psKeyCoverage   = "coverage"
	psKeyOutputDir  = "outdir"
	psKeyVariant    = "variant"
	psKeyDeployKind = "deploykind"
	psKeyDeployHost = "deployhost"
	psKeyDeployUser = "deployuser"
	psKeyRemoteDir  = "remotedir"
)

// settingsRow holds one key-value row in the settings panel.
type settingsRow struct {
	Key      string
	Label    string
	Value    string
	Editable bool
}

// ProjectSettingsPanel displays and edits a project's build configuration:
// the parsed go.mod facts (module, effective Go version, toolchain, require
// count) plus the active kit — toolchain target, build tags/mode, race and
// coverage switches, output directory and deploy profile.
//
// The panel reads and writes the project it was pointed at with
// SetProjectRoot; without that call it falls back to the process working
// directory for display only and does not persist anything (see SaveKits).
type ProjectSettingsPanel struct {
	gui.Widget
	projectDir string
	rootSet    bool // true once SetProjectRoot pinned an explicit project root
	goModule   string
	goVersion  string
	toolchain  string

	// Kit configuration, loaded from and saved to the project's kits file.
	store       *kits.Store
	kit         core.Kit
	variantName string
	saveErr     error
	cbApply     func()

	// Latest parsed go.mod summary. Populated by RefreshGoMod.
	goMod        *core.GoMod
	goModPath    string // absolute path to the go.mod found, "" if none
	goModSummary string // one-line summary, e.g. "Module: silk • Go 1.22 • 12 requires"

	rows       []settingsRow
	hoverIdx   int
	editingIdx int
	editBuf    string
	scrollY    float64
}

func NewProjectSettingsPanel() *ProjectSettingsPanel {
	p := new(ProjectSettingsPanel)
	p.Init(p)
	return p
}

func (this *ProjectSettingsPanel) Init(self gui.IWidget) {
	this.Widget.Init(self)
	this.hoverIdx = -1
	this.editingIdx = -1
	this.kit = core.DefaultKit()
	this.variantName = kits.VariantDebug
	this.loadProjectInfo()
	this.buildRows()
}

// SetProjectRoot points the panel at dir and reloads everything from there:
// the go.mod facts and the project's kits file. This is what makes the panel
// show the *selected* project rather than whatever directory the IDE process
// happens to have been started in.
func (this *ProjectSettingsPanel) SetProjectRoot(dir string) {
	if dir == "" {
		return
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	this.projectDir = dir
	this.rootSet = true
	this.loadProjectInfo()
	this.buildRows()
	this.Self().Update()
}

// ProjectRoot returns the directory the panel reads the project from.
func (this *ProjectSettingsPanel) ProjectRoot() string {
	return this.projectDir
}

// loadProjectInfo pulls go.mod info through RefreshGoMod (core.LoadGoMod is
// the single source of truth) and binds the kits store, both against the
// panel's project root. The root falls back to the process working directory
// only when SetProjectRoot was never called.
func (this *ProjectSettingsPanel) loadProjectInfo() {
	dir := this.projectDir
	if dir == "" {
		wd, err := os.Getwd()
		if err != nil {
			wd = "."
		}
		dir = wd
	}
	this.projectDir = dir
	this.RefreshGoMod(dir)
	this.loadKits(dir)
}

// loadKits binds the panel to dir's kits file and adopts its active kit and
// variant. A project with no kits file (or a corrupt one) keeps a host kit
// derived from the parsed go.mod, so the rows are never blank.
func (this *ProjectSettingsPanel) loadKits(dir string) {
	this.store = kits.New(dir)
	this.saveErr = this.store.Load()

	if k, ok := this.store.ActiveKit(); ok {
		this.kit = k
	} else {
		this.kit = core.DefaultKit()
		if v := this.goMod.EffectiveGoVersion(); v != "" {
			this.kit.GoVersion = v
		}
	}
	this.variantName = this.store.ActiveVariantName()
}

// LoadKits re-reads the project's kits file, discarding unsaved edits.
func (this *ProjectSettingsPanel) LoadKits() error {
	this.loadKits(this.projectDir)
	this.buildRows()
	this.Self().Update()
	return this.saveErr
}

// SaveKits writes the current kit and variant selection into the project's
// kits file.
//
// It is a no-op unless SetProjectRoot pinned an explicit project root:
// without one the panel does not know which project the edit belongs to and
// must not drop a kits file into the process working directory. An invalid
// kit (see core.Kit.Validate) is also refused — the edit stays in memory and
// the error is reported through SaveError so a half-configured ssh deploy
// never reaches disk.
func (this *ProjectSettingsPanel) SaveKits() error {
	if this.store == nil || !this.rootSet {
		return nil
	}
	if err := this.store.Set(this.kit); err != nil {
		return err
	}
	if err := this.store.SetActiveKit(this.kit.Name); err != nil {
		return err
	}
	if this.variantName != "" {
		if err := this.store.SetActiveVariant(this.variantName); err != nil {
			return err
		}
	}
	return this.store.Save()
}

// SaveError returns the error from the last load or save attempt, nil when
// the last one succeeded.
func (this *ProjectSettingsPanel) SaveError() error {
	return this.saveErr
}

// SigApply registers the callback fired after an edit has been applied to
// the kit. The host uses it to re-run the build with the new configuration.
// It fires even when persistence failed — the in-memory kit did change.
func (this *ProjectSettingsPanel) SigApply(fn func()) {
	this.cbApply = fn
}

// Kit returns the active kit (a copy).
func (this *ProjectSettingsPanel) Kit() core.Kit {
	return this.kit.Clone()
}

// SetKit replaces the active kit, persists it and fires SigApply.
func (this *ProjectSettingsPanel) SetKit(k core.Kit) {
	this.kit = k.Clone()
	this.apply()
}

// VariantName returns the active build variant (Debug / Release).
func (this *ProjectSettingsPanel) VariantName() string {
	return this.variantName
}

// SetVariantName switches the active build variant. Unknown names are
// rejected and leave the selection untouched.
func (this *ProjectSettingsPanel) SetVariantName(name string) error {
	if this.store == nil {
		return fmt.Errorf("no kits store")
	}
	v, ok := this.store.Variant(name)
	if !ok {
		return fmt.Errorf("unknown build variant %q", name)
	}
	if v.Name == this.variantName {
		return nil
	}
	this.variantName = v.Name
	this.apply()
	return nil
}

// ResolvedKit returns the active kit with the active variant applied — the
// configuration a build should actually run with.
func (this *ProjectSettingsPanel) ResolvedKit() core.Kit {
	if this.store != nil {
		if v, ok := this.store.Variant(this.variantName); ok {
			return v.Apply(this.kit)
		}
	}
	return this.kit.Clone()
}

// apply persists the current kit, refreshes the rows and notifies the host.
func (this *ProjectSettingsPanel) apply() {
	this.saveErr = this.SaveKits()
	this.buildRows()
	if this.cbApply != nil {
		this.cbApply()
	}
	this.Self().Update()
}

// goModSummaryString builds the one-line summary for the parsed go.mod.
// Pure helper so it can be unit-tested without instantiating a widget.
// Returns "(no go.mod found)" when m is nil.
func goModSummaryString(m *core.GoMod) string {
	if m == nil {
		return "(no go.mod found)"
	}
	mod := m.Module
	if mod == "" {
		mod = "(unknown)"
	}
	ver := m.EffectiveGoVersion()
	if ver == "" {
		ver = "(unknown)"
	}
	n := len(m.Requires)
	noun := "requires"
	if n == 1 {
		noun = "require"
	}
	return fmt.Sprintf("Module: %s • Go %s • %d %s", mod, ver, n, noun)
}

// RefreshGoMod re-parses the project's go.mod via core.LoadGoMod and updates
// the panel's cached module path, Go version, toolchain, requires count, and
// summary. The Go version shown is the *effective* one: a toolchain directive
// wins over the go directive, because that is the version the build uses.
// When no go.mod can be found, fields fall back to the no-go.mod sentinel.
func (this *ProjectSettingsPanel) RefreshGoMod(projectDir string) {
	m, err := core.LoadGoMod(projectDir)
	if err != nil || m == nil {
		this.goMod = nil
		this.goModPath = ""
		this.goModSummary = goModSummaryString(nil)
		this.goModule = "(未找到 go.mod)"
		this.goVersion = "(未知)"
		this.toolchain = ""
		return
	}
	this.goMod = m
	if path, ok := core.FindGoMod(projectDir); ok {
		this.goModPath = path
	} else {
		this.goModPath = ""
	}
	this.goModSummary = goModSummaryString(m)
	this.goModule = m.Module
	if this.goModule == "" {
		this.goModule = "(未识别)"
	}
	this.goVersion = m.EffectiveGoVersion()
	if this.goVersion == "" {
		this.goVersion = "(未识别)"
	}
	this.toolchain = m.Toolchain
}

// buildRows constructs the display rows from current settings: the read-only
// go.mod facts first, then the editable kit / build variant / deploy fields.
func (this *ProjectSettingsPanel) buildRows() {
	requires := "0"
	if this.goMod != nil {
		requires = fmt.Sprintf("%d", len(this.goMod.Requires))
	}
	toolchain := this.toolchain
	if toolchain == "" {
		toolchain = "(默认)"
	}
	buildMode := this.kit.BuildMode
	if buildMode == "" {
		buildMode = "default"
	}
	deployKind := this.kit.Deploy.Kind
	if deployKind == "" {
		deployKind = core.DeployLocal
	}
	this.rows = []settingsRow{
		{Key: psKeyDir, Label: "项目目录", Value: this.projectDir},
		{Key: psKeyModule, Label: "模块名称", Value: this.goModule},
		{Key: psKeyGoVersion, Label: "Go 版本", Value: this.goVersion},
		{Key: psKeyToolchain, Label: "工具链", Value: toolchain},
		{Key: psKeyRequires, Label: "依赖数量", Value: requires},
		{Key: psKeyKit, Label: "Kit 名称", Value: this.kit.Name, Editable: true},
		{Key: psKeyGOOS, Label: "目标系统", Value: this.kit.GOOS, Editable: true},
		{Key: psKeyGOARCH, Label: "目标架构", Value: this.kit.GOARCH, Editable: true},
		{Key: psKeyTags, Label: "构建标签", Value: core.FormatBuildTags(this.kit.Tags), Editable: true},
		{Key: psKeyBuildMode, Label: "构建模式", Value: buildMode, Editable: true},
		{Key: psKeyRace, Label: "竞态检测", Value: formatSettingsBool(this.kit.Race), Editable: true},
		{Key: psKeyCoverage, Label: "覆盖率", Value: formatSettingsBool(this.kit.Coverage), Editable: true},
		{Key: psKeyOutputDir, Label: "输出目录", Value: this.kit.OutputDir, Editable: true},
		{Key: psKeyVariant, Label: "构建变体", Value: this.variantName, Editable: true},
		{Key: psKeyDeployKind, Label: "部署方式", Value: deployKind, Editable: true},
		{Key: psKeyDeployHost, Label: "部署主机", Value: this.kit.Deploy.Host, Editable: true},
		{Key: psKeyDeployUser, Label: "部署用户", Value: this.kit.Deploy.User, Editable: true},
		{Key: psKeyRemoteDir, Label: "远程目录", Value: this.kit.Deploy.RemoteDir, Editable: true},
	}
}

// rowIndexByKey returns the index of the row with the given key, or -1.
func (this *ProjectSettingsPanel) rowIndexByKey(key string) int {
	for i := range this.rows {
		if this.rows[i].Key == key {
			return i
		}
	}
	return -1
}

// parseSettingsBool reads a boolean the user typed into a row. Accepts the
// on/off wording the rows display plus the usual true/false/yes/no/1/0.
// The second result is false for anything else, so a typo leaves the field
// untouched instead of silently clearing it.
func parseSettingsBool(s string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "on", "true", "yes", "y", "1", "开":
		return true, true
	case "off", "false", "no", "n", "0", "关":
		return false, true
	}
	return false, false
}

// formatSettingsBool renders a boolean the way the rows display it.
func formatSettingsBool(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// Refresh re-reads project info and rebuilds the display.
func (this *ProjectSettingsPanel) Refresh() {
	this.loadProjectInfo()
	this.buildRows()
	this.Self().Update()
}

// ---------------------------------------------------------------------------
// Layout constants
// ---------------------------------------------------------------------------

const (
	psHeaderH  = 26.0
	psRowH     = 36.0
	psLabelW   = 120.0
	psPadLeft  = 10.0
	psPadRight = 10.0
	psRowPadY  = 4.0
)

// ---------------------------------------------------------------------------
// Drawing
// ---------------------------------------------------------------------------

func (this *ProjectSettingsPanel) Draw(g paint.Painter) {
	t := gui.Theme()
	w, h := this.Size()

	// Background
	g.SetBrush1(t.ViewBGColor)
	g.Rectangle(0, 0, w, h)
	g.Fill()

	// Header
	g.SetBrush1(paint.Color{235, 238, 245, 255})
	g.Rectangle(0, 0, w, psHeaderH)
	g.Fill()
	g.SetPen1(paint.Color{200, 200, 210, 255}, 1)
	g.MoveTo(0, psHeaderH)
	g.LineTo(w, psHeaderH)
	g.Stroke()

	titleFont := paint.NewFont(t.Font.Family(), 12, true, false)
	g.SetFont(titleFont)
	g.SetBrush1(t.TextColor)
	g.DrawText1(8, psHeaderH-7, "Project Settings")

	// Module summary: one-line "Module: <path> • Go <ver> • <N> requires"
	// rendered right-aligned in the header so the parsed go.mod is visible
	// even before the user scrolls through individual rows.
	summary := this.goModSummary
	if summary == "" {
		summary = goModSummaryString(this.goMod)
	}
	summaryFont := paint.NewFont(t.Font.Family(), 11, false, false)
	g.SetFont(summaryFont)
	g.SetBrush1(paint.Color{90, 95, 110, 255})
	sext := summaryFont.TextExtents(summary)
	sx := w - sext.Width - 8
	if sx < 120 {
		sx = 120
	}
	g.DrawText1(sx, psHeaderH-7, summary)

	// Rows
	labelFont := paint.NewFont(t.Font.Family(), 11, true, false)
	valueFont := paint.NewFont(t.Font.Family(), 11, false, false)

	for i, row := range this.rows {
		ry := psHeaderH + float64(i)*psRowH + psRowPadY - this.scrollY
		if ry+psRowH < psHeaderH || ry > h {
			continue
		}

		// Hover highlight
		if i == this.hoverIdx {
			g.SetBrush1(paint.Color{230, 235, 245, 255})
			g.Rectangle(0, ry, w, psRowH)
			g.Fill()
		}

		// Label
		g.SetFont(labelFont)
		g.SetBrush1(t.TextColor)
		g.DrawText1(psPadLeft, ry+psRowH*0.6, row.Label)

		// Value
		valX := psPadLeft + psLabelW
		valW := w - valX - psPadRight
		if valW < 10 {
			valW = 10
		}

		if i == this.editingIdx {
			// Editing mode: show the edit buffer with a highlighted background
			g.SetBrush1(paint.Color{255, 255, 255, 255})
			g.Rectangle(valX-2, ry+4, valW+4, psRowH-8)
			g.FillPreserve()
			g.SetPen1(t.HighLightColor, 1.5)
			g.Stroke()

			g.SetFont(valueFont)
			g.SetBrush1(t.TextColor)
			g.DrawText1(valX, ry+psRowH*0.6, this.editBuf+"_")
		} else {
			g.SetFont(valueFont)
			if row.Editable {
				g.SetBrush1(t.HighLightColor)
			} else {
				g.SetBrush1(paint.Color{120, 120, 130, 255})
			}
			displayVal := row.Value
			if row.Editable && displayVal == "" {
				displayVal = "(点击编辑)"
			}
			g.DrawText1(valX, ry+psRowH*0.6, displayVal)
		}

		// Row separator
		g.SetPen1(paint.Color{230, 230, 235, 100}, 0.5)
		g.MoveTo(0, ry+psRowH)
		g.LineTo(w, ry+psRowH)
		g.Stroke()
	}

	// Refresh button at the bottom
	btnY := psHeaderH + float64(len(this.rows))*psRowH + 12 - this.scrollY
	if btnY > psHeaderH && btnY < h {
		btnW := 80.0
		btnH := 24.0
		btnX := (w - btnW) / 2
		g.SetBrush1(t.HighLightColor)
		g.Rectangle(btnX, btnY, btnW, btnH)
		g.Fill()
		g.SetFont(labelFont)
		g.SetBrush1(paint.Color{255, 255, 255, 255})
		refLabel := "刷新"
		rext := labelFont.TextExtents(refLabel)
		g.DrawText1(btnX+(btnW-rext.Width)/2, btnY+btnH*0.65, refLabel)
	}
}

// ---------------------------------------------------------------------------
// Interaction
// ---------------------------------------------------------------------------

func (this *ProjectSettingsPanel) hitRow(y float64) int {
	if y < psHeaderH {
		return -1
	}
	idx := int(math.Floor((y - psHeaderH + this.scrollY) / psRowH))
	if idx < 0 || idx >= len(this.rows) {
		return -1
	}
	return idx
}

func (this *ProjectSettingsPanel) OnMouseMove(x, y float64) {
	idx := this.hitRow(y)
	if idx != this.hoverIdx {
		this.hoverIdx = idx
		this.Self().Update()
	}
}

func (this *ProjectSettingsPanel) OnMouseLeave() {
	if this.hoverIdx != -1 {
		this.hoverIdx = -1
		this.Self().Update()
	}
}

func (this *ProjectSettingsPanel) OnLeftDown(x, y float64) {
	// Check for refresh button click
	btnY := psHeaderH + float64(len(this.rows))*psRowH + 12 - this.scrollY
	w, _ := this.Size()
	btnW := 80.0
	btnH := 24.0
	btnX := (w - btnW) / 2
	if x >= btnX && x <= btnX+btnW && y >= btnY && y <= btnY+btnH {
		this.Refresh()
		return
	}

	idx := this.hitRow(y)
	if idx < 0 || idx >= len(this.rows) {
		// Click outside -- commit any pending edit
		this.commitEdit()
		return
	}

	row := this.rows[idx]
	if !row.Editable {
		this.commitEdit()
		return
	}

	// Switch rows toggle in place — no point popping an input box for on/off.
	switch row.Key {
	case psKeyRace:
		this.commitEdit()
		this.applyEdit(idx, formatSettingsBool(!this.kit.Race))
		return
	case psKeyCoverage:
		this.commitEdit()
		this.applyEdit(idx, formatSettingsBool(!this.kit.Coverage))
		return
	}

	// Start inline editing via input box
	this.commitEdit()
	val, ok := gui.ShowInputBox(this, nil, "编辑设置", row.Label+":", row.Value)
	if ok {
		this.applyEdit(idx, val)
	}
}

func (this *ProjectSettingsPanel) commitEdit() {
	if this.editingIdx < 0 {
		return
	}
	this.applyEdit(this.editingIdx, this.editBuf)
	this.editingIdx = -1
	this.editBuf = ""
	this.Self().Update()
}

// applyEdit writes an edited row value back into the kit, persists it and
// fires SigApply. Values the field cannot accept (an unknown deploy kind, a
// non-boolean in a switch row) are dropped and the row is redrawn from the
// model, so the display never disagrees with what was stored.
func (this *ProjectSettingsPanel) applyEdit(idx int, val string) {
	if idx < 0 || idx >= len(this.rows) {
		return
	}
	if !this.applyKitField(this.rows[idx].Key, val) {
		// Nothing changed — still resync the row so a rejected edit
		// does not linger on screen.
		this.buildRows()
		this.Self().Update()
		return
	}
	this.apply()
}

// applyKitField maps a row key onto the kit / variant model.
// Returns true when the model actually changed.
func (this *ProjectSettingsPanel) applyKitField(key, val string) bool {
	val = strings.TrimSpace(val)
	switch key {
	case psKeyKit:
		if val == "" || val == this.kit.Name {
			return false
		}
		this.kit.Name = val
	case psKeyGOOS:
		if val == "" || val == this.kit.GOOS {
			return false
		}
		this.kit.GOOS = val
	case psKeyGOARCH:
		if val == "" || val == this.kit.GOARCH {
			return false
		}
		this.kit.GOARCH = val
	case psKeyTags:
		tags := core.ParseBuildTags(val)
		if core.FormatBuildTags(tags) == core.FormatBuildTags(this.kit.Tags) {
			return false
		}
		this.kit.Tags = tags
	case psKeyBuildMode:
		mode := strings.ToLower(val)
		if mode == "default" {
			mode = ""
		}
		if mode == this.kit.BuildMode {
			return false
		}
		probe := this.kit.Clone()
		probe.BuildMode = mode
		if err := probe.Validate(); err != nil {
			this.saveErr = err
			return false
		}
		this.kit.BuildMode = mode
	case psKeyRace:
		b, ok := parseSettingsBool(val)
		if !ok || b == this.kit.Race {
			return false
		}
		this.kit.Race = b
	case psKeyCoverage:
		b, ok := parseSettingsBool(val)
		if !ok || b == this.kit.Coverage {
			return false
		}
		this.kit.Coverage = b
	case psKeyOutputDir:
		if val == this.kit.OutputDir {
			return false
		}
		this.kit.OutputDir = val
	case psKeyVariant:
		if this.store == nil {
			return false
		}
		v, ok := this.store.Variant(val)
		if !ok {
			this.saveErr = fmt.Errorf("unknown build variant %q", val)
			return false
		}
		if v.Name == this.variantName {
			return false
		}
		this.variantName = v.Name
	case psKeyDeployKind:
		kind := strings.ToLower(val)
		if kind == "" {
			kind = core.DeployLocal
		}
		if kind != core.DeployLocal && kind != core.DeploySSH {
			this.saveErr = fmt.Errorf("unknown deploy kind %q", val)
			return false
		}
		if kind == this.kit.Deploy.Kind {
			return false
		}
		this.kit.Deploy.Kind = kind
	case psKeyDeployHost:
		if val == this.kit.Deploy.Host {
			return false
		}
		this.kit.Deploy.Host = val
	case psKeyDeployUser:
		if val == this.kit.Deploy.User {
			return false
		}
		this.kit.Deploy.User = val
	case psKeyRemoteDir:
		if val == this.kit.Deploy.RemoteDir {
			return false
		}
		this.kit.Deploy.RemoteDir = val
	default:
		// Read-only go.mod rows.
		return false
	}
	return true
}

// BuildTags returns the current build tags setting as a `go build -tags` list.
func (this *ProjectSettingsPanel) BuildTags() string {
	return core.FormatBuildTags(this.kit.Tags)
}

// OutputDir returns the current output directory setting.
func (this *ProjectSettingsPanel) OutputDir() string {
	return this.kit.OutputDir
}

func (this *ProjectSettingsPanel) OnMouseWheel(x, y, delta float64) {
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

func (this *ProjectSettingsPanel) contentHeight() float64 {
	return psHeaderH + float64(len(this.rows))*psRowH + 50
}

func (this *ProjectSettingsPanel) SizeHints() gui.SizeHints {
	return gui.SizeHints{Width: 300, Height: 300}
}
