package gui

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func assertEditorBytes(t *testing.T, e *CodeEditor, want string) {
	t.Helper()
	if got := e.Text(); got != want {
		t.Fatalf("bytes = %x, want %x", got, want)
	}
	if got := strings.Join(e.lines, "\n"); got != want {
		t.Fatalf("line model = %x, want %x", got, want)
	}
}

func TestEditorByteExactEditUndoRedo(t *testing.T) {
	tests := []struct {
		name, original string
		line, col      int
		input          string
		key            int
		want           string
	}{
		{"type", "a\xffb", 0, 1, " ", 0, "a \xffb"},
		{"unicode", "中\xff😀e\u0301", 0, 2, "界", 0, "中\xff界😀e\u0301"},
		{"nul", "a\x00\xffb", 0, 2, " ", 0, "a\x00 \xffb"},
		{"backspace_invalid", "a\xffb", 0, 2, "", KeyBackSpace, "ab"},
		{"delete_invalid", "a\xffb", 0, 1, "", KeyDelete, "ab"},
		{"delete_valid", "aX\xffb", 0, 1, "", KeyDelete, "a\xffb"},
		{"enter", "a\xffb", 0, 2, "", KeyEnter, "a\xff\nb"},
		{"join_backspace", "a\xff\nb\xfe", 1, 0, "", KeyBackSpace, "a\xffb\xfe"},
		{"join_delete", "a\xff\nb\xfe", 0, 2, "", KeyDelete, "a\xffb\xfe"},
		{"multiline_input", "a\xffb", 0, 1, "中\n😀\xfe", 0, "a中\n😀\xfe\xffb"},
		{"pair", "a\xffb", 0, 1, "(", 0, "a()\xffb"},
		// Byte edits may join formerly invalid sequences into a valid rune. Undo
		// must remove/reinsert the exact byte span, not its newly decoded rune count.
		{"left_seam_insert", "\xc2", 0, 1, "\x80", 0, "\xc2\x80"},
		{"right_seam_insert", "\x80", 0, 0, "\xc2", 0, "\xc2\x80"},
		{"delete_seam", "\xc2X\x80", 0, 1, "", KeyDelete, "\xc2\x80"},
		{"backspace_seam", "\xc2X\x80", 0, 2, "", KeyBackSpace, "\xc2\x80"},
		{"newline_seam", "\xc2\n\x80", 1, 0, "", KeyBackSpace, "\xc2\x80"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewCodeEditor()
			e.filePath = "bytes.txt" // No asynchronous git refresh in this model test.
			e.SetText(tt.original)
			e.ScrollToLineCol(tt.line, tt.col)
			if tt.key != 0 {
				e.onKeyDownWithShift(tt.key, false, false)
			} else {
				e.OnTextInput(tt.input)
			}
			assertEditorBytes(t, e, tt.want)
			e.Undo()
			assertEditorBytes(t, e, tt.original)
			e.Redo()
			assertEditorBytes(t, e, tt.want)
			e.Undo()
			assertEditorBytes(t, e, tt.original)
		})
	}
}

func TestEditorByteExactSelection(t *testing.T) {
	tests := []struct {
		name, original              string
		sl, sc, el, ec              int
		selected, replacement, want string
	}{
		{"same_line", "a\xffb\xfec", 0, 1, 0, 4, "\xffb\xfe", "中", "a中c"},
		{"multiline", "a\xff\nb\xfe\n😀c", 0, 1, 2, 1, "\xff\nb\xfe\n😀", "中\n\xff", "a中\n\xffc"},
		{"seam", "\xc2X\x80", 0, 1, 0, 2, "X", "", "\xc2\x80"},
		{"wrap", "a\xffb", 0, 1, 0, 2, "\xff", "(", "a(\xff)b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewCodeEditor()
			e.SetText(tt.original)
			e.selStartLine, e.selStartCol, e.selEndLine, e.selEndCol = tt.sl, tt.sc, tt.el, tt.ec
			e.hasSelection = true
			e.cursorLine, e.cursorCol = tt.el, tt.ec
			if got := e.SelectedText(); got != tt.selected {
				t.Fatalf("SelectedText bytes %x want %x", got, tt.selected)
			}
			if tt.replacement == "" {
				e.onKeyDownWithShift(KeyDelete, false, false)
			} else {
				e.OnTextInput(tt.replacement)
			}
			assertEditorBytes(t, e, tt.want)
			e.Undo()
			assertEditorBytes(t, e, tt.original)
			e.Redo()
			assertEditorBytes(t, e, tt.want)
		})
	}
}

func TestEditorByteExactRawSplice(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("a\xff\n中\xfez")
	e.cursorLine, e.cursorCol = 0, 1
	e.insertRawText("\xfe\n😀")
	e.rebuildText()
	assertEditorBytes(t, e, "a\xfe\n😀\xff\n中\xfez")
	e.deleteRange(0, 1, 3)
	e.rebuildText()
	assertEditorBytes(t, e, "a\xff\n中\xfez")
}

func TestEditorByteExactReplace(t *testing.T) {
	t.Run("all", func(t *testing.T) {
		out, n := replaceAllInText("\xffFoo\xfe foo\xc2", "foo", "中", false)
		if out != "\xff中\xfe 中\xc2" || n != 2 {
			t.Fatalf("got %x, %d", out, n)
		}
		out, n = replaceAllInText("\xffFoo\xfe", "absent", "z", false)
		if out != "\xffFoo\xfe" || n != 0 {
			t.Fatalf("no-match mutated bytes %x", out)
		}
	})
	t.Run("current_and_undo", func(t *testing.T) {
		e := NewCodeEditor()
		original := "\xffFoo\xfe"
		e.SetText(original)
		e.findText = "foo"
		e.replaceText = "中\n😀"
		e.findUpdateMatches()
		e.replaceCurrent()
		assertEditorBytes(t, e, "\xff中\n😀\xfe")
		e.Undo()
		assertEditorBytes(t, e, original)
		e.Redo()
		assertEditorBytes(t, e, "\xff中\n😀\xfe")
	})
}

func TestEditorByteExactGroupedInput(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("a\xffb")
	e.ScrollToLineCol(0, 1)
	e.OnTextInput("中")
	e.OnTextInput("😀")
	assertEditorBytes(t, e, "a中😀\xffb")
	if len(e.undoStack) != 1 {
		t.Fatalf("typing no longer groups: %d", len(e.undoStack))
	}
	e.Undo()
	assertEditorBytes(t, e, "a\xffb")
	e.Redo()
	assertEditorBytes(t, e, "a中😀\xffb")
}

func TestEditorByteExactWholeLastLineUndo(t *testing.T) {
	for _, op := range []string{"delete", "duplicate"} {
		t.Run(op, func(t *testing.T) {
			e := NewCodeEditor()
			original := "first\n中\xff"
			e.SetText(original)
			e.ScrollToLineCol(1, 1)
			want := "first"
			if op == "delete" {
				e.deleteCurrentLine()
			} else {
				e.duplicateLine()
				want = original + "\n中\xff"
			}
			assertEditorBytes(t, e, want)
			e.Undo()
			assertEditorBytes(t, e, original)
			e.Redo()
			assertEditorBytes(t, e, want)
		})
	}
}

func BenchmarkEditorByteSplice(b *testing.B) {
	for _, size := range []int{64, 4 << 20} {
		for _, end := range []bool{false, true} {
			name := "small"
			if size > 64 {
				name = "4MB"
			}
			if end {
				name += "_end"
			} else {
				name += "_prefix"
			}
			b.Run(name, func(b *testing.B) {
				line := "中😀" + strings.Repeat("x", size)
				col := 2
				if end {
					col = utf8.RuneCountInString(line)
				}
				e := NewCodeEditor()
				e.lines = []string{line}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					e.lines[0] = line
					e.cursorLine, e.cursorCol = 0, col
					e.insertTextAtCursor(" ")
				}
				b.StopTimer()
				if len(e.lines[0]) != len(line)+1 {
					b.Fatal("splice missing")
				}
			})
		}
	}
}

func TestEditorByteExactMultiCursorAtomic(t *testing.T) {
	for _, mode := range []string{"insert", "multiline", "backspace", "delete"} {
		t.Run(mode, func(t *testing.T) {
			original := "a\xffb\nc\xfed"
			e := NewCodeEditor()
			e.SetText(original)
			col := 1
			if mode == "backspace" {
				col = 2
			}
			e.cursorLine, e.cursorCol = 1, col
			e.additionalCursors = []cursorPos{{0, col}}
			want := "a!\xffb\nc!\xfed"
			switch mode {
			case "insert":
				e.OnTextInput("!")
			case "multiline":
				e.OnTextInput("!\n中")
				want = "a!\n中\xffb\nc!\n中\xfed"
			case "backspace":
				e.onKeyDownWithShift(KeyBackSpace, false, false)
				want = "ab\ncd"
			case "delete":
				e.onKeyDownWithShift(KeyDelete, false, false)
				want = "ab\ncd"
			}
			assertEditorBytes(t, e, want)
			if len(e.undoStack) != 1 {
				t.Fatalf("multi-edit must be one transaction, got %d", len(e.undoStack))
			}
			e.Undo()
			assertEditorBytes(t, e, original)
			if e.cursorLine != 1 || e.cursorCol != col || len(e.additionalCursors) != 1 || e.additionalCursors[0] != (cursorPos{0, col}) {
				t.Fatalf("undo carets lost: %d:%d %v", e.cursorLine, e.cursorCol, e.additionalCursors)
			}
			e.Redo()
			assertEditorBytes(t, e, want)
			e.Undo()
			assertEditorBytes(t, e, original)
		})
	}
	t.Run("seams_use_original_byte_anchors", func(t *testing.T) {
		original := "\xc2X\x80Y"
		e := NewCodeEditor()
		e.SetText(original)
		e.cursorCol = 3
		e.additionalCursors = []cursorPos{{0, 1}}
		e.onKeyDownWithShift(KeyDelete, false, false)
		assertEditorBytes(t, e, "\xc2\x80")
		e.Undo()
		assertEditorBytes(t, e, original)
		e.Redo()
		assertEditorBytes(t, e, "\xc2\x80")
	})
}

func TestEditorByteExactMultiLineAndBoundaryHistory(t *testing.T) {
	for _, backspace := range []bool{false, true} {
		t.Run(map[bool]string{true: "backspace_joins", false: "delete_joins"}[backspace], func(t *testing.T) {
			original := "a\xff\n中\xfe\n😀z"
			e := NewCodeEditor()
			e.SetText(original)
			if backspace {
				e.cursorLine, e.cursorCol = 2, 0
				e.additionalCursors = []cursorPos{{1, 0}}
				e.onKeyDownWithShift(KeyBackSpace, false, false)
			} else {
				e.cursorLine, e.cursorCol = 0, 2
				e.additionalCursors = []cursorPos{{1, 2}}
				e.onKeyDownWithShift(KeyDelete, false, false)
			}
			assertEditorBytes(t, e, "a\xff中\xfe😀z")
			e.Undo()
			assertEditorBytes(t, e, original)
			e.Redo()
			assertEditorBytes(t, e, "a\xff中\xfe😀z")
		})
	}
	t.Run("per_caret_enter_indent", func(t *testing.T) {
		original := "if x {\n\tif y {"
		e := NewCodeEditor()
		e.SetText(original)
		e.cursorCol = 6
		e.additionalCursors = []cursorPos{{1, 7}}
		e.onKeyDownWithShift(KeyEnter, false, false)
		assertEditorBytes(t, e, "if x {\n\t\n\tif y {\n\t\t")
		e.Undo()
		assertEditorBytes(t, e, original)
		e.Redo()
		assertEditorBytes(t, e, "if x {\n\t\n\tif y {\n\t\t")
	})
	t.Run("duplicates_and_noop", func(t *testing.T) {
		e := NewCodeEditor()
		e.SetText("a")
		e.additionalCursors = []cursorPos{{0, 0}, {0, -2}}
		e.onKeyDownWithShift(KeyBackSpace, false, false)
		assertEditorBytes(t, e, "a")
		if len(e.undoStack) != 0 {
			t.Fatal("noop recorded")
		}
		e.OnTextInput("!")
		assertEditorBytes(t, e, "!a")
		e.Undo()
		assertEditorBytes(t, e, "a")
	})
}

func TestEditorByteExactReplacementPublishesOnce(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("a\xffb")
	e.hasSelection = true
	e.selStartCol = 1
	e.selEndCol = 2
	calls := 0
	stop := e.SubscribeSymbolsChanged(func() {
		calls++
		if e.Text() != "a中\n😀b" {
			t.Fatal("transient deletion published")
		}
	})
	e.OnTextInput("中\n😀")
	stop()
	if calls != 1 {
		t.Fatalf("published %d changes, want 1", calls)
	}
	e.Undo()
	assertEditorBytes(t, e, "a\xffb")
}

// Exhaust all byte spans, including endpoints inside valid/invalid sequences,
// against plain byte slicing. Also replay the inverse using the original byte
// anchor: UTF-8 validity may change, but history must not reinterpret it.
func TestEditorByteRangeReference(t *testing.T) {
	for _, original := range []string{"", "abc", "中😀", "a\xff\n\xe2X\x82\xac\n", "a\n\n\xffb"} {
		for start := 0; start <= len(original); start++ {
			for end := start; end <= len(original); end++ {
				for _, insert := range []string{"", "!", "\n", "\xc2", "\x80", "\xe2\x82", "中\n\xff😀"} {
					prefix := original[:start]
					line := strings.Count(prefix, "\n")
					column := len(prefix) - strings.LastIndex(prefix, "\n") - 1
					e := &CodeEditor{lines: strings.Split(original, "\n")}
					p := e.replaceByteRange(line, column, end-start, insert)
					want := original[:start] + insert + original[end:]
					if got := strings.Join(e.lines, "\n"); got != want {
						t.Fatalf("%x [%d:%d] -> %x got %x want %x", original, start, end, insert, got, want)
					}
					if p.col < 0 || p.col > utf8.RuneCountInString(e.lines[p.line]) {
						t.Fatalf("invalid post-edit caret %v in %x", p, e.lines[p.line])
					}
					e.replaceByteRange(line, column, len(insert), original[start:end])
					if got := strings.Join(e.lines, "\n"); got != original {
						t.Fatalf("byte inverse lost source: %x -> %x", original, got)
					}
				}
			}
		}
	}
}

func TestEditorByteHistoryDoesNotMergeAcrossUndoBranch(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("a\xff")
	e.cursorCol = 1
	e.OnTextInput("!")
	e.OnTextInput("\n")
	e.Undo()
	e.OnTextInput("X")
	e.Undo()
	assertEditorBytes(t, e, "a!\xff")
	e.Undo()
	assertEditorBytes(t, e, "a\xff")
}

func FuzzEditorByteRange(f *testing.F) {
	for _, s := range []string{"a\xffb", "中😀\n\xe2X\x82\xac", "a\n\n", ""} {
		f.Add(s, "\xc2\n\x80", uint16(1), uint16(2))
	}
	f.Fuzz(func(t *testing.T, original, insert string, a, b uint16) {
		if len(original) > 4096 || len(insert) > 256 {
			t.Skip()
		}
		start, end := int(a)%(len(original)+1), int(b)%(len(original)+1)
		if start > end {
			start, end = end, start
		}
		prefix := original[:start]
		line := strings.Count(prefix, "\n")
		column := len(prefix) - strings.LastIndex(prefix, "\n") - 1
		e := &CodeEditor{lines: strings.Split(original, "\n")}
		p := e.replaceByteRange(line, column, end-start, insert)
		want := original[:start] + insert + original[end:]
		if got := strings.Join(e.lines, "\n"); got != want {
			t.Fatalf("splice bytes %x want %x", got, want)
		}
		if p.col < 0 || p.col > utf8.RuneCountInString(e.lines[p.line]) {
			t.Fatalf("caret %v", p)
		}
		e.replaceByteRange(line, column, len(insert), original[start:end])
		if got := strings.Join(e.lines, "\n"); got != original {
			t.Fatalf("inverse bytes %x want %x", got, original)
		}
	})
}

func TestEditorByteExactSnippetAnchor(t *testing.T) {
	lines := []string{"a\xffb", "中😀\xfe"}
	if got := byteOffsetForCursor(lines, 0, 2); got != 2 {
		t.Fatalf("byte offset %d want 2", got)
	}
	if got := byteOffsetForCursor(lines, 1, 2); got != 11 {
		t.Fatalf("byte offset %d want 11", got)
	}
	if got := byteOffsetForCursor(lines, 20, 20); got != len(strings.Join(lines, "\n")) {
		t.Fatalf("beyond EOF %d", got)
	}
	e := NewCodeEditor()
	e.filePath = "snippet.go"
	original := "/*\xff*/ iferr"
	e.SetText(original)
	e.cursorCol = utf8.RuneCountInString(original)
	if !e.tryExpandSnippetAtCursor() {
		t.Fatal("snippet did not expand")
	}
	want := "/*\xff*/ if err != nil {\n\treturn err\n}"
	assertEditorBytes(t, e, want)
	e.Undo()
	assertEditorBytes(t, e, original)
	e.Redo()
	assertEditorBytes(t, e, want)
}
