package ged

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

// 变形为 (morph): retype a placed widget in situ.
//
// A FakeWidget's type IS its factory name — SetWidget derives factoryName from
// the instance via core.FactoryNameOf, and everything downstream (codegen's
// factoryMap, the palette, defaultSizeForWidget) keys off that string. So a
// morph is "build the new factory's widget, carry across what it can accept,
// swap it in".
//
// What survives the swap for free: geometry, widget name and handler source
// live on the FakeWidget itself (graph.Item bounds, plus name/code) and
// SetWidget touches none of them; nested children are graph children, likewise
// untouched. What does not survive for free is everything the *embedded* widget
// owns — the "props" block and the event bindings — because the new type
// registers a different set of both. Filtering those two is what this file does.

// containsString reports whether list holds s.
func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// morphCandidates returns the factory names 变形为 offers for factoryName: the
// other placeable widgets in the same palette category, in palette order.
// categoryDefs is the palette's own table, so the menu and the palette can
// never disagree about what a widget is a kind of. A factory no category lists
// (an opted-out or unregistered one) has no candidates.
func morphCandidates(factoryName string) []string {
	var out []string
	for _, cd := range categoryDefs {
		if !containsString(cd.names, factoryName) {
			continue
		}
		for _, name := range cd.names {
			if name == factoryName || !isPlaceable(name) {
				continue
			}
			out = append(out, name)
		}
		break
	}
	return out
}

// propertySetterTypes returns, for every property id w registers with a usable
// setter, the type that setter takes. Derived from the widget itself rather
// than a hand-kept table, so a widget that stops registering a property stops
// accepting it on morph with no list to update.
func propertySetterTypes(w core.IEnumProperties) map[string]reflect.Type {
	r := &propSetterTypes{types: make(map[string]reflect.Type)}
	w.EnumProperties(r)
	return r.types
}

// propSetterTypes implements core.IPropertyList, recording setter signatures.
type propSetterTypes struct {
	types map[string]reflect.Type
}

func (r *propSetterTypes) AddProperty(id string, get, set interface{}) {
	if set == nil {
		return
	}
	st := reflect.TypeOf(set)
	// Same shape propApply accepts — anything else could never be restored.
	if st.Kind() != reflect.Func || st.NumIn() != 1 || st.NumOut() != 0 {
		return
	}
	r.types[id] = st.In(0)
}

// morphSurvivingProps filters a captured "props" block for a morph target: an
// entry survives when both types register the id at the same setter type.
// Property ids are display names ("值", "文本"), so two widgets can share an id
// at different types, and the value must be dropped rather than carried over.
//
// The check compares the two setter signatures instead of asking whether the
// new setter can parse the stored value, because parsing is not a type test:
// core.PersistSscan into a *bool delegates to fmt.Fscan, which accepts any
// token and yields false. A string value fed to a bool setter would therefore
// "succeed" and silently clear the new widget's flag.
func morphSurvivingProps(oldVals map[string]string, oldSetters, newSetters map[string]reflect.Type) map[string]string {
	kept := make(map[string]string, len(oldVals))
	for id, val := range oldVals {
		ot, ok := oldSetters[id]
		if !ok {
			continue
		}
		if nt, ok := newSetters[id]; !ok || nt != ot {
			continue
		}
		kept[id] = val
	}
	return kept
}

// morphSurvivingEvents splits a FakeWidget's event bindings for a morph to
// newFactory: kept holds the handlers the new type can still be wired to,
// dropped counts the rest for the status message.
func morphSurvivingEvents(handlers map[string]string, newFactory string) (kept map[string]string, dropped int) {
	kept = make(map[string]string, len(handlers))
	for evt, handler := range handlers {
		if !codegenBindsEvent(newFactory, evt) {
			dropped++
			continue
		}
		kept[evt] = handler
	}
	return kept, dropped
}

// codegenBindsEvent reports whether codegen can wire evtName on factoryName.
// The (factory, event) table lives in emitEventBinding as a switch rather than
// a list, so support is decided by asking it to emit into a throwaway buffer.
// That keeps ONE source of truth for "which events does this widget have":
// a second enumerable table would drift from the generator, and a handler kept
// here but unknown there generates a widget with a dead binding.
func codegenBindsEvent(factoryName, evtName string) bool {
	var buf strings.Builder
	return emitEventBinding(&buf, map[string]bool{}, factoryName, "x", evtName, "h")
}

// morphCommand swaps a FakeWidget's embedded widget for another factory's, and
// swaps it back on Undo. Both sides are snapshots (factory name + props + event
// bindings), not the widget instances: rebuilding from the factory and
// re-applying the captured props is the same round-trip SaveDesign/LoadDesign
// performs, so Undo restores exactly what reopening the file would.
//
// Failure mode this pins: anything outside the scalar props block is re-seeded
// by setDefaultContent rather than carried across, so a morph and its undo both
// reset e.g. a ComboBox's item list to the factory default.
type morphCommand struct {
	item                   *FakeWidget
	oldFactory, newFactory string
	oldProps, newProps     map[string]string
	oldEvents, newEvents   map[string]string
}

func (cmd *morphCommand) Redo() {
	cmd.apply(cmd.newFactory, cmd.newProps, cmd.newEvents)
}

func (cmd *morphCommand) Undo() {
	cmd.apply(cmd.oldFactory, cmd.oldProps, cmd.oldEvents)
}

func (cmd *morphCommand) Text() string {
	return "Morph to " + widgetFriendlyName(cmd.newFactory)
}

// apply rebuilds the embedded widget from factory, restores props/events onto
// it and swaps it in. Geometry is re-applied through Layout because the widget
// is new and starts at the factory's own pixel size.
func (cmd *morphCommand) apply(factory string, props, events map[string]string) {
	fresh, err := NewFakeWidgetFromFactory(factory)
	if err != nil {
		return
	}
	w := fresh.Widget()
	if ep, ok := w.(core.IEnumProperties); ok {
		applyWidgetProperties(ep, props)
	}
	cmd.item.SetWidget(w)
	cmd.item.setEventHandlers(events)
	cmd.item.Layout()
}

// setEventHandlers replaces the whole binding map. Morph swaps the set
// wholesale, and it copies because EventHandlers() hands out the live map —
// a snapshot that aliased it would mutate with the widget.
func (this *FakeWidget) setEventHandlers(m map[string]string) {
	if len(m) == 0 {
		this.eventHandlers = nil
		return
	}
	next := make(map[string]string, len(m))
	for k, v := range m {
		next[k] = v
	}
	this.eventHandlers = next
}

// morphSelection retypes item to factoryName as one undoable step, then reports
// what could not be carried over. Rejects anything the 变形为 menu would not
// have offered (the same type, a non-placeable factory, a factory in another
// category) so a stray call never lands on the undo stack.
func (this *GedView) morphSelection(item *FakeWidget, factoryName string) {
	if item == nil || !containsString(morphCandidates(item.WidgetFactoryName()), factoryName) {
		return
	}

	oldProps := map[string]string{}
	oldSetters := map[string]reflect.Type{}
	if ep, ok := item.Widget().(core.IEnumProperties); ok {
		oldProps = captureWidgetProperties(ep)
		oldSetters = propertySetterTypes(ep)
	}
	oldEvents := map[string]string{}
	for k, v := range item.EventHandlers() {
		oldEvents[k] = v
	}

	probe, err := NewFakeWidgetFromFactory(factoryName)
	if err != nil {
		return
	}
	newProps := map[string]string{}
	if ep, ok := probe.Widget().(core.IEnumProperties); ok {
		newProps = morphSurvivingProps(oldProps, oldSetters, propertySetterTypes(ep))
	}
	newEvents, dropped := morphSurvivingEvents(oldEvents, factoryName)

	this.Scene().PushCommand(&morphCommand{
		item:       item,
		oldFactory: item.WidgetFactoryName(),
		newFactory: factoryName,
		oldProps:   oldProps,
		newProps:   newProps,
		oldEvents:  oldEvents,
		newEvents:  newEvents,
	})

	// Re-select so the property sheet rebuilds its rows for the new type; it
	// only enumerates on a selection change, so without this it keeps showing
	// the old widget's properties. Done before the status message because the
	// selection handler writes one of its own.
	this.Selection().Clear()
	this.Selection().Add(item)

	msg := "已变形为 " + widgetFriendlyName(factoryName)
	if dropped > 0 {
		msg += fmt.Sprintf("，丢弃 %d 个不适用事件", dropped)
	}
	this.showStatus(msg)
	this.Self().Update()
}

// showStatus posts a one-line message on the owning frame's status bar, and
// no-ops when the view has no frame (headless tests, preview windows).
func (this *GedView) showStatus(msg string) {
	frame := gui.FindOwnerFrame(this)
	if frame == nil {
		return
	}
	if sb := frame.StatusBar(); sb != nil {
		sb.ShowMessage(msg)
	}
}
