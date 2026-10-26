package gui

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func completionLargeSource(lines int) string {
	var b strings.Builder
	for i := 0; i < lines; i++ {
		fmt.Fprintf(&b, "var candidate%06d = %d\n", i, i)
	}
	return b.String()
}

func BenchmarkCompletionRebuildLarge(b *testing.B) {
	for _, n := range []int{1000, 100000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			ed := NewCodeEditor()
			ed.SetText(completionLargeSource(n))
			ed.SetSize(800, 500)
			defer ed.CancelCompletion()
			p := NewCompletionPopup(ed)
			ed.completion = p
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				p.Show("candidate09", ed)
			}
		})
	}
}

// The previous source gatherer is kept here as a semantic oracle, not a
// production fallback: async indexing must not discard distant identifiers.
func legacyCompletionItems(text string) []CompletionItem {
	items := staticCompletionItems()
	seen := map[string]bool{}
	for _, it := range items {
		seen[it.Text] = true
	}
	for _, line := range strings.Split(text, "\n") {
		runes := []rune(line)
		for i := 0; i < len(runes); {
			if !isIdentStart(runes[i]) {
				i++
				continue
			}
			start := i
			for i < len(runes) && isIdentPart(runes[i]) {
				i++
			}
			word := string(runes[start:i])
			if len(word) >= 2 && !seen[word] {
				seen[word] = true
				items = append(items, CompletionItem{Text: word, Kind: CikVariable, Detail: "identifier"})
			}
		}
	}
	return items
}

func TestCompletionIndexScannerParity(t *testing.T) {
	for _, text := range []string{"", "var x, 名称 = alpha42, _hidden\nalpha42\n😀尾部", "a\nb\nc\n1name 123_thing 世界\r\n", "broken\xffUTF8 日本語_2", completionLargeSource(3000)} {
		got, err := scanCompletionItems(context.Background(), text)
		if err != nil || !reflect.DeepEqual(got, legacyCompletionItems(text)) {
			t.Fatalf("scanner semantics changed: err=%v", err)
		}
	}
}

func TestCompletionIndexTopKMatchesFullRanking(t *testing.T) {
	rng := rand.New(rand.NewSource(932))
	items := []CompletionItem{{Text: "Alpha"}, {Text: "alpha"}, {Text: "a_l_p_h_a"}, {Text: "界限"}, {Text: "Alpha"}}
	for i := 0; i < 3000; i++ {
		items = append(items, CompletionItem{Text: fmt.Sprintf("%c候选_%04d_%c", 'A'+rng.Intn(26), rng.Intn(9999), 'a'+rng.Intn(26))})
	}
	for _, prefix := range []string{"", "alpha", "A", "a_0", "界", "候选1", "zz", "NO_MATCH_XYZ"} {
		full := RankCompletions(items, prefix)
		for _, limit := range []int{1, 7, 50, 4000} {
			got, err := rankCompletionTop(context.Background(), items, prefix, limit)
			want := full[:min(limit, len(full))]
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("prefix=%q limit=%d: got %d want %d err=%v", prefix, limit, len(got), len(want), err)
			}
		}
	}
}

type completionCancelAfter struct {
	context.Context
	checks, after int
}

func (c *completionCancelAfter) Err() error {
	c.checks++
	if c.checks >= c.after {
		return context.Canceled
	}
	return nil
}
func TestCompletionIndexCooperativeCancellation(t *testing.T) {
	ctx := &completionCancelAfter{Context: context.Background(), after: 4}
	if _, err := scanCompletionItems(ctx, completionLargeSource(5000)); !errors.Is(err, context.Canceled) {
		t.Fatal("scan did not cancel", err)
	}
	items, _ := scanCompletionItems(context.Background(), completionLargeSource(5000))
	ctx = &completionCancelAfter{Context: context.Background(), after: 4}
	if _, err := rankCompletionTop(ctx, items, "candidate", 50); !errors.Is(err, context.Canceled) {
		t.Fatal("rank did not cancel", err)
	}
}

func waitCompletionPost(t *testing.T, posts <-chan func()) func() {
	t.Helper()
	select {
	case f := <-posts:
		return f
	case <-time.After(3 * time.Second):
		t.Fatal("completion delivery missing")
		return nil
	}
}
func waitCompletionWorker(t *testing.T, w *completionIndexWorker) {
	t.Helper()
	w.mu.Lock()
	done := w.done
	w.mu.Unlock()
	if done == nil {
		return
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("index worker leaked")
	}
}

func TestCompletionIndexLatestWinsAndWorkersExit(t *testing.T) {
	posts := make(chan func(), 10)
	started := make(chan struct{}, 1)
	var calls, active, peak atomic.Int32
	var got completionIndexResult
	w := newCompletionIndexWorker(func(f func()) { posts <- f }, func(r completionIndexResult) { got = r }, func(ctx context.Context, text string) ([]CompletionItem, error) {
		n := active.Add(1)
		defer active.Add(-1)
		if n > peak.Load() {
			peak.Store(n)
		}
		if calls.Add(1) == 1 {
			started <- struct{}{}
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return scanCompletionItems(ctx, text)
	})
	defer w.close()
	w.request(completionIndexRequest{text: "first", revision: 1, generation: 1, prefix: "f"})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("no worker")
	}
	for i := 2; i <= 1001; i++ {
		w.request(completionIndexRequest{text: "latestIdentifier", revision: uint64(i), generation: uint64(i), prefix: "latest"})
	}
	waitCompletionPost(t, posts)()
	waitCompletionWorker(t, w)
	if got.request.generation != 1001 || len(got.items) != 1 || got.items[0].Text != "latestIdentifier" || peak.Load() != 1 {
		t.Fatalf("not latest/bounded: %+v peak=%v", got, peak.Load())
	}
	if calls.Load() != 2 {
		t.Fatalf("burst was not coalesced: %v scans", calls.Load())
	}
	if len(posts) != 0 {
		t.Fatal("obsolete delivery queued")
	}
}

func TestCompletionIndexReusesSnapshotAcrossPrefixes(t *testing.T) {
	posts := make(chan func(), 4)
	var scans atomic.Int32
	w := newCompletionIndexWorker(func(f func()) { posts <- f }, func(completionIndexResult) {}, func(ctx context.Context, text string) ([]CompletionItem, error) {
		scans.Add(1)
		return scanCompletionItems(ctx, text)
	})
	defer w.close()
	for i, prefix := range []string{"a", "b", "c"} {
		w.request(completionIndexRequest{text: "alpha beta gamma", revision: 1, generation: uint64(i + 1), prefix: prefix})
		waitCompletionPost(t, posts)()
		waitCompletionWorker(t, w)
	}
	if scans.Load() != 1 {
		t.Fatalf("rescanned unchanged document %d times", scans.Load())
	}
	w.cancel(true)
	w.request(completionIndexRequest{text: "changed", revision: 2, generation: 4, prefix: "c"})
	waitCompletionPost(t, posts)()
	waitCompletionWorker(t, w)
	if scans.Load() != 2 {
		t.Fatal("revision did not invalidate cache")
	}
}

func TestCompletionIndexCloseSuppressesQueuedDelivery(t *testing.T) {
	posts := make(chan func(), 1)
	called := false
	w := newCompletionIndexWorker(func(f func()) { posts <- f }, func(completionIndexResult) { called = true }, scanCompletionItems)
	w.request(completionIndexRequest{text: "alpha", revision: 1, generation: 1, prefix: "a"})
	delivery := waitCompletionPost(t, posts)
	w.close()
	delivery()
	waitCompletionWorker(t, w)
	if called {
		t.Fatal("closed worker delivered")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cache != nil || w.latest != nil || w.running {
		t.Fatal("closed worker retained state")
	}
}

func indexedCompletionEditor(t *testing.T) (*CodeEditor, chan func()) {
	t.Helper()
	ed := NewCodeEditor()
	ed.SetSize(800, 500)
	ed.SetText("rare\n" + completionLargeSource(3000) + "RareRemoteValue\n")
	ed.ScrollToLineCol(0, 4)
	p := NewCompletionPopup(ed)
	posts := make(chan func(), 8)
	p.indexPost = func(f func()) { posts <- f }
	ed.completion = p
	t.Cleanup(func() {
		w := p.indexWorker
		ed.CancelCompletion()
		if w != nil {
			waitCompletionWorker(t, w)
		}
	})
	return ed, posts
}

func TestCompletionIndexIncludesDistantIdentifiersAndPreservesProvider(t *testing.T) {
	ed, posts := indexedCompletionEditor(t)
	ed.TriggerCompletion()
	if !ed.CompletionIndexLoading() {
		t.Fatal("large input was scanned synchronously")
	}
	ed.SetExternalCompletions([]ExternalCompletion{{Label: "RareProvider", Source: "gopls", Apply: func(*CodeEditor) {}}})
	ed.TriggerCompletion()
	if ed.completion.filtered[0].Text != "RareProvider" {
		t.Fatal("provider not immediately available")
	}
	waitCompletionPost(t, posts)()
	if ed.CompletionIndexLoading() || !ed.CompletionVisible() || findItem(ed.completion.filtered, "RareRemoteValue") == nil {
		t.Fatal("full document result missing")
	}
	if ed.completion.filtered[ed.completion.selectedIdx].Text != "RareProvider" {
		t.Fatal("late local index displaced provider selection")
	}
}

func TestCompletionIndexImmediateFallbackDoesNotDuplicateStaticItems(t *testing.T) {
	for _, prefix := range []string{"var", "string"} {
		t.Run(prefix, func(t *testing.T) {
			ed, _ := indexedCompletionEditor(t)
			ed.SetText(prefix + "\n" + completionLargeSource(3000))
			ed.ScrollToLineCol(0, len(prefix))
			ed.TriggerCompletion()
			count := 0
			for _, item := range ed.completion.filtered {
				if item.Text == prefix {
					count++
				}
			}
			if count != 0 {
				t.Fatalf("immediate fallback contains %d no-op copies of %q", count, prefix)
			}
		})
	}
}

func TestCompletionIndexLateResultNeverReopensDismissedOrChanged(t *testing.T) {
	for _, action := range []string{"escape", "focus", "text", "close", "caret"} {
		t.Run(action, func(t *testing.T) {
			ed, posts := indexedCompletionEditor(t)
			ed.TriggerCompletion()
			delivery := waitCompletionPost(t, posts)
			switch action {
			case "escape":
				ed.OnKeyDown(KeyEsc, false)
			case "focus":
				ed.OnFocusChanged(NewEdit(), ed)
			case "text":
				ed.SetText("replacement")
			case "close":
				ed.CancelCompletion()
			case "caret":
				ed.OnKeyDown(KeyLeft, false)
			}
			delivery()
			if ed.CompletionVisible() {
				t.Fatal("stale completion became visible")
			}
		})
	}
}

func TestCompletionTinyViewportEnterDoesNotAcceptHiddenChoice(t *testing.T) {
	ed := completionTestEditor()
	p := ed.completion
	ed.SetSize(80, 40) // resize before any paint/layout
	ed.OnKeyDown(KeyEnter, false)
	if strings.Contains(ed.Text(), "Print00") || p.visible || p.open {
		t.Fatal("hidden candidate consumed Enter")
	}
	if !strings.HasPrefix(ed.Text(), "Pr\n\n") {
		t.Fatalf("Enter did not reach document: %q", ed.Text())
	}
}

func TestCompletionBackspaceRefreshesAfterInvalidation(t *testing.T) {
	ed := NewCodeEditor()
	ed.SetSize(800, 500)
	ed.SetText("Print\nPrintOther")
	ed.ScrollToLineCol(0, 5)
	ed.TriggerCompletion()
	ed.OnKeyDown(KeyBackSpace, false)
	if !ed.CompletionVisible() || ed.completion.prefix != "Prin" {
		t.Fatal("backspace lost completion intent")
	}
}

func BenchmarkCompletionFullIndex(b *testing.B) {
	text := completionLargeSource(100000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		items, _ := scanCompletionItems(context.Background(), text)
		_, _ = rankCompletionTop(context.Background(), items, "candidate09", 50)
	}
}
