package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

type signatureScheduleProbe struct {
	documentProbe
	signatures      chan int
	cancelled       chan int
	releaseObsolete chan struct{}
}

func (p *signatureScheduleProbe) SignatureHintContext(ctx context.Context, _ string, _, col int) (core.LSPActiveSignature, error) {
	p.signatures <- col
	if col == 1 {
		<-ctx.Done()
		p.cancelled <- col
		if p.releaseObsolete != nil {
			<-p.releaseObsolete
		}
	}
	// Return a nominal value even upon cancellation. Obsolete replies must
	// still be dropped if the transport reports "success" during cancellation.
	return core.LSPActiveSignature{Label: "run(count int)"}, nil
}

func waitSignatureTest[T any](t *testing.T, c <-chan T) T {
	t.Helper()
	select {
	case v := <-c:
		return v
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for signature scheduler")
	}
	var zero T
	return zero
}

func TestSignatureSchedulerCoalescesAndDropsCancelledResponse(t *testing.T) {
	p := &signatureScheduleProbe{documentProbe: documentProbe{requests: make(chan int, 8)}, signatures: make(chan int, 8), cancelled: make(chan int, 8), releaseObsolete: make(chan struct{})}
	done := make(chan lspSnapshot, 8)
	d := newLSPDocumentWithSignatures(p, "file:///qa.go", "base", nil, nil, func(s lspSnapshot, _ core.LSPActiveSignature) { done <- s })
	t.Cleanup(func() {
		select {
		case <-p.releaseObsolete:
		default:
			close(p.releaseObsolete)
		}
		d.close()
		waitDocument(t, d)
	})
	d.update(lspSnapshot{text: "first", version: 2, col: 1, signature: true})
	if got := waitSignatureTest(t, p.signatures); got != 1 {
		t.Fatalf("first caret=%d", got)
	}
	for v := 3; v <= 603; v++ {
		d.update(lspSnapshot{text: "newest", version: v, line: 2, col: v, signature: true, signatureEpoch: 4})
	}
	if waitSignatureTest(t, p.cancelled) != 1 {
		t.Fatal("obsolete request not cancelled")
	}
	close(p.releaseObsolete)
	got := waitSignatureTest(t, done)
	if got.version != 603 || got.line != 2 || got.col != 603 || got.signatureEpoch != 4 {
		t.Fatalf("obsolete or wrong snapshot: %+v", got)
	}
	if waitSignatureTest(t, p.signatures) != 603 || len(p.signatures) != 0 {
		t.Fatal("burst was not coalesced")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.events[len(p.events)-1] != "change:603:newest" {
		t.Fatalf("wrong wire version before signature: %v", p.events)
	}
}

func TestSignatureSchedulerDoesNotBlockCompletion(t *testing.T) {
	p := &signatureScheduleProbe{documentProbe: documentProbe{requests: make(chan int, 4)}, signatures: make(chan int, 4), cancelled: make(chan int, 4)}
	done := make(chan lspSnapshot, 4)
	d := newLSPDocumentWithSignatures(p, "file:///qa.go", "base", nil, func(s lspSnapshot, _ []core.LSPCompletionItem, err error) {
		if err == nil {
			done <- s
		}
	}, func(lspSnapshot, core.LSPActiveSignature) {})
	t.Cleanup(func() { d.close(); waitDocument(t, d) })
	d.update(lspSnapshot{text: "call(arg)", version: 2, col: 1, complete: true, signature: true})
	if waitSignatureTest(t, p.signatures) != 1 || waitSignatureTest(t, done).version != 2 {
		t.Fatal("completion did not arrive during signature request")
	}
	if len(p.cancelled) != 0 {
		t.Fatal("completion waited for signature timeout")
	}
}

func TestSignatureSchedulerRetainsLatestCaretDuringTypingAndRetiresOnExit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hint.go")
	d := installLSPSnapshotProbe(t, path)
	ed := gui.NewCodeEditor()
	ed.SetFilePath(path)
	ed.SetText("package p\n// untouched\nvar _ = call")
	ed.NavigateToLineCol(2, 11)
	ed.SigChanged(func(text string) { lspOnEditorChanged(path, ed, text) })
	old := openEditors
	openEditors = map[string]*gui.CodeEditor{path: ed}
	t.Cleanup(func() { openEditors = old; ed.CancelCompletion(); ed.CancelSymbolIndex(); ed.CancelGitStatus() })
	ed.OnTextInput("(")
	first := d.latest
	if !first.signature || !currentSignatureRequest(path, ed, first) {
		t.Fatalf("opening request missing: %+v", first)
	}
	ed.OnTextInput("12")
	ed.OnTextInput(",")
	ed.OnTextInput("\"abc")
	latest := d.latest
	if !latest.signature || latest.signatureEpoch != first.signatureEpoch || latest.col != ed.CursorUTF16Col() ||
		latest.text != ed.Text() || !currentSignatureRequest(path, ed, latest) || currentSignatureRequest(path, ed, first) {
		t.Fatalf("typing lost current call or retained stale request: first=%+v latest=%+v", first, latest)
	}
	ed.OnKeyDown(gui.KeyEsc, false)
	if currentSignatureRequest(path, ed, latest) {
		t.Fatal("Escape allowed a late reply")
	}
	ed.OnTextInput("X")
	if d.latest.signature {
		t.Fatal("typing revived a dismissed hint")
	}
	ed.OnTextInput(",")
	next := d.latest
	if !next.signature || next.signatureEpoch == latest.signatureEpoch {
		t.Fatal("new trigger did not create a fresh intent")
	}
	ed.OnTextInput(")")
	if currentSignatureRequest(path, ed, next) {
		t.Fatal("closing paren allowed a stale reply")
	}
}
