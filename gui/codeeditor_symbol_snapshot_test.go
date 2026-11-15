package gui

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSymbolSnapshotSinceOwnsChangedDataAndSkipsIdenticalCopies(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("package p\nfunc F() {}\n// comment")
	defer e.CancelSymbolIndex()
	symbols, revision, ready := e.SymbolsSnapshotSince(0)
	if !ready || revision == 0 || len(symbols) != 1 || symbols[0].Name != "F" {
		t.Fatal(symbols, revision, ready)
	}
	symbols[0].Name = "modified owned copy"
	if got, _, _ := e.SymbolsSnapshotSince(0); got[0].Name != "F" {
		t.Fatal("changed snapshot exposed cache")
	}
	if n := testing.AllocsPerRun(100, func() {
		got, current, ready := e.SymbolsSnapshotSince(revision)
		if !ready || current != revision || got != nil {
			t.Fatal(got, current, ready)
		}
	}); n != 0 {
		t.Fatalf("unchanged snapshot allocated %v times", n)
	}
	for _, source := range []string{"package p\nfunc F() {}\n// different", "package p\nfunc F() { value := 1; _ = value }\n// different"} {
		e.SetText(source)
		if got, current, ready := e.SymbolsSnapshotSince(revision); !ready || current != revision || got != nil {
			t.Fatal("same declarations lost content identity", got, current, ready)
		}
	}
	for _, source := range []string{"package p\nfunc F(x int) {}", "package p\n\nfunc F(x int) {}", "package p\nconst F = 1", "package p\n"} {
		e.SetText(source)
		got, current, ready := e.SymbolsSnapshotSince(revision)
		if !ready || current == revision || current == 0 || !reflect.DeepEqual(got, e.ParseSymbols()) {
			t.Fatal("changed signature/line/kind/empty result was omitted", got, current, ready)
		}
		revision = current
	}
	if got, current, ready := e.SymbolsSnapshotSince(revision); !ready || current != revision || got != nil {
		t.Fatal("ready empty result was not stable")
	}
}

func TestSymbolSnapshotSinceAsyncPendingAndCommentPublication(t *testing.T) {
	e, posts := asyncSymbolsEditor(t)
	if data, revision, ready := e.SymbolsSnapshotSince(0); ready || data != nil || revision != 0 {
		t.Fatal("unobserved pending snapshot became actionable", data, revision, ready)
	}
	w := e.symbolWorker
	waitCompletionPost(t, posts)()
	waitSymbolWorker(t, w)
	first, revision, ready := e.SymbolsSnapshotSince(0)
	if !ready || revision == 0 || len(first) != 1 {
		t.Fatal(first, revision, ready)
	}
	storage := &e.symbolCache.symbols[0]
	previousProjection := e.symbolCache.projection
	previousText := previousProjection.text
	previousComments := append([]goOutlineSpan(nil), previousProjection.comments...)
	e.ReplaceAllText(strings.Replace(e.Text(), "// padding", "// changed padding", 1))
	if got, current, ready := e.SymbolsSnapshotSince(revision); !ready || got != nil || current != revision || e.symbolPending {
		t.Fatal("proven comment edit unnecessarily became pending", got, current, ready)
	}
	if previousProjection.text != previousText || !reflect.DeepEqual(previousProjection.comments, previousComments) {
		t.Fatal("UI edit mutated an immutable delivered projection")
	}
	if len(posts) != 0 {
		t.Fatal("proven comment edit queued another index publication")
	}
	// A body edit still takes the asynchronous full-parser path, even if its
	// resulting declaration set is identical. Pending data cannot be used.
	e.ReplaceAllText(strings.Replace(e.Text(), "TestAlpha() {}", "TestAlpha() { x := 1; _ = x }", 1))
	if got, current, ready := e.SymbolsSnapshotSince(revision); ready || got != nil || current != revision {
		t.Fatal("pending revision returned stale coordinates")
	}
	waitCompletionPost(t, posts)()
	waitSymbolWorker(t, w)
	if got, current, ready := e.SymbolsSnapshotSince(revision); !ready || got != nil || current != revision || &e.symbolCache.symbols[0] != storage {
		t.Fatal("same-data worker publication replaced identical symbols", got, current, ready)
	}
	e.SetText("package p\nfunc Small() {}")
	w.mu.Lock()
	closed, retained := w.closed, w.index.text
	w.mu.Unlock()
	if !closed || retained != "" || e.symbolWorker != nil {
		t.Fatal("shrinking document retained its former large worker snapshot")
	}
	got, current, ready := e.SymbolsSnapshotSince(revision)
	if !ready || current == revision || len(got) != 1 || got[0].Name != "Small" {
		t.Fatal(got, current, ready)
	}
}

func TestSymbolIndexCloseRetiresPrivateProjectionAfterActiveParse(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	w := newSymbolIndexWorker(func(fn func()) {}, func(symbolIndexResult) {})
	w.debounce = 0
	parse := w.parse
	w.parse = func(r symbolIndexRequest) codeSymbolCache {
		result := parse(r)
		close(entered)
		<-release
		return result
	}
	w.request(symbolIndexRequest{key: codeSymbolCacheKey{1, "go"}, text: "package p\nfunc F() {}\n// comment"})
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		close(release)
		w.close()
		t.Fatal("worker never reached active parse")
	}
	w.close() // must not join a non-preemptible parser on the UI thread
	close(release)
	waitSymbolWorker(t, w)
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.index.valid || w.index.text != "" || w.index.symbols != nil || w.index.comments != nil || w.language != "" {
		t.Fatal("closed worker retained source/projection")
	}
}

func TestSymbolContentRevisionAdoptionAndWrap(t *testing.T) {
	var current, next codeSymbolCache
	key := codeSymbolCacheKey{1, "go"}
	current.store(key, []CodeSymbol{{Name: "A"}})
	previous := current.contentRevision
	current.valid = false // provider invalidation need not change resulting data
	key.revision++
	next.store(key, []CodeSymbol{{Name: "A"}})
	current.adopt(next)
	if !current.valid || current.key != key || current.contentRevision != previous {
		t.Fatal("same worker content lost editor identity")
	}
	key.revision++
	next.store(key, []CodeSymbol{{Name: "B"}})
	current.adopt(next)
	if current.contentRevision == previous || current.symbols[0].Name != "B" {
		t.Fatal("new worker content was hidden")
	}
	if nextSymbolContentRevision(^uint64(0)) != 1 {
		t.Fatal("zero snapshot token was emitted")
	}
}
