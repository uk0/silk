package gui

import (
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func waitSymbolWorker(t *testing.T, w *symbolIndexWorker) {
	t.Helper()
	w.mu.Lock()
	done := w.done
	w.mu.Unlock()
	if done == nil {
		return
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("symbol worker did not exit")
	}
}
func asyncSymbolsEditor(t *testing.T) (*CodeEditor, chan func()) {
	t.Helper()
	e := NewCodeEditor()
	posts := make(chan func(), 8)
	e.symbolPost = func(fn func()) { posts <- fn }
	e.SetText("package p\n" + strings.Repeat("// padding\n", 4000) + "func TestAlpha() {}\n")
	e.filePath = "sample_test.go"
	e.cursorLine = 4001
	t.Cleanup(func() {
		w := e.symbolWorker
		e.CancelSymbolIndex()
		if w != nil {
			waitSymbolWorker(t, w)
		}
	})
	return e, posts
}
func TestSymbolIndexDrawIsNonblockingAndExactAfterDelivery(t *testing.T) {
	e, posts := asyncSymbolsEditor(t)
	if s := e.currentFunction(); s != nil {
		t.Fatal("large-file paint synchronously parsed", s)
	}
	if len(e.testFuncLines()) != 0 || e.symbolCache.valid {
		t.Fatal("pending markers are not empty")
	}
	w := e.symbolWorker
	request := e.symbolRequest
	for i := 0; i < 100; i++ {
		e.currentFunction()
		e.testFuncLines()
	}
	if e.symbolWorker != w || e.symbolRequest.key != request.key || e.symbolRequest.generation != request.generation {
		t.Fatal("repaint resubmitted indexing")
	}
	revision := e.SymbolsRevision()
	textRevision := e.TextRevision()
	waitCompletionPost(t, posts)()
	waitSymbolWorker(t, w)
	symbols, ready := e.SymbolsSnapshot()
	if !ready || !reflect.DeepEqual(symbols, e.ParseSymbols()) || len(symbols) != 1 || symbols[0].Name != "TestAlpha" {
		t.Fatal(symbols, ready)
	}
	if e.currentFunction() == nil || e.testFuncLines()[4001] != "TestAlpha" {
		t.Fatal("breadcrumb or test gutter missing")
	}
	if e.SymbolsRevision() <= revision || e.TextRevision() != textRevision {
		t.Fatal("wrong publication revision")
	}
	symbols[0].Name = "corrupted"
	if e.currentFunction().Name != "TestAlpha" {
		t.Fatal("snapshot exposed cache")
	}
	if n := testing.AllocsPerRun(100, func() { e.currentFunction(); e.testFuncLines() }); n != 0 {
		t.Fatal("hot draw allocations", n)
	}
}
func TestSymbolIndexQueuedResultsCannotOverwriteNewerState(t *testing.T) {
	for _, action := range []string{"edit", "undo", "provider", "language", "close", "sync"} {
		t.Run(action, func(t *testing.T) {
			e, posts := asyncSymbolsEditor(t)
			e.currentFunction()
			old := waitCompletionPost(t, posts)
			w := e.symbolWorker
			waitSymbolWorker(t, w)
			switch action {
			case "edit", "undo":
				e.ReplaceAllText(strings.Replace(e.Text(), "TestAlpha", "TestOmega", 1))
				if action == "undo" {
					e.Undo()
				}
			case "provider":
				e.SetAdditionalSymbols(e.TextRevision(), []CodeSymbol{{Name: "Extra", Kind: SymMacro, Line: 1}})
			case "language":
				e.filePath = "notes.txt"
				e.resetDocumentLanguage()
			case "close":
				e.CancelSymbolIndex()
			case "sync":
				e.ParseSymbols()
			}
			before := e.SymbolsRevision()
			old()
			if e.SymbolsRevision() != before {
				t.Fatal("obsolete delivery mutated symbols")
			}
			if action == "close" {
				if e.symbolWorker != nil || e.symbolPending {
					t.Fatal("closed editor retained work")
				}
				return
			}
			if _, ready := e.SymbolsSnapshot(); !ready {
				waitCompletionPost(t, posts)()
			}
			got, ready := e.SymbolsSnapshot()
			if !ready || !reflect.DeepEqual(got, e.ParseSymbols()) {
				t.Fatal("async and exact parser differ", got)
			}
			if action == "edit" && (len(got) != 1 || got[0].Name != "TestOmega") {
				t.Fatal(got)
			}
			if action == "provider" && (len(got) != 2 || got[0].Name != "Extra") {
				t.Fatal(got)
			}
			if action == "language" && len(got) != 0 {
				t.Fatal("stale dialect symbols", got)
			}
		})
	}
}
func TestSymbolIndexLatestWinsAndDeliveryIsBounded(t *testing.T) {
	posts := make(chan func(), 4)
	entered := make(chan struct{})
	release := make(chan struct{})
	var scans, active, peak atomic.Int32
	var result symbolIndexResult
	w := newSymbolIndexWorker(func(fn func()) { posts <- fn }, func(r symbolIndexResult) { result = r })
	w.debounce = 0
	original := w.parse
	w.parse = func(r symbolIndexRequest) codeSymbolCache {
		n := active.Add(1)
		defer active.Add(-1)
		if n > peak.Load() {
			peak.Store(n)
		}
		if scans.Add(1) == 1 {
			close(entered)
			<-release
		}
		return original(r)
	}
	defer w.close()
	w.request(symbolIndexRequest{key: codeSymbolCacheKey{1, "go"}, text: "package p\nfunc Old(){}"})
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("parser not started")
	}
	for i := 2; i <= 1001; i++ {
		w.request(symbolIndexRequest{key: codeSymbolCacheKey{uint64(i), "go"}, text: "package p\nfunc Latest(){}"})
	}
	close(release)
	waitSymbolWorker(t, w)
	if len(posts) != 1 || scans.Load() != 2 || peak.Load() != 1 {
		t.Fatal("unbounded work", len(posts), scans.Load(), peak.Load())
	}
	// Completed results also coalesce while the event loop is busy.
	w.request(symbolIndexRequest{key: codeSymbolCacheKey{1002, "go"}, text: "package p\nfunc Newest(){}"})
	waitSymbolWorker(t, w)
	if len(posts) != 1 {
		t.Fatal("unbounded UI deliveries")
	}
	(<-posts)()
	if result.request.key.revision != 1002 || result.cache.symbols[0].Name != "Newest" || result.request.text != "" || result.request.additional != nil {
		t.Fatal("wrong result or retained source", result.request)
	}
}
func TestSymbolIndexCloseSuppressesRunningAndQueuedResults(t *testing.T) {
	for _, queued := range []bool{false, true} {
		t.Run(map[bool]string{false: "running", true: "queued"}[queued], func(t *testing.T) {
			posts := make(chan func(), 2)
			entered := make(chan struct{})
			release := make(chan struct{})
			called := false
			w := newSymbolIndexWorker(func(fn func()) { posts <- fn }, func(symbolIndexResult) { called = true })
			w.debounce = 0
			parse := w.parse
			w.parse = func(r symbolIndexRequest) codeSymbolCache { close(entered); <-release; return parse(r) }
			w.request(symbolIndexRequest{key: codeSymbolCacheKey{1, "go"}, text: "func Last(){}"})
			<-entered
			if queued {
				close(release)
				waitSymbolWorker(t, w)
			}
			w.close()
			if !queued {
				close(release)
				waitSymbolWorker(t, w)
			}
			for len(posts) > 0 {
				(<-posts)()
			}
			if called {
				t.Fatal("closed owner received result")
			}
			w.mu.Lock()
			closed, running, latest := w.closed, w.running, w.latest
			w.mu.Unlock()
			if !closed || running || latest != nil {
				t.Fatal("worker retained live work")
			}
		})
	}
}
