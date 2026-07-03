package gui

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
)

// Placeholder is an empty state widget showing an icon, title, and subtitle.
type Placeholder struct {
	Widget
	title    string
	subtitle string
	icon     paint.Icon
}

func init() {
	core.RegisterFactory("gui.Placeholder", core.TypeOf((*Placeholder)(nil)))
}

func NewPlaceholder(title string) *Placeholder {
	p := new(Placeholder)
	p.Init(p)
	p.title = title
	return p
}

func (this *Placeholder) Title() string    { return this.title }
func (this *Placeholder) Subtitle() string { return this.subtitle }
func (this *Placeholder) Icon() paint.Icon { return this.icon }

func (this *Placeholder) SetTitle(s string) {
	this.title = s
	this.Self().Update()
}

func (this *Placeholder) SetSubtitle(s string) {
	this.subtitle = s
	this.Self().Update()
}

func (this *Placeholder) SetIcon(ico paint.Icon) {
	this.icon = ico
	this.Self().Update()
}

// --- Drawing ---

func (this *Placeholder) Draw(g paint.Painter) {
	t := Theme()
	w, h := this.Size()

	iconSize := 48.0
	gap := 8.0
	totalH := 0.0

	// calculate total height for centering
	if this.icon != nil {
		totalH += iconSize + gap
	}

	f := t.Font
	boldFont := paint.NewFont(f.Family(), f.Size(), true, false)

	if this.title != "" {
		fe := boldFont.FontExtents()
		totalH += fe.Height + gap
	}
	if this.subtitle != "" {
		fe := f.FontExtents()
		totalH += fe.Height
		_ = fe
	}

	cy := (h - totalH) / 2
	if cy < 0 {
		cy = 0
	}

	g.Save()

	// draw icon
	if this.icon != nil {
		ix := (w - iconSize) / 2
		g.DrawIcon1(this.icon, ix, cy, iconSize, false)
		cy += iconSize + gap
	}

	// draw title (bold)
	if this.title != "" {
		g.SetFont(boldFont)
		ext := boldFont.TextExtents(this.title)
		fe := boldFont.FontExtents()
		tx := (w-ext.Width)/2 - ext.XBearing
		ty := cy + fe.Ascent
		g.SetBrush1(paint.Color{60, 60, 60, 255})
		g.Translate(tx, ty)
		g.DrawText(this.title)
		g.Translate(-tx, -ty)
		cy += fe.Height + gap
	}

	// draw subtitle
	if this.subtitle != "" {
		g.SetFont(f)
		ext := f.TextExtents(this.subtitle)
		fe := f.FontExtents()
		tx := (w-ext.Width)/2 - ext.XBearing
		ty := cy + fe.Ascent
		g.SetBrush1(paint.Color{160, 160, 160, 255})
		g.Translate(tx, ty)
		g.DrawText(this.subtitle)
		g.Translate(-tx, -ty)
	}

	g.Restore()
}

func (this *Placeholder) SizeHints() SizeHints {
	return SizeHints{Width: 200, Height: 150, Policy: GrowHorizontal | GrowVertical}
}

func (this *Placeholder) EnumProperties(list core.IPropertyList) {
	list.AddProperty("标题", this.Title, this.SetTitle)
	list.AddProperty("副标题", this.Subtitle, this.SetSubtitle)
}
