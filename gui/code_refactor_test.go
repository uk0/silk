package gui

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// RenameSymbol: AST-based single-file symbol rename.
//
// Identifiers are located via go/parser + go/ast and rewritten in-place by
// byte offset. Comments, strings, and import paths must be untouched.
// ---------------------------------------------------------------------------

func TestRefactorRenameSymbolFunctionDeclAndCall(t *testing.T) {
	src := `package main

func helper() int { return 1 }

func main() {
	_ = helper()
	_ = helper()
}
`
	out, err := RenameSymbol(src, "helper", "doIt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(out, "helper") {
		t.Errorf("output still contains old name 'helper':\n%s", out)
	}
	// Decl + both calls.
	if got := strings.Count(out, "doIt"); got != 3 {
		t.Errorf("expected 3 occurrences of 'doIt', got %d:\n%s", got, out)
	}
}

func TestRefactorRenameSymbolLocalKnownLimitationNotScopeAware(t *testing.T) {
	// Locks in the documented limitation: RenameSymbol is name-based, not
	// scope-aware, so a same-named local in an unrelated function gets
	// renamed too. A future scope-aware version should flip this test.
	src := `package p

func f() { x := 1; _ = x }
func g() { x := 2; _ = x }
`
	out, err := RenameSymbol(src, "x", "y")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(out, " x ") || strings.Contains(out, " x;") || strings.Contains(out, "= x") {
		t.Errorf("expected no surviving 'x' identifiers (name-based rename touches both functions):\n%s", out)
	}
	// Each function: one decl + one use = 2 idents. Two functions = 4.
	if got := strings.Count(out, "y"); got != 4 {
		t.Errorf("expected 4 occurrences of 'y', got %d:\n%s", got, out)
	}
}

func TestRefactorRenameSymbolDoesNotTouchCommentsOrStrings(t *testing.T) {
	src := `package main

// helper is great. Call helper everywhere.
func helper() string {
	const msg = "helper is the magic word; do not rename helper inside strings"
	return msg
}

func main() {
	_ = helper()
}
`
	out, err := RenameSymbol(src, "helper", "doIt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Comment text must be untouched.
	if !strings.Contains(out, "// helper is great. Call helper everywhere.") {
		t.Errorf("comment text was modified:\n%s", out)
	}
	// String literal must be untouched.
	if !strings.Contains(out, `"helper is the magic word; do not rename helper inside strings"`) {
		t.Errorf("string literal was modified:\n%s", out)
	}
	// Real identifiers: decl + call = 2 occurrences of new name.
	if got := strings.Count(out, "doIt"); got != 2 {
		t.Errorf("expected 2 identifier occurrences of 'doIt', got %d:\n%s", got, out)
	}
}

func TestRefactorRenameSymbolDoesNotTouchImportPath(t *testing.T) {
	// "fmt" inside the import path is a string literal, not an identifier,
	// so it must NOT be renamed when we target the identifier `fmt`.
	src := `package main

import "fmt"

func fmtHelper() { fmt.Println("hi") }
`
	out, err := RenameSymbol(src, "fmt", "format")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, `import "fmt"`) {
		t.Errorf("import path was modified:\n%s", out)
	}
	if !strings.Contains(out, "format.Println") {
		t.Errorf("identifier use was not renamed:\n%s", out)
	}
	// fmtHelper has 'fmt' as a prefix but is a distinct identifier — must
	// stay intact (we rename whole idents, not substrings).
	if !strings.Contains(out, "fmtHelper") {
		t.Errorf("substring-only match was incorrectly renamed:\n%s", out)
	}
}

func TestRefactorRenameSymbolInvalidNewNameEmpty(t *testing.T) {
	src := `package p
func f() {}
`
	out, err := RenameSymbol(src, "f", "")
	if err == nil {
		t.Fatal("expected error for empty newName")
	}
	if out != src {
		t.Errorf("src must be unchanged on error")
	}
}

func TestRefactorRenameSymbolInvalidNewNameDigitStart(t *testing.T) {
	src := `package p
func f() {}
`
	out, err := RenameSymbol(src, "f", "123foo")
	if err == nil {
		t.Fatal("expected error for newName starting with digit")
	}
	if out != src {
		t.Errorf("src must be unchanged on error")
	}
}

func TestRefactorRenameSymbolInvalidNewNameKeyword(t *testing.T) {
	src := `package p
func f() {}
`
	out, err := RenameSymbol(src, "f", "if")
	if err == nil {
		t.Fatal("expected error for keyword newName")
	}
	if out != src {
		t.Errorf("src must be unchanged on error")
	}
}

func TestRefactorRenameSymbolIdenticalNames(t *testing.T) {
	src := `package p
func f() { _ = f }
`
	out, err := RenameSymbol(src, "f", "f")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != src {
		t.Errorf("src must be unchanged when oldName == newName")
	}
}

func TestRefactorRenameSymbolUnparseableSourceReturnsError(t *testing.T) {
	src := `package p

func ;;; broken {{{
`
	out, err := RenameSymbol(src, "broken", "fixed")
	if err == nil {
		t.Fatal("expected parse error")
	}
	if out != src {
		t.Errorf("src must be unchanged when source does not parse")
	}
}

func TestRefactorRenameSymbolMissingIdentIsNoOp(t *testing.T) {
	src := `package p
func keep() {}
`
	out, err := RenameSymbol(src, "absent", "renamed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != src {
		t.Errorf("src must be unchanged when oldName has no occurrences")
	}
}

func TestRefactorRenameSymbolRejectsExistingTopLevelName(t *testing.T) {
	// Renaming `helper` to `other` would collide with the existing func
	// `other` and produce a duplicate package-level symbol.
	src := `package p

func helper() {}
func other() {}
`
	out, err := RenameSymbol(src, "helper", "other")
	if err == nil {
		t.Fatal("expected collision error")
	}
	if out != src {
		t.Errorf("src must be unchanged on collision")
	}
}

func TestRefactorRenameSymbolMethodReceiver(t *testing.T) {
	src := `package p

type T struct{ x int }

func (t *T) f() int { return t.x }

func use(p *T) int { return p.f() }
`
	out, err := RenameSymbol(src, "f", "g")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "func (t *T) g()") {
		t.Errorf("method decl not renamed:\n%s", out)
	}
	if !strings.Contains(out, "p.g()") {
		t.Errorf("method call not renamed:\n%s", out)
	}
}

func TestRefactorRenameSymbolPackageClauseNotRenamed(t *testing.T) {
	// The package clause identifier is a *different* thing from a code
	// symbol of the same name; renaming a symbol must NOT touch it.
	src := `package main

var main = 1
`
	out, err := RenameSymbol(src, "main", "renamed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(out, "package main") {
		t.Errorf("package clause was modified:\n%s", out)
	}
	if !strings.Contains(out, "var renamed = 1") {
		t.Errorf("var was not renamed:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// RenameSymbolCount: companion that also reports occurrence count.
// ---------------------------------------------------------------------------

func TestRefactorRenameSymbolCountReturnsOccurrences(t *testing.T) {
	src := `package p

func helper() int { return 1 }

func main() {
	_ = helper()
	_ = helper()
	_ = helper()
}
`
	n, out, err := RenameSymbolCount(src, "helper", "doIt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 4 { // 1 decl + 3 calls
		t.Errorf("count = %d, want 4", n)
	}
	if strings.Contains(out, "helper") {
		t.Errorf("output still contains old name:\n%s", out)
	}
}

func TestRefactorRenameSymbolCountZeroWhenAbsent(t *testing.T) {
	src := `package p
func keep() {}
`
	n, out, err := RenameSymbolCount(src, "absent", "renamed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("count = %d, want 0", n)
	}
	if out != src {
		t.Errorf("src must be unchanged")
	}
}

// ---------------------------------------------------------------------------
// isGoIdent: identifier-validity helper.
// ---------------------------------------------------------------------------

func TestRefactorIsGoIdent(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"a", true},
		{"_", true},
		{"_x", true},
		{"x1", true},
		{"x_y_z", true},
		{"X", true},
		{"123foo", false},
		{"foo-bar", false},
		{"foo bar", false},
		{"if", false},      // keyword
		{"func", false},    // keyword
		{"struct", false},  // keyword
		{"package", false}, // keyword
	}
	for _, c := range cases {
		if got := isGoIdent(c.in); got != c.want {
			t.Errorf("isGoIdent(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
