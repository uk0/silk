package gui

import (
	"reflect"
	"strings"
	"testing"
)

func TestEditorHistoryAvailabilityTransitions(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("a\nb\n")
	e.MarkSaved()
	var states [][2]bool
	changed := 0
	e.SigChanged(func(string) { changed++ })
	stop := e.SubscribeHistoryChanged(func() {
		states = append(states, [2]bool{e.CanUndo(), e.CanRedo()})
		if e.textDirty {
			t.Error("published incomplete text")
		}
	})
	if e.CanUndo() || e.CanRedo() {
		t.Fatal("initial history")
	}
	e.DuplicateLines()
	e.DuplicateLines()
	e.Undo()
	e.Undo()
	e.Undo()
	e.Redo()
	e.Redo()
	e.Redo()
	e.Undo()
	e.DeleteLines()
	e.MarkSaved()
	n := len(states)
	e.ScrollToLineCol(0, 0)
	e.SetTabSize(2)
	if len(states) != n || e.Modified() {
		t.Fatal("save/caret altered history")
	}
	e.SetText(e.Text())
	want := [][2]bool{{true, false}, {true, true}, {false, true}, {true, true}, {true, false}, {true, true}, {true, false}, {false, false}}
	if !reflect.DeepEqual(states, want) || changed != 8 {
		t.Fatal(states, changed)
	}
	stop()
	stop()
	e.DuplicateLines()
	if len(states) != len(want) || len(e.historyObservers) != 0 {
		t.Fatal("unsubscribe")
	}
	e.SubscribeHistoryChanged(nil)()
}
func TestEditorHistoryObserversCanDetachAndSubscribe(t *testing.T) {
	e := NewCodeEditor()
	var calls []string
	var stopSecond, stopNew func()
	stopFirst := e.SubscribeHistoryChanged(func() {
		calls = append(calls, "first")
		stopSecond()
		if stopNew == nil {
			stopNew = e.SubscribeHistoryChanged(func() { calls = append(calls, "new") })
		}
	})
	stopSecond = e.SubscribeHistoryChanged(func() { calls = append(calls, "second") })
	e.DuplicateLines()
	e.Undo()
	if !reflect.DeepEqual(calls, []string{"first", "first", "new"}) {
		t.Fatal(calls)
	}
	stopFirst()
	stopNew()
	if e.historyObservers != nil {
		t.Fatal("retained observers")
	}
}
func TestEditorHistorySignalsCoverEditKinds(t *testing.T) {
	for name, edit := range map[string]func(*CodeEditor){
		"input":   func(e *CodeEditor) { e.OnTextInput("x") },
		"delete":  func(e *CodeEditor) { e.onKeyDownWithShift(KeyDelete, false, false) },
		"replace": func(e *CodeEditor) { e.ReplaceAllText("replacement") },
		"indent":  func(e *CodeEditor) { e.IndentLines() },
		"move":    func(e *CodeEditor) { e.MoveLinesDown() },
		"splice":  func(e *CodeEditor) { e.DuplicateLines() },
	} {
		t.Run(name, func(t *testing.T) {
			e := NewCodeEditor()
			e.SetText("a\nb\n")
			n := 0
			stop := e.SubscribeHistoryChanged(func() { n++ })
			defer stop()
			edit(e)
			if !e.CanUndo() || e.CanRedo() || n != 1 {
				t.Fatal("edit", n)
			}
			e.Undo()
			if e.CanUndo() || !e.CanRedo() || n != 2 {
				t.Fatal("undo", n)
			}
			e.Redo()
			if !e.CanUndo() || e.CanRedo() || n != 3 {
				t.Fatal("redo", n)
			}
			e.SetText(e.Text())
			if e.CanUndo() || e.CanRedo() || n != 4 {
				t.Fatal("reset", n)
			}
		})
	}
}
func TestEditorHistoryTypingAndIdleQueriesAreSilent(t *testing.T) {
	e := NewCodeEditor()
	e.SetText(strings.Repeat("large\n", 100000))
	n := 0
	stop := e.SubscribeHistoryChanged(func() { n++ })
	defer stop()
	e.OnTextInput("x")
	e.OnTextInput("y")
	e.OnTextInput("z")
	if n != 1 {
		t.Fatal("availability notified on unchanged typing", n)
	}
	rev := e.TextRevision()
	if alloc := testing.AllocsPerRun(100, func() { _ = e.CanUndo(); _ = e.CanRedo(); e.notifyHistoryChanged() }); alloc != 0 {
		t.Fatal("idle allocated", alloc)
	}
	if n != 1 || e.TextRevision() != rev {
		t.Fatal("idle mutated state")
	}
}

func BenchmarkEditorHistoryAvailability(b *testing.B) {
	e := NewCodeEditor()
	e.SetText(strings.Repeat("line\n", 100000))
	e.DuplicateLines()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = e.CanUndo()
		_ = e.CanRedo()
		e.notifyHistoryChanged()
	}
}
