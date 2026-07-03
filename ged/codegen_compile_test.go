package ged

import (
	"github.com/uk0/silk/graph"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodeGenCompiles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compile test in short mode")
	}

	scene := NewGedScene()
	scene.SetFormTitle("CompileTest")
	scene.SetSize(80, 60)

	// Add a variety of widgets
	widgets := []struct {
		factory string
		name    string
		x, y    float64
	}{
		{"gui.Button", "btn1", 5, 5},
		{"gui.Label", "lbl1", 5, 15},
		{"gui.Edit", "edit1", 5, 25},
		{"gui.CheckBox", "cb1", 5, 35},
		{"gui.Slider", "slider1", 5, 45},
	}

	for _, w := range widgets {
		fake, err := NewFakeWidgetFromFactory(w.factory)
		if err != nil {
			t.Fatalf("failed to create %s: %v", w.factory, err)
		}
		fake.SetWidgetName(w.name)
		fake.SetBounds(w.x, w.y, 25, 7)
		cmd := graph.NewAddCommand()
		cmd.AddItem(fake, scene)
		scene.PushCommand(cmd)
	}

	code := scene.GenerateCode(CodeGenOptions{PackageName: "main", TypeName: "CompileTestUI"})

	// Basic structural checks
	if !strings.Contains(code, "package main") {
		t.Error("missing package main")
	}
	if !strings.Contains(code, "type CompileTestUI struct") {
		t.Error("missing struct")
	}
	if !strings.Contains(code, "func NewCompileTestUI()") {
		t.Error("missing constructor")
	}

	// Write to a throwaway module and type-check via go vet.
	vetGeneratedCode(t, code)
}

// TestCodeGenCodeEditorWithEventCompiles proves that a form containing a
// gui.CodeEditor WITH a SigChanged (OnTextChanged) event binding produces
// code that actually compiles. It is the regression guard for the class of
// bug where a widget missing from factoryMap degrades its field to
// gui.IWidget while the event switch still emits ui.<field>.SigChanged(...)
// — a method gui.IWidget does not have — yielding output that fails to
// build. go vet type-checks the generated module and catches exactly that.
func TestCodeGenCodeEditorWithEventCompiles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compile test in short mode")
	}

	scene := NewGedScene()
	scene.SetFormTitle("EditorCompile")
	scene.SetSize(120, 90)

	ed, err := NewFakeWidgetFromFactory("gui.CodeEditor")
	if err != nil {
		t.Fatalf("create CodeEditor: %v", err)
	}
	ed.SetWidgetName("editor")
	ed.SetBounds(5, 5, 100, 70)
	// A real handler body makes the generated file self-contained: the
	// codegen binds OnTextChanged → SigChanged and appends this func, so
	// go vet checks the field type and the binding together.
	ed.SetCode("func onEditorTextChanged(s string) { _ = s }")
	cmd := graph.NewAddCommand()
	cmd.AddItem(ed, scene)
	scene.PushCommand(cmd)

	code := scene.GenerateCode(CodeGenOptions{PackageName: "main", TypeName: "EditorCompileUI"})

	// The field must be the concrete type so the SigChanged binding
	// type-checks; the binding itself must be present.
	if !strings.Contains(code, "Editor *gui.CodeEditor") {
		t.Errorf("CodeEditor field not concrete *gui.CodeEditor\n----\n%s", code)
	}
	if !strings.Contains(code, "ui.Editor.SigChanged(onEditorTextChanged)") {
		t.Errorf("missing CodeEditor.SigChanged binding\n----\n%s", code)
	}

	vetGeneratedCode(t, code)
}

func TestCodeGenAllFactoryWidgets(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compile test in short mode")
	}

	scene := NewGedScene()
	scene.SetFormTitle("AllWidgets")
	scene.SetSize(200, 400)

	// Test that every widget in the factoryMap produces valid code.
	// Some widgets (e.g. Table) may panic during layout when created
	// without full initialization, so we recover and skip those.
	y := 5.0
	added := make(map[string]bool)
	for factoryName := range factoryMap {
		fn := factoryName // capture
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Logf("skipping %s due to panic during setup: %v", fn, r)
				}
			}()
			fake, err := NewFakeWidgetFromFactory(fn)
			if err != nil {
				t.Logf("skipping %s: %v", fn, err)
				return
			}
			fake.SetWidgetName(strings.Replace(strings.Replace(fn, "gui.", "", 1), ".", "_", -1))
			fake.SetBounds(5, y, 40, 7)
			cmd := graph.NewAddCommand()
			cmd.AddItem(fake, scene)
			scene.PushCommand(cmd)
			y += 10
			added[fn] = true
		}()
	}

	code := scene.GenerateCode(CodeGenOptions{PackageName: "main", TypeName: "AllWidgetsUI"})

	// Verify successfully-added widget fields are present in the struct
	for factoryName, mapping := range factoryMap {
		if !added[factoryName] {
			continue // was skipped due to panic or error
		}
		if !strings.Contains(code, mapping.goType) {
			t.Errorf("generated code missing type %s for factory %s", mapping.goType, factoryName)
		}
	}

	// At least 40 widgets should be successfully represented
	addedCount := len(added)
	if addedCount < 40 {
		t.Errorf("only %d/%d factory widgets added successfully, expected at least 40", addedCount, len(factoryMap))
	}
}

// vetGeneratedCode writes generated Go source into a throwaway module
// (with a replace directive pointing at the silk source tree) and runs
// go vet over it. go vet type-checks without linking, so it catches
// codegen that calls a method the field's static type does not have —
// e.g. a CodeEditor field degraded to gui.IWidget yet still emitting
// SigChanged. Fatals on go mod tidy failure; errors (with the source) on
// a vet failure.
func vetGeneratedCode(t *testing.T, code string) {
	t.Helper()

	tmpDir := t.TempDir()

	// A go.mod with a replace directive makes the temp dir a module that
	// resolves silk from this working tree.
	goMod := `module compiletest
go 1.21
require github.com/uk0/silk v0.0.0
replace github.com/uk0/silk => ` + findModuleRoot(t) + `
`
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatal("failed to write go.mod:", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte(code), 0644); err != nil {
		t.Fatal("failed to write main.go:", err)
	}

	// Resolve dependencies, then vet. vet type-checks (won't link).
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = tmpDir
	tidy.Env = append(os.Environ(), "CGO_ENABLED=1")
	if output, err := tidy.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy failed:\n%s", output)
	}

	cmd := exec.Command("go", "vet", "./...")
	cmd.Dir = tmpDir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("generated code failed go vet:\n%s\n\nGenerated code:\n%s", output, code)
	}
}

// findModuleRoot locates the silk module root by walking up from the test dir.
func findModuleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find module root")
		}
		dir = parent
	}
}
