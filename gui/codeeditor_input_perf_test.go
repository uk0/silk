package gui

import (
	"fmt"
	"strings"
	"testing"
)

// One keystroke + paint + undo + paint, on a stable-size document. No raster
// device or IDE/LSP callback: profile these separately rather than calling
// this a native end-to-end input-latency benchmark.
func BenchmarkEditorInputDrawUndo(b *testing.B) {
	for _, n := range []int{1000, 100000} {
		b.Run(fmt.Sprintf("%d", n), func(b *testing.B) {
			e := NewCodeEditor()
			defer e.CancelSymbolIndex()
			e.SetSize(1000, 700)
			e.SetText("package p\n\n" + strings.Repeat("// stable source for scrolled rendering\n", n) + "var Bottom = 42\n")
			e.ScrollToLineCol(n+2, 15)
			e.MarkSaved()
			g := editorNoopPainter{}
			e.Draw(g)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				e.OnTextInput(" ")
				e.Draw(g)
				e.Undo()
				e.Draw(g)
			}
		})
	}
}
