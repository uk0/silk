package ged

import (
	"reflect"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/prop"
)

// markDirtyPropertyList adapts a prop.IPropertyList to core.IPropertyList so a
// FakeWidget can forward its embedded widget's core.IEnumProperties entries into
// the property sheet. Beyond the plain forwarding the old inline adapter did, it
// wraps every settable property's setter so invoking it from the sheet also fires
// markDirty: the embedded widget's value changed, so the FakeWidget's cached
// offscreen pixmap must be rebuilt for the designer preview to update.
type markDirtyPropertyList struct {
	list      prop.IPropertyList
	markDirty func()
}

// newMarkDirtyPropertyList wraps list, tagging setter invocations with markDirty.
func newMarkDirtyPropertyList(list prop.IPropertyList, markDirty func()) *markDirtyPropertyList {
	return &markDirtyPropertyList{list: list, markDirty: markDirty}
}

// AddProperty forwards id and get unchanged and passes a setter wrapped to also
// call markDirty. The return values of the underlying prop.IPropertyList are
// discarded so this satisfies core.IPropertyList (no return values).
func (a *markDirtyPropertyList) AddProperty(id string, get, set interface{}) {
	a.list.AddProperty(id, get, wrapSetterMarkDirty(set, a.markDirty))
}

// wrapSetterMarkDirty returns a setter with the same func(T) signature as set
// that calls set and then markDirty. It returns set untouched when set is nil,
// markDirty is nil, or set is not shaped like a property setter (exactly one
// argument, no results, non-variadic) — the property sheet only accepts that
// shape, and preserving the original type keeps the sheet's get/set type-match
// check satisfied.
func wrapSetterMarkDirty(set interface{}, markDirty func()) interface{} {
	if set == nil || markDirty == nil {
		return set
	}
	v := reflect.ValueOf(set)
	t := v.Type()
	if t.Kind() != reflect.Func || t.NumIn() != 1 || t.NumOut() != 0 || t.IsVariadic() {
		return set
	}
	return reflect.MakeFunc(t, func(args []reflect.Value) []reflect.Value {
		v.Call(args)
		markDirty()
		return nil
	}).Interface()
}

// Compile-time guard: the adapter must satisfy core.IPropertyList so it can be
// handed to a widget's core.IEnumProperties.EnumProperties.
var _ core.IPropertyList = (*markDirtyPropertyList)(nil)
