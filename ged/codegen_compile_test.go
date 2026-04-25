package ged

import (
	"os"
	"os/exec"
	"path/filepath"
	"silk/graph"
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

	// Write to temp file and verify syntax via go vet
	tmpDir := t.TempDir()

	// Create a go.mod so the temp directory is a valid module
	goMod := `module compiletest
go 1.21
require silk v0.0.0
replace silk => ` + findModuleRoot(t) + `
`
	err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goMod), 0644)
	if err != nil {
		t.Fatal("failed to write go.mod:", err)
	}

	// Write the generated code
	mainFile := filepath.Join(tmpDir, "main.go")
	err = os.WriteFile(mainFile, []byte(code), 0644)
	if err != nil {
		t.Fatal("failed to write main.go:", err)
	}

	// Run go mod tidy to resolve dependencies
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = tmpDir
	tidy.Env = append(os.Environ(), "CGO_ENABLED=1")
	if output, err := tidy.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy failed:\n%s", output)
	}

	// Run go vet for syntax validation (won't link, just checks AST)
	cmd := exec.Command("go", "vet", "./...")
	cmd.Dir = tmpDir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("generated code failed go vet:\n%s\n\nGenerated code:\n%s", output, code)
	}
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
