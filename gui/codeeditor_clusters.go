package gui

import (
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/uk0/silk/paint"
)

type editorClusterGeometry struct {
	source, font       string
	tabs               int
	checked, candidate bool
	layout             paint.TextLayout
}

// Cache the current complex line's geometry, not prefix-shaped fragments.
// ASCII keeps its existing allocation-free viewport/measurement fast paths.
func (e *CodeEditor) clusterTextGeometry(s string) *paint.TextLayout {
	cache := &e.clusterGeometry
	if !cache.checked || cache.source != s {
		*cache = editorClusterGeometry{source: s, checked: true, candidate: paint.NeedsClusterLayout(s)}
	}
	if !cache.candidate {
		return nil
	}
	key := e.font.String()
	if cache.source == s && cache.font == key && cache.tabs == e.TabSize() {
		return &cache.layout
	}
	out := paint.TextLayout{Carets: []float64{0}}
	parts := strings.Split(s, "\t")
	for i, part := range parts {
		layout := paint.LayoutText(e.font.ScaledFont(nil), part)
		begin := len(out.Carets) - 1
		x := out.Carets[begin]
		for _, c := range layout.Carets[1:] {
			out.Carets = append(out.Carets, x+c)
		}
		for _, c := range layout.Clusters {
			out.Clusters = append(out.Clusters, paint.TextCluster{Begin: begin + c.Begin, End: begin + c.End})
		}
		if i+1 < len(parts) {
			out.Carets = append(out.Carets, e.nextTabStop(out.Carets[len(out.Carets)-1]))
		}
	}
	*cache = editorClusterGeometry{source: s, font: key, tabs: e.TabSize(), layout: out, checked: true, candidate: true}
	return &cache.layout
}

func (e *CodeEditor) columnAdvance(line, col int) float64 {
	if line < 0 || line >= len(e.lines) {
		return 0
	}
	if layout := e.clusterTextGeometry(e.lines[line]); layout != nil {
		return layout.Carets[max(0, min(col, len(layout.Carets)-1))]
	}
	return e.measureText(e.linePrefix(line, col))
}

func clusterColumnAtX(layout *paint.TextLayout, x float64) int {
	hi := sort.Search(len(layout.Carets), func(i int) bool { return layout.Carets[i] > x })
	if hi == len(layout.Carets) {
		return hi - 1
	}
	if hi == 0 {
		return 0
	}
	lo := layout.Boundary(hi-1, false)
	hi = layout.Boundary(hi, true)
	if x-layout.Carets[lo] < layout.Carets[hi]-x {
		return lo
	}
	return hi
}

// Syntax highlighting must not split a single shaped cluster. Its leading
// token owns the cluster's color, while later text keeps its original style.
func (e *CodeEditor) clusterSafeTokens(line string, tokens []token) []token {
	layout := e.clusterTextGeometry(line)
	if layout == nil || len(layout.Clusters) == 0 {
		return tokens
	}
	result := make([]token, 0, len(tokens))
	byteEnd, col, consumed := 0, 0, 0
	for _, tok := range tokens {
		byteEnd += len(tok.text)
		col += utf8.RuneCountInString(tok.text)
		end := byteEnd
		if safe := layout.Boundary(col, true); safe != col {
			end = textPositionInLine(line, safe).ByteColumn
		}
		if end > consumed {
			tok.text = line[consumed:end]
			result = append(result, tok)
			consumed = end
		}
	}
	return result
}
