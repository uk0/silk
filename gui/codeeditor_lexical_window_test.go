package gui

import (
	"math/rand"
	"strings"
	"testing"
	"unsafe"
)

func TestEditorGoLocalEditDoesNotRebindUntouchedRows(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("package p\n" + strings.Repeat("var n = 42\n", 100_000))

	// Give an unchanged cache row equivalent bytes in separate storage.
	// Full prefix/suffix reconciliation would rebind it to the model line;
	// a local edit must not have visited this unrelated row.
	untouched := strings.Clone(e.lines[0])
	e.goLineStates.lines[0].text = untouched
	e.ScrollToLineCol(75_000, 4)
	e.OnTextInput("x")
	if got := e.goLineStates.lines[0].text; unsafe.StringData(got) != unsafe.StringData(untouched) {
		t.Fatal("local edit reconciled an untouched prefix row")
	}
	assertGoLineStates(t, &e.goLineStates, e.lines)

	// Reloaded equal bytes still need to release any old document storage.
	e.SetText(strings.Clone(e.Text()))
	if unsafe.StringData(e.goLineStates.lines[0].text) != unsafe.StringData(e.lines[0]) {
		t.Fatal("whole-buffer reload retained old storage")
	}
}

func TestEditorGoLineWindowThroughHistoryAndStructuralFallback(t *testing.T) {
	e := NewCodeEditor()
	original := "package p\nstart\nbody\n*/\nvar value = 1\n"
	e.SetText(original)
	e.ScrollToLineCol(1, 0)
	e.OnTextInput("/*")
	if e.goLineStates.lines[2].in != goLexBlock || e.goLineStates.lines[4].in != goLexNormal {
		t.Fatal("block-comment state did not propagate and converge")
	}
	assertGoLineStates(t, &e.goLineStates, e.lines)
	e.Undo()
	assertGoLineStates(t, &e.goLineStates, e.lines)
	e.Redo()
	assertGoLineStates(t, &e.goLineStates, e.lines)

	n := len(e.lines)
	untouched := n - 2
	e.goLineStates.lines[untouched].text = strings.Clone(e.lines[untouched])
	e.replaceByteRange(1, 0, 0, "// insert\n")
	e.replaceByteRange(3, len(e.lines[3]), 1, "")
	if len(e.lines) != n || !e.pendingGoEdit.bulk {
		t.Fatal("shifting edits must take the reconciliation path even when they cancel in count")
	}
	e.Text()
	assertGoLineStates(t, &e.goLineStates, e.lines)
	if unsafe.StringData(e.goLineStates.lines[untouched].text) != unsafe.StringData(e.lines[untouched]) {
		t.Fatal("structural edit did not reconcile suffix storage")
	}
}

func TestGoLineWindowVisitsOnlyUntilStateConverges(t *testing.T) {
	lines := []string{"package p", "start", "other", "*/", "tail", "tail"}
	var c goLineStateCache
	c.update(lines)
	lines[1] = "updated"
	if n := c.updateChangedRows(lines, 1, 2); n != 1 {
		t.Fatalf("ordinary edit visited %d rows, want 1", n)
	}
	lines[1] = "/*"
	if n := c.updateChangedRows(lines, 1, 2); n != 3 {
		t.Fatalf("block edit visited %d rows, want 3", n)
	}
	assertGoLineStates(t, &c, lines)
	lines[1], lines[3] = "var s = `", "`"
	var hint goLineEdit
	hint.add(3, 1, 1)
	hint.add(1, 1, 1)
	if n := c.updateChangedRows(lines, hint.from, hint.to); n != 3 {
		t.Fatalf("coalesced raw string edit visited %d rows, want 3", n)
	}
	assertGoLineStates(t, &c, lines)
}

func TestGoLineWindowRandomizedEditsMatchFullScan(t *testing.T) {
	r := rand.New(rand.NewSource(51196))
	corpus := []string{"", "/*", " */ ", "`", "var raw = `", "` // end", "/* ` comment", "'/'", "中文😀\r", "\xff/`", "\"/*\""}
	lines := make([]string, 128)
	var c goLineStateCache
	c.update(lines)
	for i := 0; i < 500; i++ {
		var hint goLineEdit
		for j := 0; j <= i%4; j++ {
			p := r.Intn(len(lines))
			lines[p] = corpus[r.Intn(len(corpus))]
			hint.add(p, 1, 1)
		}
		c.updateChangedRows(lines, hint.from, hint.to)
		assertGoLineStates(t, &c, lines)
	}
}

func TestEditorBookmarkObserverSeesPublishedGoLineState(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("package p\nstart\nbody\n*/\ntail")
	e.ScrollToLineCol(1, 0)
	e.ToggleBookmark()
	seen := false
	stop := e.SubscribeBookmarksChanged(func() {
		seen = true
		if e.goLineStates.lines[2].in != goLexBlock {
			t.Error("bookmark consumer observed old syntax for the published buffer")
		}
	})
	defer stop()
	e.OnTextInput("/*")
	if !seen {
		t.Fatal("expected post-publication bookmark notification")
	}
}
