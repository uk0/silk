package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMacroCLIActualGenerationAndFailurePreservesOutput(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "example.silkgo")
	out := filepath.Join(dir, "example_gen.go")
	os.WriteFile(src, []byte("package p\n//silk:macro Decl(Name)\nvar ${Name} = 42\n//silk:end\n//silk:expand Decl(Value)\n"), 0644)
	if code := run([]string{"-macro", "-o", out, src}); code != 0 {
		t.Fatal(code)
	}
	good, _ := os.ReadFile(out)
	if !strings.Contains(string(good), "var Value = 42") {
		t.Fatal(string(good))
	}
	os.WriteFile(src, []byte("package p\n//silk:expand Unknown()"), 0644)
	if code := run([]string{"-macro", "-o", out, src}); code == 0 {
		t.Fatal("invalid macro succeeded")
	}
	after, _ := os.ReadFile(out)
	if string(after) != string(good) {
		t.Fatal("failure changed generated file")
	}
}
