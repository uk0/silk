package ged

import (
	"reflect"

	"github.com/uk0/silk/core"
)

// This file adds round-trip persistence for the *embedded* widget's editable
// scalar properties (text, values, tag bindings, device settings, ...) that a
// concrete widget exposes through core.IEnumProperties. FakeWidget.SaveDesign /
// LoadDesign already persist the FakeWidget's own geometry, name, code, event
// handlers and nested children; the widget-specific fields enumerated by the
// embedded widget (gui.Tank's level/min/max/tag, gui.Edit's text, ...) were
// dropped on save until now, so a configured design lost those values on reload.
//
// Scope (v1): only scalar-typed properties are captured — string, bool, the
// signed/unsigned integer kinds and float32/float64. Complex property types
// (paint.Color, structs, slices) are intentionally skipped; they need dedicated
// serializers and are left for a follow-up. Values are (de)serialized with the
// same core.PersistString / core.PersistSscan pair TDoc itself uses for node
// values, so the captured form matches the rest of the design document.

// captureWidgetProperties drives w.EnumProperties with a recording stand-in and
// returns a map of property id -> serialized scalar value. Read-only properties
// (nil setter) and non-scalar types are skipped, so every captured entry can be
// fed back through applyWidgetProperties.
func captureWidgetProperties(w core.IEnumProperties) map[string]string {
	c := &propCapture{vals: make(map[string]string)}
	w.EnumProperties(c)
	return c.vals
}

// applyWidgetProperties drives w.EnumProperties to obtain each property's setter
// and, for every id present in vals, parses the stored string into the setter's
// parameter type and calls it. Ids absent from vals, read-only properties and
// non-scalar setter types are left untouched.
func applyWidgetProperties(w core.IEnumProperties, vals map[string]string) {
	if len(vals) == 0 {
		return
	}
	w.EnumProperties(&propApply{vals: vals})
}

// propCapture implements core.IPropertyList, serializing each scalar property
// that also has a setter (a read-only value could never be restored).
type propCapture struct {
	vals map[string]string
}

func (c *propCapture) AddProperty(id string, get, set interface{}) {
	if get == nil || set == nil {
		return
	}
	gv := reflect.ValueOf(get)
	gt := gv.Type()
	if gt.Kind() != reflect.Func || gt.NumIn() != 0 || gt.NumOut() != 1 {
		return
	}
	if s, ok := scalarToString(gv.Call(nil)[0]); ok {
		c.vals[id] = s
	}
}

// propApply implements core.IPropertyList, calling each setter with the parsed
// stored value.
type propApply struct {
	vals map[string]string
}

func (a *propApply) AddProperty(id string, get, set interface{}) {
	if set == nil {
		return
	}
	s, ok := a.vals[id]
	if !ok {
		return
	}
	sv := reflect.ValueOf(set)
	st := sv.Type()
	if st.Kind() != reflect.Func || st.NumIn() != 1 || st.NumOut() != 0 {
		return
	}
	if val, ok := parseScalar(s, st.In(0)); ok {
		sv.Call([]reflect.Value{val})
	}
}

// scalarToString serializes a scalar reflect.Value with core.PersistString,
// normalizing named types to their base kind first. It reports false for any
// non-scalar kind (structs such as paint.Color, slices, ...), which the caller
// skips.
func scalarToString(v reflect.Value) (string, bool) {
	var raw interface{}
	switch v.Kind() {
	case reflect.String:
		raw = v.String()
	case reflect.Bool:
		raw = v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		raw = v.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		raw = v.Uint()
	case reflect.Float32, reflect.Float64:
		raw = v.Float()
	default:
		return "", false
	}
	s, err := core.PersistString(raw)
	if err != nil {
		return "", false
	}
	return s, true
}

// parseScalar parses s into a value of the scalar type pt using the
// core.PersistSscan counterpart of scalarToString, converting through the base
// kind so named types (e.g. a defined float64) are handled. It reports false
// for non-scalar target types.
func parseScalar(s string, pt reflect.Type) (reflect.Value, bool) {
	switch pt.Kind() {
	case reflect.String:
		var v string
		if _, err := core.PersistSscan(s, &v); err != nil {
			return reflect.Value{}, false
		}
		return reflect.ValueOf(v).Convert(pt), true
	case reflect.Bool:
		var v bool
		if _, err := core.PersistSscan(s, &v); err != nil {
			return reflect.Value{}, false
		}
		return reflect.ValueOf(v).Convert(pt), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		var v int64
		if _, err := core.PersistSscan(s, &v); err != nil {
			return reflect.Value{}, false
		}
		return reflect.ValueOf(v).Convert(pt), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		var v uint64
		if _, err := core.PersistSscan(s, &v); err != nil {
			return reflect.Value{}, false
		}
		return reflect.ValueOf(v).Convert(pt), true
	case reflect.Float32, reflect.Float64:
		var v float64
		if _, err := core.PersistSscan(s, &v); err != nil {
			return reflect.Value{}, false
		}
		return reflect.ValueOf(v).Convert(pt), true
	default:
		return reflect.Value{}, false
	}
}
