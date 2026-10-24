//go:build darwin && !silk_pure_go

package gui

import (
	"math"
	"os"
	"reflect"
	"testing"

	"github.com/uk0/silk/paint"
)

// This also runs unchanged on the pre-fix revision: it reproduces the caret
// width mismatch without depending on any new implementation helper.
func TestDarwinCodeEditorCJKFallbackGeometry(t *testing.T) {
	cjk := paint.NewFont("PingFang SC", 16, false, false)
	glyphs := cjk.TextToGlyphs(0, 0, "你好")
	covered := len(glyphs) == 2
	for _, g := range glyphs {
		covered = covered && reflect.ValueOf(g).FieldByName("index").Uint() != 0
	}
	if !covered {
		if os.Getenv("SILK_TEST_FONT_FALLBACK") == "1" {
			t.Fatal("required native CJK font unavailable")
		}
		t.Skip("PingFang SC CJK coverage unavailable on this host")
	}
	e := NewCodeEditor()
	e.SetSize(800, 300)
	e.SetFont(paint.NewFont("Menlo", 16, false, false))
	e.SetText("A你好B")
	want := e.font.TextExtents("AB").XAdvance + cjk.TextExtents("你好").XAdvance
	if got := e.measureText("A你好B"); math.Abs(got-want) > .01 {
		t.Fatalf("mixed text advance=%g, want native font runs=%g", got, want)
	}
	e.cursorLine, e.cursorCol = 0, 3
	prefix := e.font.TextExtents("A").XAdvance + cjk.TextExtents("你好").XAdvance
	wantX := e.gutterW + 10 + prefix
	x, y := e.caretLocalXY()
	if math.Abs(x-wantX) > .01 {
		t.Fatalf("caret x=%g, want rendered prefix x=%g", x, wantX)
	}
	if r := e.InputMethodCursorRect(); math.Abs(r.X-wantX) > .01 {
		t.Fatalf("IME x=%g, want %g", r.X, wantX)
	}
	if line, col := e.posFromXY(wantX, y-1); line != 0 || col != 3 {
		t.Fatalf("hit at rendered caret=%d:%d, want 0:3", line, col)
	}
}
