package gui

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
	"math"
)

// RadioGroup manages mutual exclusion among RadioButtons
type RadioGroup struct {
	buttons  []*RadioButton
	selected int
}

func NewRadioGroup() *RadioGroup {
	return &RadioGroup{selected: -1}
}

func (g *RadioGroup) add(rb *RadioButton) {
	g.buttons = append(g.buttons, rb)
}

func (g *RadioGroup) selectButton(rb *RadioButton) {
	for i, b := range g.buttons {
		if b == rb {
			g.selected = i
			if !b.checked {
				b.checked = true
				b.fireChanged(true)
				b.Self().Update()
			}
		} else if b.checked {
			b.checked = false
			b.fireChanged(false)
			b.Self().Update()
		}
	}
}

// SelectedIndex returns the index of the currently selected button, or -1
func (g *RadioGroup) SelectedIndex() int {
	return g.selected
}

// nextEnabledRadio returns the index of the next enabled button starting from
// index `from` and stepping by `dir` (+1 forward, -1 backward), wrapping at the
// ends of the slice and skipping disabled buttons. It returns -1 when no other
// enabled button exists. Pure helper kept free of widgets so arrow navigation
// can be unit-tested over a plain slice.
func nextEnabledRadio(buttons []*RadioButton, from, dir int) int {
	n := len(buttons)
	if n == 0 {
		return -1
	}
	for i := 1; i <= n; i++ {
		idx := ((from+dir*i)%n + n) % n
		if buttons[idx].IsEnabled() {
			return idx
		}
	}
	return -1
}

// navigate moves selection one step (dir +1/-1) from rb to the next enabled
// sibling in the group, wrapping at the ends. The newly selected radio also
// takes keyboard focus so subsequent arrow keys continue from there (Qt moves
// selection and focus together among radios in the same group).
func (g *RadioGroup) navigate(rb *RadioButton, dir int) {
	from := -1
	for i, b := range g.buttons {
		if b == rb {
			from = i
			break
		}
	}
	idx := nextEnabledRadio(g.buttons, from, dir)
	if idx < 0 {
		return
	}
	target := g.buttons[idx]
	g.selectButton(target)
	target.SetFocus()
}

// RadioButton is a mutually-exclusive toggle within a RadioGroup
type RadioButton struct {
	Widget
	text      string
	checked   bool
	group     *RadioGroup
	cbChanged func(interface{}, bool)
}

func init() {
	core.RegisterFactory("gui.RadioButton", core.TypeOf((*RadioButton)(nil)))
}

func NewRadioButton(text string, group *RadioGroup) *RadioButton {
	p := new(RadioButton)
	p.Init(p)
	p.text = text
	p.group = group
	if group != nil {
		group.add(p)
	}
	return p
}

func (this *RadioButton) IsChecked() bool {
	return this.checked
}

func (this *RadioButton) SetChecked(b bool) {
	if b == this.checked {
		return
	}
	if b && this.group != nil {
		this.group.selectButton(this)
	} else if !b {
		this.checked = false
		this.fireChanged(false)
		this.Self().Update()
	}
}

func (this *RadioButton) Text() string {
	return this.text
}

func (this *RadioButton) SetText(s string) {
	this.text = s
	this.Self().Update()
}

func (this *RadioButton) SetChangedCallback(cb func(interface{}, bool)) {
	this.cbChanged = cb
}

func (this *RadioButton) fireChanged(checked bool) {
	if this.cbChanged != nil {
		this.cbChanged(this.Self(), checked)
	}
}

func (this *RadioButton) EnumProperties(list core.IPropertyList) {
	list.AddProperty("文本", this.Text, this.SetText)
	list.AddProperty("选中", this.IsChecked, this.SetChecked)
}

// --- Events ---

func (this *RadioButton) OnMouseEnter() {
	this.Self().Update()
}

func (this *RadioButton) OnMouseLeave() {
	this.Self().Update()
}

func (this *RadioButton) OnLeftDown(x, y float64) {
	if !this.IsEnabled() {
		return
	}
	this.SetFocus()
	if !this.checked {
		if this.group != nil {
			this.group.selectButton(this)
		} else {
			this.checked = true
			this.fireChanged(true)
			this.Self().Update()
		}
	}
}

// OnKeyDown implements Qt QRadioButton keyboard behaviour. Space selects this
// radio (firing the exclusive-select path so siblings deselect and callbacks
// fire). Within a group the arrow keys move the selection AND focus to the
// previous (Up/Left) or next (Down/Right) enabled radio, wrapping at the ends.
func (this *RadioButton) OnKeyDown(key int, repeat bool) {
	if !this.IsEnabled() {
		return
	}
	switch key {
	case KeySpace:
		if !this.checked {
			if this.group != nil {
				this.group.selectButton(this)
			} else {
				this.checked = true
				this.fireChanged(true)
				this.Self().Update()
			}
		}
	case KeyUp, KeyLeft:
		if this.group != nil {
			this.group.navigate(this, -1)
		}
	case KeyDown, KeyRight:
		if this.group != nil {
			this.group.navigate(this, +1)
		}
	}
}

// --- Drawing ---

func (this *RadioButton) Draw(g paint.Painter) {
	t := Theme()
	_, h := this.Size()
	m := t.ButtonMargin

	radius := t.CheckBoxSize * 0.5
	cx := m.L + radius
	cy := h * 0.5

	// subtle focus ring just outside the outer circle when focused, matching
	// the highlight-colour focus convention used by DrawEditFrame.
	if this.HasFocus() {
		g.Arc(cx, cy, radius+2, 0, 2*math.Pi)
		g.SetPen1(t.HighLightColor, 1)
		g.Stroke()
	}

	// outer circle
	g.Arc(cx, cy, radius, 0, 2*math.Pi)
	g.SetPen1(t.BorderColor, 1)
	g.Stroke()

	// filled inner circle when checked
	if this.checked {
		innerRadius := radius * 0.55
		g.Arc(cx, cy, innerRadius, 0, 2*math.Pi)
		g.SetBrush1(t.HighLightColor)
		g.Fill()
	}

	// text label
	if this.text != "" {
		if this.IsEnabled() {
			g.SetBrush1(t.TextColor)
		} else {
			g.SetBrush1(t.MenuGrayTextColor)
		}
		g.SetFont(t.Font)
		ext := g.Font().TextExtents(this.text)
		xt := m.L + t.CheckBoxSize + m.L - ext.XBearing
		yt := 0.5*(h+ext.YBearing) - ext.YBearing
		g.Translate(xt, yt)
		g.DrawText(this.text)
		g.Translate(-xt, -yt)
	}
}

// --- SizeHints ---

func (this *RadioButton) SizeHints() SizeHints {
	t := Theme()
	m := t.ButtonMargin

	if this.text != "" {
		fe := t.Font.FontExtents()
		ext := t.Font.TextExtents(this.text)
		h := math.Max(t.CheckBoxSize, fe.Height)
		w := ext.Width + t.CheckBoxSize + m.L
		w += m.L + m.R
		h += m.T + m.B
		return SizeHints{Width: w, Height: h, Policy: GrowHorizontal | GrowVertical}
	}

	w := m.L + t.CheckBoxSize + m.R
	h := m.T + t.CheckBoxSize + m.B
	return SizeHints{Width: w, Height: h, Policy: GrowHorizontal | GrowVertical}
}
