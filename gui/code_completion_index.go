package gui

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/uk0/silk/async"
)

const completionAsyncBytes = 32 << 10
const completionCandidateLimit = 50

// At most two editors scan/rank concurrently. Idle editors have no worker
// goroutine or timer. Each active editor has one worker and one latest request.
var completionIndexPermits = make(chan struct{}, 2)

type completionIndexRequest struct {
	text, prefix         string
	language             string
	revision, generation uint64
	line, col            int
}
type completionIndexResult struct {
	request completionIndexRequest
	items   []CompletionItem
}
type completionIndexJob struct {
	request completionIndexRequest
	ctx     context.Context
	cancel  context.CancelFunc
}
type completionIndexWorker struct {
	mu              sync.Mutex
	latest          *completionIndexJob
	activeCancel    context.CancelFunc
	running, closed bool
	done            chan struct{}
	cacheRevision   uint64
	cacheValid      bool
	cache           []CompletionItem // immutable; guarded by mu, never GUI-owned
	scan            func(context.Context, string) ([]CompletionItem, error)
	delivery        *async.Latest[completionIndexResult]
}

func newCompletionIndexWorker(post func(func()), consume func(completionIndexResult), scan func(context.Context, string) ([]CompletionItem, error)) *completionIndexWorker {
	delivery, _ := async.NewLatest(post, consume)
	return &completionIndexWorker{delivery: delivery, scan: scan}
}

func (w *completionIndexWorker) request(r completionIndexRequest) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	if w.latest != nil {
		w.latest.cancel()
	}
	if w.activeCancel != nil {
		w.activeCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.latest = &completionIndexJob{request: r, ctx: ctx, cancel: cancel}
	if !w.running {
		w.running = true
		w.done = make(chan struct{})
		go w.run(w.done)
	}
}

// cancel retires work without waiting on the UI thread. clearCache is used on
// text changes; cursor/prefix changes can reuse the immutable identifier index.
func (w *completionIndexWorker) cancel(clearCache bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.latest != nil {
		w.latest.cancel()
		w.latest = nil
	}
	if w.activeCancel != nil {
		w.activeCancel()
	}
	if clearCache {
		w.cache = nil
		w.cacheValid = false
	}
}
func (w *completionIndexWorker) close() {
	w.mu.Lock()
	w.closed = true
	if w.latest != nil {
		w.latest.cancel()
		w.latest = nil
	}
	if w.activeCancel != nil {
		w.activeCancel()
	}
	w.cache = nil
	w.cacheValid = false
	w.mu.Unlock()
	w.delivery.Close() // releases the editor closure and queued payload
}

func (w *completionIndexWorker) run(done chan struct{}) {
	defer close(done)
	for {
		w.mu.Lock()
		job := w.latest
		w.latest = nil
		if w.closed || job == nil {
			w.running = false
			w.activeCancel = nil
			w.mu.Unlock()
			return
		}
		w.activeCancel = job.cancel
		w.mu.Unlock()
		w.compute(job)
		job.cancel()
	}
}

func (w *completionIndexWorker) compute(job *completionIndexJob) {
	// Debounce belongs to the worker, not an editor repaint timer. An edit
	// cancels this wait immediately and replaces the one pending snapshot.
	timer := time.NewTimer(90 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-job.ctx.Done():
		return
	case <-timer.C:
	}
	select {
	case <-job.ctx.Done():
		return
	case completionIndexPermits <- struct{}{}:
	}
	defer func() { <-completionIndexPermits }()
	w.mu.Lock()
	items, valid := w.cache, w.cacheValid && w.cacheRevision == job.request.revision
	w.mu.Unlock()
	if !valid {
		var err error
		items, err = w.scan(job.ctx, job.request.text)
		if err != nil {
			return
		}
		w.mu.Lock()
		if !w.closed && job.ctx.Err() == nil {
			w.cache = items
			w.cacheRevision = job.request.revision
			w.cacheValid = true
		}
		w.mu.Unlock()
	}
	result, err := rankCompletionTop(job.ctx, items, job.request.prefix, completionCandidateLimit)
	if err != nil || job.ctx.Err() != nil {
		return
	}
	// The UI rechecks generation, revision and caret. A cancellation between
	// this check and Publish cannot resurrect an obsolete popup.
	r := job.request
	r.text = "" // delivery does not retain a multi-MB source snapshot
	w.delivery.Publish(completionIndexResult{request: r, items: result})
}

func staticCompletionItems() []CompletionItem {
	items := make([]CompletionItem, 0, len(completionKeywords)+len(completionTypes)+len(completionGuiItems))
	items = append(items, completionKeywords...)
	items = append(items, completionTypes...)
	return append(items, completionGuiItems...)
}

// scanCompletionItems preserves the old lexical identifier semantics, file
// order and dedup precedence, without allocating a rune slice for every line.
func scanCompletionItems(ctx context.Context, text string) ([]CompletionItem, error) {
	return scanCompletionItemsForLanguage(ctx, text, "go")
}

func staticCompletionItemsForLanguage(language string) []CompletionItem {
	if language == "plaintext" {
		return nil
	}
	return staticCompletionItems()
}

func scanCompletionItemsForLanguage(ctx context.Context, text, language string) ([]CompletionItem, error) {
	items := staticCompletionItemsForLanguage(language)
	seen := make(map[string]bool, len(items))
	for _, it := range items {
		seen[it.Text] = true
	}
	start, nextCheck := -1, 0
	add := func(end int) {
		if start < 0 {
			return
		}
		word := text[start:end]
		if len(word) >= 2 && !seen[word] {
			seen[word] = true
			items = append(items, CompletionItem{Text: word, Kind: CikVariable, Detail: "identifier"})
		}
		start = -1
	}
	for pos, r := range text {
		if pos >= nextCheck {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			nextCheck = pos + 4096
		}
		if start >= 0 {
			if isIdentPart(r) {
				continue
			}
			add(pos)
		}
		if isIdentStart(r) {
			start = pos
		}
	}
	add(len(text))
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

type completionRank struct {
	item         CompletionItem
	score, order int
}

// rankCompletionTop matches RankCompletions' ordering, but retains only a
// bounded worst-first heap while scanning. Cancellation is checked during
// ranking too: moving only the lexer off-thread would leave a large sort stall.
func rankCompletionTop(ctx context.Context, items []CompletionItem, prefix string, limit int) ([]CompletionItem, error) {
	if limit <= 0 {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if prefix == "" {
		return append([]CompletionItem{}, items[:min(limit, len(items))]...), nil
	}
	better := func(a, b completionRank) bool {
		if prefix == "" {
			return a.order < b.order
		}
		if a.score != b.score {
			return a.score > b.score
		}
		if len(a.item.Text) != len(b.item.Text) {
			return len(a.item.Text) < len(b.item.Text)
		}
		return a.order < b.order
	}
	heap := make([]completionRank, 0, min(limit, len(items)))
	for i, item := range items {
		if i%256 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		score, matched := fuzzyMatch(item.Text, prefix)
		if !matched || score == 0 {
			continue
		}
		entry := completionRank{item: item, score: score, order: i}
		if len(heap) < limit {
			heap = append(heap, entry)
			for child := len(heap) - 1; child > 0; {
				parent := (child - 1) / 2
				if !better(heap[parent], heap[child]) {
					break
				}
				heap[parent], heap[child] = heap[child], heap[parent]
				child = parent
			}
		} else if better(entry, heap[0]) {
			heap[0] = entry
			for parent := 0; ; {
				child := parent*2 + 1
				if child >= len(heap) {
					break
				}
				if child+1 < len(heap) && better(heap[child], heap[child+1]) {
					child++
				}
				if !better(heap[parent], heap[child]) {
					break
				}
				heap[parent], heap[child] = heap[child], heap[parent]
				parent = child
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sort.Slice(heap, func(i, j int) bool { return better(heap[i], heap[j]) })
	result := make([]CompletionItem, len(heap))
	for i, v := range heap {
		result[i] = v.item
	}
	return result, nil
}
