package gui

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
)

// BreadcrumbItem 面包屑项
type BreadcrumbItem struct {
	Text string
	Data interface{}
}

// Breadcrumb 面包屑导航控件
type Breadcrumb struct {
	Widget
	items     []BreadcrumbItem
	separator string
	hoverIdx  int
	cbClick   func(int, BreadcrumbItem)
}

func init() {
	core.RegisterFactory("gui.Breadcrumb", core.TypeOf((*Breadcrumb)(nil)))
}

func NewBreadcrumb() *Breadcrumb {
	p := new(Breadcrumb)
	p.Init(p)
	p.separator = "/"
	p.hoverIdx = -1
	return p
}

func (this *Breadcrumb) Items() []BreadcrumbItem { return this.items }

func (this *Breadcrumb) SetItems(items []BreadcrumbItem) {
	this.items = items
	this.Self().Update()
}

func (this *Breadcrumb) AddItem(text string, data interface{}) {
	this.items = append(this.items, BreadcrumbItem{Text: text, Data: data})
	this.Self().Update()
}

func (this *Breadcrumb) Separator() string { return this.separator }

func (this *Breadcrumb) SetSeparator(s string) {
	this.separator = s
	this.Self().Update()
}

func (this *Breadcrumb) SigClick(fn func(int, BreadcrumbItem)) {
	this.cbClick = fn
}

// itemRanges calculates the x-range for each item for hit testing
func (this *Breadcrumb) itemRanges() []struct{ x1, x2 float64 } {
	t := Theme()
	f := t.Font
	pad := 4.0
	x := 0.0
	sepExt := f.TextExtents(this.separator)
	ranges := make([]struct{ x1, x2 float64 }, len(this.items))

	for i, item := range this.items {
		ext := f.TextExtents(item.Text)
		ranges[i].x1 = x
		ranges[i].x2 = x + ext.Width + pad*2
		x = ranges[i].x2
		if i < len(this.items)-1 {
			x += sepExt.Width + pad*2
		}
	}
	return ranges
}

// --- Events ---

func (this *Breadcrumb) OnMouseEnter() {
	this.Self().Update()
}

func (this *Breadcrumb) OnMouseLeave() {
	this.hoverIdx = -1
	this.Self().Update()
}

func (this *Breadcrumb) OnMouseMove(x, y float64) {
	ranges := this.itemRanges()
	old := this.hoverIdx
	this.hoverIdx = -1
	for i, r := range ranges {
		if x >= r.x1 && x < r.x2 && i < len(this.items)-1 {
			this.hoverIdx = i
			break
		}
	}
	if old != this.hoverIdx {
		this.Self().Update()
	}
}

func (this *Breadcrumb) OnLeftDown(x, y float64) {
	ranges := this.itemRanges()
	for i, r := range ranges {
		if x >= r.x1 && x < r.x2 && i < len(this.items)-1 {
			if this.cbClick != nil {
				this.cbClick(i, this.items[i])
			}
			break
		}
	}
}

// --- Drawing ---

func (this *Breadcrumb) Draw(g paint.Painter) {
	t := Theme()
	f := t.Font
	g.SetFont(f)
	fe := f.FontExtents()
	_, h := this.Size()
	pad := 4.0
	x := 0.0

	baseY := 0.5 * (h + fe.Ascent - fe.Descent)

	if len(this.items) == 0 {
		// Designer placeholder
		g.SetBrush1(paint.Color{100, 100, 100, 255})
		homeExt := f.TextExtents("Home")
		tx := pad - homeExt.XBearing
		g.Translate(tx, baseY)
		g.DrawText("Home")
		g.Translate(-tx, -baseY)
		x += homeExt.Width + pad*2

		sepExt := f.TextExtents(this.separator)
		g.SetBrush1(paint.Color{180, 180, 180, 255})
		sx := x + pad - sepExt.XBearing
		g.Translate(sx, baseY)
		g.DrawText(this.separator)
		g.Translate(-sx, -baseY)
		x += sepExt.Width + pad*2

		g.SetBrush1(t.TextColor)
		pageExt := f.TextExtents("Page")
		px := x + pad - pageExt.XBearing
		g.Translate(px, baseY)
		g.DrawText("Page")
		g.Translate(-px, -baseY)
		return
	}

	for i, item := range this.items {
		ext := f.TextExtents(item.Text)
		isLast := i == len(this.items)-1

		// item text
		if isLast {
			g.SetBrush1(t.TextColor)
		} else if this.hoverIdx == i {
			g.SetBrush1(paint.Color{66, 133, 244, 255})
		} else {
			g.SetBrush1(paint.Color{100, 100, 100, 255})
		}

		tx := x + pad - ext.XBearing
		g.Translate(tx, baseY)
		g.DrawText(item.Text)
		g.Translate(-tx, -baseY)

		x += ext.Width + pad*2

		// separator
		if !isLast {
			sepExt := f.TextExtents(this.separator)
			g.SetBrush1(paint.Color{180, 180, 180, 255})
			sx := x + pad - sepExt.XBearing
			g.Translate(sx, baseY)
			g.DrawText(this.separator)
			g.Translate(-sx, -baseY)
			x += sepExt.Width + pad*2
		}
	}
}

func (this *Breadcrumb) SizeHints() SizeHints {
	t := Theme()
	f := t.Font
	fe := f.FontExtents()
	pad := 4.0
	w := 0.0

	for i, item := range this.items {
		ext := f.TextExtents(item.Text)
		w += ext.Width + pad*2
		if i < len(this.items)-1 {
			sepExt := f.TextExtents(this.separator)
			w += sepExt.Width + pad*2
		}
	}
	return SizeHints{Width: w, Height: fe.Height + 8, Policy: GrowHorizontal | GrowVertical}
}

func (this *Breadcrumb) EnumProperties(list core.IPropertyList) {
	list.AddProperty("分隔符", this.Separator, this.SetSeparator)
}
