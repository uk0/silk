package gui

import (
	"strings"
	"testing"
	"unicode/utf16"
	"unicode/utf8"
)

func TestCoordinateReadClampsNegativeWithoutAllocatingWholeLine(t *testing.T) {
	line := "x😀" + strings.Repeat("tail", 1<<18)
	t.Run("allocations", func(t *testing.T) {
		if n := testing.AllocsPerRun(5, func() {
			if got := utf16ColumnOf(line, 2); got != 3 {
				t.Fatal(got)
			}
		}); n != 0 {
			t.Fatalf("prefix conversion allocated %v times for an untouched long suffix", n)
		}
	})
	t.Run("negative", func(t *testing.T) {
		defer func() {
			if p := recover(); p != nil {
				t.Errorf("negative column panicked: %v", p)
			}
		}()
		if got := utf16ColumnOf(line, -1); got != 0 {
			t.Fatal(got)
		}
	})
}

func TestEditorTextPositionCoordinateUnitsAndReadOnlyLines(t *testing.T) {
	e := NewCodeEditor()
	// Includes BMP, astral, combining, NUL, invalid UTF-8, CR and a trailing empty line.
	text := "a中文😀e\u0301\x00\xffz\r\n\nlast\n"
	e.SetText(text)
	e.ScrollToLineCol(0, 4)
	e.MarkSaved()
	beforeRevision := e.TextRevision()
	lineSnapshot, ok := e.LineText(0)
	if !ok {
		t.Fatal("missing first line")
	}
	for line, content := range e.lines {
		runes := []rune(content)
		for col := -2; col <= len(runes)+2; col++ {
			p, ok := e.TextPositionAt(line, col)
			clamped := max(0, min(col, len(runes)))
			wantByte, wantUTF16, n := 0, 0, 0
			for pos, r := range content {
				if n == clamped {
					break
				}
				_, width := utf8.DecodeRuneInString(content[pos:])
				wantByte = pos + width
				wantUTF16 += len(utf16.Encode([]rune{r}))
				n++
			}
			if !ok || p.Line != line || p.RuneColumn != clamped || p.ByteColumn != wantByte || p.UTF16Column != wantUTF16 || p.HasPreviousRune != (clamped > 0) {
				t.Fatalf("%d:%d: %+v want bytes=%d UTF16=%d", line, col, p, wantByte, wantUTF16)
			}
			if clamped > 0 && p.PreviousRune != runes[clamped-1] {
				t.Fatal("wrong previous code point", p)
			}
			if e.UTF16ColumnAt(line, col) != p.UTF16Column {
				t.Fatal("inconsistent public conversion")
			}
		}
	}
	for _, line := range []int{-1, len(e.lines), int(^uint(0) >> 1)} {
		if s, ok := e.LineText(line); ok || s != "" {
			t.Fatal("invalid line accepted")
		}
		if p, ok := e.TextPositionAt(line, 100); ok || p != (EditorTextPosition{}) {
			t.Fatal("invalid position accepted")
		}
	}
	p, ok := e.CursorTextPosition()
	if !ok || p.RuneColumn != 4 || p.ByteColumn != 11 || p.UTF16Column != 5 || p.PreviousRune != '😀' {
		t.Fatal(p)
	}
	if e.CursorLine() != 0 || e.CursorCol() != 4 || e.TextRevision() != beforeRevision || e.Modified() {
		t.Fatal("read access mutated editor")
	}
	// Coordinate snapshots and the byte-preserving edit history agree even for
	// malformed UTF-8; this includes the original invalid byte on undo.
	e.SetText(text)
	e.ScrollToLineCol(0, 4)
	e.OnTextInput(" ")
	if lineSnapshot != "a中文😀e\u0301\x00\xffz\r" {
		t.Fatal("old line snapshot was mutated")
	}
	e.Undo()
	p, _ = e.CursorTextPosition()
	if e.Text() != text || p.RuneColumn != 4 {
		t.Fatal("undo did not preserve coordinates")
	}
	e.Redo()
	p, _ = e.CursorTextPosition()
	if p.RuneColumn != 5 || p.PreviousRune != ' ' {
		t.Fatal("redo coordinates stale", p)
	}
}

func TestEditorTextPositionDoesNotJoinDirtyBufferOrAllocate(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("x😀" + strings.Repeat("tail", 100000) + "\nlast")
	e.cursorLine, e.cursorCol = 0, 2
	e.insertTextAtCursor("中")
	if !e.textDirty {
		t.Fatal("fixture did not leave pending splice")
	}
	p, ok := e.CursorTextPosition()
	if !ok || p.UTF16Column != 4 || p.ByteColumn != 8 || p.PreviousRune != '中' || !e.textDirty {
		t.Fatal("position read rebuilt text or missed splice", p)
	}
	if n := testing.AllocsPerRun(100, func() { e.CursorTextPosition(); e.LineText(1); e.UTF16ColumnAt(0, 2) }); n != 0 {
		t.Fatal("coordinate access allocations", n)
	}
	e.cursorCol = -1
	if p, ok := e.CursorTextPosition(); !ok || p.RuneColumn != 0 || p.HasPreviousRune {
		t.Fatal(p, ok)
	}
	if e.CursorCol() != -1 {
		t.Fatal("accessor changed caret")
	}
	e.cursorLine = -1
	if _, ok := e.CursorTextPosition(); ok {
		t.Fatal("invalid caret line accepted")
	}
	var zero CodeEditor
	if _, ok := zero.CursorTextPosition(); ok {
		t.Fatal("zero editor has no line")
	}
}
