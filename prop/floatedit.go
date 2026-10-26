package prop

import (
	"math"
	"reflect"
	"strconv"
	"strings"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

// FloatEdit keeps the complete float32/float64 value in its editable text.
// Only the unfocused paint is compact (7/10 significant digits); neither
// displaying, focusing nor leaving the field rounds the bound property.
// Invalid intermediate input is not committed. Enter/blur restores the real
// value; Up/Down step it by one, sharing the property's selection/undo path.
type FloatEdit struct {
	TextEdit
	compact string
}

func init() { core.RegisterFactory("prop.control.FloatEdit", core.TypeOf((*FloatEdit)(nil))) }

func NewFloatEdit() *FloatEdit { e := new(FloatEdit); e.Init(e); return e }

func (e *FloatEdit) Init(self gui.IWidget) {
	e.Edit.Init(self)
	core.Connect2(e.SigTextEdited, e, "OnEdited")
	core.Connect3(e, "Submit", e.OnSubmit)
}

func compactPropertyFloat(v float64, bits int) string {
	precision := 10
	if bits == 32 {
		precision = 7
	}
	return strconv.FormatFloat(v, 'g', precision, bits)
}

func (e *FloatEdit) updatePresentation() {
	e.compact = ""
	if e.item == nil {
		return
	}
	if e.item.IsMultiValue() {
		gui.SetToolTip(e.Self(), e.item.Label()+"\n"+e.item.MultiValueHint())
		return
	}
	value := reflect.ValueOf(e.item.GetValue())
	if value.Kind() != reflect.Float32 && value.Kind() != reflect.Float64 {
		return
	}
	exact := strconv.FormatFloat(value.Float(), 'g', -1, value.Type().Bits())
	e.compact = compactPropertyFloat(value.Float(), value.Type().Bits())
	gui.SetToolTip(e.Self(), e.item.Label()+"\n"+exact)
}

func (e *FloatEdit) UpdateValue() {
	e.finishEdit()
	e.updateValueText()
}

func (e *FloatEdit) updateValueText() {
	if e.item == nil {
		return
	}
	text := ""
	if !e.item.IsMultiValue() {
		v := reflect.ValueOf(e.item.GetValue())
		if v.Kind() == reflect.Float32 || v.Kind() == reflect.Float64 {
			text = strconv.FormatFloat(v.Float(), 'g', -1, v.Type().Bits())
		} else {
			text = e.item.GetValueStr()
		}
	}
	e.SetText(text)
	e.updatePresentation()
}

func (e *FloatEdit) OnEdited(text string) {
	if !e.canEditProperty() {
		return
	}
	typ := e.item.Type()
	if typ.Kind() != reflect.Float32 && typ.Kind() != reflect.Float64 {
		return
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(text), typ.Bits())
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return
	}
	value := reflect.New(typ).Elem()
	value.SetFloat(v) // preserve named float types
	if !e.item.IsMultiValue() && reflect.DeepEqual(value.Interface(), e.item.GetValue()) {
		return
	}
	e.setEditedValue(value.Interface())
	e.updatePresentation()
}

func (e *FloatEdit) OnSubmit(string) { e.Deactivate() }
func (e *FloatEdit) Deactivate() {
	if e.item != nil {
		e.OnEdited(e.Text())
		e.UpdateValue()
	}
}

func (e *FloatEdit) OnKeyDown(key int, repeat bool) {
	if key != gui.KeyUp && key != gui.KeyDown {
		e.Edit.OnKeyDown(key, repeat)
		return
	}
	if e.item == nil || e.item.IsReadOnly() || !e.IsEnabled() {
		return
	}
	if !repeat {
		e.finishEdit()
	}
	// Mixed rows step from the first selected value, just like IntEdit; a single
	// command captures all original values, so Undo restores each independently.
	value := reflect.ValueOf(e.item.prop.GetValue())
	if value.Kind() != reflect.Float32 && value.Kind() != reflect.Float64 {
		return
	}
	delta := 1.0
	if key == gui.KeyDown {
		delta = -1
	}
	e.OnEdited(strconv.FormatFloat(value.Float()+delta, 'g', -1, value.Type().Bits()))
	e.updateValueText()
}

func (e *FloatEdit) Draw(g paint.Painter) {
	if e.HasFocus() || e.item == nil || e.item.IsMultiValue() {
		e.TextEdit.Draw(g)
		return
	}
	t := gui.Theme()
	w, h := e.Size()
	g.Save()
	defer g.Restore()
	g.Rectangle(0, 0, w, h)
	if e.IsReadOnly() {
		g.SetBrush1(t.FormColor)
	} else {
		g.SetBrush1(t.ViewBGColor)
	}
	g.Fill()
	t.DrawEditFrame(g, 0, 0, w, h, false, e.IsHover(), e.IsReadOnly())
	drawPropertyText(g, t.Font, t.TextColor, t.EditPadding.L, 0, w-t.EditPadding.L-t.EditPadding.R, h, e.compact)
}
