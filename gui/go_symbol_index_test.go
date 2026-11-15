package gui

import (
	"reflect"
	"strings"
	"testing"
)

func TestGoSymbolIndexCommentEditsMatchFullParser(t *testing.T) {
	for _, source := range []string{
		"package p\n// title\nfunc F() {}\n// tail",
		"// title\nfunc F() {}\n// tail", // synthetic package offsets
		"package p\nfunc F() {\n // body\n}\n// tail",
		"package p\n//line virtual.go:900\nfunc F() {}\n// tail",
		"package p\ntype T[P any] struct{}\nfunc (t *T[P]) F(x int) int {\n // body\n return x\n}\n// tail",
		"package p\n//\nfunc F() {}\n// tail\r\n",
	} {
		t.Run(source, func(t *testing.T) {
			var p goSymbolIndex
			want := parseGoCodeSymbolsText(source)
			if got := p.parse(source); !p.valid || !reflect.DeepEqual(got, want) || len(p.comments) == 0 {
				t.Fatal("metadata changed initial parse", got, want, p.valid)
			}
			storage := &p.symbols[0]
			for _, payload := range []string{"中文 / * { } ` \" //", "short", "", "much longer comment text"} {
				// Edit every eligible payload, exercising offset rebasing for
				// later comments after both growth and shrinkage.
				for i := range p.comments {
					span := p.comments[i]
					end := span.end
					if end > span.start && p.text[end-1] == '\r' {
						end--
					}
					next := p.text[:span.start] + payload + p.text[end:]
					if !p.reuseCommentEdit(next) || &p.symbols[0] != storage {
						t.Fatal("eligible payload rebuilt symbols", payload)
					}
					if got := parseGoCodeSymbolsText(next); !reflect.DeepEqual(got, p.symbols) {
						t.Fatalf("reused projection differs from full parser: %q\ngot=%v want=%v", next, p.symbols, got)
					}
				}
			}
		})
	}
}

func TestGoSymbolIndexStructuralAndHeaderEditsFallBack(t *testing.T) {
	cases := []struct{ source, old, replacement string }{
		{"package p\nfunc F(\n// header\nx int,\n) {}", "header", "changed detail"},
		{"package p\ntype T struct{}\nfunc (\n// receiver\nt *T,\n) F() {}", "receiver", "changed receiver"},
		{"package p\nvar x = `// literal`\nfunc F() {}", "literal", "different"},
		{"package p\n/* block */\nfunc F() {}", "block", "different"},
		{"package p\n// comment\nfunc F() {}", "comment", "comment\nfunc G() {}"},
		{"package p\n// comment\nfunc F() {}", "//", "/*"},
		{"package p\nfunc F() {}\n// comment", "F", "Renamed"},
		{"package p\n// comment\nfunc F() {}", "comment", "carriage\rreturn"},
		{"// comment\npackage p\nfunc F() {}", " comment", "line :0"},
		{"//line valid.go:10\npackage p\nfunc F() {}", "valid.go:10", ":0"},
		{"// comment\npackage p\nfunc F() {}", " comment", "go:build invalid &&"},
		{"// comment\npackage p\nfunc F() {}", "comment", "\x00"},
		{"// comment\npackage p\nfunc F() {}", "comment", "\xff"},
		{"// comment\npackage p\nfunc F() {}", "comment", "\ufeff"},
	}
	for _, tc := range cases {
		t.Run(tc.source+"/"+tc.old, func(t *testing.T) {
			var p goSymbolIndex
			p.parse(tc.source)
			if !p.valid {
				t.Fatal("fixture is not a complete Go parse")
			}
			next := strings.Replace(tc.source, tc.old, tc.replacement, 1)
			if p.reuseCommentEdit(next) {
				t.Fatal("structural/header edit reused stale projection")
			}
			if got, want := p.parse(next), parseGoCodeSymbolsText(next); !reflect.DeepEqual(got, want) {
				t.Fatal(got, want)
			}
		})
	}
}

func TestGoSymbolIndexRecoveryAndEveryTruncationMatch(t *testing.T) {
	source := "package p\n// before\ntype T[P interface{ ~int | ~string }] struct{}\nfunc (t *T[P]) F(\n// header\nx P,\n) struct{ V P } {\n// body\nreturn struct{ V P }{x}\n}\n// after"
	for n := 0; n <= len(source); n++ {
		text := source[:n]
		var p goSymbolIndex
		if got, want := p.parse(text), parseGoCodeSymbolsText(text); !reflect.DeepEqual(got, want) {
			t.Fatalf("truncation %d changed AST recovery: %v vs %v", n, got, want)
		}
		for _, span := range append([]goOutlineSpan(nil), p.comments...) {
			next := text[:span.start] + " edited " + text[span.start:]
			got := p.parse(next)
			if want := parseGoCodeSymbolsText(next); !reflect.DeepEqual(got, want) {
				t.Fatalf("truncation %d comment edit differs: %v vs %v", n, got, want)
			}
			p.parse(text)
		}
	}
}

func TestGoSymbolIndexMultipleRangesAndRepairedInvalidSource(t *testing.T) {
	for _, source := range []string{"package p\nfunc F() {\n// one\n", "package p\nfunc F(x ???) {}\n// one", "package p\n// \xff\nfunc F() {}\n// one"} {
		var p goSymbolIndex
		p.parse(source)
		next := strings.Replace(source, "one", "two", 1)
		if got, want := p.parse(next), parseGoCodeSymbolsText(next); !reflect.DeepEqual(got, want) {
			t.Fatal(got, want)
		}
		next = "package p\n// first\nfunc F() {}\n// last"
		p.parse(next)
		changed := strings.ReplaceAll(next, "first", "FIRST")
		changed = strings.ReplaceAll(changed, "last", "LAST")
		if p.reuseCommentEdit(changed) {
			t.Fatal("two disjoint edits were treated as one comment payload")
		}
		if got, want := p.parse(changed), parseGoCodeSymbolsText(changed); !reflect.DeepEqual(got, want) {
			t.Fatal(got, want)
		}
	}
}

func TestGoSymbolIndexReuseDoesNotAllocateOrMutatePublishedSymbols(t *testing.T) {
	a := "package p\n" + strings.Repeat("func F(x int) int { return x }\n", 5000) + "// aaa"
	b := a[:len(a)-3] + "longer中文"
	var p goSymbolIndex
	shared := p.parse(a)
	want := append([]CodeSymbol(nil), shared...)
	if n := testing.AllocsPerRun(100, func() {
		if !p.reuseCommentEdit(b) || !p.reuseCommentEdit(a) {
			t.Fatal("eligible comment edit was not reused")
		}
	}); n != 0 {
		t.Fatalf("comment rebase allocated %v times", n)
	}
	if !reflect.DeepEqual(shared, want) || &shared[0] != &p.symbols[0] {
		t.Fatal("published symbols were mutated/replaced")
	}
	combined := mergeAdditionalCodeSymbols(shared, []CodeSymbol{{Name: "Macro", Line: 0, Kind: SymMacro}})
	combined[1].Name = "mutated"
	if !reflect.DeepEqual(shared, want) {
		t.Fatal("additional provider symbols mutated incremental base")
	}
}
