package gui

import (
	"reflect"
	"strings"
	"testing"
)

func TestGoDeclarationsSharedLexicalBoundaries(t *testing.T) {
	cases := []struct {
		name, source string
		names        []string
	}{
		{"inline comment", "package p\n/* exported */ var Target = 42\n", []string{"Target"}},
		{"block comment", "package p\n/* documentation\nfunc Phantom() {}\n*/\nfunc Real(){}\n", []string{"Real"}},
		{"raw string", "package p\nvar Template = `\nfunc Phantom(){}\n`\nfunc Real(){}\n", []string{"Template", "Real"}},
		{"groups and locals", "package p\ntype(\n Item struct{Name string}\n Count int\n)\nvar A,B int\nconst (\n _, C = 1,2\n D=3\n)\nfunc Real(){var Local int; _=Local}\n", []string{"Item", "Count", "A", "B", "C", "D", "Real"}},
		{"partial function", "package p\nvar Target=42\nfunc Done(){}\nfunc Broken(\n", []string{"Target", "Done", "Broken"}},
		{"snippet", "/* title */\nvar Value=1\nfunc Done(){}\n", []string{"Value", "Done"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ed := NewCodeEditor()
			ed.SetText(tc.source)
			var got, sdk []string
			for _, s := range ed.ParseSymbols() {
				got = append(got, s.Name)
			}
			for _, s := range OutlineSymbols(tc.source) {
				sdk = append(sdk, s.Name)
			}
			if !reflect.DeepEqual(got, tc.names) || !reflect.DeepEqual(sdk, tc.names) {
				t.Fatalf("editor=%v SDK=%v want=%v", got, sdk, tc.names)
			}
		})
	}
}

func TestGoDeclarationsActualUTF8LocationsAndGenericReceiver(t *testing.T) {
	source := "package p\r\n//line decoy.go:900\r\n/* 注释 */ var 目标 = 1\r\ntype Box[T any] struct{}\r\nfunc (b *Box[T]) Get(\r\n value T,\r\n) T { return value }\r\n"
	out := OutlineSymbols(source)
	if len(out) != 3 {
		t.Fatal(out)
	}
	first := out[0]
	if first.Name != "目标" || first.Line != 2 || first.Column != len("/* 注释 */ var ") {
		t.Fatalf("adjusted/directive coordinates escaped actual buffer: %+v", first)
	}
	if out[2].Receiver != "*Box[T]" || out[2].Kind != "method" || out[2].Line != 4 {
		t.Fatal(out[2])
	}
	e := NewCodeEditor()
	e.SetText(source)
	symbols := e.ParseSymbols()
	if len(symbols) != 3 || symbols[2].Receiver != "Box[T]" || symbols[2].Kind != SymMethod {
		t.Fatal(symbols)
	}
	if strings.Contains(symbols[2].Detail, "return value") {
		t.Fatal("function body leaked into symbol signature")
	}
}

func BenchmarkGoDeclarationsChangedSource(b *testing.B) {
	for _, count := range []int{100, 1000} {
		source := "package p\n" + strings.Repeat("// docs\nfunc Work(x int) int {\n y := x + 1\n if y > 2 { return y }\n return x\n}\n", count)
		lines := strings.Split(source, "\n")
		b.Run(map[int]string{100: "100-functions", 1000: "1000-functions"}[count], func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(source)))
			for i := 0; i < b.N; i++ {
				_ = parseGoCodeSymbols(lines)
			}
		})
	}
}

func TestGoDeclarationsPreservesComplexSignaturesWhileSkippingBodies(t *testing.T) {
	source := `package p
func Struct() struct{ Name string } { return struct{ Name string }{} }
func Interface() interface{ Method() } { panic("unused") }
func Multiple() (struct{ N int }, error) { return struct{ N int }{}, nil }
func Generic[T interface{ ~int | ~int64 }](value T) T { return value }
func Callback() func(int) int { return func(n int) int { return n } }
var Lambda = func() struct{ N int } { return struct{ N int }{} }
func BodyStrings() { _ = "}"; _ = ` + "`func Phantom(){}\n}`" + `; /* } */ }
func After() {}
`
	out := OutlineSymbols(source)
	var names []string
	for _, s := range out {
		names = append(names, s.Name)
	}
	want := []string{"Struct", "Interface", "Multiple", "Generic", "Callback", "Lambda", "BodyStrings", "After"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("%v != %v", names, want)
	}
	syms := parseGoCodeSymbols(strings.Split(source, "\n"))
	for _, s := range syms {
		if strings.Contains(s.Detail, "return") || strings.Contains(s.Detail, "panic") {
			t.Fatal("body parsed as declaration", s)
		}
	}
	if !strings.Contains(syms[0].Detail, "struct{ Name string }") || !strings.Contains(syms[3].Detail, "interface{ ~int | ~int64 }") {
		t.Fatal("type braces were stripped", syms)
	}
}

func TestGoDeclarationsEveryTruncationKeepsRealSourceLocations(t *testing.T) {
	source := "/* leading */\npackage p\n//line hidden:800\nvar 目标, Other = 1,2\ntype Box[T any] struct{Value T}\nfunc (b *Box[T]) Get(x T) T { /* } */ return x }\nfunc Partial(value int) {}\n"
	for length := 0; length <= len(source); length++ {
		text := source[:length]
		for _, s := range OutlineSymbols(text) {
			lines := strings.Split(text, "\n")
			if s.Line < 0 || s.Line >= len(lines) || s.Column < 0 || s.Column+len(s.Name) > len(lines[s.Line]) || lines[s.Line][s.Column:s.Column+len(s.Name)] != s.Name {
				t.Fatalf("length=%d invalid original source location: %+v", length, s)
			}
		}
	}
}

func TestGoDeclarationsNavigationAndReferencesRecoverWithoutPhantoms(t *testing.T) {
	source := "/* docs\nfunc Phantom(){}\n*/\n//line decoy:900\n/* 注释 */ var 目标 = 1\nfunc Done(){ _=目标 }\nfunc Partial(\n"
	if target := findInContent("Phantom", "test.go", source); target != nil {
		t.Fatal("comment became a definition", target)
	}
	target := findInContent("目标", "test.go", source)
	if target == nil || target.Line != 4 || target.Column != len("/* 注释 */ var ") {
		t.Fatal("partial/snippet definition lost physical coordinates", target)
	}
	refs := FindReferences("目标", source)
	if len(refs) != 2 || refs[0].Line != 4 || refs[0].Column != len("/* 注释 */ var ") || refs[1].Line != 5 {
		t.Fatal("partial/snippet references", refs)
	}
	if got := FindReferences("Phantom", source); len(got) != 0 {
		t.Fatal("comment became a reference", got)
	}
}
