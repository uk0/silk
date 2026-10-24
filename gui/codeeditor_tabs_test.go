package gui

import (
	"github.com/uk0/silk/paint"
	"math"
	"strings"
	"testing"
)

type editorTextRun struct {
	x    float64
	text string
}
type editorTextRecorder struct {
	paint.Painter
	runs []editorTextRun
}

func (r *editorTextRecorder) SetBrush1(paint.Color) {}
func (r *editorTextRecorder) DrawText1(x, y float64, s string) {
	r.runs = append(r.runs, editorTextRun{x, s})
}

func TestEditorTabsUseStopsWithoutDrawingControlGlyphs(t *testing.T) {
	e := NewCodeEditor()
	for _, line := range []string{"\t\treturn 42", "a\tb\tc", " // 中文\tcomment", "s := `a\tb`"} {
		r := &editorTextRecorder{}
		e.SetText(line)
		e.drawHighlightedLine(r, 0, 60, 20)
		for _, run := range r.runs {
			if strings.ContainsAny(run.text, "\t\r\n") {
				t.Errorf("control glyph rendered: %q", run.text)
			}
		}
	}
	want := e.measureText("    ")
	if got := e.measureText("\t"); math.Abs(got-want) > .01 {
		t.Errorf("tab width %v want %v", got, want)
	}
	if got := e.measureText("a\tb"); math.Abs(got-e.measureText("    b")) > .01 {
		t.Errorf("tab stop after a: %v", got)
	}
}

func TestEditorTabCaretHitTestRoundTrip(t *testing.T) {
	e := NewCodeEditor()
	e.SetSize(1000, 400)
	text := "\t\t中文😀value\t:= 42"
	e.SetText(text)
	for col := 0; col <= len([]rune(text)); col++ {
		e.cursorCol = col
		x, y := e.caretLocalXY()
		line, got := e.posFromXY(x, y)
		if line != 0 || got != col {
			t.Fatalf("col %d -> %v,%v -> %d:%d", col, x, y, line, got)
		}
	}
	if e.Text() != text {
		t.Fatal("drawing changed original tabs")
	}
}
