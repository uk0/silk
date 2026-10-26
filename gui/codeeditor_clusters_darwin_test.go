//go:build darwin && cgo && !silk_pure_go

package gui

import (
	"math"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/uk0/silk/paint"
)

func TestEditorClustersCaretHitAndTokens(t *testing.T) {
	if !paint.NeedsClusterLayout("e\u0301") {
		t.Skip("Cairo Quartz cluster shaping unavailable")
	}
	e := NewCodeEditor()
	defer e.CancelSymbolIndex()
	e.SetSize(700, 400)
	text := "Ae\u0301🙂‍↕️Z\tB"
	e.SetText(text)
	l := e.clusterTextGeometry(text)
	if l == nil || len(l.Clusters) != 2 {
		t.Fatal("complex line has no geometry")
	}
	for _, c := range l.Clusters {
		left, right := l.Carets[c.Begin], l.Carets[c.End]
		for col := c.Begin + 1; col < c.End; col++ {
			if e.columnAdvance(0, col) != left {
				t.Fatal("caret prefix was re-shaped independently")
			}
		}
		if e.textColumnAtX(text, left+(right-left)*.25) != c.Begin || e.textColumnAtX(text, left+(right-left)*.75) != c.End {
			t.Fatal("mouse entered cluster interior")
		}
		e.ScrollToLineCol(0, c.End)
		x, _ := e.caretLocalXY()
		if math.Abs(x-(e.gutterW+10+right)) > .001 {
			t.Fatal("popup/caret anchor diverged")
		}
	}
	tokens := e.clusterSafeTokens(text, e.tokensForLine(0))
	col := 0
	var full strings.Builder
	for _, tok := range tokens {
		col += utf8.RuneCountInString(tok.text)
		full.WriteString(tok.text)
		if l.Boundary(col, false) != col {
			t.Fatal("syntax draw split cluster")
		}
	}
	if full.String() != text {
		t.Fatal("token coalescing altered source")
	}
	end := e.columnAdvance(0, utf8.RuneCountInString(text))
	e.SetFont(paint.NewFont("Menlo", 30, false, false))
	if e.columnAdvance(0, utf8.RuneCountInString(text)) <= end {
		t.Fatal("font change reused stale geometry")
	}
	e.SetText("Ae\u0301Z")
	if len(e.clusterTextGeometry(e.Text()).Carets) != 5 {
		t.Fatal("document change reused stale geometry")
	}
}

func TestTextBlockClustersWrapNoDuplication(t *testing.T) {
	if !paint.NeedsClusterLayout("e\u0301") {
		t.Skip("Cairo Quartz cluster shaping unavailable")
	}
	var b TextBlock
	b.SetFont(paint.NewFont("Menlo", 18, false, false))
	b.SetWrap(true)
	const text = "A🙂‍↕️Be\u0301C🇨🇳D"
	b.SetText(text)
	for _, width := range []float64{1, 20, 70, 400} {
		b.Layout(width)
		var rebuilt strings.Builder
		glyphs := 0
		for ri, row := range b.rows {
			rebuilt.WriteString(string(row.text))
			for _, c := range row.clusters {
				left, right := row.caretX(c.Begin), row.caretX(c.End)
				if got := b.PointToPos(left+(right-left)*.25, (float64(ri)+.5)*b.RowHeight()); got != row.begin+c.Begin {
					t.Fatalf("field hit entered cluster at %d", got)
				}
				if got := b.PointToPos(left+(right-left)*.75, (float64(ri)+.5)*b.RowHeight()); got != row.begin+c.End {
					t.Fatalf("field hit missed trailing edge %d", got)
				}
			}
			for _, run := range row.runs {
				glyphs += len(run.Glyphs)
			}
		}
		if rebuilt.String() != text || glyphs != 7 {
			t.Fatalf("wrap width=%g altered/doubled glyphs: %d %q", width, glyphs, rebuilt.String())
		}
	}
}
