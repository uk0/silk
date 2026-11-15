package gui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestGoBodyProjectionExactAndImmutable(t *testing.T) {
	for _, body := range []string{"", ` _ = "" `, ` x:=1;_ = x `, "\n// body\nx:=1;_ =x\n", ` if true { _ = func(){ _ = "}" } } `, ` _ = []int{1,2}[0] `} {
		source := "package p\n// title\nfunc F(){" + body + "}\n// after\nfunc G(){}\n// tail"
		var p goSymbolIndex
		symbols := p.parse(source)
		if !p.valid || len(p.bodies) != 2 {
			t.Fatal("bad source metadata", p.valid, p.bodies)
		}
		original := append([]CodeSymbol(nil), symbols...)
		for _, extra := range []string{" x:=2; ", ` _ = "{} /* //"; `, " ", ` _ = struct{X int}{1}; `} {
			span := p.bodies[0]
			next := p.text[:span.start] + extra + p.text[span.start:]
			immutable := p.projection()
			oldText := immutable.text
			oldBodies := append([]goOutlineSpan(nil), immutable.bodies...)
			advanced := immutable.advance(next)
			if advanced == nil {
				t.Fatal("eligible UI body edit rejected", body, extra)
			}
			if immutable.text != oldText || !reflect.DeepEqual(immutable.bodies, oldBodies) {
				t.Fatal("published body metadata mutated")
			}
			if !p.reuseBodyEdit(next) {
				t.Fatal("private body edit rejected")
			}
			if got := parseGoCodeSymbolsText(next); !reflect.DeepEqual(got, p.symbols) || !reflect.DeepEqual(symbols, original) {
				t.Fatal("body edit changed exact symbols", got, p.symbols)
			}
		}
	}
}
func TestGoBodyProjectionRejectsEscapesAndHeaderEdits(t *testing.T) {
	source := "package p\nfunc F(){ x:=1;_ = x }\nfunc G(){}"
	for _, change := range []struct{ old, next string }{{"x:=1", `_ = "`}, {"x:=1", "/*"}, {"x:=1", "}"}, {"x:=1", "x:=1\n"}, {"F()", "Renamed()"}, {"x:=1", string([]byte{0xff})}} {
		var p goSymbolIndex
		p.parse(source)
		next := strings.Replace(source, change.old, change.next, 1)
		if p.reuseBodyEdit(next) {
			t.Fatal("unproved edit accepted", change)
		}
		if got, want := p.parse(next), parseGoCodeSymbolsText(next); !reflect.DeepEqual(got, want) {
			t.Fatal(got, want)
		}
	}
	// An empty body must expand, not shift its start past inserted tokens.
	var p goSymbolIndex
	p.parse("package p\nfunc F(){}\nfunc G(){}")
	span := p.bodies[0]
	next := p.text[:span.start] + "/" + p.text[span.start:]
	if !p.reuseBodyEdit(next) || p.bodies[0].start != span.start {
		t.Fatal("empty body did not expand in place")
	}
	next = p.text[:p.bodies[0].end] + "*" + p.text[p.bodies[0].end:]
	if p.reuseBodyEdit(next) {
		t.Fatal("cross-edit comment opener escaped the body")
	}
	if got, want := p.parse(next), parseGoCodeSymbolsText(next); !reflect.DeepEqual(got, want) {
		t.Fatal("fallback changed recovery", got, want)
	}
}
func TestGoBodyProjectionCapAndMixedRebasing(t *testing.T) {
	source := "package p\n// before\nfunc F(){" + strings.Repeat("x:=1;", 2000) + "}\n// after\nfunc G(){}"
	var p goSymbolIndex
	p.parse(source)
	next := strings.Replace(source, "x:=1;", "x:=2;", 1)
	if p.projection().advance(next) != nil {
		t.Fatal("large body scanned on UI")
	}
	if !p.reuseBodyEdit(next) {
		t.Fatal("bounded background worker lost large-body proof")
	}
	next = strings.Replace(p.text, "// before", "// much longer before", 1)
	if !p.reuseCommentEdit(next) {
		t.Fatal("outer comment could not rebase body")
	}
	next = strings.Replace(p.text, "x:=2;", "x:=3;", 1)
	if !p.reuseBodyEdit(next) {
		t.Fatal("body did not rebase after outer comment")
	}
	if got := parseGoCodeSymbolsText(next); !reflect.DeepEqual(got, p.symbols) {
		t.Fatal("mixed edits changed symbols")
	}
}

// Each operation below is TWO alternating changes, on unique declarations
// shaped like the native 5,000-function control. No native paint is measured.
func BenchmarkGoBodyProjection(b *testing.B) {
	var sourceBuilder strings.Builder
	sourceBuilder.WriteString("package p\n")
	for i := 0; i < 5000; i++ {
		fmt.Fprintf(&sourceBuilder, "func value%05d(x int) int { return x + %d }\n", i, i)
	}
	sourceBuilder.WriteString(`func Edit(){ _ = "a" }`)
	source := sourceBuilder.String()
	next := strings.Replace(source, `"a"`, `"longer text"`, 1)
	b.Run("full", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = parseGoCodeSymbolsText(next)
			_ = parseGoCodeSymbolsText(source)
		}
	})
	b.Run("worker", func(b *testing.B) {
		var p goSymbolIndex
		p.parse(source)
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			p.parse(next)
			p.parse(source)
		}
	})
	b.Run("UI", func(b *testing.B) {
		var p goSymbolIndex
		p.parse(source)
		view := p.projection()
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			view = view.advance(next)
			if view == nil {
				b.Fatal("projection missing")
			}
			view = view.advance(source)
			if view == nil {
				b.Fatal("projection missing")
			}
		}
	})
}

func TestGoBodyProjectionSpanBudgetAndUnchangedSharing(t *testing.T) {
	source := "package p\n" + strings.Repeat("func F(){_ = 1}\n", goProjectionUISpans+1) + "// tail"
	var p goSymbolIndex
	p.parse(source)
	view := p.projection()
	next := strings.Replace(source, "_ = 1", "_ = 12", 1)
	if view.advance(next) != nil {
		t.Fatal("unbounded span copies on UI")
	}
	if !p.reuseBodyEdit(next) {
		t.Fatal("worker lost safe edit beyond UI metadata budget")
	}
	// Same-size body edits do not move any spans. Even a huge document can
	// share immutable metadata instead of allocating a whole array per key.
	next = strings.Replace(source, "_ = 1", "_ = 2", 1)
	advanced := view.advance(next)
	if advanced == nil || &advanced.bodies[0] != &view.bodies[0] {
		t.Fatal("same-size edit copied unchanged body metadata")
	}
	next = source + " longer"
	advanced = view.advance(next)
	if advanced == nil || &advanced.bodies[0] != &view.bodies[0] {
		t.Fatal("EOF comment copied preceding body ranges")
	}
	if view.bodies[0] != advanced.bodies[0] || view.text != source {
		t.Fatal("shared immutable metadata was changed")
	}
	source = "package p\n" + strings.Repeat("// comment\n", goProjectionUISpans+1) + "func F(){}"
	p.parse(source)
	if p.projection().advance(strings.Replace(source, "comment", "longer comment", 1)) != nil {
		t.Fatal("unbounded comment-span copies on UI")
	}
}

func TestGoBodyProjectionLexicalBoundaryGridMatchesAST(t *testing.T) {
	for _, source := range []string{
		"func F(){ _ = \"😀\" }\nfunc G(){}", // synthetic package
		"package p\r\nfunc F(){\r\n _ = `a { // }`\r\n}\r\nfunc G(){}",
		"package p\ntype T[P any] struct{}\nfunc(t *T[P]) F() struct{V P}{ return struct{V P}{} }\nfunc G(){}",
		"package p\nvar f = func(){ _=1 };func G(){}",
	} {
		var original goSymbolIndex
		original.parse(source)
		if !original.valid || len(original.bodies) == 0 {
			t.Fatal("bad boundary fixture")
		}
		for _, part := range []string{" ", ` _ = "}"; `, "// trailing", "/*", "}", "{", "\x00", "\ufeff", "😀", "\n", "\r", "//line :0", `_ = '\'';`} {
			for _, span := range original.bodies {
				for _, pos := range []int{span.start, span.end} {
					var p goSymbolIndex
					before := p.parse(source)
					snapshot := p.projection()
					next := source[:pos] + part + source[pos:]
					advanced := snapshot.advance(next)
					want := parseGoCodeSymbolsText(next)
					if advanced != nil && !reflect.DeepEqual(before, want) {
						t.Fatalf("UI lexer changed fields: %q -> %q", source, next)
					}
					if got := p.parse(next); !reflect.DeepEqual(got, want) {
						t.Fatalf("worker lexer changed fields: %q -> %q", source, next)
					}
				}
			}
		}
	}
}
