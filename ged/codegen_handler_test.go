package ged

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uk0/silk/core"
)

// TestParseHandlerDeclShapes pins what the generator reads out of a snippet.
// Every row here reached the generated file as a broken identifier, a dropped
// body, or a handler that was never written, back when the name came from
// splitting the "func " line on whitespace.
func TestParseHandlerDeclShapes(t *testing.T) {
	cases := []struct {
		name   string
		code   string
		want   handlerDecl
		broken bool // the snippet does not parse
	}{
		{
			name: "free function",
			code: "func onGo() {\n\tprintln(1)\n}",
			want: handlerDecl{name: "onGo"},
		},
		{
			name: "pointer method",
			code: "func (ui *AppUI) onGo() {\n\tprintln(1)\n}",
			want: handlerDecl{name: "onGo", method: true},
		},
		{
			name: "value method",
			code: "func (ui AppUI) onGo() {}",
			want: handlerDecl{name: "onGo", method: true},
		},
		{
			name: "method behind a doc comment",
			code: "// counts clicks\nfunc (ui *AppUI) onGo() {}",
			want: handlerDecl{name: "onGo", method: true},
		},
		{
			name: "generic free function",
			code: "func onGo[T any](v T) {}",
			want: handlerDecl{name: "onGo"},
		},
		{
			name: "declaration inside a block comment is not one",
			code: "/*\nfunc onOld() {}\n*/\nfunc onNew() {}",
			want: handlerDecl{name: "onNew"},
		},
		{
			name:   "unterminated body",
			code:   "func onGo() {",
			broken: true,
		},
		{
			name: "no declaration at all",
			code: "// nothing here yet",
		},
		{
			name: "empty",
			code: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseHandlerDecl(tc.code)
			if tc.broken {
				if err == nil {
					t.Fatalf("parseHandlerDecl(%q) = %+v, want an error", tc.code, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseHandlerDecl(%q): %v", tc.code, err)
			}
			if got != tc.want {
				t.Errorf("parseHandlerDecl(%q) = %+v, want %+v", tc.code, got, tc.want)
			}
		})
	}
}

// TestMethodHandlerSourceSurvivesReload is decision 4 end to end: a design whose
// handler source is a method on the UI struct is saved to a .silkui file, read
// back into a fresh scene, and exported — and the pair that comes out still
// carries the developer's body and still compiles.
//
// Two widgets, because the two ways a handler gets bound fail differently. btnGo
// has its OnClick recorded in the property sheet: the name came from the
// binding, so the export used to write an EMPTY method and drop the body on the
// floor. btnAuto has only the source: the name had to come from the snippet,
// which yielded nothing, so no binding and no method were written at all. Both
// still produce a pair that compiles, which is why go vet alone cannot see this
// and the assertions below read the two files instead.
func TestMethodHandlerSourceSurvivesReload(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compile test in short mode")
	}

	const goBody = `ui.BtnGo.SetText("clicked")`
	const autoBody = `ui.BtnAuto.SetText("auto")`

	scene := NewGedScene()
	scene.SetFormTitle("App")
	scene.SetSize(160, 100)

	btnGo := addFakeTo(t, scene, "gui.Button", "btnGo")
	btnGo.SetEventHandler("OnClick", "onGo")
	btnGo.SetCode("// what the developer pasted\nfunc (ui *AppUI) onGo() {\n\t" + goBody + "\n}")

	btnAuto := addFakeTo(t, scene, "gui.Button", "btnAuto")
	btnAuto.SetCode("func (ui *AppUI) onAuto() {\n\t" + autoBody + "\n}")

	dir := t.TempDir()
	design := filepath.Join(dir, "app.silkui")
	if err := scene.SaveDesign().SaveFile(design); err != nil {
		t.Fatalf("save design: %v", err)
	}
	doc, err := core.LoadTDocFile(design)
	if err != nil {
		t.Fatalf("load design: %v", err)
	}
	reloaded := NewGedScene()
	if err := reloaded.LoadDesign(doc); err != nil {
		t.Fatalf("LoadDesign: %v", err)
	}

	// TypeName is left to the generator so the receiver the design was written
	// against is the one the export derives from the form title.
	res, err := reloaded.GenerateCodeFile(filepath.Join(dir, "app.go"), CodeGenOptions{PackageName: "main"})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	pair := readPair(t, res)

	for _, want := range []struct{ handler, body string }{{"onGo", goBody}, {"onAuto", autoBody}} {
		body, n := methodBody(t, pair["user.go"], "AppUI", want.handler)
		if n != 1 {
			t.Errorf("user file declares %s on AppUI %d times, want 1:\n%s", want.handler, n, pair["user.go"])
			continue
		}
		if !strings.Contains(body, want.body) {
			t.Errorf("%s lost its body: got %s, want it to contain %s", want.handler, body, want.body)
		}
		if !referencesMethodValue(t, pair["ui.silk.go"], want.handler) {
			t.Errorf("machine file never binds ui.%s:\n%s", want.handler, pair["ui.silk.go"])
		}
	}

	vetGeneratedFiles(t, pair)
}

// TestSingleFileMethodHandlerCallsThroughUI: the 生成代码 preview is single-file
// output, where handlers are appended package-level declarations. A method
// cannot be called by its bare name there either, so the binding has to go
// through ui — the constructor has one in scope and the method hangs off the
// struct the same file declares.
func TestSingleFileMethodHandlerCallsThroughUI(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compile test in short mode")
	}

	scene := NewGedScene()
	scene.SetFormTitle("Solo")
	scene.SetSize(120, 80)
	btn := addFakeTo(t, scene, "gui.Button", "btnGo")
	btn.SetCode("func (ui *SoloUI) onGo() {\n\tui.BtnGo.SetText(\"clicked\")\n}")

	gen := scene.GenerateCodeIndexed(CodeGenOptions{PackageName: "main", TypeName: "SoloUI"})
	if gen.Err != nil {
		t.Fatalf("unexpected generation error: %v", gen.Err)
	}
	if !referencesMethodValue(t, gen.Code, "onGo") {
		t.Errorf("single-file output does not bind ui.onGo:\n%s", gen.Code)
	}
	vetGeneratedCode(t, gen.Code)
}

// TestGenerateCodeFileRefusesUnreadableHandler: a design saved while the code
// panel held half a function cannot be told from one whose handler is simply
// empty, and generating from it silently swaps the developer's work for a stub.
// Report and write nothing instead.
func TestGenerateCodeFileRefusesUnreadableHandler(t *testing.T) {
	dir := t.TempDir()
	scene := NewGedScene()
	scene.SetFormTitle("Half")
	scene.SetSize(120, 80)
	btn := addFakeTo(t, scene, "gui.Button", "btnGo")
	btn.SetEventHandler("OnClick", "onGo")
	btn.SetCode("func onGo() {\n\tprintln(\"go\")")

	res, err := scene.GenerateCodeFile(filepath.Join(dir, "half.go"), CodeGenOptions{PackageName: "main", TypeName: "HalfUI"})
	if err == nil {
		t.Fatal("generated a pair from source that does not parse")
	}
	if !strings.Contains(err.Error(), "btnGo") {
		t.Errorf("error does not name the widget: %v", err)
	}
	for _, path := range []string{res.MachineFile, res.UserFile} {
		if _, statErr := os.Stat(path); statErr == nil {
			t.Errorf("%s was written despite the refusal", path)
		}
	}

	// The same design in the 生成代码 view: the listing would be missing the
	// handler, so the view names the widget instead of showing it.
	genErr := scene.GenerateCodeIndexed(CodeGenOptions{PackageName: "main", TypeName: "HalfUI"}).Err
	if genErr == nil || !strings.Contains(genErr.Error(), "btnGo") {
		t.Errorf("GenerateCodeIndexed did not report the unreadable handler: %v", genErr)
	}
}

// TestForeignReceiverIsRehung: the receiver saved in a design is a placeholder,
// because nothing in the design decides which struct the handlers belong to —
// the export route does, and the two routes disagree on purpose. Generating
// interactively takes the name from the form title (defaultCodeGenOptions),
// while GenerateDesign takes it from the file stem so two forms in one package
// can share a title. A handler that named the other one still has to land on the
// struct being generated, or clicking a widget in the code panel (which seeds a
// method naming the form-title struct) would leave the design un-exportable
// project-wide.
func TestForeignReceiverIsRehung(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compile test in short mode")
	}

	dir := t.TempDir()
	scene := NewGedScene()
	scene.SetFormTitle("Renamed")
	scene.SetSize(120, 80)
	btn := addFakeTo(t, scene, "gui.Button", "btnGo")
	btn.SetCode("// still mine\nfunc (ui *OldUI) onGo() {\n\tprintln(\"kept\")\n}")

	res, err := scene.GenerateCodeFile(filepath.Join(dir, "renamed.go"), CodeGenOptions{PackageName: "main", TypeName: "RenamedUI"})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	pair := readPair(t, res)
	body, n := methodBody(t, pair["user.go"], "RenamedUI", "onGo")
	if n != 1 {
		t.Fatalf("user file declares onGo on RenamedUI %d times, want 1:\n%s", n, pair["user.go"])
	}
	if !strings.Contains(body, `println("kept")`) {
		t.Errorf("re-hanging the method ate the body: %s", body)
	}
	if !strings.Contains(pair["user.go"], "// still mine") {
		t.Errorf("the developer's comment did not survive:\n%s", pair["user.go"])
	}
	vetGeneratedFiles(t, pair)

	// The same design in single-file output: the handler block is appended to
	// the file that declares the struct, so it has to hang off that one too.
	gen := scene.GenerateCodeIndexed(CodeGenOptions{PackageName: "main", TypeName: "SoloRenamedUI"})
	if gen.Err != nil {
		t.Fatalf("unexpected generation error: %v", gen.Err)
	}
	if _, n := methodBody(t, gen.Code, "SoloRenamedUI", "onGo"); n != 1 {
		t.Fatalf("single-file output declares onGo on SoloRenamedUI %d times, want 1:\n%s", n, gen.Code)
	}
	vetGeneratedCode(t, gen.Code)
}

// TestProjectRouteExportsPanelSeededHandler is the whole chain the two type-name
// rules run through: select a widget in the code panel (which stamps its
// template into the design), save the design, and generate it the way a project
// wide build does — from the file stem, which here is deliberately not the form
// title. The pair must build.
func TestProjectRouteExportsPanelSeededHandler(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compile test in short mode")
	}

	dir := t.TempDir()
	design := filepath.Join(dir, "login.silkui")

	view := NewGedView()
	scene := view.GedScene()
	scene.SetFormTitle("Login Window")
	scene.SetSize(120, 80)
	btn := addFakeViaCommand(t, scene, "gui.Button", "btnOK")

	panel := NewCodePanel()
	panel.BindGedView(view)
	panel.SetWidget(btn) // seeds the template
	panel.SaveCode()     // ... and the editor writes it back into the design
	if err := scene.SaveDesign().SaveFile(design); err != nil {
		t.Fatal(err)
	}

	out, err := GenerateDesign(design, "probe")
	if err != nil {
		t.Fatalf("project-wide generate refused a panel-seeded handler: %v", err)
	}
	machine, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	user, err := os.ReadFile(filepath.Join(dir, "login.go"))
	if err != nil {
		t.Fatalf("user file: %v", err)
	}
	// LoginUI: from the file stem, not the "Login Window" the template named.
	if _, n := methodBody(t, string(user), "LoginUI", "onBtnOKClick"); n != 1 {
		t.Fatalf("user file does not declare onBtnOKClick on LoginUI:\n%s", user)
	}
	vetGeneratedFiles(t, map[string]string{"login.silk.go": string(machine), "login.go": string(user)})
}

// TestCodePanelTemplateIsAMethodThatExports closes the loop the panel opens: the
// template it seeds a fresh handler with is saved into the design unedited by
// plenty of developers, so it has to be the shape the export owns. Take the
// template the panel produces, save it back the way the editor does, and export
// — the method must land in the user file with its body and the pair must build.
func TestCodePanelTemplateIsAMethodThatExports(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compile test in short mode")
	}

	view := NewGedView()
	scene := view.GedScene()
	scene.SetFormTitle("Panel")
	scene.SetSize(120, 80)
	btn := addFakeViaCommand(t, scene, "gui.Button", "btnOK")

	panel := NewCodePanel()
	panel.BindGedView(view)
	panel.SetWidget(btn)
	panel.SaveCode()

	code := btn.GetCode()
	decl, err := parseHandlerDecl(code)
	if err != nil {
		t.Fatalf("the template does not parse: %v\n%s", err, code)
	}
	if !decl.method {
		t.Errorf("template is a free function, the shape the split export replaced:\n%s", code)
	}

	res, err := scene.GenerateCodeFile(filepath.Join(t.TempDir(), "panel.go"), CodeGenOptions{PackageName: "main"})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	pair := readPair(t, res)
	body, n := methodBody(t, pair["user.go"], "PanelUI", decl.name)
	if n != 1 {
		t.Fatalf("user file declares %s %d times, want 1:\n%s", decl.name, n, pair["user.go"])
	}
	if !strings.Contains(body, "btnOK clicked") {
		t.Errorf("the template body did not reach the user file: %s", body)
	}
	vetGeneratedFiles(t, pair)
}

// methodBody returns the source of typeName's `name` method as declared in src,
// and how many declarations of it there are. It parses rather than searches for
// a substring: the assertions above have to tell the developer's body from an
// empty stub, and a renamed identifier has to make the test fail instead of
// quietly matching something else in the file.
func methodBody(t *testing.T, src, typeName, name string) (string, int) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "user.go", src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("generated user file does not parse: %v\n%s", err, src)
	}
	var body string
	n := 0
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 || fn.Name.Name != name {
			continue
		}
		if receiverTypeName(fn.Recv.List[0].Type) != typeName {
			continue
		}
		n++
		body = src[fset.Position(fn.Body.Pos()).Offset:fset.Position(fn.Body.End()).Offset]
	}
	return body, n
}

// referencesMethodValue reports whether src hands ui.<name> around as a value —
// the expression a signal binding is written as. An AST walk, so a handler named
// in a comment or in a string literal cannot make it true.
func referencesMethodValue(t *testing.T, src, name string) bool {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "gen.go", src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("generated file does not parse: %v\n%s", err, src)
	}
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != name {
			return true
		}
		if id, ok := sel.X.(*ast.Ident); ok && id.Name == "ui" {
			found = true
		}
		return true
	})
	return found
}
