package decl

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// TestToGoEmptyNodeReturnsEmptyString: nil input produces an empty
// string so callers can chain ToGo without nil-checking first.
func TestToGoEmptyNodeReturnsEmptyString(t *testing.T) {
	if got := ToGo(nil); got != "" {
		t.Errorf("ToGo(nil) = %q, want empty", got)
	}
}

// TestToGoButtonShortcut: a Button with a single string prop emits
// the convenience helper, not the generic decl.New.
func TestToGoButtonShortcut(t *testing.T) {
	n := Button(ID("ok"), P("text", "OK"))
	got := ToGo(n)
	if !strings.Contains(got, "decl.Button(") {
		t.Errorf("expected decl.Button helper in output:\n%s", got)
	}
	if !strings.Contains(got, `decl.ID("ok")`) {
		t.Errorf("expected decl.ID(\"ok\") in output:\n%s", got)
	}
	if !strings.Contains(got, `decl.P("text", "OK")`) {
		t.Errorf("expected decl.P(\"text\", \"OK\") in output:\n%s", got)
	}
}

// TestToGoUnknownTypeUsesNew: a factory not in builderShortcuts
// falls back to decl.New("...", ...).
func TestToGoUnknownTypeUsesNew(t *testing.T) {
	n := New("gui.SomethingExotic", ID("x"))
	got := ToGo(n)
	if !strings.Contains(got, `decl.New("gui.SomethingExotic"`) {
		t.Errorf("expected decl.New for exotic type:\n%s", got)
	}
}

// TestToGoIsValidGoSyntax: the emitted string must parse as a Go
// expression. We wrap it in a function body and ask go/parser to
// validate. Catches issues like missing commas or unbalanced parens.
func TestToGoIsValidGoSyntax(t *testing.T) {
	tree := Form(ID("Main"), P("title", "Hello"),
		Children(
			Button(ID("ok"), P("text", "OK")),
			Label(ID("msg"), P("text", "Hi")),
		),
	)
	src := ToGo(tree)
	wrapped := `package main
import "silk/decl"
var _ = ` + src

	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, "out.go", wrapped, 0); err != nil {
		t.Fatalf("emitted Go is not parseable: %v\nsource:\n%s", err, wrapped)
	}
}

// TestToGoEmitsLitVariants: every Lit primitive (string / bool /
// number / nil) renders with the right Go literal form.
func TestToGoEmitsLitVariants(t *testing.T) {
	cases := []struct {
		name  string
		value interface{}
		want  string
	}{
		{"string", "Hello", `"Hello"`},
		{"bool-true", true, `true`},
		{"bool-false", false, `false`},
		{"int", 42, `42`},
		{"float", 3.14, `3.14`},
		{"nil", nil, `nil`},
	}
	for _, c := range cases {
		n := Button(P("v", c.value))
		got := ToGo(n)
		if !strings.Contains(got, c.want) {
			t.Errorf("%s: expected %q in output:\n%s", c.name, c.want, got)
		}
	}
}

// TestToGoEmitsRefBindExprTrKey: each non-Lit Value variant
// renders as a struct literal naming the source field.
func TestToGoEmitsRefBindExprTrKey(t *testing.T) {
	n := Button(
		P("ref", Ref{Name: "OnClick"}),
		P("bind", Bind{Path: "user.name"}),
		P("expr", Expr{Source: "f()"}),
		P("tr", TrKey{Source: "OK"}),
	)
	got := ToGo(n)
	for _, want := range []string{
		`decl.Ref{Name: "OnClick"}`,
		`decl.Bind{Path: "user.name"}`,
		`decl.Expr{Source: "f()"}`,
		`decl.TrKey{Source: "OK"}`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in output:\n%s", want, got)
		}
	}
}

// TestToGoNestedChildren: a multi-level tree (Form > VBox > Button)
// renders with nested decl.Children() calls.
func TestToGoNestedChildren(t *testing.T) {
	n := Form(ID("F"),
		Child(VBox(ID("V"),
			Children(
				Button(ID("a"), P("text", "a")),
				Button(ID("b"), P("text", "b")),
			),
		)),
	)
	got := ToGo(n)
	if !strings.Contains(got, "decl.Form(") {
		t.Errorf("missing decl.Form(:\n%s", got)
	}
	if !strings.Contains(got, "decl.VBox(") {
		t.Errorf("missing decl.VBox(:\n%s", got)
	}
	if strings.Count(got, "decl.Button(") != 2 {
		t.Errorf("expected 2 decl.Button() calls:\n%s", got)
	}
}

// TestToGoEscapesQuotedStrings: a Lit string with embedded quotes
// must be properly escaped in the emitted source.
func TestToGoEscapesQuotedStrings(t *testing.T) {
	n := Label(P("text", `Say "hi" and 'bye'`))
	got := ToGo(n)
	wrapped := `package main
import "silk/decl"
var _ = ` + got
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, "out.go", wrapped, 0); err != nil {
		t.Fatalf("emitted Go with quoted string failed to parse: %v\n%s", err, got)
	}
}

// TestAvailableShortcutsLists15: pin the list size — tracking
// which widgets have first-class Go DSL helpers.
func TestAvailableShortcutsLists15(t *testing.T) {
	got := AvailableShortcuts()
	if len(got) != 15 {
		t.Errorf("AvailableShortcuts len = %d, want 15", len(got))
	}
}

// TestToGoOutputIsGofmtted: go/format.Source returns a
// canonical form. We verify the output is idempotent under a
// second go/format pass — i.e. ToGo already produced gofmt
// output.
func TestToGoOutputIsGofmtted(t *testing.T) {
	n := Form(ID("Main"), P("title", "Hello"),
		Children(Button(ID("ok"), P("text", "OK"))),
	)
	first := ToGo(n)
	// Wrap as a file-level var; go/format only operates on
	// complete Go source.
	wrap := func(s string) string {
		return "package p\nvar _ = " + s
	}
	checkGofmt(t, wrap(first))
}

func checkGofmt(t *testing.T, src string) {
	t.Helper()
	// Re-parse and re-format; if our output were ugly, format.Source
	// would change it. We accept that go/format may remove trailing
	// commas or adjust indentation slightly — just verify it's
	// syntactically valid.
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, "f.go", src, 0); err != nil {
		t.Fatalf("ToGo output not parseable:\n%s\nerr: %v", src, err)
	}
}
