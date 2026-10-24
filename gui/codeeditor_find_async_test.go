package gui

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func waitFindWorker(t *testing.T, w *editorFindWorker) {
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
		t.Fatal("find worker did not stop")
	}
}
func asyncFindEditor(t *testing.T) (*CodeEditor, chan func()) {
	t.Helper()
	e := NewCodeEditor()
	posts := make(chan func(), 8)
	e.findState.post = func(f func()) { posts <- f }
	e.SetText(strings.Repeat("foo safe\n", 5000))
	e.findActive = true
	t.Cleanup(func() {
		w := e.findState.worker
		e.CancelFind()
		if w != nil {
			waitFindWorker(t, w)
		}
	})
	return e, posts
}
func TestEditorFindAsyncRevisionQueryOptionsAndCloseGuards(t *testing.T) {
	for _, action := range []string{"edit", "undo", "query", "options", "close", "hide"} {
		t.Run(action, func(t *testing.T) {
			e, posts := asyncFindEditor(t)
			e.SetFindQuery("foo")
			if !e.FindSnapshot().Pending || len(e.findMatches) != 0 {
				t.Fatal("large query scanned synchronously")
			}
			old := waitCompletionPost(t, posts)
			waitFindWorker(t, e.findState.worker)
			switch action {
			case "edit", "undo":
				e.ReplaceAllText(strings.ReplaceAll(e.Text(), "foo", "bar"))
				if action == "undo" {
					e.Undo()
				}
			case "query":
				e.SetFindQuery("safe")
			case "options":
				e.SetFindOptions(EditorFindOptions{MatchCase: true})
			case "close":
				e.closeFindBar()
			case "hide":
				e.Hide()
			}
			old()
			if len(e.findMatches) != 0 {
				t.Fatal("stale delivery applied")
			}
			if action == "close" || action == "hide" {
				if e.findState.worker != nil || e.findState.pending {
					t.Fatal("work retained")
				}
				return
			}
			waitCompletionPost(t, posts)()
			want := 5000
			if action == "edit" {
				want = 0
			}
			if e.FindSnapshot().Pending || e.FindSnapshot().Count != want {
				t.Fatal(e.FindSnapshot())
			}
		})
	}
}
func TestEditorFindAsyncReplaceIsAtomicAndStaleSafe(t *testing.T) {
	for _, action := range []string{"apply", "edit", "replacement", "query", "options", "close"} {
		t.Run(action, func(t *testing.T) {
			e, posts := asyncFindEditor(t)
			e.SetFindQuery("foo")
			waitCompletionPost(t, posts)()
			before := e.Text()
			e.replaceText = "🙂"
			changes := 0
			e.SigChanged(func(string) { changes++ })
			e.replaceAll()
			delivery := waitCompletionPost(t, posts)
			waitFindWorker(t, e.findState.worker)
			switch action {
			case "edit":
				e.ReplaceAllText("unrelated")
			case "replacement":
				e.replaceText = "other"
			case "query":
				e.SetFindQuery("safe")
			case "options":
				e.SetFindOptions(EditorFindOptions{WholeWord: true})
			case "close":
				e.closeFindBar()
			}
			delivery()
			if action != "apply" {
				if action != "edit" && e.Text() != before {
					t.Fatal("stale replace applied")
				}
				if action == "edit" && e.Text() != "unrelated" {
					t.Fatal("edit overwritten")
				}
				return
			}
			if changes != 1 || e.Text() != strings.ReplaceAll(before, "foo", "🙂") {
				t.Fatal("non-atomic replacement", changes)
			}
			e.Undo()
			if e.Text() != before {
				t.Fatal("one undo did not restore exact buffer")
			}
			e.Redo()
			if e.Text() != strings.ReplaceAll(before, "foo", "🙂") {
				t.Fatal("redo failed")
			}
		})
	}
}
func TestEditorFindWorkerLatestWinsCancellationAndBoundedDelivery(t *testing.T) {
	posts := make(chan func(), 4)
	entered := make(chan struct{})
	release := make(chan struct{})
	var scans, active, peak atomic.Int32
	var got editorFindResult
	w := newEditorFindWorker(func(f func()) { posts <- f }, func(r editorFindResult) { got = r })
	w.debounce = 0
	w.compute = func(ctx context.Context, r editorFindRequest) editorFindResult {
		n := active.Add(1)
		defer active.Add(-1)
		if n > peak.Load() {
			peak.Store(n)
		}
		if scans.Add(1) == 1 {
			close(entered)
			<-release
		}
		return computeEditorFind(ctx, r)
	}
	defer w.close()
	w.request(editorFindRequest{text: "old", query: "old"})
	<-entered
	for i := 0; i < 100; i++ {
		w.request(editorFindRequest{text: "latest latest", query: "latest", generation: uint64(i + 1)})
	}
	close(release)
	waitFindWorker(t, w)
	if scans.Load() != 2 || peak.Load() != 1 || len(posts) != 1 {
		t.Fatal(scans.Load(), peak.Load(), len(posts))
	}
	(<-posts)()
	if got.count != 2 || got.request.generation != 100 {
		t.Fatal(got)
	}
	w.request(editorFindRequest{text: "tail", query: "tail"})
	waitFindWorker(t, w)
	w.close()
	(<-posts)()
	if got.request.generation != 100 {
		t.Fatal("delivery after close")
	}
}

func TestEditorFindWorkersBoundGlobalCPUAndReleaseOnClose(t *testing.T) {
	var active, peak atomic.Int32
	entered := make(chan struct{}, 8)
	release := make(chan struct{})
	var workers []*editorFindWorker
	for i := 0; i < 8; i++ {
		w := newEditorFindWorker(func(func()) {}, func(editorFindResult) {})
		w.debounce = 0
		w.compute = func(ctx context.Context, r editorFindRequest) editorFindResult {
			n := active.Add(1)
			defer active.Add(-1)
			for {
				p := peak.Load()
				if n <= p || peak.CompareAndSwap(p, n) {
					break
				}
			}
			entered <- struct{}{}
			select {
			case <-release:
			case <-ctx.Done():
			}
			return computeEditorFind(ctx, r)
		}
		workers = append(workers, w)
		w.request(editorFindRequest{text: "foo", query: "foo"})
	}
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("workers did not start")
		}
	}
	if active.Load() != 2 {
		t.Fatal("wrong admission count", active.Load())
	}
	// No third compute may enter while both permits are held.
	select {
	case <-entered:
		t.Fatal("unbounded process-wide compute")
	case <-time.After(20 * time.Millisecond):
	}
	for _, w := range workers {
		w.close()
	}
	close(release)
	for _, w := range workers {
		waitFindWorker(t, w)
	}
	if peak.Load() > 2 || active.Load() != 0 || len(editorFindPermits) != 0 {
		t.Fatal(peak.Load(), active.Load(), len(editorFindPermits))
	}
}
