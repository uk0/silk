package main

import (
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/settings"
	"path/filepath"
	"testing"
)

func TestAppearancePersistsAndAppliesWithoutChangingBuffer(t *testing.T) {
	p := &preferences{store: settings.New(filepath.Join(t.TempDir(), "prefs.tdoc"))}
	a := p.Appearance()
	a.Locale = "zh-CN"
	a.EditorSize = 19
	a.TabSize = 8
	a.InsertSpaces = true
	a.Background = "#121826"
	a.Foreground = "#E0E8F8"
	if err := p.SetAppearance(a); err != nil {
		t.Fatal(err)
	}
	flushTestPreferences(t, p)
	q := &preferences{store: settings.New(p.store.Path())}
	if q.Appearance() != a {
		t.Fatalf("readback %#v", q.Appearance())
	}
	e := gui.NewCodeEditor()
	e.SetText("\t中文😀")
	e.MarkSaved()
	applyEditorAppearance(e, a)
	if e.EditorFont().Size() != 19 || e.TabSize() != 8 || !e.InsertSpaces() || e.Text() != "\t中文😀" || e.Modified() {
		t.Fatal("appearance altered content or did not apply")
	}
	t.Setenv("SILKIDE_LOCALE", "")
	if appearanceLocale(q) != "zh-CN" {
		t.Fatal("locale not restored")
	}
	t.Setenv("SILKIDE_LOCALE", "en")
	if appearanceLocale(q) != "en" {
		t.Fatal("environment override not honored")
	}
}
func TestAppearanceRejectsInvalidBeforeSaving(t *testing.T) {
	p := &preferences{store: settings.NewMemory()}
	a := p.Appearance()
	old := a
	for _, mutate := range []func(*ideAppearance){func(a *ideAppearance) { a.EditorSize = 0 }, func(a *ideAppearance) { a.Accent = "#xyzxyz" }, func(a *ideAppearance) { a.TabSize = 100 }, func(a *ideAppearance) { a.UIFont = "" }} {
		a = old
		mutate(&a)
		if p.SetAppearance(a) == nil {
			t.Fatal("invalid accepted")
		}
		if p.Appearance() != old {
			t.Fatal("invalid changed preferences")
		}
	}
}

func TestAppearanceIndentControlRoundTrip(t *testing.T) {
	a := defaultIDEAppearance()
	a.InsertSpaces = true
	a.TabSize = 2
	d, read := buildAppearanceDialog(nil, a)
	var boxes []*gui.ComboBox
	var walk func(gui.IWidget)
	walk = func(w gui.IWidget) {
		if b, ok := w.(*gui.ComboBox); ok {
			boxes = append(boxes, b)
		}
		for _, c := range w.Children() {
			walk(c)
		}
	}
	walk(d)
	if len(boxes) != 3 || boxes[2].ActiveIndex() != 1 {
		t.Fatal("missing indentation policy control", len(boxes))
	}
	got, err := read()
	if err != nil || got != a {
		t.Fatal(got, err)
	}
	boxes[2].SetActiveIndex(0)
	got, err = read()
	if err != nil || got.InsertSpaces || got.TabSize != 2 {
		t.Fatal(got, err)
	}
}
