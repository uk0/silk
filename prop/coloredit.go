package prop

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

func init() {
	core.RegisterFactory("prop.control.ColorEdit", core.TypeOf((*ColorEdit)(nil)))
}

// ColorEdit is the property-sheet editor for a paint.Color row: it shows the
// value and drops down the shared gui.ColorPicker to change it.
//
// Without it a color fell through to TextEdit, which could render the value
// (core.VisualString -> Color.String) but never commit one: parsePropertyValue
// goes through core.PersistSscan, and fmt has no scanner for a struct, so every
// edit died with "can't scan type: *paint.Color" and colors were read-only in
// the designer. The commit here bypasses the string round-trip entirely and
// hands the typed value to PropertyItem.SetValue.
//
// The color swatch beside the label is drawn by the sheet itself
// (PropertySheet.drawColorSwatch), so this control draws only the value text
// and the drop-down arrow — two swatches a splitter apart read as one broken
// control.
type ColorEdit struct {
	gui.Button
	item   *PropertyItem1
	picker *colorPopup

	// syncing suppresses the commit while UpdateValue pushes the property's
	// current value into the picker; gui.ColorPicker reports every change
	// through the same callback a user pick arrives on.
	syncing bool
}

func NewColorEdit() *ColorEdit {
	p := new(ColorEdit)
	p.Init(p)
	return p
}

func (this *ColorEdit) Init(self gui.IWidget) {
	this.Button.Init(self)
	// Materialise the Action: gui.Button.IsEnabled() is false while its action
	// is nil, and a disabled button never reaches emit(), which is what toggles
	// the drop-down.
	this.SetText("")

	pop := new(colorPopup)
	pop.Init(pop)
	// The palette lives in gui.NewColorPicker, which we cannot call here (the
	// popup has to be its own Self() to get the dismiss rule below). SetPalette
	// with no colors installs that same built-in default.
	pop.SetPalette(nil)
	pop.SigColorChanged(this.onPicked)
	this.picker = pop
	this.SetSubPopup(pop)
}

func (this *ColorEdit) BindProperty(item *PropertyItem1) {
	this.item = item
}

func (this *ColorEdit) UpdateValue() {
	c, _ := this.item.GetValue().(paint.Color)
	this.syncing = true
	this.picker.SetColor(c)
	this.syncing = false
	this.Self().Update()
}

func (this *ColorEdit) UpdateConfig() {
	// A read-only property must not look editable: PropertyItem.SetValue drops
	// the write, so an openable picker would show a value the object never took.
	// Mirrors CheckBox.UpdateConfig / TextEdit.SetReadOnly.
	this.SetEnabled(!this.item.IsReadOnly())
}

func (this *ColorEdit) Activate() {}

func (this *ColorEdit) Deactivate() {
	this.HideSubPopup()
}

// Color is the color currently displayed, which is the property's value for an
// agreeing selection and the zero color for a 多值 row.
func (this *ColorEdit) Color() paint.Color {
	return this.picker.Color()
}

// onPicked commits a pick to the whole selection and closes the drop-down.
func (this *ColorEdit) onPicked(c paint.Color) {
	if this.syncing || this.item == nil {
		return
	}
	this.item.SetValue(c)
	this.HideSubPopup()
	this.Self().Update()
}

func (this *ColorEdit) Draw(g paint.Painter) {
	t := gui.Theme()
	w, h := this.Size()

	g.Rectangle(0, 0, w, h)
	g.SetBrush1(t.ViewBGColor)
	g.Fill()
	t.DrawEditFrame(g, 0, 0, w, h, this.HasFocus(), this.IsHover(), false)

	// A disagreeing selection says so instead of showing one object's color;
	// the sheet's swatch is already blank for the same reason (GetValue hands
	// back the zero color).
	text := ""
	if this.item != nil {
		text = this.item.MultiValueHint()
	}
	if text == "" {
		text = this.picker.Color().String()
	}
	if t.Font != nil {
		fe := t.Font.FontExtents()
		g.SetFont(t.Font)
		g.SetBrush1(t.TextColor)
		g.DrawText1(t.EditPadding.L, (h+fe.Ascent-fe.Descent)/2, text)
	}

	// Drop-down arrow: without it the row is indistinguishable from the text
	// box a color used to get, and nothing says the picker is a click away.
	const arrowW = 8.0
	ax := w - arrowW - t.EditPadding.R
	ay := h*0.5 - 2
	if ax > 0 {
		g.SetBrush1(t.TextColor)
		g.MoveTo(ax, ay)
		g.LineTo(ax+arrowW, ay)
		g.LineTo(ax+arrowW*0.5, ay+4)
		g.LineTo(ax, ay)
		g.Fill()
	}
}

// colorPopup is the drop-down half of a ColorEdit: the shared gui.ColorPicker
// plus the one rule a popup needs and the picker does not have — a click
// outside it closes it. gui.ComboBox's list carries the identical rule; without
// it the picker keeps the mouse capture ShowSubPopup pushed and nothing can
// dismiss it.
type colorPopup struct {
	gui.ColorPicker
}

func (this *colorPopup) OnLeftDown(x, y float64) {
	w, h := this.Size()
	if !this.IsPopup() || (x >= 0 && y >= 0 && x < w && y < h) {
		this.ColorPicker.OnLeftDown(x, y)
		return
	}
	this.PopCapture()
	this.Hide()
}
