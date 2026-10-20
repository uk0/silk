package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/uk0/silk/macro"
)

func TestMacroMenuGuardsAndOutputPreservesDirtyEditor(t *testing.T) {
	w := testWorkbench(t)
	path := filepath.Join(t.TempDir(), "machine.silkgo")
	source := []byte("package p\n//silk:macro Decl(Name)\nvar ${Name} = 42\n//silk:end\n//silk:expand Decl(Value)\n")
	os.WriteFile(path, source, 0644)
	if !openFileInEditor(w.tabs, path) {
		t.Fatal("open")
	}
	w.activate(modeEdit)
	if !menuCommandButton(buildWorkbenchSubMenu("Tools", w.tabs, w.canvas), "tools.expandMacro").IsEnabled() {
		t.Fatal("macro action missing/disabled")
	}
	w.activate(modeDesign)
	if canExpandMacro(w.tabs, w.canvas) {
		t.Fatal("hidden editor eligible")
	}
	w.activate(modeEdit)
	generated, err := macro.Expand(path, source)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeMacroOutput(w.tabs, path, generated); err != nil {
		t.Fatal(err)
	}
	output := macroOutputPath(path)
	if !openFileInEditor(w.tabs, output) {
		t.Fatal("open generated")
	}
	target := activeEditor(w.tabs)
	target.ReplaceAllText("package handwritten\n")
	if err := writeMacroOutput(w.tabs, path, generated); err == nil {
		t.Fatal("overwrote dirty editor")
	}
	disk, _ := os.ReadFile(output)
	if string(disk) != string(generated) || target.Text() != "package handwritten\n" {
		t.Fatal("dirty protection changed output")
	}
	if canExpandMacro(w.tabs, w.canvas) {
		t.Fatal("ordinary Go file eligible")
	}
}
