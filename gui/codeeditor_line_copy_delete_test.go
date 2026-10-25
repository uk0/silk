package gui

import (
	"reflect"
	"strings"
	"testing"
	"unsafe"
)

func TestDuplicateLinesExcludesColumnZeroEndpoint(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("a\nb\nc\nd\n")
	e.hasSelection = true
	e.selStartLine = 1
	e.selEndLine = 3
	e.cursorLine = 3
	e.DuplicateLines()
	if e.Text() != "a\nb\nc\nb\nc\nd\n" {
		t.Fatalf("exclusive endpoint: %q", e.Text())
	}
	e.Undo()
	if !e.hasSelection || e.selStartLine != 1 || e.selEndLine != 3 {
		t.Fatal("Undo lost selection")
	}
}
func TestDuplicateLinesKeepsCRLFTail(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("a\r\nb")
	e.cursorLine = 1
	e.DuplicateLines()
	if e.Text() != "a\r\nb\r\nb" {
		t.Fatalf("mixed tail: %q", e.Text())
	}
}
func TestDuplicateLinesGroupsMultipleCarets(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("a\nb\nc\nd\n")
	e.additionalCursors = []cursorPos{{1, 0}, {3, 0}}
	e.DuplicateLines()
	if e.Text() != "a\nb\na\nb\nc\nd\nd\n" {
		t.Fatalf("groups: %q", e.Text())
	}
}

func TestDeleteLinesGroupsAndEOF(t *testing.T) {
	for _, tc := range []struct {
		text string
		rows []int
		want string
	}{
		{"a\r\nb", []int{1}, "a"}, {"a\r\n", []int{1}, "a"}, {"a\n\n", []int{1}, "a\n"},
		{"a\nb\nc", []int{0, 2}, "b"}, {"0\n1\n2\n3\n4", []int{0, 2, 4}, "1\n3"},
		{"a\r\nb\nc\r", []int{1}, "a\r\nc\r"}, {"x\r\r\ny\xff", []int{1}, "x\r"},
		{"a\nb\n", []int{0, 1, 2}, ""}, {"one", []int{0}, ""},
	} {
		e := NewCodeEditor()
		e.SetText(tc.text)
		e.cursorLine = tc.rows[0]
		for _, r := range tc.rows[1:] {
			e.additionalCursors = append(e.additionalCursors, cursorPos{r, 0})
		}
		before := append([]cursorPos{{e.cursorLine, 0}}, e.additionalCursors...)
		e.DeleteLines()
		if e.Text() != tc.want || e.hasSelection {
			t.Fatalf("%q rows=%v -> %q", tc.text, tc.rows, e.Text())
		}
		e.Undo()
		if e.Text() != tc.text || !reflect.DeepEqual(append([]cursorPos{{e.cursorLine, e.cursorCol}}, e.additionalCursors...), before) {
			t.Fatal("Undo positions/bytes")
		}
		e.Redo()
		if e.Text() != tc.want {
			t.Fatal("Redo")
		}
	}
}
func TestLineCopyDeleteSelectionDirectionAndHistory(t *testing.T) {
	for _, remove := range []bool{false, true} {
		for _, reverse := range []bool{false, true} {
			e := NewCodeEditor()
			e.SetText("old")
			source := "a\r\n中\xff🙂\nb\r\nlast"
			e.ReplaceAllText(source)
			e.hasSelection = true
			e.selStartLine, e.selStartCol = 1, 1
			e.selEndLine, e.selEndCol = 3, 0
			if reverse {
				e.selStartLine, e.selEndLine = e.selEndLine, e.selStartLine
				e.selStartCol, e.selEndCol = e.selEndCol, e.selStartCol
			}
			e.cursorLine, e.cursorCol = e.selEndLine, e.selEndCol
			selection := e.selectionSnapshot()
			caret := cursorPos{e.cursorLine, e.cursorCol}
			changed := 0
			e.SigChanged(func(string) { changed++ })
			if remove {
				e.DeleteLines()
			} else {
				e.DuplicateLines()
			}
			want := "a\r\n中\xff🙂\nb\r\n中\xff🙂\nb\r\nlast"
			if remove {
				want = "a\r\nlast"
			}
			if e.Text() != want || changed != 1 {
				t.Fatal(remove, reverse, e.Text(), changed)
			}
			next := e.selectionSnapshot()
			after := cursorPos{e.cursorLine, e.cursorCol}
			if !remove && (next.start.line != selection.start.line+2 || next.end.line != selection.end.line+2) {
				t.Fatal("selection copy direction", next)
			}
			e.Undo()
			if e.Text() != source || e.selectionSnapshot() != selection || (cursorPos{e.cursorLine, e.cursorCol}) != caret {
				t.Fatal("Undo selection")
			}
			e.Redo()
			if e.Text() != want || e.selectionSnapshot() != next || (cursorPos{e.cursorLine, e.cursorCol}) != after || changed != 3 {
				t.Fatal("Redo selection")
			}
			e.Undo()
			e.Undo()
			if e.Text() != "old" {
				t.Fatal("earlier history")
			}
		}
	}
}
func TestDuplicateLinesTracksDistinctCarets(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("0\n11\n22\n33\n44")
	e.additionalCursors = []cursorPos{{1, 1}, {1, 0}, {3, 1}}
	e.DuplicateLines()
	want := []cursorPos{{2, 0}, {3, 1}, {3, 0}, {6, 1}}
	if e.Text() != "0\n11\n0\n11\n22\n33\n33\n44" || !reflect.DeepEqual(append([]cursorPos{{e.cursorLine, e.cursorCol}}, e.additionalCursors...), want) {
		t.Fatal(e.Text(), e.additionalCursors)
	}
}
func TestLineCopyDeleteEmptyAndTransientState(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("")
	e.MarkSaved()
	r := e.TextRevision()
	e.snippetSess = &snippetSession{}
	s := e.snippetSess
	if e.CanDeleteLines() || !e.CanDuplicateLines() {
		t.Fatal("availability")
	}
	e.DeleteLines()
	if e.Modified() || e.TextRevision() != r || len(e.undoStack) != 0 || e.snippetSess != s {
		t.Fatal("empty delete changed state")
	}
	e.DuplicateLines()
	if e.Text() != "\n" || e.snippetSess != nil {
		t.Fatal("empty duplicate")
	}
	e.Undo()
	if e.Text() != "" {
		t.Fatal("empty Undo")
	}
	for _, remove := range []bool{false, true} {
		e.SetText("one\ntwo\n")
		e.hasSelection = true
		e.selStartLine = 0
		e.selEndLine = 1
		e.selEndCol = 1
		if err := e.SetFindInSelection(true); err != nil {
			t.Fatal(err)
		}
		e.snippetSess = &snippetSession{}
		e.renameActive = true
		e.gotoLineActive = true
		if remove {
			e.DeleteLines()
		} else {
			e.DuplicateLines()
		}
		if e.snippetSess != nil || e.renameActive || e.gotoLineActive || e.findState.scope.valid || !e.findState.scope.enabled {
			t.Fatal("stale transient state")
		}
	}
}
func TestLineSpliceHistoryDoesNotRetainUnchangedBodies(t *testing.T) {
	for _, remove := range []bool{false, true} {
		e := NewCodeEditor()
		original := strings.Repeat("x", 1<<20) + "\nsmall\ntail"
		e.SetText(original)
		e.cursorLine = 1
		if remove {
			e.DeleteLines()
		} else {
			e.DuplicateLines()
		}
		h := e.undoStack[len(e.undoStack)-1]
		if h.kind != 6 || h.text != "" || h.oldText != "" || len(h.group) > 0 {
			t.Fatal("whole document history")
		}
		total := 0
		lo := uintptr(unsafe.Pointer(unsafe.StringData(original)))
		hi := lo + uintptr(len(original))
		for _, c := range h.lineSplices {
			for _, list := range [][]string{c.before, c.after} {
				for _, s := range list {
					total += len(s)
					ptr := uintptr(unsafe.Pointer(unsafe.StringData(s)))
					if len(s) > 0 && ptr >= lo && ptr < hi {
						t.Fatal("retained original text backing storage")
					}
				}
			}
		}
		if total > 10 {
			t.Fatal("unchanged body retained", total)
		}
	}
}
func TestLineSplicesRebaseUntouchedFolds(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("header\nfunc a() {\n a\n}\nfunc b() {\n b\n}\ntail")
	e.ToggleFold(1)
	e.ToggleFold(4)
	e.DuplicateLines()
	if !e.foldedLines[2] || !e.foldedLines[5] || len(e.foldedLines) != 2 {
		t.Fatal("copy fold rebasing", e.foldedLines)
	}
	e.Undo()
	if !e.foldedLines[1] || !e.foldedLines[4] {
		t.Fatal("Undo fold rebasing")
	}
	e.cursorLine = 2
	e.DeleteLines()
	if len(e.foldedLines) != 1 || !e.foldedLines[3] {
		t.Fatal("touched fold expansion", e.foldedLines)
	}
	e.Undo()
	if len(e.foldedLines) != 1 || !e.foldedLines[4] {
		t.Fatal("delete Undo rebase", e.foldedLines)
	}
}
func TestLineCopyDeleteRoundTripMatrix(t *testing.T) {
	parts := []string{"", "a", "中\xff", "x\r"}
	for _, a := range parts {
		for _, b := range parts {
			for _, c := range parts {
				for mask := 1; mask < 8; mask++ {
					for _, remove := range []bool{false, true} {
						e := NewCodeEditor()
						source := a + "\n" + b + "\n" + c
						e.SetText(source)
						first := true
						for row := 0; row < 3; row++ {
							if mask&(1<<uint(row)) != 0 {
								if first {
									e.cursorLine = row
									first = false
								} else {
									e.additionalCursors = append(e.additionalCursors, cursorPos{row, 0})
								}
							}
						}
						before := append([]cursorPos{{e.cursorLine, 0}}, e.additionalCursors...)
						if remove {
							e.DeleteLines()
						} else {
							e.DuplicateLines()
						}
						result := e.Text()
						e.Undo()
						if e.Text() != source || !reflect.DeepEqual(append([]cursorPos{{e.cursorLine, e.cursorCol}}, e.additionalCursors...), before) {
							t.Fatalf("roundtrip %q mask=%d remove=%v", source, mask, remove)
						}
						e.Redo()
						if e.Text() != result {
							t.Fatal("matrix Redo")
						}
					}
				}
			}
		}
	}
}
func TestLineSpliceBatchAllocationCount(t *testing.T) {
	e := &CodeEditor{lines: make([]string, 10000)}
	changes := make([]editorLineSplice, 64)
	for i := range changes {
		changes[i] = editorLineSplice{at: i * 100, after: []string{"x"}}
	}
	allocs := testing.AllocsPerRun(5, func() { e.applyLineSplices(changes, false); e.applyLineSplices(changes, true) })
	if allocs > 4 {
		t.Fatalf("per-caret line index allocations: %.0f", allocs)
	}
}
func BenchmarkLineSpliceBatchIndex(b *testing.B) {
	e := &CodeEditor{lines: make([]string, 100000)}
	changes := make([]editorLineSplice, 64)
	for i := range changes {
		changes[i] = editorLineSplice{at: i * 1000, after: []string{"x"}}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.applyLineSplices(changes, false)
		e.applyLineSplices(changes, true)
	}
}
func TestCutWithoutSelectionDoesNotDeleteOtherCaretRows(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("a\nb\nc")
	e.additionalCursors = []cursorPos{{2, 0}}
	e.deleteCurrentLine()
	if e.Text() != "b\nc" || e.CursorCount() != 2 {
		t.Fatal("cut deleted extra rows")
	}
	e.Undo()
	if e.Text() != "a\nb\nc" || e.additionalCursors[0].line != 2 {
		t.Fatal("cut Undo")
	}
}
