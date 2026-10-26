package gui

import (
	"strings"
	"testing"
)

func TestDefinitionRecoveryAfterUnfinishedBody(t *testing.T) {
	for _, body := range []string{"if c.ready\nc.Close()", "x := ", "x := []int{1, 2,", "if true {"} {
		src := "package p\nfunc(c *Client) Run(){\n" + body + "\n}\nfunc (c *Client) Close() {}\n"
		target := FindDefinition("Close", "client.go", src)
		line := strings.Count(src[:strings.Index(src, "func (c *Client) Close")], "\n")
		if target == nil || target.FilePath != "client.go" || target.Line != line || target.Kind != "method" {
			t.Fatalf("%q: %+v expected line %d", body, target, line)
		}
	}
	for _, text := range []string{"// func Fake() {}", "/* func Fake() {} */", "var str = `func Fake() {}`"} {
		if target := findInContent("Fake", "p.go", "package p\nfunc broken(){ if true\n}\n"+text); target != nil {
			t.Fatalf("invented lexical declaration %+v", target)
		}
	}
}

func TestSignatureHelpSurvivesArgumentInputAndModifiers(t *testing.T) {
	e := NewCodeEditor()
	e.SetSize(600, 300)
	e.SetText("fn(")
	e.cursorCol = 3
	// No native window in package tests; model the visible owner/state directly.
	tip := newTooltipWindow("fn(value string)")
	tip.owner = e
	tooltipMu.Lock()
	currentTooltip = tip
	tooltipMu.Unlock()
	e.signatureHintVisible = true
	defer HideToolTip()
	for _, key := range []int{KeyShift, 'X'} {
		e.OnKeyDown(key, false)
		if !IsToolTipVisible() {
			t.Fatal("key dismissed signature", key)
		}
	}
	e.OnTextInput("x")
	if !IsToolTipVisible() || !e.signatureHintVisible {
		t.Fatal("typing dismissed signature")
	}
	e.OnKeyDown(KeyEsc, false)
	if IsToolTipVisible() || e.signatureHintVisible {
		t.Fatal("Escape kept signature alive")
	}
}

func TestExternalCompletionExpiresOnEditsAndUndo(t *testing.T) {
	e := NewCodeEditor()
	e.SetSize(500, 300)
	e.SetText("//silk:expand ")
	e.cursorCol = len([]rune(e.Text()))
	called := false
	set := func() {
		e.SetExternalCompletions([]ExternalCompletion{{Label: "Declare", Apply: func(*CodeEditor) { called = true }}})
		e.TriggerCompletion()
	}
	set()
	e.OnTextInput("D")
	if len(e.externalCompletions) != 0 {
		t.Fatal("stale external completion retained")
	}
	e.OnKeyDown(KeyEnter, false)
	if called || e.Text() == "//silk:expand D" {
		t.Fatalf("Enter swallowed by stale action: %q", e.Text())
	}
	set()
	e.Undo()
	if len(e.externalCompletions) != 0 {
		t.Fatal("undo retained provider snapshot")
	}
	set()
	e.Redo()
	if len(e.externalCompletions) != 0 {
		t.Fatal("redo retained provider snapshot")
	}
}

func TestTextBlockLinearWrapPreservesUnicodeAndRows(t *testing.T) {
	for _, width := range []float64{1, 24, 400} {
		var b TextBlock
		b.SetWrap(true)
		original := strings.Repeat("中a🚀", 4096) + "\r\nlast"
		b.SetText(original)
		b.Layout(width)
		end := 0
		for i, row := range b.rows {
			if row.begin != end || row.end <= row.begin {
				t.Fatalf("gap at %d: %+v", i, row)
			}
			if len(row.carets) != len(row.text)+1 {
				t.Fatal("missing caret boundaries")
			}
			if row.caretX(0) != 0 {
				t.Fatal("carets not rebased")
			}
			end = row.end
		}
		if end != len(b.text) || b.Text() != original {
			t.Fatal("wrap lost text/newlines")
		}
	}
}

func TestMessageBoxDefersUnmeasuredWrap(t *testing.T) {
	box := NewMessageBox()
	text := strings.Repeat("x", 32768)
	box.SetMessage(text)
	if box.edit.SoftRowsCount() != 0 {
		t.Fatal("wrapped unmeasured viewport")
	}
	box.SetSize(620, 280)
	if box.edit.SoftRowsCount() < 2 || box.edit.Text() != text {
		t.Fatal("real geometry did not wrap")
	}
}

func BenchmarkReviewMessageBox32K(b *testing.B) {
	text := strings.Repeat("x", 32768)
	for b.Loop() {
		box := NewMessageBox()
		box.SetMessage(text)
		box.SetSize(620, 280)
	}
}

func TestExternalCompletionExpiresWhenCaretMoves(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("alpha\nD")
	e.cursorLine = 1
	e.cursorCol = 1
	called := false
	e.SetExternalCompletions([]ExternalCompletion{{Label: "Declare", Apply: func(*CodeEditor) { called = true }}})
	e.TriggerCompletion()
	e.cursorLine, e.cursorCol = 0, 5
	e.OnKeyDown(KeyEnter, false)
	if called || e.Text() != "alpha\n\nD" {
		t.Fatalf("stale position swallowed input: %q", e.Text())
	}
}

func TestNilExternalCompletionDismissesOldActions(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("D")
	e.cursorCol = 1
	e.SetExternalCompletions([]ExternalCompletion{{Label: "Declare", Apply: func(*CodeEditor) { t.Fatal("retired provider action") }}})
	e.TriggerCompletion()
	e.SetExternalCompletions(nil)
	e.OnKeyDown(KeyEnter, false)
	if e.Text() != "D\n" {
		t.Fatalf("nil provider kept stale popup: %q", e.Text())
	}
}
