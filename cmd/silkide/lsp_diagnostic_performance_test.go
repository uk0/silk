package main

import (
	"strings"
	"testing"

	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
)

func diagnosticLineEditor(lines int) *gui.CodeEditor {
	ed := gui.NewCodeEditor()
	ed.SetText(strings.Repeat("// filler\n", lines-1) + "/*😀🧪中*/ var X")
	return ed
}

func TestDiagnosticColumnsDoNotCopyEditorLines(t *testing.T) {
	const n = 100_000
	ed := diagnosticLineEditor(n)
	if ed.LineCount() != n {
		t.Fatalf("fixture loaded %d lines, want %d", ed.LineCount(), n)
	}
	var rows [4]ged.Problem
	allocs := testing.AllocsPerRun(30, func() {
		rows[0] = ged.Problem{Line: n, Col: 10, Message: "field not found", Severity: ged.SeverityWarning}
		rows[1] = ged.Problem{Line: n + 1, Col: 20}
		rows[2] = ged.Problem{Line: 1, Col: 5}
		rows[3] = ged.Problem{Line: 0, Col: 5}
		normalizeLSPProblemColumns(rows[:], ed)
	})
	if rows[0].Col != 8 || rows[0].Message != "field not found" || rows[0].Severity != ged.SeverityWarning ||
		rows[1].Col != 0 || rows[2].Col != 5 || rows[3].Col != 0 {
		t.Fatalf("incorrect UTF-16 to rune conversion: %+v", rows)
	}
	if allocs != 0 {
		t.Fatalf("%v heap allocations per diagnostic update; do not copy the document's line index", allocs)
	}
}

func BenchmarkDiagnosticColumnLookup(b *testing.B) {
	for _, tc := range []struct {
		name string
		n    int
	}{{"100-lines", 100}, {"100k-lines", 100_000}} {
		b.Run(tc.name, func(b *testing.B) {
			ed := diagnosticLineEditor(tc.n)
			rows := []ged.Problem{{Line: tc.n, Col: 10}}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				rows[0].Col = 10
				normalizeLSPProblemColumns(rows, ed)
			}
			if rows[0].Col != 8 {
				b.Fatal(rows[0])
			}
		})
	}
}
