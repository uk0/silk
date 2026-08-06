package ged

import (
	"fmt"
	"path/filepath"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
	"github.com/uk0/silk/prop"
)

// DefaultDesignExt is the preferred file extension for SilkUI design files.
const DefaultDesignExt = ".silkui"

type Design struct {
	form  *gui.Form
	index map[string]gui.IWidget
}

func (this *Design) Form() *gui.Form {
	return this.form
}

func (this *Design) Widget(name string) gui.IWidget {
	p, _ := this.index[name]
	return p
}

type GedScene struct {
	graph.SceneItem
	filename string
	title    string
	// User-placed ruler guides, saved with the design (see guides.go).
	guides sceneGuides
	grid   GridModel
	// Tags the design declares (标签字典), saved with it (see tag-dictionary.go).
	tagDict []TagDecl
}

func NewGedScene() *GedScene {
	p := new(GedScene)
	p.Init(p)
	return p
}

func (this *GedScene) Init(self graph.IItem) {
	this.SceneItem.Init(self)
	this.SetLockPos(false)
	this.SetLockSize(false)
	this.SetSelectable(true)
	this.SetSize(100, 100)
	this.title = "Form"
	this.grid = defaultGridModel()
}

// Grid returns the scene's grid model — the single source the canvas painting,
// the drag/drop/paste snap and the coarse keyboard nudge all read.
func (this *GedScene) Grid() GridModel {
	return this.grid
}

// SetGrid replaces the grid model, clamping the pitch into its legal range so
// no caller (dialog, loader, or a hand-edited document) can install a pitch
// that paints a smear or disables snapping by accident.
func (this *GedScene) SetGrid(g GridModel) {
	g.Pitch = clampGridPitch(g.Pitch)
	this.grid = g
}

// TagDict returns the tags the design declares — what the 标签字典 dialog edits
// and what the code generator seeds into the generated app's tag registry.
func (this *GedScene) TagDict() []TagDecl {
	return this.tagDict
}

// SetTagDict replaces the declared tags, dropping nameless and duplicate entries
// so what the generator emits is what the runtime registry can actually key on.
func (this *GedScene) SetTagDict(decls []TagDecl) {
	this.tagDict = normalizeTagDict(decls)
}

//func (this *GedScene) Form() *FakeWidget {
//	return this.form
//}

func (this *GedScene) DrawSelf(g paint.Painter) {
	g.SetBrush1(gui.Theme().FormColor)
	g.Rectangle(this.Bounds())
	g.Fill()

	x0, y0, w, h := this.Bounds()

	// Two-tier alignment grid for design-mode rulers, both pitches derived
	// from the scene's grid model:
	//
	//   - Minor lines every pitch mm in light grey (230, 230, 235, 220) at
	//     0.1mm pen width — visible enough to align widgets against
	//     without overpowering the design itself.
	//   - Major lines every majorGridRatio cells in slightly darker
	//     (200, 210, 220, 240) at 0.15mm — give the eye an "every 10
	//     squares" anchor for judging form size without counting cells.
	//
	// Both tiers paint the same path system as the original single-
	// tier grid; the only cost is one extra Stroke pass, well below
	// the per-frame budget.
	if minorStep, majorStep, wantGrid := gridTiers(this.grid); wantGrid {
		g.SetPen1(paint.Color{230, 230, 235, 220}, 0.1)
		for x := x0; x <= x0+w; x += minorStep {
			g.MoveTo(x, y0)
			g.LineTo(x, y0+h)
		}
		for y := y0; y <= y0+h; y += minorStep {
			g.MoveTo(x0, y)
			g.LineTo(x0+w, y)
		}
		g.Stroke()

		g.SetPen1(paint.Color{200, 210, 220, 240}, 0.15)
		for x := x0; x <= x0+w; x += majorStep {
			g.MoveTo(x, y0)
			g.LineTo(x, y0+h)
		}
		for y := y0; y <= y0+h; y += majorStep {
			g.MoveTo(x0, y)
			g.LineTo(x0+w, y)
		}
		g.Stroke()
	}

	// Draw subtle form size indicator at bottom-right
	g.Save()
	sizeLabel := fmt.Sprintf("%.0f x %.0f mm", w, h)
	labelFont := paint.NewFont("", sizeLabelFontSize, false, false)
	g.SetFont(labelFont)
	g.SetBrush1(paint.Color{160, 160, 170, 140})
	g.DrawText1(sizeLabelX(x0, w, labelFont.TextExtents(sizeLabel).XAdvance, sizeLabelInset),
		y0+h-sizeLabelInset, sizeLabel)
	g.Restore()
}

// sizeLabelFontSize is the form-size readout's font size. The painter draws
// the scene in millimetres, so this is 7mm of line height, not 7 points.
const sizeLabelFontSize = 7

// sizeLabelInset is the gap (mm) between the readout and the form's bottom-
// right corner.
const sizeLabelInset = 1.5

// sizeLabelX returns the x where the form-size readout starts so that it ends
// `inset` inside the form's right edge. textWidth is the measured advance of
// the string: DrawAll clips every item to its own bounds, so laying the label
// out from a fixed offset silently cut it in half whenever the string was
// wider than that offset. Clamped to the form's left edge so a form narrower
// than its own readout shows the leading digits instead of starting outside
// the form.
func sizeLabelX(x0, w, textWidth, inset float64) float64 {
	x := x0 + w - inset - textWidth
	if x < x0 {
		return x0
	}
	return x
}

func (this *GedScene) Layout() {
	//this.form.SetPos(0, 0)
	//this.form.SetSize(this.Size())
}
func (this *GedScene) OpenFile(filename string) error {
	doc, err := core.LoadTDocFile(filename)
	if err != nil {
		return err
	}
	err = this.LoadDesign(doc)
	if err != nil {
		return err
	}
	this.filename = filename
	this.SetTitle(filepath.Base(this.filename))
	return nil
}

func (this *GedScene) SaveDesign() *core.TDoc {
	doc := core.NewTDoc()
	doc.SetValue("form")
	doc.WriteAttr("bounds", this.Bounds1())
	doc.WriteAttr("title", this.title)
	// Omitted when there are none, so designs without guides keep the exact
	// file shape they had before guides existed.
	if s := encodeGuides(this.guides); s != "" {
		doc.WriteAttr("guides", s)
	}
	doc.WriteAttr("grid_pitch", this.grid.Pitch)
	doc.WriteAttr("grid_visible", this.grid.Visible)
	doc.WriteAttr("grid_snap", this.grid.Snap)
	writeTagDict(doc, this.tagDict)
	if this.HasChildren() {
		child := core.NewTDoc()
		child.SetKey("children")
		for _, c := range this.Children() {
			if ia, ok := c.(interface {
				SaveDesign() *core.TDoc
			}); ok {
				p := ia.SaveDesign()
				if p != nil {
					child.AddChild(p)
				}
			}
		}
		doc.AddChild(child)
	}
	return doc
}

func (this *GedScene) Save() bool {
	core.Debug("GedScene.Save(): filename=", this.filename)
	if this.filename == "" {
		this.filename = gui.SaveFileDialog()
		if this.filename == "" {
			return false
		}
	}

	// Default to the .silkui extension when the user (or native dialog)
	// returned a path without any extension. Legacy extensions such as
	// .cml / .silk / .form are left intact for backwards compat.
	if filepath.Ext(this.filename) == "" {
		this.filename += DefaultDesignExt
	}

	doc := this.SaveDesign()
	err := doc.SaveFile(this.filename)
	if err == nil {
		this.UndoStack().SetClean()
		this.SetTitle(filepath.Base(this.filename))
		return true
	}
	return false
}

func (this *GedScene) LoadDesign(doc *core.TDoc) error {
	var bounds geom.Rect
	doc.ReadAttr("bounds", &bounds)
	this.SetBounds1(bounds)
	doc.ReadAttr("title", &this.title)
	var guides string
	doc.ReadAttr("guides", &guides)
	this.guides = decodeGuides(guides)
	// The grid attrs arrived after the format shipped. Seeding from the
	// defaults (rather than reading onto whatever the previous design left
	// in place) is what makes a document without them open with the grid the
	// designer has always drawn, no matter what was loaded before it.
	grid := defaultGridModel()
	doc.ReadAttr("grid_pitch", &grid.Pitch)
	doc.ReadAttr("grid_visible", &grid.Visible)
	doc.ReadAttr("grid_snap", &grid.Snap)
	this.SetGrid(grid)
	// Assigned, not merged: a document written before the dictionary existed has
	// no block, and must open as an empty dictionary even in a designer that
	// already had one loaded.
	this.tagDict = readTagDict(doc)
	for _, v := range this.Children() {
		v.Detach()
	}
	// Reconstruct top-level widgets via the shared loader. A node whose
	// factory is not registered (a widget renamed or removed across
	// versions, or a plugin widget that isn't loaded) is skipped together
	// with its subtree and counted, rather than aborting the whole load —
	// so one unknown widget no longer stops the entire .silkui file from
	// opening. A load that skipped some nodes still returns nil (partial
	// success); an all-unknown file simply yields an empty scene.
	var skipped int
	loadChildWidgets(doc.ChildByKey("children", false), this, &skipped)
	if skipped > 0 {
		core.Warn("silkui load: skipped ", skipped, " widget(s) with unknown factories")
	}
	return nil
}

//func (this *GedScene) SizeHints() gui.SizeHints {
//	x, y, w, h := this.Form().Bounds()
//	r := x + w + 20
//	b := y + h + 20

//	return gui.SizeHints{Width: r, Height: b}
//}

func (this *GedScene) Generate() *Design {

	form := gui.NewForm()
	design := new(Design)
	design.form = form
	design.index = make(map[string]gui.IWidget)

	w := gui.MmToPixelZ(this.Width())
	h := gui.MmToPixelZ(this.Height())
	form.SetSize(w, h)
	form.SetTitle(this.FormTitle())
	//core.Debug("w  h=", w, h)

	for _, v := range this.Children() {
		if fake, ok := v.(*FakeWidget); ok {
			w := fake.Generate()
			if w != nil {
				w.SetParent(form)
				if fake.name != "" {
					design.index[fake.name] = w
				}
			}
		}
	}

	form.SetVisible(false)
	return design
}

func (this *GedScene) Filename() string {
	return this.filename
}

func (this *GedScene) SetFilename(f string) {
	this.filename = f
}

func (this *GedScene) FormTitle() string {
	return this.title
}

func (this *GedScene) SetFormTitle(s string) {
	this.title = s
}

func (this *GedScene) EnumProperties(list prop.IPropertyList) {
	list.AddProperty("类型", func() string { return "Form (窗体)" }, nil)
	list.AddProperty("标题", this.FormTitle, this.SetFormTitle)
	list.AddProperty("宽度 (mm)", func() float64 { _, _, w, _ := this.Bounds(); return w },
		func(v float64) { _, _, _, h := this.Bounds(); this.SetSize(v, h) })
	list.AddProperty("高度 (mm)", func() float64 { _, _, _, h := this.Bounds(); return h },
		func(v float64) { _, _, w, _ := this.Bounds(); this.SetSize(w, v) })
}
