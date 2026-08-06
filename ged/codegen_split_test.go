package ged

import (
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
	got, added, err := appendMissingStubs(existing, "AppUI", stubs)
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
	got, added, err := appendMissingStubs(broken, "AppUI", []handlerStub{{name: "onGo"}})
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
	// Source the developer already wrote as a method is left alone.
	if got := methodize(want, "func (ui *OtherUI) "); got != want {
		t.Errorf("a receiver was added twice: %q", got)
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
