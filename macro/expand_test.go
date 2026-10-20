package macro

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const fixture = `package sample
//silk:macro Accessor(Owner, Field, Type)
func (s *${Owner}) Set${Field}(v ${Type}) { s.${Field} = v }
//silk:end
type Machine struct { Pressure float64; Count int }
//silk:expand Accessor(Machine, Pressure, float64)
//silk:expand Accessor(Machine, Count, int)
`

func TestExpandGoCompilesAndRuns(t *testing.T) {
	out, err := Expand("device.silkgo", []byte(fixture))
	if err != nil {
		t.Fatal(err)
	}
	again, err := Expand("device.silkgo", []byte(fixture))
	if err != nil || !bytes.Equal(out, again) {
		t.Fatal("nondeterministic expansion", err)
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module sample\n\ngo 1.26\n"), 0644)
	if err := WriteFile(filepath.Join(dir, "device_gen.go"), out); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "device_test.go"), []byte(`package sample
import "testing"
func TestGenerated(t *testing.T){m:=Machine{};m.SetPressure(42.5);m.SetCount(7);if m.Pressure!=42.5||m.Count!=7{t.Fatal(m)}}`), 0644)
	cmd := exec.Command("go", "test", "-count=1", ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=local")
	if log, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated Go failed: %v\n%s\n%s", err, log, out)
	}
}
func TestExpandDirectivesAreLexicalAndCRLFSafe(t *testing.T) {
	source := fixture + "\nvar literal = `\n//silk:expand Fake(nope)\n`\n/*\n//silk:macro NotReal()\n*/\n"
	for _, src := range []string{source, strings.ReplaceAll(source, "\n", "\r\n")} {
		out, err := Expand("x.silkgo", []byte(src))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(out, []byte("//silk:expand Fake(nope)")) {
			t.Fatal("literal was changed")
		}
	}
}
func TestExpandRejectsMalformedAndUnboundedInput(t *testing.T) {
	for _, src := range []string{
		"package p\n//silk:expand Missing()", "package p\n//silk:end", "package p\n//silk:macro A(x,x)\n//silk:end", "package p\n//silk:macro A(x)\n", "package p\n//silk:macro A(x)\n${bad}\n//silk:end", "package p\n//silk:macro A()\n//silk:expand A()\n//silk:end", "package p\n//silk:macro A()\n//silk:macro B()\n//silk:end", "package p\n//silk:macro A()\n//silk:end\n//silk:macro A()\n//silk:end", "package p\n//silk:macro A(x)\n//silk:end\n//silk:expand A()", "package p\n//silk:unknown", "package p //silk:expand A()", "package p\n//silk:macro A(x)\nfunc ${x}(\n//silk:end\n//silk:expand A(F)", strings.Repeat("x", MaxSourceBytes+1),
		"package p\n//silk:macro A()\n//silk:end\n" + strings.Repeat("//silk:expand A()\n", maxExpansions+1),
		"package p\n//silk:macro A(x)\nvar S = `" + strings.Repeat("${x}", 5000) + "`\n//silk:end\n//silk:expand A(" + strings.Repeat("a", 1000) + ")",
	} {
		if _, err := Expand("bad.silkgo", []byte(src)); err == nil {
			t.Errorf("accepted bad input %.100q", src)
		}
	}
}
func TestWriteFilePreservesUserFilesAndRejectsSymlinks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.go")
	original := []byte("package mine\n")
	os.WriteFile(path, original, 0644)
	generated, _ := Expand("x.silkgo", []byte(fixture))
	if err := WriteFile(path, generated); err == nil {
		t.Fatal("overwrote handwritten file")
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, original) {
		t.Fatal("user file changed")
	}
	owned := filepath.Join(dir, "owned.go")
	if err := WriteFile(owned, generated); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(owned, generated); err != nil {
		t.Fatal("replace owned file", err)
	}
	link := filepath.Join(dir, "link.go")
	if err := os.Symlink(owned, link); err != nil {
		t.Logf("symlink creation unavailable: %v", err)
		return
	}
	if err := WriteFile(link, generated); err == nil {
		t.Fatal("accepted symlink")
	}
}
func FuzzExpand(f *testing.F) {
	for _, s := range []string{fixture, "package p\n", "//silk:macro A(x)\n${x}\n//silk:end"} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, src []byte) {
		if len(src) > MaxSourceBytes {
			return
		}
		out, err := Expand("fuzz.silkgo", src)
		if err == nil {
			if !isGenerated(out) || len(out) > MaxOutputBytes {
				t.Fatal("invalid output contract")
			}
		}
	})
}

func TestRejectIncompleteGoFiles(t *testing.T) {
	for _, src := range []string{";", "", "func F() {}", "x := 1"} {
		if _, err := Expand("fragment.silkgo", []byte(src)); err == nil {
			t.Fatalf("accepted Go fragment %q", src)
		}
	}
}

func TestBuildConstraintsPreserved(t *testing.T) {
	out, err := Expand("tagged.silkgo", []byte("//go:build windows\n\npackage p\n"))
	if err != nil || !isGenerated(out) || !bytes.Contains(out, []byte("//go:build windows\n")) {
		t.Fatalf("invalid build-tagged output: %s, %v", out, err)
	}
	path := filepath.Join(t.TempDir(), "tagged_gen.go")
	for i := 0; i < 2; i++ {
		if err := WriteFile(path, out); err != nil {
			t.Fatal(err)
		}
	}
	if isGenerated([]byte("package p\nconst s = `" + Header + "`")) {
		t.Fatal("accepted a marker inside a string")
	}
}
