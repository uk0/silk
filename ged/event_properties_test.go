package ged

import (
	"sort"
	"strings"
	"testing"

	"github.com/uk0/silk/prop"
)

// propRecorder records what a FakeWidget hands to the property sheet, so the
// tests below can assert on the rows without building a real sheet (which
// needs a window).
type propRecorder struct {
	ids  []string
	gets map[string]interface{}
	sets map[string]interface{}
}

func newPropRecorder() *propRecorder {
	return &propRecorder{gets: map[string]interface{}{}, sets: map[string]interface{}{}}
}

func (r *propRecorder) AddProperty(id string, get, set interface{}) (*prop.PropertyItem, bool) {
	r.ids = append(r.ids, id)
	r.gets[id] = get
	r.sets[id] = set
	return nil, true
}

// eventRows returns the recorded row ids that name an event codegen knows for
// any widget type, sorted — the rows the sheet's 事件 category is made of.
func (r *propRecorder) eventRows(universe map[string]bool) []string {
	out := []string{}
	for _, id := range r.ids {
		if universe[id] {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// allKnownEventNames is the union of every event name codegen mentions, over
// every widget type. It is the probe alphabet for the agreement tests: a name
// in it must be bindable exactly where it is offered, and nowhere else.
func allKnownEventNames() map[string]bool {
	names := map[string]bool{}
	for factory := range factoryMap {
		for _, e := range AvailableEvents(factory) {
			names[e] = true
		}
		if e := defaultEventForFactory(factory); e != "" {
			names[e] = true
		}
	}
	return names
}

// TestFakeWidgetEnumeratesEventRows: the property sheet gets one editable row
// per event codegen can emit for the selected widget, whose value is the
// handler function name. Blank clears the binding, a non-identifier is
// rejected — a handler name that is not a Go identifier would generate code
// that cannot compile.
func TestFakeWidgetEnumeratesEventRows(t *testing.T) {
	btn, err := NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatal(err)
	}

	rec := newPropRecorder()
	btn.EnumProperties(rec)

	get, ok := rec.gets["OnClick"].(func() string)
	if !ok {
		t.Fatalf("no OnClick event row for gui.Button; rows = %v", rec.ids)
	}
	set, ok := rec.sets["OnClick"].(func(string))
	if !ok {
		t.Fatalf("OnClick row is not editable (setter %T)", rec.sets["OnClick"])
	}

	if v := get(); v != "" {
		t.Errorf("unbound OnClick reads %q, want empty", v)
	}

	set("onOkClicked")
	if v := btn.EventHandlers()["OnClick"]; v != "onOkClicked" {
		t.Errorf("EventHandlers()[OnClick] = %q, want %q", v, "onOkClicked")
	}
	if v := get(); v != "onOkClicked" {
		t.Errorf("OnClick row reads %q after bind, want %q", v, "onOkClicked")
	}

	// Not a Go identifier: keep the previous binding rather than persist a
	// handler name codegen would turn into a syntax error.
	set("on click()")
	if v := btn.EventHandlers()["OnClick"]; v != "onOkClicked" {
		t.Errorf("invalid handler name accepted: EventHandlers()[OnClick] = %q", v)
	}

	set("")
	if v, bound := btn.EventHandlers()["OnClick"]; bound {
		t.Errorf("clearing the row left OnClick bound to %q", v)
	}
}

// TestSetEventHandlerCheckedValidates pins the handler-name rule both entry
// points (sheet row and 绑定事件 dialog) share: a Go identifier binds, blank
// unbinds, anything else — spaces, punctuation, a leading digit, a keyword —
// leaves the binding alone.
func TestSetEventHandlerCheckedValidates(t *testing.T) {
	fake, err := NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatal(err)
	}

	valid := []string{"onClick", "OnClick", "_handler", "h2", "处理点击"}
	for _, name := range valid {
		fake.SetEventHandlerChecked("OnClick", name)
		if got := fake.EventHandlers()["OnClick"]; got != name {
			t.Errorf("SetEventHandlerChecked(%q): bound %q", name, got)
		}
	}

	invalid := []string{"on click", "on-click", "2click", "on.Click", "func", "on()"}
	for _, name := range invalid {
		fake.SetEventHandler("OnClick", "keepMe")
		fake.SetEventHandlerChecked("OnClick", name)
		if got := fake.EventHandlers()["OnClick"]; got != "keepMe" {
			t.Errorf("SetEventHandlerChecked(%q) accepted it: bound %q", name, got)
		}
	}

	// Blank (and whitespace-only) unbinds rather than storing an empty name.
	for _, blank := range []string{"", "   "} {
		fake.SetEventHandler("OnClick", "keepMe")
		fake.SetEventHandlerChecked("OnClick", blank)
		if v, bound := fake.EventHandlers()["OnClick"]; bound {
			t.Errorf("SetEventHandlerChecked(%q) left OnClick bound to %q", blank, v)
		}
	}
}

// TestEventUIMatchesCodegen checks both directions of the UI/codegen contract
// for every widget type: the sheet rows and the 绑定事件 menu offer exactly the
// events emitEventBinding can emit. An event the UI accepts but codegen drops
// is a silent lie to the user (the menu even ticks it as bound); an event
// codegen binds but no UI offers is unreachable. The two lists lived apart
// before and had drifted — the menu offered gui.Edit.OnSubmit and
// gui.ComboBox.OnSelected, which codegen cannot emit.
func TestEventUIMatchesCodegen(t *testing.T) {
	universe := allKnownEventNames()
	if len(universe) == 0 {
		t.Fatal("no event names known to codegen")
	}
	probe := make([]string, 0, len(universe))
	for name := range universe {
		probe = append(probe, name)
	}
	sort.Strings(probe)

	for factory := range factoryMap {
		factory := factory
		t.Run(factory, func(t *testing.T) {
			avail := AvailableEvents(factory)
			want := append([]string{}, avail...)
			sort.Strings(want)

			// codegen side: emitEventBinding accepts a pair iff the UI offers it.
			for _, name := range probe {
				var buf strings.Builder
				emitted := emitEventBinding(&buf, map[string]bool{}, factory, "W", name, "onX")
				if offered := contains(avail, name); emitted != offered {
					t.Errorf("%s.%s: emitEventBinding=%v but UI offers=%v", factory, name, emitted, offered)
				}
			}

			fake, err := NewFakeWidgetFromFactory(factory)
			if err != nil {
				t.Skipf("cannot build %s: %v", factory, err)
			}

			// sheet side: the event rows of a selected widget are exactly avail.
			rec := newPropRecorder()
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Skipf("cannot enumerate %s: %v", factory, r)
					}
				}()
				fake.EnumProperties(rec)
			}()
			if got := rec.eventRows(universe); !equalStrings(got, want) {
				t.Errorf("%s sheet event rows = %v, want %v", factory, got, want)
			}

			// menu side: the 绑定事件 entries are the same set.
			got := append([]string{}, eventsForItem(fake)...)
			sort.Strings(got)
			if !equalStrings(got, want) {
				t.Errorf("%s menu events = %v, want %v", factory, got, want)
			}
		})
	}
}

// TestEventsForItemIgnoresNonWidgets: a scene item that is not a FakeWidget
// generates no code, so no event of it can be bound.
func TestEventsForItemIgnoresNonWidgets(t *testing.T) {
	if got := eventsForItem(NewGedScene()); len(got) != 0 {
		t.Errorf("eventsForItem(scene) = %v, want none", got)
	}
}

// TestDefaultEventIsBindable: the auto-default event picked for a widget whose
// designer wrote a bare func must itself be one codegen can bind. That table
// is separate from widgetEvents and can drift from it.
func TestDefaultEventIsBindable(t *testing.T) {
	for factory := range factoryMap {
		evt := defaultEventForFactory(factory)
		if evt == "" {
			continue
		}
		if !contains(AvailableEvents(factory), evt) {
			t.Errorf("defaultEventForFactory(%q) = %q, which codegen cannot bind (available: %v)",
				factory, evt, AvailableEvents(factory))
		}
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
