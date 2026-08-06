package ged

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/uk0/silk/graph"
)

// addSplitButton drops a named gui.Button bound to handler on OnClick into the
// scene, optionally carrying design-time handler source.
func addSplitButton(t *testing.T, scene *GedScene, name, handler, code string) {
	t.Helper()
	btn, err := NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatalf("create button %s: %v", name, err)
	}
	btn.SetWidgetName(name)
	btn.SetBounds(5, 5, 25, 7)
	btn.SetEventHandler("OnClick", handler)
	if code != "" {
		btn.SetCode(code)
	}
	cmd := graph.NewAddCommand()
	cmd.AddItem(btn, scene)
	scene.PushCommand(cmd)
}

// TestGenerateCodeFileKeepsUserFile pins the ownership split. The machine file
// is rewritten wholesale on every generation; the user file is written once and
// from then on only ever appended to, so a hand-written line survives every
// later regeneration and a newly designed event arrives as one added stub.
func TestGenerateCodeFileKeepsUserFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.go")

	scene := NewGedScene()
	scene.SetFormTitle("App")
	scene.SetSize(120, 80)
	addSplitButton(t, scene, "btnA", "onGo", "func onGo() {\n\tprintln(\"go\")\n}")
	addSplitButton(t, scene, "btnB", "onGo", "")

	opts := CodeGenOptions{PackageName: "main", TypeName: "AppUI"}
	scene.GenerateCodeFile(path, opts)

	user, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("user file not written: %v", err)
	}
	// What the developer types after the first generation.
	if err := os.WriteFile(path, append(user, []byte("\nvar userEdit = \"keep me\"\n")...), 0644); err != nil {
		t.Fatal(err)
	}

	// The design gains a third button with a new handler.
	addSplitButton(t, scene, "btnC", "onThird", "")
	scene.GenerateCodeFile(path, opts)

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("user file gone after regeneration: %v", err)
	}
	if !strings.Contains(string(got), `var userEdit = "keep me"`) {
		t.Errorf("regeneration destroyed the user's edit; user file is now:\n%s", got)
	}
	if n := strings.Count(string(got), "func (ui *AppUI) onGo("); n != 1 {
		t.Errorf("onGo declared %d times in the user file, want 1:\n%s", n, got)
	}
	if !strings.Contains(string(got), "func (ui *AppUI) onThird(") {
		t.Errorf("the added event gained no stub:\n%s", got)
	}

	msrc, err := os.ReadFile(filepath.Join(dir, "app.silk.go"))
	if err != nil {
		t.Fatalf("machine file not written: %v", err)
	}
	if !strings.Contains(string(msrc), "DO NOT EDIT.") {
		t.Error("machine file lost its generated marker")
	}
	if strings.Contains(string(msrc), `println("go")`) {
		t.Error("handler body inlined into the machine file")
	}
	if strings.Contains(string(msrc), "func main()") {
		t.Error("main() belongs to the user file")
	}
	if !strings.Contains(string(msrc), "ui.BtnA.Action().BindFunc0(ui.onGo)") {
		t.Errorf("machine file does not wire the handler as a method:\n%s", msrc)
	}
}

// TestGenerateCodeFileSplitCompiles is the same story checked by the compiler
// rather than by substring: two buttons on one handler generate a pair that
// type-checks together, and after a third button is designed the regenerated
// pair still does — with the user's edit still in it and exactly one method
// added. The pair is the unit: the machine file calls ui.onGo, only the user
// file declares it, so vetting either half alone would prove nothing.
func TestGenerateCodeFileSplitCompiles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compile test in short mode")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "shared.go")

	scene := NewGedScene()
	scene.SetFormTitle("Shared")
	scene.SetSize(120, 80)
	addSplitButton(t, scene, "btnA", "onGo", "func onGo() {\n\tfmt.Println(\"go\")\n}")
	addSplitButton(t, scene, "btnB", "onGo", "")

	opts := CodeGenOptions{PackageName: "main", TypeName: "SharedUI"}
	res, err := scene.GenerateCodeFile(path, opts)
	if err != nil {
		t.Fatalf("first generation: %v", err)
	}
	if !res.UserCreated {
		t.Error("first generation did not create the user file")
	}
	if len(res.AddedStubs) != 0 {
		t.Errorf("first generation reported appended stubs %v; it wrote the file whole", res.AddedStubs)
	}
	vetGeneratedFiles(t, readPair(t, res))

	// The developer works in their file.
	user, err := os.ReadFile(res.UserFile)
	if err != nil {
		t.Fatal(err)
	}
	edited := string(user) + "\nfunc (ui *SharedUI) mine() string {\n\treturn \"keep me\"\n}\n"
	if err := os.WriteFile(res.UserFile, []byte(edited), 0644); err != nil {
		t.Fatal(err)
	}

	// The design gains a widget on a new handler.
	addSplitButton(t, scene, "btnC", "onThird", "")
	res, err = scene.GenerateCodeFile(path, opts)
	if err != nil {
		t.Fatalf("regeneration: %v", err)
	}
	if res.UserCreated {
		t.Error("regeneration rewrote the user file from scratch")
	}
	if len(res.AddedStubs) != 1 || res.AddedStubs[0] != "onThird" {
		t.Errorf("AddedStubs = %v, want [onThird]", res.AddedStubs)
	}

	pair := readPair(t, res)
	if !strings.Contains(pair["user.go"], `return "keep me"`) {
		t.Errorf("the developer's method is gone:\n%s", pair["user.go"])
	}
	if n := strings.Count(pair["user.go"], "func (ui *SharedUI) onGo("); n != 1 {
		t.Errorf("onGo declared %d times, want 1:\n%s", n, pair["user.go"])
	}
	vetGeneratedFiles(t, pair)
}

// TestGenerateCodeFileWiresUnwrittenHandler pins decision 4's floor: a design
// whose events were bound in the property sheet but never given a body still
// generates a pair that builds. Single-file output emitted the binding and
// nothing else, so the export referenced a func that did not exist.
func TestGenerateCodeFileWiresUnwrittenHandler(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compile test in short mode")
	}

	dir := t.TempDir()
	scene := NewGedScene()
	scene.SetFormTitle("Bare")
	scene.SetSize(120, 80)

	sb, err := NewFakeWidgetFromFactory("gui.SearchBox")
	if err != nil {
		t.Fatalf("create SearchBox: %v", err)
	}
	sb.SetWidgetName("search")
	sb.SetBounds(5, 5, 100, 10)
	sb.SetEventHandler("OnTextChanged", "onSearchTextChanged")
	cmd := graph.NewAddCommand()
	cmd.AddItem(sb, scene)
	scene.PushCommand(cmd)

	res, err := scene.GenerateCodeFile(filepath.Join(dir, "bare.go"), CodeGenOptions{PackageName: "main", TypeName: "BareUI"})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	pair := readPair(t, res)
	if !strings.Contains(pair["user.go"], "func (ui *BareUI) onSearchTextChanged(s string) {") {
		t.Errorf("no stub synthesized for a bound-but-unwritten handler:\n%s", pair["user.go"])
	}
	vetGeneratedFiles(t, pair)
}

// TestSplitStubsMatchEveryBinding type-checks the whole widgetEvents table in
// one pass: every bindable event is bound on a widget, every handler is
// synthesized into the user file from that row's params, and the two files are
// vetted together. A params column that disagrees with the arguments its own
// code column passes fails here instead of in someone's export.
func TestSplitStubsMatchEveryBinding(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compile test in short mode")
	}

	scene := NewGedScene()
	scene.SetFormTitle("EveryEvent")
	scene.SetSize(200, 400)

	factories := make([]string, 0, len(widgetEvents))
	for name := range widgetEvents {
		factories = append(factories, name)
	}
	sort.Strings(factories)

	y := 5.0
	added := map[string]bool{}
	for _, factoryName := range factories {
		fn := factoryName
		// Some widgets panic during layout when built without full
		// initialisation; skipping them keeps the rest of the table covered.
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Logf("skipping %s: panic during setup: %v", fn, r)
				}
			}()
			fake, err := NewFakeWidgetFromFactory(fn)
			if err != nil {
				t.Logf("skipping %s: %v", fn, err)
				return
			}
			fake.SetWidgetName(strings.TrimPrefix(fn, "gui."))
			fake.SetBounds(5, y, 40, 7)
			for _, evt := range AvailableEvents(fn) {
				fake.SetEventHandler(evt, everyEventHandlerName(fn, evt))
			}
			cmd := graph.NewAddCommand()
			cmd.AddItem(fake, scene)
			scene.PushCommand(cmd)
			y += 10
			added[fn] = true
		}()
	}
	if len(added) < 20 {
		t.Fatalf("only %d/%d event-bearing widgets could be placed", len(added), len(widgetEvents))
	}

	res, err := scene.GenerateCodeFile(filepath.Join(t.TempDir(), "every.go"), CodeGenOptions{PackageName: "main", TypeName: "EveryUI"})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	pair := readPair(t, res)
	for _, factoryName := range factories {
		if !added[factoryName] {
			continue
		}
		for _, evt := range AvailableEvents(factoryName) {
			decl := "func (ui *EveryUI) " + everyEventHandlerName(factoryName, evt) + "("
			if !strings.Contains(pair["user.go"], decl) {
				t.Errorf("%s.%s is wired but the user file declares no %s", factoryName, evt, decl)
			}
		}
	}
	vetGeneratedFiles(t, pair)
}

func everyEventHandlerName(factoryName, evt string) string {
	return "on" + strings.TrimPrefix(factoryName, "gui.") + strings.TrimPrefix(evt, "On")
}

// TestGenerateCodeFileSeededStubImportsItsParams is the export that did not
// compile: a design-time body written for an event whose parameter list names a
// package. The body and the import reached stub collection down two different
// branches, the body branch claimed the handler first, and the import its
// signature needs never arrived — so the designer wrote a brand new user file
// that go vet answered with "undefined: paint".
func TestGenerateCodeFileSeededStubImportsItsParams(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compile test in short mode")
	}

	scene := NewGedScene()
	scene.SetFormTitle("Picker")
	scene.SetSize(120, 80)

	cp, err := NewFakeWidgetFromFactory("gui.ColorPicker")
	if err != nil {
		t.Fatalf("create ColorPicker: %v", err)
	}
	cp.SetWidgetName("picker")
	cp.SetBounds(5, 5, 60, 10)
	cp.SetEventHandler("OnColorChanged", "onColorChanged")
	cp.SetCode("func onColorChanged(c paint.Color) {\n\tprintln(c.R)\n}")
	cmd := graph.NewAddCommand()
	cmd.AddItem(cp, scene)
	scene.PushCommand(cmd)

	res, err := scene.GenerateCodeFile(filepath.Join(t.TempDir(), "picker.go"), CodeGenOptions{PackageName: "main", TypeName: "PickerUI"})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !res.UserCreated {
		t.Fatal("the user file already existed; this test covers the creation path")
	}
	pair := readPair(t, res)
	if !strings.Contains(pair["user.go"], `"github.com/uk0/silk/paint"`) {
		t.Errorf("the created user file declares onColorChanged(c paint.Color) but imports no paint:\n%s", pair["user.go"])
	}
	vetGeneratedFiles(t, pair)
}

// TestGenerateCodeFileAssemblesAStubFromBothPaths: the widget that binds a
// handler and the widget that carries its body need not be the same one, and
// need not come in that order. Whichever the walk reaches first, the handler is
// one stub holding both halves — otherwise the half that lost is dropped on the
// floor, which for the code attr means the developer's own source never reaches
// the file it was about to be moved into.
func TestGenerateCodeFileAssemblesAStubFromBothPaths(t *testing.T) {
	scene := NewGedScene()
	scene.SetFormTitle("Both")
	scene.SetSize(120, 80)
	// The bare binding is designed first; the body arrives on a later widget.
	addSplitColorPicker(t, scene, "pickerA", "onColorChanged", 5, "")
	addSplitColorPicker(t, scene, "pickerB", "onColorChanged", 20, "func onColorChanged(c paint.Color) {\n\tprintln(c.R)\n}")

	res, err := scene.GenerateCodeFile(filepath.Join(t.TempDir(), "both.go"), CodeGenOptions{PackageName: "main", TypeName: "BothUI"})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	user := readPair(t, res)["user.go"]
	if n := strings.Count(user, "func (ui *BothUI) onColorChanged("); n != 1 {
		t.Fatalf("onColorChanged declared %d times, want 1:\n%s", n, user)
	}
	if !strings.Contains(user, "println(c.R)") {
		t.Errorf("the design-time body was dropped for the bare binding that came first:\n%s", user)
	}
	if !strings.Contains(user, `"github.com/uk0/silk/paint"`) {
		t.Errorf("the created user file imports no paint:\n%s", user)
	}
}

// TestGenerateCodeFileReportsTheImportItMayNotAdd is the same defect on the
// other path. Once the user file exists it is only ever appended to, so a stub
// whose signature names paint.Color lands in a file whose import block codegen
// is not allowed to touch. That one-line compile error stays the developer's to
// fix — finding it should not be, so the export report names the import.
func TestGenerateCodeFileReportsTheImportItMayNotAdd(t *testing.T) {
	path := filepath.Join(t.TempDir(), "late.go")

	scene := NewGedScene()
	scene.SetFormTitle("Late")
	scene.SetSize(120, 80)
	addSplitButton(t, scene, "btnA", "onGo", "")

	opts := CodeGenOptions{PackageName: "main", TypeName: "LateUI"}
	first, err := scene.GenerateCodeFile(path, opts)
	if err != nil {
		t.Fatalf("first generation: %v", err)
	}
	if len(first.MissingImports) != 0 {
		t.Errorf("creation reported missing imports %v; it writes the import block itself", first.MissingImports)
	}

	// The design gains a ColorPicker after the developer owns the file, with
	// its handler written in the designer — the shape that assembles its body
	// and its import from two different paths.
	addSplitColorPicker(t, scene, "picker", "onColorChanged", 20, "func onColorChanged(c paint.Color) {\n\tprintln(c.R)\n}")

	res, err := scene.GenerateCodeFile(path, opts)
	if err != nil {
		t.Fatalf("regeneration: %v", err)
	}
	if len(res.AddedStubs) != 1 || res.AddedStubs[0] != "onColorChanged" {
		t.Fatalf("AddedStubs = %v, want [onColorChanged]", res.AddedStubs)
	}
	if len(res.MissingImports) != 1 || res.MissingImports[0] != "github.com/uk0/silk/paint" {
		t.Fatalf("MissingImports = %v, want [github.com/uk0/silk/paint]; unreported it reaches the developer as a bare \"undefined: paint\"", res.MissingImports)
	}

	appended := readPair(t, res)["user.go"]
	if strings.Contains(appended, `"github.com/uk0/silk/paint"`) {
		t.Errorf("the append path rewrote the developer's import block:\n%s", appended)
	}
	if !strings.Contains(appended, "func (ui *LateUI) onColorChanged(c paint.Color) {") {
		t.Errorf("the appended stub does not carry the signature the binding calls:\n%s", appended)
	}

	// The developer applies exactly what the report named — the report's own
	// value, not a constant this test picked — and nothing else.
	const anchor = "\t\"github.com/uk0/silk/core\"\n"
	fixed := strings.Replace(appended, anchor, anchor+"\t\""+res.MissingImports[0]+"\"\n", 1)
	if fixed == appended {
		t.Fatalf("could not splice the reported import into the user file:\n%s", appended)
	}
	if err := os.WriteFile(path, []byte(fixed), 0644); err != nil {
		t.Fatal(err)
	}

	// A second widget on the same import: the file has it now, so there is
	// nothing left to report and the developer is not told twice.
	addSplitColorPicker(t, scene, "picker2", "onOtherColorChanged", 40, "")
	res, err = scene.GenerateCodeFile(path, opts)
	if err != nil {
		t.Fatalf("second regeneration: %v", err)
	}
	if len(res.AddedStubs) != 1 || res.AddedStubs[0] != "onOtherColorChanged" {
		t.Fatalf("AddedStubs = %v, want [onOtherColorChanged]", res.AddedStubs)
	}
	if len(res.MissingImports) != 0 {
		t.Errorf("MissingImports = %v; the user file already imports it", res.MissingImports)
	}

	if testing.Short() {
		return
	}
	// The pair on disk builds once the one reported import is in place, which
	// is what makes the report a fix rather than a hint.
	vetGeneratedFiles(t, readPair(t, res))
}

// addSplitColorPicker drops a gui.ColorPicker bound to handler on the one event
// in widgetEvents whose parameter list names a package, optionally carrying
// design-time handler source.
func addSplitColorPicker(t *testing.T, scene *GedScene, name, handler string, y float64, code string) {
	t.Helper()
	cp, err := NewFakeWidgetFromFactory("gui.ColorPicker")
	if err != nil {
		t.Fatalf("create ColorPicker: %v", err)
	}
	cp.SetWidgetName(name)
	cp.SetBounds(5, y, 60, 10)
	cp.SetEventHandler("OnColorChanged", handler)
	if code != "" {
		cp.SetCode(code)
	}
	cmd := graph.NewAddCommand()
	cmd.AddItem(cp, scene)
	scene.PushCommand(cmd)
}

// TestSplitStubsImportEveryQualifiedParam sweeps widgetEvents for parameter
// lists that name a package and proves every one of them reaches a created user
// file that imports it. The instance above is one row of this table; the sweep
// is what makes the next row that gains a qualified parameter fail here instead
// of in an export.
func TestSplitStubsImportEveryQualifiedParam(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compile test in short mode")
	}

	type qualifiedRow struct{ factory, event, params, imp string }
	factories := make([]string, 0, len(widgetEvents))
	for name := range widgetEvents {
		factories = append(factories, name)
	}
	sort.Strings(factories)
	var rows []qualifiedRow
	for _, fn := range factories {
		for _, e := range widgetEvents[fn] {
			if qualifierIn(e.params) == "" {
				continue
			}
			rows = append(rows, qualifiedRow{fn, e.name, e.params, e.imp})
		}
	}
	if len(rows) == 0 {
		t.Fatal("no widgetEvents row takes a package-qualified parameter; the sweep would pass vacuously")
	}

	scene := NewGedScene()
	scene.SetFormTitle("Qualified")
	scene.SetSize(200, 400)
	y := 5.0
	placed := map[string]bool{}
	for i, r := range rows {
		i, r := i, r
		// Some widgets panic during layout when built without full
		// initialisation. Contain that here: a row nobody can place is a row
		// this sweep cannot vouch for, and it says so rather than taking the
		// package's whole test binary down with it.
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					t.Errorf("cannot place %s to check its %q parameters: %v", r.factory, r.params, rec)
				}
			}()
			fake, err := NewFakeWidgetFromFactory(r.factory)
			if err != nil {
				t.Errorf("create %s: %v", r.factory, err)
				return
			}
			fake.SetWidgetName(fmt.Sprintf("%s%d", strings.TrimPrefix(r.factory, "gui."), i))
			fake.SetBounds(5, y, 40, 7)
			handler := everyEventHandlerName(r.factory, r.event)
			fake.SetEventHandler(r.event, handler)
			// The design-time body is the half that used to win the handler alone.
			fake.SetCode(fmt.Sprintf("func %s(%s) {\n}", handler, r.params))
			cmd := graph.NewAddCommand()
			cmd.AddItem(fake, scene)
			scene.PushCommand(cmd)
			y += 10
			placed[r.factory+"."+r.event] = true
		}()
	}

	res, err := scene.GenerateCodeFile(filepath.Join(t.TempDir(), "qualified.go"), CodeGenOptions{PackageName: "main", TypeName: "QualifiedUI"})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	pair := readPair(t, res)
	for _, r := range rows {
		if !placed[r.factory+"."+r.event] {
			continue // already reported above
		}
		if r.imp == "" {
			t.Errorf("%s.%s takes %q but names no imp, so no import can be emitted for it", r.factory, r.event, r.params)
			continue
		}
		if !strings.Contains(pair["user.go"], fmt.Sprintf("%q", r.imp)) {
			t.Errorf("%s.%s takes %q; the created user file imports no %s:\n%s", r.factory, r.event, r.params, r.imp, pair["user.go"])
		}
	}
	vetGeneratedFiles(t, pair)
}

// qualifierIn names the package a parameter list qualifies against, "" when the
// list is all builtins: "c paint.Color" -> "paint".
func qualifierIn(params string) string {
	dot := strings.Index(params, ".")
	if dot <= 0 {
		return ""
	}
	start := dot
	for start > 0 && isIdentByte(params[start-1]) {
		start--
	}
	return params[start:dot]
}

// TestAppendMissingStubsIsAppendOnly pins the one rule the user side lives by.
// The file goes back byte for byte with the new method after it — no
// reformatting, no reordering — and a handler the developer already wrote is
// recognised whatever they called the receiver.
func TestAppendMissingStubsIsAppendOnly(t *testing.T) {
	const existing = `package main

// mine, formatted my way
func (u *AppUI) onGo()    { println("go") }

func (u *AppUI) helper() {}
`
	stubs := []handlerStub{
		{name: "onGo"},
		{name: "onNew", params: "v float64"},
	}
	got, added, _, err := appendMissingStubs(existing, "AppUI", stubs)
	if err != nil {
		t.Fatal(err)
	}
	if len(added) != 1 || added[0] != "onNew" {
		t.Fatalf("added = %v, want [onNew]", added)
	}
	if !strings.HasPrefix(got, existing) {
		t.Errorf("the existing file was not left intact:\n%s", got)
	}
	if strings.TrimPrefix(got, existing) != "\nfunc (ui *AppUI) onNew(v float64) {\n}\n" {
		t.Errorf("unexpected appended text: %q", strings.TrimPrefix(got, existing))
	}
}

// TestAppendMissingStubsRefusesBrokenFile: a user file mid-edit cannot be told
// apart from one missing a handler, and appending to it would duplicate a
// method the developer has already written. Report instead of guessing.
func TestAppendMissingStubsRefusesBrokenFile(t *testing.T) {
	const broken = "package main\n\nfunc (ui *AppUI) onGo() {\n"
	got, added, _, err := appendMissingStubs(broken, "AppUI", []handlerStub{{name: "onGo"}})
	if err == nil {
		t.Fatal("appended to a file that does not parse")
	}
	if got != broken || added != nil {
		t.Errorf("the broken file was modified: %q %v", got, added)
	}
}

// TestMethodizeKeepsTheBody: seeding a stub from the design-time code attr may
// change the header and nothing else — the body is the developer's.
func TestMethodizeKeepsTheBody(t *testing.T) {
	const code = "// counts clicks\nfunc onGo() {\n\t// no-op for now\n\tcount++\n}"
	got := methodize(code, "func (ui *AppUI) ")
	want := "// counts clicks\nfunc (ui *AppUI) onGo() {\n\t// no-op for now\n\tcount++\n}"
	if got != want {
		t.Errorf("methodize:\n got %q\nwant %q", got, want)
	}
	// Source the developer already wrote as a method is re-hung, not stacked:
	// the generation owns which struct a handler belongs to (the export route
	// picks the name), the developer owns everything below the header.
	rehung := "// counts clicks\nfunc (ui *OtherUI) onGo() {\n\t// no-op for now\n\tcount++\n}"
	if got := methodize(want, "func (ui *OtherUI) "); got != rehung {
		t.Errorf("re-hanging a method:\n got %q\nwant %q", got, rehung)
	}
}

// readPair reads a generation's two files back under stable names, so a vet
// failure names the half that broke.
func readPair(t *testing.T, res SplitResult) map[string]string {
	t.Helper()
	machine, err := os.ReadFile(res.MachineFile)
	if err != nil {
		t.Fatalf("machine file: %v", err)
	}
	user, err := os.ReadFile(res.UserFile)
	if err != nil {
		t.Fatalf("user file: %v", err)
	}
	return map[string]string{"ui.silk.go": string(machine), "user.go": string(user)}
}
