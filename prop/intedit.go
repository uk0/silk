package prop

import (
	"reflect"
	"strconv"
	"strings"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

// IntEdit keeps integer values exact (including int64/uint64), rather than
// converting them through float64. Invalid intermediate text is not submitted;
// Return/deactivation restores the current property value. Up/Down step by one.
type IntEdit struct{ TextEdit }

func init() {
	core.RegisterFactory("prop.control.IntEdit", core.TypeOf((*IntEdit)(nil)))
}

func (edit *IntEdit) Init(self gui.IWidget) {
	edit.Edit.Init(self)
	core.Connect2(edit.SigTextEdited, edit, "OnEdited")
	core.Connect3(edit, "Submit", edit.OnSubmit)
}

func (edit *IntEdit) validInteger(text string) bool {
	if edit.item == nil {
		return false
	}
	typ := edit.item.Type()
	text = strings.TrimSpace(text)
	switch typ.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		_, err := strconv.ParseInt(text, 10, typ.Bits())
		return err == nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		_, err := strconv.ParseUint(text, 10, typ.Bits())
		return err == nil
	}
	return false
}

func (edit *IntEdit) OnEdited(text string) {
	if edit.item != nil && !edit.item.IsReadOnly() && edit.validInteger(text) {
		value, err := parsePropertyValue(edit.item.Type(), strings.TrimSpace(text))
		if err != nil || !edit.item.IsMultiValue() && reflect.DeepEqual(value, edit.item.GetValue()) {
			return // No-op boundary steps/deactivation must not create undo entries.
		}
		edit.item.SetValue(value)
	}
}

func (edit *IntEdit) OnSubmit(string) { edit.Deactivate() }

func (edit *IntEdit) Deactivate() {
	if edit.item == nil {
		return
	}
	edit.OnEdited(edit.Text())
	edit.UpdateValue()
}

func (edit *IntEdit) OnKeyDown(key int, repeat bool) {
	if key != gui.KeyUp && key != gui.KeyDown {
		edit.Edit.OnKeyDown(key, repeat)
		return
	}
	if edit.item == nil || edit.item.IsReadOnly() || !edit.IsEnabled() {
		return
	}
	// Start from the real value, not a mixed-selection placeholder or an
	// invalid half-typed string. Check bounds before arithmetic to avoid wrap.
	value := reflect.ValueOf(edit.item.PropertyItem.prop.GetValue())
	if value.Kind() < reflect.Int || value.Kind() > reflect.Uintptr {
		return // A user-specified control mapping may bind a noninteger.
	}
	bits := value.Type().Bits()
	var next string
	if value.Kind() >= reflect.Int && value.Kind() <= reflect.Int64 {
		v := value.Int()
		max := int64(^uint64(0) >> 1)
		if bits < 64 {
			max = (int64(1) << (bits - 1)) - 1
		}
		if key == gui.KeyUp && v < max {
			v++
		} else if key == gui.KeyDown && v > -max-1 {
			v--
		}
		next = strconv.FormatInt(v, 10)
	} else {
		v := value.Uint()
		max := ^uint64(0)
		if bits < 64 {
			max = (uint64(1) << bits) - 1
		}
		if key == gui.KeyUp && v < max {
			v++
		} else if key == gui.KeyDown && v > 0 {
			v--
		}
		next = strconv.FormatUint(v, 10)
	}
	edit.OnEdited(next)
	edit.UpdateValue()
}
