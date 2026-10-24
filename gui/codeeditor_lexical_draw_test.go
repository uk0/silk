package gui

import (
	"github.com/uk0/silk/paint"
	"strings"
	"testing"
)

type editorLexicalRecorder struct {
	editorNoopPainter
	color     paint.Color
	fragments map[string]paint.Color
}

func (r *editorLexicalRecorder) SetBrush1(c paint.Color)             { r.color = c }
func (r *editorLexicalRecorder) DrawText1(_, _ float64, text string) { r.fragments[text] = r.color }

func TestEditorDrawMultilineGoState(t *testing.T) {
	for _, tc := range []struct {
		name, source, fragment string
		typ                    tokenType
		folded                 bool
	}{
		{"raw", "package p\nvar raw = `\n/* not a comment\nfunc return\n`\nfunc AfterRaw() {}", "/* not a comment", tokString, false},
		{"after_raw", "package p\nvar raw = `\n/* not a comment\n`\nfunc AfterRaw() {}", "AfterRaw", tokFunction, false},
		{"long_close", "/*\n" + strings.Repeat("x", maxHighlightLineLength) + "*/\nfunc AfterLong() {}", "AfterLong", tokFunction, false},
		{"folded_comment", "/*\nhidden\n*/\nfunc AfterFold() {}", "AfterFold", tokFunction, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := NewCodeEditor()
			e.SetSize(1000, 700)
			e.SetText(tc.source)
			if tc.folded {
				e.visLineCache = []int{0, 3}
				e.visLineValid = true
			}
			r := &editorLexicalRecorder{fragments: make(map[string]paint.Color)}
			e.Draw(r)
			if got, ok := r.fragments[tc.fragment]; !ok || got != e.editorColors().tokens[tc.typ] {
				t.Fatalf("fragment %q: present=%v color=%v want=%v", tc.fragment, ok, got, e.editorColors().tokens[tc.typ])
			}
		})
	}
}

// Full editor Draw, with font metrics but no raster/device work. Warm caches,
// no active cursor animation. This is not GPU throughput or input latency.
func BenchmarkEditorDrawScrolledGo(b *testing.B) {
	for _, n := range []int{1000, 100000} {
		name := "1k"
		if n == 100000 {
			name = "100k"
		}
		b.Run(name, func(b *testing.B) {
			e := NewCodeEditor()
			e.SetSize(1000, 700)
			e.SetText("package p\n" + strings.Repeat("// stable source for scrolled rendering\n", n) + "var Bottom = 42\n")
			e.ScrollToLineCol(n+1, 0)
			g := editorNoopPainter{}
			e.Draw(g)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				e.Draw(g)
			}
		})
	}
}
