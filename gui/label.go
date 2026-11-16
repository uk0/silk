package gui

import (
	"strings"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
)

type TextAlign int

const (
	AlignLeft TextAlign = iota
	AlignCenter
	AlignRight
)

func init() {
	core.RegisterFactory("gui.Label", core.TypeOf((*Label)(nil)))
}

// Label is a non-editable text display widget.
type Label struct {
	Widget
	text      string
	textColor paint.Color
	font      paint.Font
	align     TextAlign
	wrap      bool

	// Draw and SizeHints share the width-dependent layout. Ordinary repaints
	// never reshape text; a mutable font's attributes also participate in the key.
	labelLayoutCache
	// Natural + allocated width are the common two-pass layout pair. Retain
	// two entries, not an unbounded width map, so parent measurement does not
	// force two complete reshapes on every unchanged Layout call.
	alternateLayout labelLayoutCache
}

type labelLayoutCache struct {
	cachedHints  SizeHints
	hintText     string
	hintFont     paint.Font
	hintThemeRev uint64
	hintsValid   bool
	hintWidth    float64
	hintWrap     bool
	hintFamily   string
	hintSize     int
	hintBold     bool
	hintItalic   bool
	lines        []labelLine
	lineHeight   float64
	lineAscent   float64
}

type labelLine struct {
	text string
	ext  paint.TextExtents
}

func NewLabel(text string) *Label {
	p := new(Label)
	p.Init(p)
	p.text = text
	return p
}

func (this *Label) Text() string {
	return this.text
}

func (this *Label) SetText(s string) {
	if this.text == s {
		return
	}
	this.text = s
	this.hintsValid = false
	this.Self().Update()
}

func (this *Label) SetTextColor(c paint.Color) {
	this.textColor = c
	this.Self().Update()
}

func (this *Label) SetFont(f paint.Font) {
	this.font = f
	this.hintsValid = false
	this.Self().Update()
}

func (this *Label) SetAlign(a TextAlign) {
	if this.align == a {
		return
	}
	this.align = a
	this.Self().Update()
}

func (this *Label) SetWrap(b bool) {
	if this.wrap == b {
		return
	}
	this.wrap = b
	this.hintsValid = false
	this.Self().Update()
}

func (this *Label) effectiveFont() paint.Font {
	if this.font != nil {
		return this.font
	}
	return Theme().Font
}

func (this *Label) effectiveTextColor() paint.Color {
	if this.textColor.A != 0 {
		return this.textColor
	}
	return Theme().TextColor
}

func (this *Label) Draw(g paint.Painter) {
	if this.text == "" {
		return
	}

	w, h := this.Self().Size()
	if w <= 0 || h <= 0 {
		return
	}
	this.SizeHints()
	f := this.effectiveFont()
	g.Save()
	defer g.Restore()
	g.Rectangle(0, 0, w, h)
	g.Clip()
	g.SetFont(f)
	g.SetBrush1(this.effectiveTextColor())
	// Center a fitting block, but keep its first line visible when constrained.
	top := max(0, (h-this.cachedHints.Height)*0.5)
	for i, line := range this.lines {
		if top+float64(i)*this.lineHeight >= h {
			break // Paint only visible rows, even for a large label.
		}
		x := -line.ext.XBearing
		switch this.align {
		case AlignCenter:
			x += (w - line.ext.Width) * 0.5
		case AlignRight:
			x += w - line.ext.Width
		}
		y := top + float64(i)*this.lineHeight + this.lineAscent
		if len(this.lines) == 1 && this.cachedHints.Height <= h {
			y = (h - line.ext.YBearing) * 0.5 // Preserve single-line optical centering.
			// The optical formula can put a taller fallback emoji's descender
			// below a perfectly sufficient measured label. Keep the old baseline
			// when it fits; otherwise contain the complete actual ink rectangle.
			low := -line.ext.YBearing
			high := h - line.ext.YBearing - line.ext.Height
			if high >= low {
				y = max(low, min(high, y))
			}
		}
		if line.text != "" {
			g.Translate(x, y)
			g.DrawText(line.text)
			g.Translate(-x, -y)
		}
	}
}

func (this *Label) SizeHints() SizeHints {
	w := 0.0
	if this.wrap {
		w, _ = this.Self().Size()
	}
	return this.measureLabelWidth(w)
}

// HeightForWidth shares Draw's cached layout without resizing the label.
func (this *Label) HeightForWidth(width float64) float64 {
	return this.SizeHintsForWidth(width).Height
}

func (this *Label) SizeHintsForWidth(width float64) SizeHints {
	if hint, ok := inheritedNaturalHints(this, width); ok {
		return hint
	}
	return preserveInheritedHints(this, this.measureLabelWidth(width))
}

func (this *Label) measureLabelWidth(width float64) SizeHints {
	f := this.effectiveFont()
	w := 0.0
	if this.wrap {
		w = layoutDimension(width)
	}
	if this.labelLayoutCache.matches(this.text, f, this.wrap, w) {
		return this.cachedHints
	}
	if this.alternateLayout.matches(this.text, f, this.wrap, w) {
		this.labelLayoutCache, this.alternateLayout = this.alternateLayout, this.labelLayoutCache
		return this.cachedHints
	}
	this.alternateLayout = this.labelLayoutCache

	fe := f.FontExtents()
	this.lines = nil
	appendLine := func(text string) {
		this.lines = append(this.lines, labelLine{text: text, ext: *f.TextExtents(text)})
	}
	if !this.wrap && !strings.ContainsAny(this.text, "\r\n") {
		appendLine(this.text)
	} else {
		// Reuse the SDK's linear glyph-based breaker: shape each paragraph once,
		// never repeatedly measure the remaining suffix of a long string.
		var block TextBlock
		block.SetFont(f)
		block.SetText(this.text)
		block.SetMultiLine(true)
		block.SetWrap(this.wrap)
		block.Layout(w)
		for _, row := range block.rows {
			appendLine(string(row.text))
		}
	}
	ascent, descent := max(0, fe.Ascent), max(0, fe.Descent)
	for _, line := range this.lines {
		ascent = max(ascent, -line.ext.YBearing)
		descent = max(descent, line.ext.YBearing+line.ext.Height)
	}
	this.lineHeight = max(1, fe.Height, ascent+descent)
	this.lineAscent = ascent
	hints := SizeHints{Height: this.lineHeight * float64(max(1, len(this.lines))), Policy: GrowHorizontal | GrowVertical}
	if this.wrap {
		hints.Policy |= ShrinkHorizontal
	}
	for _, line := range this.lines {
		// Wrapping uses advances, not ink bounds. Reporting only ink width can
		// move a final punctuation mark or trailing space onto a clipped row
		// even when a parent assigns precisely the requested natural width.
		hints.Width = max(hints.Width, line.ext.Width, line.ext.XAdvance)
	}

	this.hintText = this.text
	this.hintFont = f
	this.hintThemeRev = themeRev
	this.hintWidth, this.hintWrap = w, this.wrap
	this.hintFamily, this.hintSize = f.Family(), f.Size()
	this.hintBold, this.hintItalic = f.Bold(), f.Italic()
	this.cachedHints = hints
	this.hintsValid = true
	return hints
}

func (cache *labelLayoutCache) matches(text string, font paint.Font, wrap bool, width float64) bool {
	return cache.hintsValid && cache.hintText == text && cache.hintFont == font &&
		cache.hintThemeRev == themeRev && cache.hintWidth == width && cache.hintWrap == wrap &&
		cache.hintFamily == font.Family() && cache.hintSize == font.Size() &&
		cache.hintBold == font.Bold() && cache.hintItalic == font.Italic()
}

func (this *Label) Align() TextAlign {
	return this.align
}

func (this *Label) Wrap() bool {
	return this.wrap
}

func (this *Label) EnumProperties(list core.IPropertyList) {
	list.AddProperty("文本", this.Text, this.SetText)
	list.AddProperty("对齐", this.Align, this.SetAlign)
	list.AddProperty("换行", this.Wrap, this.SetWrap)
}
