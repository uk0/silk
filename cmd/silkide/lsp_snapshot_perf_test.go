package main

import (
	"strings"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

// Isolate the actual UI-thread snapshot path, without RPCs, a renderer or a
// background worker. The inert document still exercises versioning/coalescing.
func installLSPSnapshotProbe(tb testing.TB, path string) *lspDocument {
	tb.Helper()
	client, docs, versions, highlights := globalLSP, lspDocuments, lspVersions, lspHighlightGen
	hover, sig := lspHoverGen, lspSigGen
	globalLSP = &core.LSPClient{}
	d := &lspDocument{wake: make(chan struct{}, 1)}
	lspDocuments = map[string]*lspDocument{path: d}
	lspVersions = map[string]int{path: 1}
	lspHighlightGen = map[string]int{}
	tb.Cleanup(func() {
		globalLSP, lspDocuments, lspVersions, lspHighlightGen = client, docs, versions, highlights
		lspHoverGen, lspSigGen = hover, sig
	})
	return d
}

func BenchmarkLSPInputSnapshot(b *testing.B) {
	for _, tc := range []struct {
		name, text string
		line, col  int
	}{
		{"small", "package p\nfmt.Pr\n", 1, 6},
		{"100k_lines", "package p\n" + strings.Repeat("// stable source for scrolled rendering\n", 100000) + "fmt.Pr\n", 100001, 6},
		{"4MB_line_prefix", "fmt.Pr" + strings.Repeat(" ", 4000000), 0, 6},
		{"4MB_line_end", strings.Repeat(" ", 4000000) + "fmt.Pr", 0, 4000006},
	} {
		b.Run(tc.name, func(b *testing.B) {
			const path = "/silk-qa/snapshot.go"
			d := installLSPSnapshotProbe(b, path)
			e := gui.NewCodeEditor()
			defer e.CancelCompletion()
			defer e.CancelSymbolIndex()
			e.SetText(tc.text)
			e.ScrollToLineCol(tc.line, tc.col)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				updateLSPDocument(path, e, tc.text)
			}
			b.StopTimer()
			if !d.latest.complete || d.latest.text != tc.text || d.latest.line != tc.line {
				b.Fatal("snapshot path not exercised", d.latest.line, d.latest.complete)
			}
		})
	}
}
