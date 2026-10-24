package main

import (
	"context"
	"fmt"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type documentProbe struct {
	mu       sync.Mutex
	events   []string
	requests chan int
	block    bool
}

func (p *documentProbe) event(s string) { p.mu.Lock(); p.events = append(p.events, s); p.mu.Unlock() }
func (p *documentProbe) DidOpen(_, _, s string, v int) error {
	p.event(fmt.Sprintf("open:%d:%s", v, s))
	return nil
}
func (p *documentProbe) DidChange(_ string, v int, s string) error {
	p.event(fmt.Sprintf("change:%d:%s", v, s))
	return nil
}
func (p *documentProbe) DidClose(string) error { p.event("close"); return nil }
func (p *documentProbe) CompletionContext(ctx context.Context, _ string, _, col int) ([]core.LSPCompletionItem, error) {
	p.requests <- col
	if p.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return nil, nil
}
func waitDocument(t *testing.T, d *lspDocument) {
	t.Helper()
	select {
	case <-d.done:
	case <-time.After(3 * time.Second):
		t.Fatal("document worker leaked")
	}
}
func TestLSPDocumentBurstOrderedCoalesced(t *testing.T) {
	p := &documentProbe{requests: make(chan int, 10)}
	delivered := make(chan lspSnapshot, 4)
	d := newLSPDocument(p, "file:///a.go", "start", nil, func(s lspSnapshot, _ []core.LSPCompletionItem, err error) {
		if err == nil {
			delivered <- s
		}
	})
	if _, err := d.sync(); err != nil {
		t.Fatal(err)
	}
	for v := 2; v <= 1001; v++ {
		d.update(lspSnapshot{text: "latest", version: v, col: v, complete: true})
	}
	select {
	case s := <-delivered:
		if s.version != 1001 {
			t.Fatal(s.version)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no completion")
	}
	d.close()
	waitDocument(t, d)
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.events) != 3 || p.events[0] != "open:1:start" || p.events[1] != "change:1001:latest" || p.events[2] != "close" {
		t.Fatal(p.events)
	}
	if len(p.requests) != 1 {
		t.Fatalf("got %d requests for 1000 edits", len(p.requests))
	}
}
func TestLSPDocumentCancelsObsoleteAndCloses(t *testing.T) {
	p := &documentProbe{requests: make(chan int, 5), block: true}
	d := newLSPDocument(p, "file:///a.go", "x", nil, nil)
	d.update(lspSnapshot{text: "xy", version: 2, col: 2, complete: true})
	select {
	case <-p.requests:
	case <-time.After(time.Second):
		t.Fatal("missing request")
	}
	d.update(lspSnapshot{text: "xyz", version: 3, col: 3, complete: true})
	select {
	case col := <-p.requests:
		if col != 3 {
			t.Fatal(col)
		}
	case <-time.After(time.Second):
		t.Fatal("obsolete request not cancelled")
	}
	d.close()
	waitDocument(t, d)
	if _, err := d.sync(); err != context.Canceled {
		t.Fatal(err)
	}
}
func TestLSPDocumentReopenWaitsForClose(t *testing.T) {
	p := &documentProbe{requests: make(chan int, 5)}
	first := newLSPDocument(p, "file:///a.go", "first", nil, nil)
	_, _ = first.sync()
	first.close()
	next := newLSPDocument(p, "file:///a.go", "second", first.done, nil)
	_, _ = next.sync()
	next.close()
	waitDocument(t, next)
	p.mu.Lock()
	defer p.mu.Unlock()
	want := []string{"open:1:first", "close", "open:1:second", "close"}
	if fmt.Sprint(p.events) != fmt.Sprint(want) {
		t.Fatal(p.events)
	}
}
func TestLSPCompletionAppliesUTF16EditsAndImportAtomically(t *testing.T) {
	text := "package demo\n\nfunc f(){ /*😀*/ fmt.Pr }\n"
	ed := gui.NewCodeEditor()
	ed.SetText(text)
	ed.ScrollToLineCol(2, 22)
	s := lspSnapshot{text: text, line: 2, col: 23}
	item := core.LSPCompletionItem{Label: "Println", TextEdit: &core.LSPTextEdit{Range: core.LSPRange{Start: core.LSPPosition{Line: 2, Character: 21}, End: core.LSPPosition{Line: 2, Character: 23}}, NewText: "Println"}, AdditionalTextEdits: []core.LSPTextEdit{{Range: core.LSPRange{Start: core.LSPPosition{Line: 1}, End: core.LSPPosition{Line: 1}}, NewText: "import \"fmt\"\n"}}}
	if err := applyLSPCompletion(ed, s, item); err != nil {
		t.Fatal(err)
	}
	want := "package demo\nimport \"fmt\"\n\nfunc f(){ /*😀*/ fmt.Println }\n"
	if ed.Text() != want {
		t.Fatalf("%q != %q", ed.Text(), want)
	}
	if ed.CursorLine() != 3 || ed.CursorCol() != 27 {
		t.Fatalf("caret %d:%d", ed.CursorLine(), ed.CursorCol())
	}
	ed.Undo()
	if ed.Text() != text {
		t.Fatalf("undo: %q", ed.Text())
	}
	ed.Redo()
	if ed.Text() != want || ed.CursorLine() != 3 || ed.CursorCol() != 27 {
		t.Fatalf("redo lost completion caret: %d:%d %q", ed.CursorLine(), ed.CursorCol(), ed.Text())
	}
}

func TestLSPFileURIRoundTripEscapesReservedCharacters(t *testing.T) {
	for _, name := range []string{"hello world.go", "中文😀.go", "a#b%20?.go"} {
		path := filepath.Join(t.TempDir(), name)
		uri := fileURIOf(path)
		if strings.ContainsAny(uri, " #?") {
			t.Fatalf("unescaped URI: %q", uri)
		}
		if got := uriToPath(uri); got != path {
			t.Fatalf("%q -> %q -> %q", path, uri, got)
		}
	}
	if uriToPath("https://example.invalid/a.go") != "" {
		t.Fatal("accepted non-file URI")
	}
}
