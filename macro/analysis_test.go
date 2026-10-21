package macro

import (
	"errors"
	"github.com/uk0/silk/diagnostic"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAnalyzeNavigationScopesAndLexicalBoundaries(t *testing.T) {
	src := `package demo
var inert = "//silk:expand Fake()"
/* //silk:macro Fake() */
//silk:expand Setter(Sensor, Value, int)
//silk:macro Setter(Owner, Field, Type)
func (s *${Owner}) Set${Field}(v ${Type}) { s.${Field} = v }
//silk:end
//silk:macro Other(Field)
var ${Field} int
//silk:end
type Sensor struct {Value int}
`
	a := Analyze("中文.silkgo", []byte(src))
	if len(a.Diagnostics) != 0 {
		t.Fatal(a.Diagnostics)
	}
	if len(a.Symbols) != 6 {
		t.Fatalf("symbols: %+v", a.Symbols)
	}
	for _, probe := range []struct {
		token, want string
		kind        SymbolKind
	}{{"Setter(Sensor", "Setter", SymbolMacro}, {"${Owner}", "Owner", SymbolParameter}, {"${Field}", "Field", SymbolParameter}} {
		target, ok := a.DefinitionAt(strings.Index(src, probe.token) + 2)
		if !ok || target.Name != probe.want || target.Kind != probe.kind || src[target.Selection.Start:target.Selection.End] != probe.want {
			t.Fatalf("%s: %+v %v", probe.token, target, ok)
		}
	}
	last := strings.LastIndex(src, "${Field}")
	target, ok := a.DefinitionAt(last + 3)
	if !ok || target.Selection.Start < strings.Index(src, "Other(Field)") {
		t.Fatal("parameter leaked across scope", target)
	}
	for _, h := range a.Highlights {
		if h.Start < 0 || h.End > len(src) || h.Start >= h.End {
			t.Fatal(h)
		}
		if strings.Contains(src[h.Start:h.End], "Fake") {
			t.Fatal("inert text highlighted")
		}
	}
}
func TestMacroErrorsHaveStableCodesAndSourceLocations(t *testing.T) {
	cases := []struct {
		src  string
		code diagnostic.Code
		line int
	}{
		{"package p\n//silk:expand Missing()\n", diagnostic.MacroUnknown, 2},
		{"package p\n//silk:macro M(X)\nvar ${X} int\n", diagnostic.MacroUnclosed, 2},
		{"package p\n//silk:macro M(中文)\n//silk:end\n", diagnostic.MacroDeclaration, 2},
		{"package p\n//silk:macro M(X)\nvar ${X} int\n//silk:end\n//silk:expand M()\n", diagnostic.MacroArity, 5},
		{"package p\n//silk:macro M()\nvar ! int\n//silk:end\n//silk:expand M()\n", diagnostic.MacroGeneratedSyntax, 5},
		{"package p\nvar ! int\n", diagnostic.MacroGeneratedSyntax, 2},
	}
	for _, tt := range cases {
		t.Run(string(tt.code)+tt.src, func(t *testing.T) {
			_, err := Expand("example.silkgo", []byte(tt.src))
			var d *diagnostic.Error
			if !errors.As(err, &d) || d.Code != tt.code || d.Location.Line != tt.line || d.Hint == "" {
				t.Fatalf("got %+v", err)
			}
			a := Analyze("example.silkgo", []byte(tt.src))
			if len(a.Diagnostics) != 1 || a.Diagnostics[0].Code != tt.code {
				t.Fatal(a.Diagnostics)
			}
		})
	}
}
func TestAnalyzeCRLFForwardUseAndIncompleteBuffer(t *testing.T) {
	src := "package p\r\n//silk:expand M(X)\r\n//silk:macro M(Name)\r\nvar ${Name} int\r\n//silk:end\r\nfunc broken("
	a := Analyze("test.silkgo", []byte(src))
	s, ok := a.DefinitionAt(strings.Index(src, "M(X)"))
	if !ok || s.Name != "M" || len(a.Diagnostics) != 1 {
		t.Fatal(a)
	}
	if src[s.Selection.Start:s.Selection.End] != "M" {
		t.Fatal(s)
	}
}
func TestMacroWriteDiagnosticRetainsFilesystemCause(t *testing.T) {
	data, err := Expand("test.silkgo", []byte("package p\n"))
	if err != nil {
		t.Fatal(err)
	}
	err = WriteFile(filepath.Join(t.TempDir(), "missing", "out.go"), data)
	if diagnostic.CodeOf(err) != diagnostic.MacroOutputWrite || !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}

func TestAnalyzeUsesPhysicalPositionsAndEmbeddedCR(t *testing.T) {
	src := "package p\n//line fake.go:999\n//silk:macro Decl\r(Name)\nvar ${Name} int\n//silk:end\n//silk:expand Decl(Value)\n"
	a := Analyze("real.silkgo", []byte(src))
	if len(a.Diagnostics) > 0 {
		t.Fatal(a.Diagnostics)
	}
	s, ok := a.DefinitionAt(strings.LastIndex(src, "Decl(Value)") + 2)
	if !ok || src[s.Selection.Start:s.Selection.End] != "Decl" {
		t.Fatal(s)
	}
	src = strings.Replace(src, "${Name}", "${Unknown}", 1)
	a = Analyze("real.silkgo", []byte(src))
	if len(a.Diagnostics) != 1 || a.Diagnostics[0].Location.Line != 4 || a.Diagnostics[0].Location.File != "real.silkgo" {
		t.Fatal(a.Diagnostics)
	}
}
func FuzzAnalyze(f *testing.F) {
	f.Add([]byte("package p\n//silk:macro M(X)\nvar ${X} int\n//silk:end\n//silk:expand M(A)\n"))
	f.Add([]byte("package p\nvar raw = `//silk:macro Inert()`\n"))
	f.Fuzz(func(t *testing.T, src []byte) {
		if len(src) > MaxSourceBytes {
			return
		}
		a := Analyze("fuzz.silkgo", src)
		valid := func(r Span) bool { return r.Start >= 0 && r.End >= r.Start && r.End <= len(src) }
		for _, s := range a.Symbols {
			if !valid(s.Selection) || !valid(s.Range) {
				t.Fatal(s)
			}
		}
		for _, h := range a.Highlights {
			if !valid(h.Span) {
				t.Fatal(h)
			}
		}
		for _, r := range a.References {
			if !valid(r.Span) || r.Target < 0 || r.Target >= len(a.Symbols) {
				t.Fatal(r)
			}
		}
	})
}
