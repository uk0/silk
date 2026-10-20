package gui

import (
	"math"
	"strings"
	"testing"

	"github.com/uk0/silk/geom"
)

func TestUTF16TextInput(t *testing.T) {
	tests := []struct {
		name  string
		units []uint16
		want  string
	}{
		{"ASCII and BMP", []uint16{'a', 0x4E2D, 0x6587}, "a中文"},
		{"supplementary", []uint16{0xD83D, 0xDE80, 0xD840, 0xDC00}, "🚀𠀀"},
		{"orphan low", []uint16{0xDE80, 'x'}, "x"},
		{"orphan high", []uint16{0xD83D, 'x'}, "x"},
		{"successive high", []uint16{0xD840, 0xD83D, 0xDE80}, "🚀"},
		{"controls break pair", []uint16{0xD83D, 13, 0xDE80, 0, 9, 8, 'a'}, "a"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var d utf16TextInput
			var got strings.Builder
			for _, u := range tc.units {
				got.WriteString(d.decode(u))
			}
			if got.String() != tc.want {
				t.Fatalf("decode = %q, want %q", got.String(), tc.want)
			}
		})
	}
	var d utf16TextInput
	if d.decode(0xD83D) != "" {
		t.Fatal("partial surrogate committed")
	}
	d.reset()
	if d.decode(0xDE80) != "" {
		t.Fatal("surrogate survived reset")
	}
	if n := testing.AllocsPerRun(100, func() { d.decode(0xD83D) }); n != 0 {
		t.Fatalf("buffering allocated %g", n)
	}
}

func TestInputMethodCursorGeometry(t *testing.T) {
	e := NewCodeEditor()
	e.SetSize(420, 240)
	e.SetText("first\n中文 ab")
	e.cursorLine, e.cursorCol = 1, 4
	r := e.InputMethodCursorRect()
	x, bottom := e.caretLocalXY()
	if math.Abs(r.X-x) > .01 || math.Abs(r.Bottom()-bottom) > .01 {
		t.Fatalf("code caret=%+v, want %v,%v", r, x, bottom)
	}
	// Search may receive an IME event before Draw adjusts its horizontal scroll.
	s := NewSearchBox()
	s.SetSize(180, 30)
	s.SetText(strings.Repeat("wide ", 40))
	before := s.scrollX
	r = s.InputMethodCursorRect()
	if r.Right() > 180-searchBoxClearW+.01 || r.X < searchBoxTextX {
		t.Fatalf("search caret covered an icon: %+v", r)
	}
	if s.scrollX != before {
		t.Fatal("geometry query changed search state")
	}
	edit := NewEdit()
	edit.SetSize(300, 100)
	edit.SetText("hello 世界")
	edit.SetCaretPos(4)
	r = edit.InputMethodCursorRect()
	if r.X <= edit.padding.L || r.Y != edit.padding.T {
		t.Fatalf("edit caret=%+v", r)
	}
	edit.SetCaretPos(0)
	if r = edit.InputMethodCursorRect(); r.X != edit.padding.L {
		t.Fatalf("edit home caret=%+v", r)
	}
	for _, widget := range []IWidget{e, s, edit} {
		widget.SetSize(1, 1)
		r = widget.(IInputMethodCursor).InputMethodCursorRect()
		if r.X < 0 || r.Y < 0 || r.Right() > 1 || r.Bottom() > 1 {
			t.Fatalf("tiny %T caret=%+v", widget, r)
		}
		widget.SetSize(0, 0)
		if r = widget.(IInputMethodCursor).InputMethodCursorRect(); r != (geom.Rect{}) {
			t.Fatalf("zero-size caret=%+v", r)
		}
	}
}

func TestInputMethodRectClampsInvalidGeometry(t *testing.T) {
	for _, r := range []geom.Rect{{X: math.NaN(), Y: math.Inf(1), Height: math.Inf(-1)}, {X: -50, Y: 100, Height: 300}} {
		got := clampInputMethodRect(r, 60, 40)
		if math.IsNaN(got.X) || math.IsNaN(got.Y) || got.X < 0 || got.Y < 0 || got.Right() > 60 || got.Bottom() > 40 {
			t.Fatalf("invalid clamp: %+v", got)
		}
	}
}
