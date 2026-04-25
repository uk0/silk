package gui

import (
	"silk/core"
	"silk/paint"
	"math"
)

// AccordionSection 折叠面板的单个区段
type AccordionSection struct {
	Title    string
	Content  IWidget
	Expanded bool
}

// Accordion 手风琴/折叠面板控件
type Accordion struct {
	Widget
	sections    []AccordionSection
	multiExpand bool
	hoverIdx    int
	headerH     float64
	cbExpand    func(int, bool)
}

func init() {
	core.RegisterFactory("gui.Accordion", core.TypeOf((*Accordion)(nil)))
}

func NewAccordion() *Accordion {
	p := new(Accordion)
	p.Init(p)
	p.multiExpand = false
	p.hoverIdx = -1
	p.headerH = 32
	return p
}

func (this *Accordion) MultiExpand() bool { return this.multiExpand }

func (this *Accordion) SetMultiExpand(b bool) {
	this.multiExpand = b
}

func (this *Accordion) SectionCount() int { return len(this.sections) }

func (this *Accordion) AddSection(title string, content IWidget) {
	sec := AccordionSection{
		Title:    title,
		Content:  content,
		Expanded: len(this.sections) == 0, // first section expanded by default
	}
	this.sections = append(this.sections, sec)
	if content != nil {
		content.SetParent(this.Self())
		if !sec.Expanded {
			content.SetVisible(false)
		}
	}
	if i, ok := this.Self().(ILayout); ok {
		i.Layout()
	}
	this.Self().Update()
}

func (this *Accordion) ToggleSection(idx int) {
	if idx < 0 || idx >= len(this.sections) {
		return
	}

	if !this.multiExpand {
		// collapse all others
		for i := range this.sections {
			if i != idx && this.sections[i].Expanded {
				this.sections[i].Expanded = false
				if this.sections[i].Content != nil {
					this.sections[i].Content.SetVisible(false)
				}
			}
		}
	}

	this.sections[idx].Expanded = !this.sections[idx].Expanded
	if this.sections[idx].Content != nil {
		this.sections[idx].Content.SetVisible(this.sections[idx].Expanded)
	}

	if this.cbExpand != nil {
		this.cbExpand(idx, this.sections[idx].Expanded)
	}

	if i, ok := this.Self().(ILayout); ok {
		i.Layout()
	}
	this.Self().Update()
}

func (this *Accordion) SigExpand(fn func(int, bool)) {
	this.cbExpand = fn
}

// --- Events ---

func (this *Accordion) OnMouseEnter() { this.Self().Update() }
func (this *Accordion) OnMouseLeave() { this.hoverIdx = -1; this.Self().Update() }

func (this *Accordion) OnMouseMove(x, y float64) {
	old := this.hoverIdx
	this.hoverIdx = this.hitTestHeader(y)
	if old != this.hoverIdx {
		this.Self().Update()
	}
}

func (this *Accordion) OnLeftDown(x, y float64) {
	idx := this.hitTestHeader(y)
	if idx >= 0 {
		this.ToggleSection(idx)
	}
}

func (this *Accordion) hitTestHeader(y float64) int {
	cy := 0.0
	for i, sec := range this.sections {
		if y >= cy && y < cy+this.headerH {
			return i
		}
		cy += this.headerH
		if sec.Expanded && sec.Content != nil {
			hints := sec.Content.SizeHints()
			cy += hints.Height
		}
	}
	return -1
}

// --- Drawing ---

func (this *Accordion) Draw(g paint.Painter) {
	t := Theme()
	w, _ := this.Size()
	hh := this.headerH
	cy := 0.0

	if len(this.sections) == 0 {
		_, h := this.Size()
		g.SetFont(t.Font)
		g.SetBrush1(paint.Color{180, 185, 200, 255})
		g.DrawText1(4, h/2+4, "Accordion")
		return
	}

	for i, sec := range this.sections {
		// header background
		g.Rectangle(0, cy, w, hh)
		if this.hoverIdx == i {
			g.SetBrush1(t.FormLightColor)
		} else {
			g.SetBrush1(t.FormColor)
		}
		g.FillPreserve()
		g.SetPen1(t.BorderColor, 1)
		g.Stroke()

		// expand/collapse arrow
		arrowX := 12.0
		arrowY := cy + hh/2
		arrowS := 4.0

		g.Save()
		if sec.Expanded {
			// down arrow
			g.MoveTo(arrowX-arrowS, arrowY-arrowS/2)
			g.LineTo(arrowX, arrowY+arrowS/2)
			g.LineTo(arrowX+arrowS, arrowY-arrowS/2)
		} else {
			// right arrow
			g.MoveTo(arrowX-arrowS/2, arrowY-arrowS)
			g.LineTo(arrowX+arrowS/2, arrowY)
			g.LineTo(arrowX-arrowS/2, arrowY+arrowS)
		}
		g.SetPen1(t.FormDarkColor, 1.5)
		g.Stroke()
		g.Restore()

		// title text
		g.SetFont(t.Font)
		fe := t.Font.FontExtents()
		ext := t.Font.TextExtents(sec.Title)
		tx := 28.0 - ext.XBearing
		ty := cy + 0.5*(hh+ext.YBearing) - ext.YBearing

		g.SetBrush1(t.TextColor)
		g.Translate(tx, ty)
		g.DrawText(sec.Title)
		g.Translate(-tx, -ty)

		cy += hh

		// content area
		if sec.Expanded && sec.Content != nil {
			hints := sec.Content.SizeHints()
			contentH := hints.Height
			// border around content
			g.Rectangle(0, cy, w, contentH)
			g.SetPen1(t.BorderColor, 0.5)
			g.Stroke()
			cy += contentH
		}

		_ = fe
	}
}

func (this *Accordion) Layout() {
	w, _ := this.Self().Size()
	hh := this.headerH
	cy := 0.0

	for _, sec := range this.sections {
		cy += hh
		if sec.Expanded && sec.Content != nil {
			hints := sec.Content.SizeHints()
			contentH := hints.Height
			sec.Content.SetBounds(4, cy, w-8, contentH)
			cy += contentH
		}
	}
}

func (this *Accordion) SizeHints() SizeHints {
	w := 200.0
	h := 0.0
	hh := this.headerH
	for _, sec := range this.sections {
		h += hh
		if sec.Expanded && sec.Content != nil {
			hints := sec.Content.SizeHints()
			h += hints.Height
			if hints.Width+8 > w {
				w = hints.Width + 8
			}
		}
	}
	return SizeHints{Width: math.Max(w, 200), Height: math.Max(h, hh), Policy: GrowHorizontal | GrowVertical}
}

func (this *Accordion) EnumProperties(list core.IPropertyList) {
	list.AddProperty("多展开", this.MultiExpand, this.SetMultiExpand)
}
