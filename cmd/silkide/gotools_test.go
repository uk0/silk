package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// requireGofmt skips the calling test when `gofmt` is not on PATH —
// the silkide repo's CI ships a Go toolchain, but isolated environments
// (e.g. a freshly-cloned dev container before the install step) may
// not. Better to skip than to fail on missing toolchain.
func requireGofmt(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("gofmt"); err != nil {
		t.Skipf("gofmt not on PATH: %v", err)
	}
}

// TestGofmtSourceRoundTrip: a well-known under-spaced "func  main()"
// goes in, the canonical "func main()" comes out. Locks in the
// happy path so a future refactor that swaps the exec for a different
// formatter (go/format.Source) still produces toolchain-identical
// output.
func TestGofmtSourceRoundTrip(t *testing.T) {
	requireGofmt(t)

	in := "package main\nfunc  main(){println(1)}"
	got, err := gofmtSource(in)
	if err != nil {
		t.Fatalf("gofmtSource returned error: %v", err)
	}
	// gofmt normalises whitespace + line breaks: "func main()" with a
	// single space, the brace on the same line, a newline before the
	// closing brace, and a trailing newline at EOF.
	for _, want := range []string{
		"package main",
		"func main() {",
		"println(1)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("gofmtSource output missing %q\n---\n%s", want, got)
		}
	}
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("gofmtSource output should end with newline, got %q", got)
	}
}

// TestGofmtSourceMalformedReturnsError: a syntax-broken buffer must
// surface a non-nil error and an empty string, never panic. The
// on-save path in saveActiveEditorToDisk relies on this contract to
// fall back to "save unformatted + toast warn" when WIP code can't
// be parsed.
func TestGofmtSourceMalformedReturnsError(t *testing.T) {
	requireGofmt(t)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("gofmtSource panicked on malformed input: %v", r)
		}
	}()

	got, err := gofmtSource("package main\nfunc main( {")
	if err == nil {
		t.Fatalf("gofmtSource should have returned an error on malformed input, got output %q", got)
	}
	if got != "" {
		t.Errorf("gofmtSource error path should return empty string, got %q", got)
	}
}

// TestLoadModulePathWalksUp: drop a go.mod at the tree root and call
// loadModulePath from a deeply-nested subdirectory; it must walk up
// and find the module path. Mirrors the real silkide use case where
// the binary's cwd is somewhere under the project root.
func TestLoadModulePathWalksUp(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"),
		[]byte("module silk/example\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	got := loadModulePath(sub)
	if got != "silk/example" {
		t.Errorf("loadModulePath(%q) = %q, want %q", sub, got, "silk/example")
	}
}

// TestLoadModulePathMissingReturnsEmpty: walking up from a directory
// whose ancestors hold no go.mod must terminate and return "" rather
// than loop forever. Uses t.TempDir() to guarantee no go.mod siblings.
// Skip when even the temp dir's filesystem root happens to be on a
// branch that contains a go.mod (the real repo would do this if the
// test runner inherits the silk module's root); the contract we care
// about is "doesn't hang", and t.TempDir's typical /tmp parent has no
// go.mod.
func TestLoadModulePathMissingReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	// Confirm no ancestor go.mod to avoid a false positive from a
	// host-level checkout.
	for d := dir; d != filepath.Dir(d); d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			t.Skipf("ancestor go.mod at %s would shadow the missing-case test", d)
		}
	}
	if got := loadModulePath(dir); got != "" {
		t.Errorf("loadModulePath(missing) = %q, want empty", got)
	}
}

// TestLoadModulePathStripsInlineComment: "module silk/example // foo"
// should return "silk/example", not the comment-tainted form. Inline
// comments are rare but legal in go.mod; the 5-line scan we ship needs
// to handle them.
func TestLoadModulePathStripsInlineComment(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"),
		[]byte("module silk/example // optional comment\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := loadModulePath(root); got != "silk/example" {
		t.Errorf("loadModulePath = %q, want %q", got, "silk/example")
	}
}
