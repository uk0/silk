package ged

import (
	"fmt"
	"math"
	"os"

	"silk/core"
	"silk/gui"
	"silk/paint"
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

// settingsRow holds one key-value row in the settings panel.
type settingsRow struct {
	Label    string
	Value    string
	Editable bool
}

// ProjectSettingsPanel displays and edits project configuration settings
// such as module name, Go version, build tags, and output directory.
type ProjectSettingsPanel struct {
	gui.Widget
	projectDir string
	goModule   string
	goVersion  string
	buildTags  string
	outputDir  string

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
	this.outputDir = "."
	this.loadProjectInfo()
	this.buildRows()
}

// loadProjectInfo determines the project directory and pulls go.mod info
// through RefreshGoMod, which uses core.LoadGoMod as the single source of truth.
func (this *ProjectSettingsPanel) loadProjectInfo() {
	dir, err := os.Getwd()
	if err != nil {
		dir = "."
	}
	this.projectDir = dir
	this.RefreshGoMod(dir)
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
	ver := m.GoVersion
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
// the panel's cached module path, Go version, requires count, and summary.
// When no go.mod can be found, fields fall back to the no-go.mod sentinel.
func (this *ProjectSettingsPanel) RefreshGoMod(projectDir string) {
	m, err := core.LoadGoMod(projectDir)
	if err != nil || m == nil {
		this.goMod = nil
		this.goModPath = ""
		this.goModSummary = goModSummaryString(nil)
		this.goModule = "(未找到 go.mod)"
		this.goVersion = "(未知)"
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
	this.goVersion = m.GoVersion
	if this.goVersion == "" {
		this.goVersion = "(未识别)"
	}
}

// buildRows constructs the display rows from current settings.
func (this *ProjectSettingsPanel) buildRows() {
	requires := "0"
	if this.goMod != nil {
		requires = fmt.Sprintf("%d", len(this.goMod.Requires))
	}
	this.rows = []settingsRow{
		{Label: "项目目录", Value: this.projectDir, Editable: false},
		{Label: "模块名称", Value: this.goModule, Editable: false},
		{Label: "Go 版本", Value: this.goVersion, Editable: false},
		{Label: "依赖数量", Value: requires, Editable: false},
		{Label: "构建标签", Value: this.buildTags, Editable: true},
		{Label: "输出目录", Value: this.outputDir, Editable: true},
	}
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
	psHeaderH    = 26.0
	psRowH       = 36.0
	psLabelW     = 120.0
	psPadLeft    = 10.0
	psPadRight   = 10.0
	psRowPadY    = 4.0
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

func (this *ProjectSettingsPanel) applyEdit(idx int, val string) {
	if idx < 0 || idx >= len(this.rows) {
		return
	}
	this.rows[idx].Value = val
	switch idx {
	case 4: // Build Tags
		this.buildTags = val
	case 5: // Output Dir
		this.outputDir = val
	}
	this.Self().Update()
}

// BuildTags returns the current build tags setting.
func (this *ProjectSettingsPanel) BuildTags() string {
	return this.buildTags
}

// OutputDir returns the current output directory setting.
func (this *ProjectSettingsPanel) OutputDir() string {
	return this.outputDir
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
