package ged

import (
	"strings"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

// pluginTestMeter stands in for a widget declared outside package ged: the
// designer never sees the type, only the core factory and whatever
// RegisterWidget wrote into the four tables — exactly what a real plugin gives
// it.
type pluginTestMeter struct {
	gui.Widget
}

// The factory is registered once for the whole test binary: core.RegisterFactory
// panics on a duplicate name (core/factory.go:124), so it cannot live inside a
// test that may run twice. The ged tables are deliberately NOT written here —
// every test below writes them through RegisterWidget and undoes it in
// t.Cleanup, so the package's registry drift tests still see the built-in
// tables whatever order the tests run in.
func init() {
	core.RegisterFactory(pluginTestFactory, core.TypeOf((*pluginTestMeter)(nil)))
}

const (
	pluginTestFactory  = "plugintest.Meter"
	pluginTestCategory = "测试 (Plugin)"
)

// pluginTestDef is the def under test: a widget with its own category, its own
// import, one event and that event as its auto-default.
func pluginTestDef() WidgetDef {
	return WidgetDef{
		Factory:     pluginTestFactory,
		Category:    pluginTestCategory,
		GoType:      "*plugintest.Meter",
		ImportPath:  "example.com/plugintest",
		Constructor: "plugintest.NewMeter()",
		Events: []WidgetEvent{
			{Name: "OnReading", Code: "\tui.%s.SigReading(%s)\n", Params: "v float64"},
		},
		DefaultEvent: "OnReading",
	}
}

// registerPluginTestWidget registers d and undoes all four writes when the test
// ends. categoryDefs is restored from a copy of the slice rather than edited
// back: that covers both shapes RegisterWidget can leave it in (a new category
// appended, or a name appended to an existing one) without the cleanup
// re-implementing the write it is undoing.
func registerPluginTestWidget(t *testing.T, d WidgetDef) {
	t.Helper()
	cats := append([]widgetCategoryDef(nil), categoryDefs...)
	if err := RegisterWidget(d); err != nil {
		t.Fatalf("RegisterWidget(%+v): %v", d, err)
	}
	t.Cleanup(func() {
		delete(factoryMap, d.Factory)
		delete(widgetEvents, d.Factory)
		delete(pluginDefaultEvents, d.Factory)
		categoryDefs = cats
		if isPlaceable(d.Factory) {
			t.Errorf("%s is still placeable after cleanup; the rest of the package's "+
				"registry tests now depend on test order", d.Factory)
		}
	})
}

// TestRegisterWidgetReachesPalette: the palette is what a plugin widget exists
// for. The row is found by its Data payload — the factory name the drag handler
// hands the canvas — because a row whose label is right and whose payload is
// missing drags nothing, and by the category header above it, because writing
// factoryMap and skipping categoryDefs leaves a widget codegen can emit that no
// user can place.
func TestRegisterWidgetReachesPalette(t *testing.T) {
	registerPluginTestWidget(t, pluginTestDef())

	list := NewWidgetList()
	header := ""
	found := false
	for i := 0; i < list.Count() && !found; i++ {
		item := list.Item(i)
		if isHeader, _ := list.isCategoryHeader(i); isHeader {
			// The row carries the collapse indicator; the category name is the rest.
			header = strings.TrimSpace(strings.TrimPrefix(item.Text, "▼"))
			continue
		}
		if name, _ := item.Data.(string); name == pluginTestFactory {
			found = true
			if item.Text != "Meter" {
				t.Errorf("palette row for %s is labelled %q, want %q",
					pluginTestFactory, item.Text, "Meter")
			}
		}
	}
	if !found {
		t.Fatalf("no palette row carries %q as its drag payload; rows: %s",
			pluginTestFactory, pluginTestPaletteDump(list))
	}
	if header != pluginTestCategory {
		t.Errorf("palette row for %s sits under category header %q, want %q",
			pluginTestFactory, header, pluginTestCategory)
	}
}

// TestRegisterWidgetJoinsExistingCategory covers the branch every other test
// here misses: a plugin asking for a category the palette already has joins it
// instead of appending a second one under the same name. The other tests all
// register 测试 (Plugin), which matches nothing built in, so only the append
// branch ever ran — and TestRegisterWidgetReachesPalette reads the header
// immediately above the row, which a duplicate header satisfies exactly as well
// as the real one. What the user would see is the group listed twice, its
// widgets in the first copy and the plugin alone in the second.
func TestRegisterWidgetJoinsExistingCategory(t *testing.T) {
	const existing = "图表 (Charts)"
	const sibling = "gui.LineChart" // categoryDefs already lists it under existing

	def := pluginTestDef()
	def.Category = existing
	registerPluginTestWidget(t, def)

	list := NewWidgetList()
	headers := 0
	header := -1
	pluginUnder, siblingUnder := -1, -1
	for i := 0; i < list.Count(); i++ {
		item := list.Item(i)
		if isHeader, _ := list.isCategoryHeader(i); isHeader {
			header = i
			// The row carries the collapse indicator; the category name is the rest.
			if strings.TrimSpace(strings.TrimPrefix(item.Text, "▼")) == paletteCategoryLabel(existing) {
				headers++
			}
			continue
		}
		name, _ := item.Data.(string)
		switch name {
		case pluginTestFactory:
			pluginUnder = header
		case sibling:
			siblingUnder = header
		}
	}

	if headers != 1 {
		t.Errorf("palette shows %d headers named %q, want 1", headers, existing)
	}
	if siblingUnder < 0 {
		t.Fatalf("%s left the palette when %s joined %q; rows: %s",
			sibling, pluginTestFactory, existing, pluginTestPaletteDump(list))
	}
	if pluginUnder < 0 {
		t.Fatalf("no palette row carries %q as its drag payload; rows: %s",
			pluginTestFactory, pluginTestPaletteDump(list))
	}
	if pluginUnder != siblingUnder {
		t.Errorf("%s sits under the header at row %d, but %q's existing member %s sits under the "+
			"header at row %d; the plugin got a category of its own instead of joining; rows: %s",
			pluginTestFactory, pluginUnder, existing, sibling, siblingUnder, pluginTestPaletteDump(list))
	}
}

// pluginTestPaletteDump renders the palette rows for a failure message.
func pluginTestPaletteDump(list *WidgetList) string {
	var rows []string
	for i := 0; i < list.Count(); i++ {
		rows = append(rows, list.Item(i).Text)
	}
	return strings.Join(rows, " | ")
}

// TestRegisterWidgetGeneratesPluginType: a registered plugin widget generates
// as itself — its own field type, its own constructor, its own import — instead
// of degrading to the gui.IWidget + core.New(...) fallback, which for a package
// the generated program does not link returns nil and panics at startup.
func TestRegisterWidgetGeneratesPluginType(t *testing.T) {
	registerPluginTestWidget(t, pluginTestDef())

	scene := NewGedScene()
	scene.SetFormTitle("Plugin")
	scene.SetSize(120, 90)
	addNamedWidget(t, scene, pluginTestFactory, "Meter1", 5)

	gen := scene.GenerateCodeIndexed(CodeGenOptions{PackageName: "main", TypeName: "PluginUI"})
	if gen.Err != nil {
		t.Fatalf("GenerateCodeIndexed: %v\n----\n%s", gen.Err, gen.Code)
	}
	if !hasStructField(gen.Code, "Meter1", "*plugintest.Meter") {
		t.Errorf("struct field Meter1 is not *plugintest.Meter\n----\n%s", gen.Code)
	}
	for _, want := range []string{
		"*plugintest.Meter",
		"plugintest.NewMeter()",
		`"example.com/plugintest"`,
	} {
		if !strings.Contains(gen.Code, want) {
			t.Errorf("generated code is missing %s\n----\n%s", want, gen.Code)
		}
	}
	if strings.Contains(gen.Code, `core.New("`+pluginTestFactory+`")`) {
		t.Errorf("plugin widget still generated through the degraded core.New path\n----\n%s", gen.Code)
	}
}

// TestRegisterWidgetDefaultEventIsWired pins the call site, not the lookup: a
// widget carrying one handler func and no recorded event binding is wired
// through handlerBindingsFor -> defaultEventForFactory -> emitEventBinding, so
// the plugin's DefaultEvent has to survive all three to become a line of code.
// Asserting AvailableEvents or defaultEventForFactory alone would stay green
// with the binding never emitted.
func TestRegisterWidgetDefaultEventIsWired(t *testing.T) {
	registerPluginTestWidget(t, pluginTestDef())

	scene := NewGedScene()
	scene.SetFormTitle("PluginEvent")
	scene.SetSize(120, 90)
	m := addNamedWidget(t, scene, pluginTestFactory, "Meter1", 5)
	m.SetCode("func onReading(v float64) { _ = v }")

	code := scene.GenerateCode(CodeGenOptions{PackageName: "main", TypeName: "PluginEventUI"})
	if !strings.Contains(code, "ui.Meter1.SigReading(onReading)") {
		t.Errorf("the plugin's DefaultEvent produced no binding\n----\n%s", code)
	}
}

// TestRegisterWidgetRejectsBadDefs: every rejection writes nothing at all.
// Validating after the first write would leave the widget in some tables and
// not others — a factoryMap entry with no palette row, or a palette row for a
// factory nobody registered — which is the state the package's drift tests fail
// the whole build on, from a test that never asked for it.
func TestRegisterWidgetRejectsBadDefs(t *testing.T) {
	noConstructor := pluginTestDef()
	noConstructor.Constructor = ""

	noFactory := pluginTestDef()
	noFactory.Factory = "plugintest.Nope"

	badDefault := pluginTestDef()
	badDefault.DefaultEvent = "OnNope"

	cases := []struct {
		name    string
		def     WidgetDef
		factory string
	}{
		{"empty Constructor", noConstructor, pluginTestFactory},
		{"unregistered factory", noFactory, "plugintest.Nope"},
		{"DefaultEvent not among Events", badDefault, pluginTestFactory},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Undo whatever a broken RegisterWidget wrote, or every later test in
			// the package inherits the half-registered row this one is about.
			cats := append([]widgetCategoryDef(nil), categoryDefs...)
			t.Cleanup(func() {
				delete(factoryMap, c.factory)
				delete(widgetEvents, c.factory)
				delete(pluginDefaultEvents, c.factory)
				categoryDefs = cats
			})
			if err := RegisterWidget(c.def); err == nil {
				t.Fatalf("RegisterWidget(%+v) returned nil", c.def)
			}
			assertPluginTestUnregistered(t, c.factory)
		})
	}
}

// TestRegisterWidgetRejectsDuplicateFactory: the same factory twice is the
// rejection that has to leave a live registration alone. A second write would
// silently retype the widget and give it a second palette row — which
// TestPaletteEntryIsARegisteredWidget then reports as a duplicate category
// listing, far from the call that caused it.
func TestRegisterWidgetRejectsDuplicateFactory(t *testing.T) {
	registerPluginTestWidget(t, pluginTestDef())

	second := pluginTestDef()
	second.Constructor = "plugintest.NewOther()"
	second.Category = "另一个 (Other)"
	if err := RegisterWidget(second); err == nil {
		t.Fatalf("RegisterWidget accepted %s a second time", pluginTestFactory)
	}
	if got := factoryMap[pluginTestFactory].constructor; got != "plugintest.NewMeter()" {
		t.Errorf("the rejected def overwrote the mapping: constructor = %q", got)
	}
	rows := 0
	for _, name := range paletteNames() {
		if name == pluginTestFactory {
			rows++
		}
	}
	if rows != 1 {
		t.Errorf("%s has %d palette rows after a rejected re-registration, want 1", pluginTestFactory, rows)
	}
	for _, cd := range categoryDefs {
		if cd.name == second.Category {
			t.Errorf("the rejected def appended category %q", second.Category)
		}
	}
}

// TestRegisteredPluginKeepsRegistryInvariants runs the package's three registry
// drift tests over the tables with a plugin widget live in them. RegisterWidget
// writes what those tests read, so a def that satisfies it must not be able to
// break them — the bodies are called rather than restated so they cannot drift
// from the ones that guard the built-in widgets.
func TestRegisteredPluginKeepsRegistryInvariants(t *testing.T) {
	registerPluginTestWidget(t, pluginTestDef())

	TestPaletteEntryHasCodegenMapping(t)
	TestCodegenMappingIsOfferedByPalette(t)
	TestPaletteEntryIsARegisteredWidget(t)
}

// assertPluginTestUnregistered fails when any of the four tables RegisterWidget
// writes gained an entry for factory.
func assertPluginTestUnregistered(t *testing.T, factory string) {
	t.Helper()
	if _, ok := factoryMap[factory]; ok {
		t.Errorf("factoryMap gained %q from a rejected def", factory)
	}
	for _, name := range paletteNames() {
		if name == factory {
			t.Errorf("the palette gained %q from a rejected def", factory)
		}
	}
	if _, ok := widgetEvents[factory]; ok {
		t.Errorf("widgetEvents gained %q from a rejected def", factory)
	}
	if _, ok := pluginDefaultEvents[factory]; ok {
		t.Errorf("pluginDefaultEvents gained %q from a rejected def", factory)
	}
}
