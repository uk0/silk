package main

import (
	"bytes"
	"encoding/json"
	"github.com/uk0/silk/diagnostic"
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

func TestMacroCLIJSONDiagnostic(t *testing.T) {
	var out bytes.Buffer
	err := diagnostic.New(diagnostic.MacroUnknown, "unknown macro Missing", nil).At(diagnostic.Location{File: "中文.silkgo", Line: 4, Column: 1})
	if reportDiagnostic(&out, err, true) != 1 {
		t.Fatal("exit code")
	}
	var got diagnostic.Error
	if e := json.Unmarshal(out.Bytes(), &got); e != nil {
		t.Fatal(e, out.String())
	}
	if got.Code != diagnostic.MacroUnknown || got.Location.Line != 4 || got.Hint == "" {
		t.Fatal(got)
	}
	out.Reset()
	reportDiagnostic(&out, err, false)
	if !strings.Contains(out.String(), "[SILK-MAC008]") || !strings.Contains(out.String(), "Hint:") {
		t.Fatal(out.String())
	}
}
