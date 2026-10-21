package gui

import (
	"github.com/uk0/silk/paint"
	"strings"
	"testing"
)

func TestEditorCustomPaletteDoesNotMutateOtherEditors(t *testing.T) {
	e, other := NewCodeEditor(), NewCodeEditor()
	c := paint.Color{R: 1, G: 2, B: 3, A: 255}
	want := c
	e.SetColors(EditorColors{Background: &c})
	c.R = 99
	if e.editorColors().bg != want {
		t.Fatal("caller alias")
	}
	if other.editorColors().bg != editorColorsFor(CurrentThemeMode()).bg {
		t.Fatal("global palette mutated")
	}
	e.SetColors(EditorColors{})
	if e.editorColors().bg != other.editorColors().bg {
		t.Fatal("reset failed")
	}
}
func TestTooltipMultilineNoControlGlyphs(t *testing.T) {
	lines := tooltipLines("```go\r\nfunc f()\n\t中文😀\n```")
	if len(lines) != 2 || lines[0] != "func f()" || lines[1] != "    中文😀" {
		t.Fatal(lines)
	}
	lines = tooltipLines(strings.Repeat("文", 4000))
	if len(lines) > 17 {
		t.Fatal("unbounded tooltip")
	}
	for _, line := range lines {
		if strings.ContainsAny(line, "\n\r\t") {
			t.Fatal("control glyph")
		}
	}
}
