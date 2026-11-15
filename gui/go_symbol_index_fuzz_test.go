package gui

import (
	"reflect"
	"testing"
)

func FuzzGoSymbolIndexEditsMatchFullParser(f *testing.F) {
	for _, source := range []string{
		"package p\nfunc F(){}\nfunc G(){}", "package p\nfunc F(){_ = \"\"}\nfunc G(){}",
		"package p\nvar f = func(){ x:=1;_ =x };func G(){}",

		"package p\nfunc F() {}\n// comment",
		"// before\npackage p\nfunc F(x int) int {\n// body\nreturn x\n}",
		"//line v.go:10\npackage p\nfunc (t *T[P]) F(\n// header\nx P,\n) {}\n// tail",
		"package p\nvar s = `// not a comment`\n/* block */\nfunc F() {}",
	} {
		f.Add(source, uint16(0), uint16(0), "// new\n")
		f.Add(source, uint16(len(source)-3), uint16(len(source)), "中文")
	}
	f.Fuzz(func(t *testing.T, source string, a, b uint16, replacement string) {
		if len(source) > 4096 || len(replacement) > 256 {
			return
		}
		start, end := int(a)%(len(source)+1), int(b)%(len(source)+1)
		if start > end {
			start, end = end, start
		}
		var p goSymbolIndex
		before := p.parse(source)
		projection := p.projection()
		next := source[:start] + replacement + source[end:]
		if projection != nil && projection.advance(next) != nil {
			if want := parseGoCodeSymbolsText(next); !reflect.DeepEqual(before, want) || projection.text != source {
				t.Fatalf("UI fast path changed symbols or its immutable source: %q -> %q", source, next)
			}
		}
		if got, want := p.parse(next), parseGoCodeSymbolsText(next); !reflect.DeepEqual(got, want) {
			t.Fatalf("incremental/full differ: source=%q edit=%d:%d %q\ngot=%v want=%v", source, start, end, replacement, got, want)
		}
	})
}
