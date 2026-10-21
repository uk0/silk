package main

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

func waitFileSearch(t *testing.T, w *fileSearchService, check func() bool) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		w.mu.Lock()
		ok := check()
		w.mu.Unlock()
		if ok {
			return
		}
		select {
		case <-deadline:
			t.Fatal("file worker wait timeout")
		case <-time.After(time.Millisecond):
		}
	}
}
func TestFileSearchSingleScanForTypingBurst(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	posts := make(chan func(), 4)
	delivered := make(chan fileSearchResult, 4)
	var scans, ranks atomic.Int32
	w := newFileSearchService(func(ctx context.Context, root string) ([]fileEntry, error) {
		scans.Add(1)
		close(started)
		select {
		case <-release:
		case <-ctx.Done():
		}
		return []fileEntry{{Path: "/p/last.go", Display: "last.go"}}, nil
	}, func(files []fileEntry, q string) []fileEntry { ranks.Add(1); return []fileEntry{{Display: q}} }, func(fn func()) { posts <- fn })
	s := w.open("root", func(r fileSearchResult) { delivered <- r })
	defer w.close(s)
	receiveSymbolTest(t, started)
	for i := 0; i < 10000; i++ {
		w.update(s, fmt.Sprint(i))
	}
	close(release)
	receiveSymbolTest(t, posts)()
	r := receiveSymbolTest(t, delivered)
	if len(r.files) != 1 || r.files[0].Display != "9999" || scans.Load() != 1 || ranks.Load() != 1 {
		t.Fatalf("burst not coalesced: %+v scans=%d ranks=%d", r, scans.Load(), ranks.Load())
	}
	w.update(s, "current")
	receiveSymbolTest(t, posts)()
	r = receiveSymbolTest(t, delivered)
	if r.files[0].Display != "current" || scans.Load() != 1 {
		t.Fatal("typing rescanned filesystem")
	}
}
func TestFileSearchReopenBoundsBlockedIOAndDropsOldResults(t *testing.T) {
	started := make(chan string, 4)
	release := make(chan struct{})
	posts := make(chan func(), 4)
	delivered := make(chan fileSearchResult, 4)
	var active, maxActive atomic.Int32
	w := newFileSearchService(func(ctx context.Context, root string) ([]fileEntry, error) {
		n := active.Add(1)
		defer active.Add(-1)
		if n > maxActive.Load() {
			maxActive.Store(n)
		}
		started <- root
		if root == "old" {
			<-release
		} // Deliberately non-cooperative filesystem syscall.
		return []fileEntry{{Display: root}}, nil
	}, rankFiles, func(fn func()) { posts <- fn })
	s := w.open("old", func(r fileSearchResult) { t.Error("old dialog callback ran") })
	receiveSymbolTest(t, started)
	for i := 0; i < 1000; i++ {
		w.close(s)
		s = w.open(fmt.Sprint(i), func(r fileSearchResult) { delivered <- r })
	}
	defer w.close(s)
	select {
	case root := <-started:
		t.Fatalf("parallel scan started: %s", root)
	default:
	}
	close(release)
	if root := receiveSymbolTest(t, started); root != "999" {
		t.Fatal(root)
	}
	receiveSymbolTest(t, posts)()
	r := receiveSymbolTest(t, delivered)
	if r.files[0].Display != "999" || maxActive.Load() != 1 {
		t.Fatalf("stale or parallel: %+v %d", r, maxActive.Load())
	}
}
func TestFileSearchCoalescesDeliveryAndRetiresOnClose(t *testing.T) {
	posts := make(chan func(), 2)
	var deliveries int
	w := newFileSearchService(func(context.Context, string) ([]fileEntry, error) { return []fileEntry{{Display: "x"}}, nil }, func(_ []fileEntry, q string) []fileEntry { return []fileEntry{{Display: q}} }, func(fn func()) { posts <- fn })
	w.debounce = 0
	s := w.open("root", func(r fileSearchResult) { deliveries++ })
	callback := receiveSymbolTest(t, posts)
	for i := 0; i < 20; i++ {
		q := fmt.Sprint(i)
		w.update(s, q)
		waitFileSearch(t, w, func() bool { return w.result != nil && w.result.files[0].Display == q })
	}
	if len(posts) != 0 {
		t.Fatal("unbounded GUI callbacks")
	}
	w.close(s)
	callback()
	if deliveries != 0 {
		t.Fatal("closed view updated")
	}
	waitFileSearch(t, w, func() bool { return !w.running })
}

func BenchmarkQuickOpenRank50k(b *testing.B) {
	files := make([]fileEntry, 50000)
	for i := range files {
		name := fmt.Sprintf("pkg%05d/component.go", i)
		files[i] = fileEntry{Path: "/project/" + name, Display: name}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rankFiles(files, "component")
	}
}
