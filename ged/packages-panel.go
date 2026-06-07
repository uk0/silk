package ged

import (
	"path/filepath"
	"strconv"

	"silk/core"
	"silk/gui"
	"silk/paint"
)

func init() {
	core.RegisterFactory("ged.Packages", gui.TypeOf(PackagesPanel{}))
	gui.RegisterToolView(gui.ToolViewDef{
		Id:   "ged.Packages",
		Name: "包列表",
		Icon: "tree-view",
		Desc: "Go 项目包列表 (go list -json)",
	})
}

// packageRow is one drawable line in the PackagesPanel: either a
// collapsible package header or, when its owning package is expanded,
// one of its files. Splitting the flat row list out of the widget keeps
// the layout decision (which rows show in what order) pure and unit-
// testable; the panel itself only walks this slice when drawing and
// hit-testing.
//
// PkgIdx points back into the parent packages slice so the panel can
// resolve the click target without re-walking the tree.
type packageRow struct {
	Kind    packageRowKind
	PkgIdx  int    // index into PackagesPanel.packages
	File    string // empty for header rows
	IsTest  bool   // file rows: true for entries from TestGoFiles
}

type packageRowKind int

const (
	packageRowHeader packageRowKind = iota
	packageRowFile
)

// buildPackagesRows flattens a package list plus expansion state into the
// row sequence the panel renders. Order is deterministic: packages keep
// the input order from `go list -json`, and within an expanded package
// GoFiles come before TestGoFiles, each in their original order. Pure
// helper — no widget, no GL — so the layout decision is directly
// testable.
func buildPackagesRows(pkgs []core.GoListPackage, expanded map[string]bool) []packageRow {
	rows := make([]packageRow, 0, len(pkgs))
	for i, p := range pkgs {
		rows = append(rows, packageRow{Kind: packageRowHeader, PkgIdx: i})
		if !expanded[p.ImportPath] {
			continue
		}
		for _, f := range p.GoFiles {
			rows = append(rows, packageRow{Kind: packageRowFile, PkgIdx: i, File: f, IsTest: false})
		}
		for _, f := range p.TestGoFiles {
			rows = append(rows, packageRow{Kind: packageRowFile, PkgIdx: i, File: f, IsTest: true})
		}
	}
	return rows
}

// totalFileCount sums GoFiles + TestGoFiles across all packages. Pulled
// into its own helper so the header tally is computed the same way the
// tests assert against.
func totalFileCount(pkgs []core.GoListPackage) int {
	n := 0
	for _, p := range pkgs {
		n += len(p.GoFiles) + len(p.TestGoFiles)
	}
	return n
}

// PackagesPanel is a navigable view of the Go project's packages, as
// reported by `go list -json ./...`. Each package gets a collapsible
// header row showing its ImportPath and a "N go, M test" file-count
// badge; expanding the header reveals one row per GoFile/TestGoFile.
//
// The host (silkide) feeds in either a directory (LoadFromDir, which
// shells out via core.LoadGoListJSON) or a pre-parsed slice
// (SetPackages, used by tests and by hosts that already cache the
// list). Clicks fire two channels: SigPackageActivated for header rows
// (host policy decides whether to "open" the package), SigFileActivated
// for file rows (typically the IDE jumps the editor to that file).
type PackagesPanel struct {
	gui.Widget

	packages   []core.GoListPackage
	expanded   map[string]bool
	scrollY    float64
	hoverIdx   int
	rowHeight  float64
	cbPkgAct   func(pkg core.GoListPackage)
	cbFileAct  func(pkg core.GoListPackage, file string)
}

// NewPackagesPanel creates an empty packages panel.
func NewPackagesPanel() *PackagesPanel {
	p := new(PackagesPanel)
	p.Init(p)
	return p
}

func (this *PackagesPanel) Init(self gui.IWidget) {
	this.Widget.Init(self)
	this.rowHeight = 22
	this.hoverIdx = -1
	this.expanded = make(map[string]bool)
}

// LoadFromDir runs `go list -json ./...` in dir via core.LoadGoListJSON
// and replaces the package list with the result. The error is returned
// verbatim so the host can decide whether to surface it; on partial
// failures core.LoadGoListJSON returns both packages and an error, in
// which case the slice we keep still reflects whatever was parsable.
func (this *PackagesPanel) LoadFromDir(dir string) error {
	pkgs, err := core.LoadGoListJSON(dir)
	this.SetPackages(pkgs)
	return err
}

// SetPackages replaces the package list. Resets scroll and hover state
// so the new project starts at the top. Expansion state is wiped — the
// previous ImportPaths may not exist in the new list and keeping stale
// entries around just leaks memory.
func (this *PackagesPanel) SetPackages(pkgs []core.GoListPackage) {
	this.packages = pkgs
	this.expanded = make(map[string]bool)
	this.scrollY = 0
	this.hoverIdx = -1
	this.Self().Update()
}

// Packages returns the current package list as fed in by LoadFromDir
// or SetPackages.
func (this *PackagesPanel) Packages() []core.GoListPackage {
	return this.packages
}

// IsExpanded reports whether the package with the given ImportPath is
// currently expanded. Exposed for tests and for hosts that mirror the
// state into a persisted UI layout.
func (this *PackagesPanel) IsExpanded(importPath string) bool {
	return this.expanded[importPath]
}

// toggleExpanded flips the expansion state for a package's ImportPath.
// Pulled out of OnLeftDown so tests can drive the toggle directly
// without faking row geometry.
func (this *PackagesPanel) toggleExpanded(importPath string) {
	if this.expanded == nil {
		this.expanded = make(map[string]bool)
	}
	this.expanded[importPath] = !this.expanded[importPath]
	this.Self().Update()
}

// SigPackageActivated registers the callback fired when a package
// header row is clicked. Host policy decides what "activate" means —
// jump to the package directory, run go list refresh, etc.
func (this *PackagesPanel) SigPackageActivated(fn func(pkg core.GoListPackage)) {
	this.cbPkgAct = fn
}

// SigFileActivated registers the callback fired when a file row inside
// an expanded package is clicked. Hosts typically open the file at the
// package's Dir + file basename in the editor.
func (this *PackagesPanel) SigFileActivated(fn func(pkg core.GoListPackage, file string)) {
	this.cbFileAct = fn
}

// --- Drawing ---

const packagesHeaderH = 22.0

// Draw renders the header tally followed by one row per packageRow.
// Header rows show the expand glyph, the ImportPath, and a "N go, M
// test" file-count badge; file rows are indented and prefixed with a
// small file icon. Alternating row tint and hover highlight follow the
// sibling-panel idiom.
func (this *PackagesPanel) Draw(g paint.Painter) {
	w, h := this.Size()
	t := gui.Theme()

	// Background
	g.SetBrush1(t.ViewBGColor)
	g.Rectangle(0, 0, w, h)
	g.Fill()

	// Header band: "N packages, M files".
	g.SetBrush1(paint.Color{R: 235, G: 238, B: 245, A: 255})
	g.Rectangle(0, 0, w, packagesHeaderH)
	g.Fill()
	g.SetPen1(paint.Color{R: 200, G: 200, B: 210, A: 255}, 1)
	g.MoveTo(0, packagesHeaderH)
	g.LineTo(w, packagesHeaderH)
	g.Stroke()

	headerFont := paint.NewFont(t.Font.Family(), 12, true, false)
	g.SetFont(headerFont)
	g.SetBrush1(t.TextColor)
	g.DrawText1(8, packagesHeaderH-5,
		strconv.Itoa(len(this.packages))+" packages, "+
			strconv.Itoa(totalFileCount(this.packages))+" files")

	if len(this.packages) == 0 {
		emptyFont := paint.NewFont(t.Font.Family(), 11, false, false)
		g.SetFont(emptyFont)
		g.SetBrush1(paint.Color{R: 150, G: 150, B: 160, A: 200})
		g.DrawText1(8, packagesHeaderH+20, "No packages")
		return
	}

	rowFont := paint.NewFont(t.Font.Family(), 11, false, false)
	boldFont := paint.NewFont(t.Font.Family(), 11, true, false)
	g.SetFont(rowFont)
	fe := rowFont.FontExtents()

	rows := buildPackagesRows(this.packages, this.expanded)
	rh := this.rowHeight
	startY := packagesHeaderH - this.scrollY

	for i, r := range rows {
		rowY := startY + float64(i)*rh
		if rowY+rh < packagesHeaderH || rowY > h {
			continue
		}

		// Alternating row tint for readability.
		if i%2 == 1 {
			g.SetBrush1(paint.Color{R: 245, G: 247, B: 250, A: 255})
			g.Rectangle(0, rowY, w, rh)
			g.Fill()
		}
		// Hover highlight wins over the stripe.
		if i == this.hoverIdx {
			g.SetBrush1(paint.Color{R: 230, G: 235, B: 245, A: 255})
			g.Rectangle(0, rowY, w, rh)
			g.Fill()
		}

		textY := rowY + fe.Ascent + (rh-fe.Ascent-fe.Descent)/2
		pkg := this.packages[r.PkgIdx]

		switch r.Kind {
		case packageRowHeader:
			// Expand glyph: ▶ collapsed / ▼ expanded.
			glyph := "▶"
			if this.expanded[pkg.ImportPath] {
				glyph = "▼"
			}
			g.SetFont(rowFont)
			g.SetBrush1(paint.Color{R: 110, G: 120, B: 140, A: 255})
			g.DrawText1(8, textY, glyph)

			// ImportPath in bold accent.
			g.SetFont(boldFont)
			g.SetBrush1(t.HighLightColor)
			g.DrawText1(24, textY, pkg.ImportPath)
			ext := boldFont.TextExtents(pkg.ImportPath)

			// File-count badge — "N go, M test" — in muted text.
			g.SetFont(rowFont)
			g.SetBrush1(paint.Color{R: 130, G: 145, B: 165, A: 255})
			badge := strconv.Itoa(len(pkg.GoFiles)) + " go"
			if len(pkg.TestGoFiles) > 0 {
				badge += ", " + strconv.Itoa(len(pkg.TestGoFiles)) + " test"
			}
			g.DrawText1(24+ext.Width+10, textY, badge)

		case packageRowFile:
			// File icon glyph + name, indented under the header.
			g.SetFont(rowFont)
			g.SetBrush1(paint.Color{R: 130, G: 145, B: 165, A: 255})
			g.DrawText1(28, textY, "▤")

			name := filepath.Base(r.File)
			if r.IsTest {
				g.SetBrush1(paint.Color{R: 180, G: 130, B: 60, A: 255})
			} else {
				g.SetBrush1(t.TextColor)
			}
			g.DrawText1(44, textY, name)
		}
	}
}

// --- Events ---

// rowAt maps a y coordinate to a flat-row index, or -1 when y lands on
// the header band or below the last row. Walks the same buildPackagesRows
// slice the draw path uses so hit-testing tracks the rendered layout.
func (this *PackagesPanel) rowAt(y float64) int {
	if y < packagesHeaderH {
		return -1
	}
	idx := int((y - packagesHeaderH + this.scrollY) / this.rowHeight)
	rows := buildPackagesRows(this.packages, this.expanded)
	if idx < 0 || idx >= len(rows) {
		return -1
	}
	return idx
}

// OnLeftDown toggles expansion on a header click and fires the matching
// activation callback. The callbacks are independent: SigPackageActivated
// always fires for a header click, SigFileActivated fires for file rows.
// Toggling expansion is the panel's job; the host doesn't see it.
func (this *PackagesPanel) OnLeftDown(x, y float64) {
	this.SetFocus()
	idx := this.rowAt(y)
	if idx < 0 {
		return
	}
	rows := buildPackagesRows(this.packages, this.expanded)
	if idx >= len(rows) {
		return
	}
	r := rows[idx]
	pkg := this.packages[r.PkgIdx]
	switch r.Kind {
	case packageRowHeader:
		this.toggleExpanded(pkg.ImportPath)
		if this.cbPkgAct != nil {
			this.cbPkgAct(pkg)
		}
	case packageRowFile:
		if this.cbFileAct != nil {
			this.cbFileAct(pkg, r.File)
		}
	}
}

// OnMouseMove tracks hover state for visual feedback.
func (this *PackagesPanel) OnMouseMove(x, y float64) {
	idx := this.rowAt(y)
	if idx != this.hoverIdx {
		this.hoverIdx = idx
		this.Self().Update()
	}
}

// OnMouseLeave resets hover state.
func (this *PackagesPanel) OnMouseLeave() {
	if this.hoverIdx != -1 {
		this.hoverIdx = -1
		this.Self().Update()
	}
}

// OnMouseWheel scrolls the row list vertically. Bound by the total row
// extent so the user can't scroll past the last entry.
func (this *PackagesPanel) OnMouseWheel(x, y, z float64) {
	this.scrollY -= z * 3 * this.rowHeight
	if this.scrollY < 0 {
		this.scrollY = 0
	}
	_, h := this.Size()
	rows := buildPackagesRows(this.packages, this.expanded)
	maxScroll := float64(len(rows))*this.rowHeight - (h - packagesHeaderH)
	if maxScroll < 0 {
		maxScroll = 0
	}
	if this.scrollY > maxScroll {
		this.scrollY = maxScroll
	}
	this.Self().Update()
}

func (this *PackagesPanel) SizeHints() gui.SizeHints {
	return gui.SizeHints{MinWidth: 220, MinHeight: 80}
}
