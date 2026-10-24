package gui

import "context"

// The fields here and CompletionPopup are UI-thread owned. The worker receives
// only immutable strings/value snapshots, never an editor or its mutable lines.
func (p *CompletionPopup) asynchronousItems(ed *CodeEditor, text string) []CompletionItem {
	r := completionIndexRequest{revision: ed.TextRevision(), prefix: p.prefix, line: ed.cursorLine, col: ed.cursorCol, language: ed.Language()}
	if p.localKey.language != r.language {
		p.cancelLocal(true)
		if p.indexWorker != nil {
			p.indexWorker.close()
			p.indexWorker = nil
		}
	}
	if p.localReady && p.localKey.revision == r.revision && p.localKey.prefix == r.prefix {
		return append([]CompletionItem(nil), p.localItems...)
	}
	if !p.localPending || p.localKey.revision != r.revision || p.localKey.prefix != r.prefix || p.localKey.line != r.line || p.localKey.col != r.col {
		p.localGeneration++
		r.generation = p.localGeneration
		p.localKey = r
		p.localPending = true
		p.localReady = false
		if p.indexWorker == nil {
			post := p.indexPost
			if post == nil {
				post = Post
			}
			var worker *completionIndexWorker
			language := r.language // do not retain the whole mutable request/snapshot
			worker = newCompletionIndexWorker(post, func(result completionIndexResult) {
				if p.indexWorker != worker {
					return
				}
				p.receiveLocalIndex(ed, result)
			}, func(ctx context.Context, text string) ([]CompletionItem, error) {
				return scanCompletionItemsForLanguage(ctx, text, language)
			})
			p.indexWorker = worker
		}
		r.text = text
		p.indexWorker.request(r)
	}
	items := staticCompletionItemsForLanguage(r.language)
	// Keep the currently typed fragment available while the COMPLETE document
	// is indexed. Do not scan a long/minified current line on the input thread.
	if p.prefix != "" {
		for _, item := range items {
			if item.Text == p.prefix {
				return items // preserve the keyword/type kind, without a duplicate row
			}
		}
		items = append(items, CompletionItem{Text: p.prefix, Kind: CikVariable, Detail: "identifier"})
	}
	return items
}

func (p *CompletionPopup) receiveLocalIndex(ed *CodeEditor, result completionIndexResult) {
	r := result.request
	if ed.completion != p || !p.open || !p.localPending || p.localGeneration != r.generation || ed.TextRevision() != r.revision || ed.Language() != r.language || ed.cursorLine != r.line || ed.cursorCol != r.col || p.prefix != r.prefix {
		return
	}
	p.localPending = false
	p.localReady = true
	p.localKey = r
	p.localItems = result.items
	selected := ""
	if p.visible && p.selectedIdx >= 0 && p.selectedIdx < len(p.filtered) {
		selected = p.filtered[p.selectedIdx].Text
	}
	scroll := p.scrollY
	p.items = append([]CompletionItem(nil), result.items...)
	p.filter()
	p.visible = len(p.filtered) > 0
	ed.mergeExternalIntoPopup()
	// A late local index must not move an already highlighted semantic choice.
	p.scrollY = scroll
	index := 0
	for i, item := range p.filtered {
		if item.Text == selected {
			index = i
			break
		}
	}
	p.selectIndex(index)
	ed.Self().Update()
}

func (p *CompletionPopup) cancelLocal(clearCache bool) {
	p.localGeneration++
	p.localPending = false
	if clearCache {
		p.localReady = false
		p.localItems = nil
		p.items = nil
		p.filtered = nil
	}
	if p.indexWorker != nil {
		p.indexWorker.cancel(clearCache)
	}
}

// CancelCompletion stops pending local indexing, releases retained snapshots
// and dismisses the list. Call on the UI thread when permanently removing an
// SDK editor/before stopping its event loop. A later request starts fresh.
func (ed *CodeEditor) CancelCompletion() {
	if p := ed.completion; p != nil {
		visible := p.visible
		p.Dismiss()
		if p.indexWorker != nil {
			p.indexWorker.close()
			p.indexWorker = nil
		}
		p.localItems = nil
		p.localReady = false
		p.items = nil
		p.filtered = nil
		if visible {
			ed.Self().Update()
		}
	}
}

// CompletionIndexLoading reports background local candidate collection/ranking.
// It does not report LSP state, and must be read on the UI thread.
func (ed *CodeEditor) CompletionIndexLoading() bool {
	return ed.completion != nil && ed.completion.localPending
}
