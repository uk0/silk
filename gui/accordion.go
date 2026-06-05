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
	curIdx      int // 键盘当前区段(高亮的 header), -1 表示无
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
	p.curIdx = -1
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
	this.SetFocus() // 取得焦点, 之后键盘事件才会派发到本控件
	idx := this.hitTestHeader(y)
	if idx >= 0 {
		this.curIdx = idx
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

// accordionMoveCurrent 计算键盘导航后的当前区段下标(纯函数, 便于单测).
// Up/Down 在 [0,n-1] 内上移/下移(不回绕), Home/End 跳到首/末; 其它键不改动.
// cur 为 -1(尚无当前项)时, Down/Home 落到 0, Up/End 落到 n-1.
func accordionMoveCurrent(cur, n, key int) int {
	if n <= 0 {
		return -1
	}
	switch key {
	case KeyDown:
		if cur < 0 {
			return 0
		}
		return clampIndex(cur+1, n)
	case KeyUp:
		if cur < 0 {
			return n - 1
		}
		return clampIndex(cur-1, n)
	case KeyHome:
		return 0
	case KeyEnd:
		return n - 1
	}
	return clampIndex(cur, n)
}

// OnKeyDown 实现 IEventKeyDown, 给折叠面板加键盘导航(对标 Qt QAccordion 风格):
// Up/Down 移动当前 header, Home/End 跳到首/末区段, 回车/空格 切换当前区段的
// 展开/折叠 — 走与点击相同的 ToggleSection, 因此单展开/多展开语义与回调都一致.
// 仅在控件持有焦点时被调用(OnLeftDown 里 SetFocus 后才能收到键盘事件).
func (this *Accordion) OnKeyDown(key int, repeat bool) {
	n := len(this.sections)
	if n == 0 {
		return
	}

	switch key {
	case KeyUp, KeyDown, KeyHome, KeyEnd:
		old := this.curIdx
		this.curIdx = accordionMoveCurrent(this.curIdx, n, key)
		if this.curIdx != old {
			this.Self().Update()
		}
	case KeyEnter, KeySpace:
		if this.curIdx < 0 {
			this.curIdx = 0 // 还没有当前项时, 回车/空格先落到首个区段
		}
		this.ToggleSection(this.curIdx)
	}
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
		// header background (hover 或 键盘当前项均高亮)
		g.Rectangle(0, cy, w, hh)
		if this.hoverIdx == i || this.curIdx == i {
			g.SetBrush1(t.FormLightColor)
		} else {
			g.SetBrush1(t.FormColor)
		}
		g.FillPreserve()
		g.SetPen1(t.BorderColor, 1)
		g.Stroke()

		// 键盘当前项: 左侧细强调条, 标示焦点所在 header
		if this.curIdx == i {
			g.Rectangle(0, cy, 3, hh)
			g.SetBrush1(t.HighLightColor)
			g.Fill()
		}

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
