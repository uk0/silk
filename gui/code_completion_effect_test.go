package gui

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
	"unsafe"
)

func TestCompletionEffectNoopDoesNotConsumeKeys(t *testing.T) {
	for _, key := range []int{KeyEnter, KeyTab} {
		t.Run(fmt.Sprint(key), func(t *testing.T) {
			e := NewCodeEditor()
			t.Cleanup(e.CancelCompletion)
			e.SetSize(800, 500)
			e.SetText("UniqueQAVerb")
			e.ScrollToLineCol(0, 12)
			e.TriggerCompletion()
			if e.CompletionVisible() {
				t.Fatalf("no-op popup: %+v", e.completion.filtered)
			}
			e.OnKeyDown(key, false)
			want := "UniqueQAVerb\n"
			if key == KeyTab {
				want = "UniqueQAVerb\t"
			}
			assertEditorBytes(t, e, want)
			e.Undo()
			assertEditorBytes(t, e, "UniqueQAVerb")
		})
	}
}

func TestCompletionEffectCaseAndProviderRemainUseful(t *testing.T) {
	e := NewCodeEditor()
	t.Cleanup(e.CancelCompletion)
	e.SetText("uniqueqaverb\nUniqueQAVerb")
	e.ScrollToLineCol(0, 12)
	e.TriggerCompletion()
	if findItem(e.completion.filtered, "uniqueqaverb") != nil || findItem(e.completion.filtered, "UniqueQAVerb") == nil {
		t.Fatal("case correction hidden or no-op retained", e.completion.filtered)
	}
	called := 0
	e.SetExternalCompletions([]ExternalCompletion{{Label: "uniqueqaverb", Apply: func(*CodeEditor) { called++ }}})
	e.TriggerCompletion()
	if e.completion.filtered[0].Text != "uniqueqaverb" {
		t.Fatal("semantic same-label action hidden")
	}
	e.completion.Accept(e)
	if called != 1 {
		t.Fatal("provider action not run exactly once")
	}
	// The standalone SDK ranking API continues to rank exact matches first.
	if RankCompletions([]CompletionItem{{Text: "UniqueQAVerb"}, {Text: "uniqueqaverb"}}, "uniqueqaverb")[0].Text != "uniqueqaverb" {
		t.Fatal("standalone ranking changed")
	}
}

func TestCompletionEffectByteExactAtomicAcceptance(t *testing.T) {
	for _, tc := range []struct{ prefix, suffix, insertion string }{
		{"\xff中🙂 Qa", "\xfe tail\r\nnext\nlast  ", "QaValue"},
		{"\xff中🙂 Qa", "\xfe tail\r\nnext\nlast  ", "QaValue\r\n\t第二行"},
		{"Qa", "\x80", "Qa\xc2"},
	} {
		t.Run(fmt.Sprintf("%x", tc.insertion), func(t *testing.T) {
			e := NewCodeEditor()
			t.Cleanup(e.CancelCompletion)
			before := tc.prefix + tc.suffix
			e.SetText(before)
			e.ScrollToLineCol(0, utf8.RuneCountInString(tc.prefix))
			col := e.cursorCol
			e.SetExternalCompletions([]ExternalCompletion{{Label: "Qa", Insert: tc.insertion}})
			e.TriggerCompletion()
			for i, item := range e.completion.filtered {
				if item.Text == tc.insertion {
					e.completion.selectedIdx = i
				}
			}
			calls := 0
			e.onChanged = func(string) { calls++ }
			e.completion.Accept(e)
			want := strings.TrimSuffix(tc.prefix, "Qa") + tc.insertion + tc.suffix
			assertEditorBytes(t, e, want)
			if calls != 1 || len(e.undoStack) != 1 {
				t.Fatal("completion did not publish one atomic change", calls)
			}
			e.Undo()
			assertEditorBytes(t, e, before)
			if e.cursorCol != col {
				t.Fatal("undo lost original caret", e.cursorCol, col)
			}
			e.Redo()
			assertEditorBytes(t, e, want)
			if e.undoStack[0].kind != 4 || len(e.undoStack[0].group) != 1 || e.undoStack[0].group[0].before != "Qa" {
				t.Fatal("completion retained full document history")
			}
		})
	}
}

func TestCompletionEffectPendingEmptyEscapeAndDigits(t *testing.T) {
	e, posts := indexedCompletionEditor(t)
	e.SetText("UniqueQAVerb\n" + strings.Repeat("// padding\n", 4000) + "UniqueQAVerbOther")
	e.ScrollToLineCol(0, 12)
	e.TriggerCompletion()
	if e.CompletionVisible() {
		t.Fatal("pending fallback offered unchanged text")
	}
	delivery := waitCompletionPost(t, posts)
	e.OnKeyDown(KeyEsc, false)
	delivery()
	if e.CompletionVisible() || e.completion.open {
		t.Fatal("Escape did not retire invisible request")
	}
	e.SetText("Version\nVersion2Next")
	e.ScrollToLineCol(0, 7)
	e.OnTextInput("2")
	if !e.CompletionVisible() || e.completion.prefix != "Version2" || findItem(e.completion.filtered, "Version2Next") == nil {
		t.Fatal("digit input canceled useful completion")
	}
}

func TestCompletionEffectPendingResultFillsAllRows(t *testing.T) {
	e, posts := indexedCompletionEditor(t)
	text := "UniqueQAVerb\n" + strings.Repeat("// padding\n", 4000)
	for i := 0; i < 60; i++ {
		text += fmt.Sprintf("UniqueQAVerb%02d\n", i)
	}
	e.SetText(text)
	e.ScrollToLineCol(0, 12)
	e.TriggerCompletion()
	waitCompletionPost(t, posts)()
	if len(e.completion.filtered) != completionCandidateLimit || findItem(e.completion.filtered, "UniqueQAVerb") != nil {
		t.Fatal("no-op occupied bounded candidate capacity", len(e.completion.filtered))
	}
}

func TestCompletionEffectSelectionAndHistoryAreBounded(t *testing.T) {
	e := NewCodeEditor()
	t.Cleanup(e.CancelCompletion)
	before := "Qa tail\r\n" + strings.Repeat("untouched\xff\n", 10000)
	e.SetText(before)
	e.ScrollToLineCol(0, 2)
	e.hasSelection = true
	e.selStartCol = 0
	e.selEndCol = 2
	selection := e.selectionSnapshot()
	e.additionalCursors = []cursorPos{{1, 3}}
	e.SetExternalCompletions([]ExternalCompletion{{Label: "QaValue"}})
	e.TriggerCompletion()
	e.completion.Accept(e)
	assertEditorBytes(t, e, "QaValue"+before[2:])
	if e.hasSelection || len(e.additionalCursors) != 0 {
		t.Fatal("accept did not keep one insertion point")
	}
	a := e.undoStack[len(e.undoStack)-1]
	if a.text != "" || a.oldText != "" || len(a.group) != 1 || a.group[0].before != "Qa" || a.group[0].after != "QaValue" {
		t.Fatal("history retains unrelated source")
	}
	e.Undo()
	assertEditorBytes(t, e, before)
	if e.selectionSnapshot() != selection || len(e.additionalCursors) != 1 || e.additionalCursors[0] != (cursorPos{1, 3}) || e.cursorCol != 2 {
		t.Fatal("undo lost selection/caret state")
	}
	e.Redo()
	assertEditorBytes(t, e, "QaValue"+before[2:])
	if e.cursorCol != 7 || e.hasSelection || len(e.additionalCursors) != 0 {
		t.Fatal("redo state wrong")
	}
}

func TestCompletionEffectPendingNavigationAndFocus(t *testing.T) {
	for _, action := range []string{"escape", "left", "right", "home", "end", "up", "down", "pageup", "pagedown", "scroll", "jump", "focus"} {
		t.Run(action, func(t *testing.T) {
			e, posts := indexedCompletionEditor(t)
			e.SetText("UniqueQAVerb\n" + strings.Repeat("// padding\n", 4000) + "UniqueQAVerbOther")
			e.ScrollToLineCol(0, 12)
			e.TriggerCompletion()
			delivery := waitCompletionPost(t, posts)
			if e.CompletionVisible() {
				t.Fatal("unexpected pending row")
			}
			keys := map[string]int{"escape": KeyEsc, "left": KeyLeft, "right": KeyRight, "home": KeyHome, "end": KeyEnd, "up": KeyUp, "down": KeyDown, "pageup": KeyPageUp, "pagedown": KeyPageDown}
			switch action {
			case "scroll":
				e.OnMouseWheel(750, 420, -1)
			case "jump":
				e.NavigateToLineCol(0, 12)
			case "focus":
				e.OnFocusChanged(NewEdit(), e)
			default:
				e.OnKeyDown(keys[action], false)
			}
			delivery()
			if e.completion.open || e.CompletionVisible() {
				t.Fatal("pending request survived", action)
			}
		})
	}
}

func TestCompletionEffectStaleLiteralAndInvalidIndex(t *testing.T) {
	e := NewCodeEditor()
	t.Cleanup(e.CancelCompletion)
	e.SetText("Qa")
	e.ScrollToLineCol(0, 2)
	e.SetExternalCompletions([]ExternalCompletion{{Label: "QaValue"}})
	e.TriggerCompletion()
	e.cursorCol = 1 // model an uncoordinated host; prefix no longer belongs here
	e.completion.Accept(e)
	assertEditorBytes(t, e, "Qa")
	if len(e.undoStack) != 0 {
		t.Fatal("stale prefix recorded edit")
	}
	for _, index := range []int{-1, 999} {
		e.cursorCol = 2
		e.TriggerCompletion()
		e.completion.selectedIdx = index
		e.completion.Accept(e)
		assertEditorBytes(t, e, "Qa")
	}
}

func BenchmarkCompletionEffectAcceptance(b *testing.B) {
	for _, n := range []int{1000, 100000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			e := NewCodeEditor()
			b.Cleanup(e.CancelCompletion)
			e.SetSize(800, 500)
			text := "Qa\n" + strings.Repeat("unchanged source\n", n)
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				e.SetText(text)
				e.ScrollToLineCol(0, 2)
				p := NewCompletionPopup(e)
				p.prefix = "Qa"
				p.visible = true
				p.filtered = []CompletionItem{{Text: "QaValue"}}
				e.completion = p
				b.StartTimer()
				p.Accept(e)
			}
		})
	}
}

func TestCompletionEffectHistoryDoesNotRetainSourceSlice(t *testing.T) {
	e := NewCodeEditor()
	t.Cleanup(e.CancelCompletion)
	source := "QaValue\n" + strings.Repeat("old source payload\n", 10000) + "Qa"
	e.SetText(source)
	e.ScrollToLineCol(10001, 2)
	p := NewCompletionPopup(e)
	e.completion = p
	p.prefix = "Qa"
	p.visible = true
	p.filtered = []CompletionItem{{Text: source[:7]}}
	p.Accept(e)
	if len(e.undoStack) != 1 || len(e.undoStack[0].group) != 1 {
		t.Fatal("not a local transaction")
	}
	kept := e.undoStack[0].group[0].after
	if kept != "QaValue" {
		t.Fatal("wrong completion", kept)
	}
	start := uintptr(unsafe.Pointer(unsafe.StringData(source)))
	ptr := uintptr(unsafe.Pointer(unsafe.StringData(kept)))
	if ptr >= start && ptr < start+uintptr(len(source)) {
		t.Fatal("history retains entire source allocation through candidate substring")
	}
}
