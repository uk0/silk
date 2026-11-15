package gui

import (
	"context"
	"sort"
	"strings"
	"unicode/utf8"
)

// These bound the optional sidecar, never the completion source. Documents
// beyond a budget keep the existing complete, cancellable scanner.
const completionLexicalChunkBytes = 8 << 10
const completionLexicalMaxBytes = 2 << 20
const completionLexicalMaxWords = 32 << 10

type completionLexicalChunk struct {
	end   int
	words []string // unique within this chunk, in lexical occurrence order
}

// Single-worker-owned scratch state. Only ranked, independently owned results
// leave the worker. Word strings own their bytes: sharing slices of successive
// document revisions here would pin many otherwise dead multi-MB snapshots.
type completionLexicalIndex struct {
	valid          bool
	text, language string
	chunks         []completionLexicalChunk
	items          []CompletionItem
	seen           map[string]bool
	disabledAbove  int // retry a word-budget overflow only after substantial shrink
}

func (p *completionLexicalIndex) reset() { *p = completionLexicalIndex{} }

func (p *completionLexicalIndex) fallback(ctx context.Context, text, language string, wordOverflow bool) ([]CompletionItem, error) {
	p.reset()
	if wordOverflow {
		p.disabledAbove = max(1, len(text)/2)
	}
	return scanCompletionItemsForLanguage(ctx, text, language)
}

func (p *completionLexicalIndex) scan(ctx context.Context, text, language string) ([]CompletionItem, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(text) > completionLexicalMaxBytes {
		return p.fallback(ctx, text, language, false)
	}
	if p.disabledAbove > 0 && len(text) >= p.disabledAbove {
		return scanCompletionItemsForLanguage(ctx, text, language)
	}
	if p.valid && p.text == text && p.language == language {
		return p.items, nil
	}
	var chunks []completionLexicalChunk
	if p.valid {
		start, oldEnd, newEnd, err := completionLexicalDifference(ctx, p.text, text)
		if err != nil {
			return nil, err
		}
		// Include neighbours on both sides. Besides joining/splitting words,
		// an edit can turn a previously invalid UTF-8 separator into a rune.
		lo := sort.Search(len(p.chunks), func(i int) bool { return p.chunks[i].end >= start })
		lo = max(0, lo-1)
		hi := sort.Search(len(p.chunks), func(i int) bool { return p.chunks[i].end >= oldEnd })
		hi = min(len(p.chunks), hi+2)
		begin, end := 0, len(p.text)
		if lo > 0 {
			begin = p.chunks[lo-1].end
		}
		if hi > 0 {
			end = p.chunks[hi-1].end
		}
		delta := newEnd - oldEnd
		changed, ok, err := completionLexicalChunks(ctx, text, begin, end+delta)
		if err != nil {
			return nil, err
		}
		if !ok {
			return p.fallback(ctx, text, language, true)
		}
		chunks = make([]completionLexicalChunk, 0, lo+len(changed)+len(p.chunks)-hi)
		chunks = append(chunks, p.chunks[:lo]...)
		chunks = append(chunks, changed...)
		for _, c := range p.chunks[hi:] {
			c.end += delta
			chunks = append(chunks, c)
		}
	} else {
		var ok bool
		var err error
		chunks, ok, err = completionLexicalChunks(ctx, text, 0, len(text))
		if err != nil {
			return nil, err
		}
		if !ok {
			return p.fallback(ctx, text, language, true)
		}
	}
	count := 0
	for _, c := range chunks {
		count += len(c.words)
	}
	if count > completionLexicalMaxWords {
		return p.fallback(ctx, text, language, true)
	}
	// The cache must be retired before reusing this private candidate storage.
	// A cancellation during merge invalidates it, never publishing half a list.
	p.valid = false
	if p.seen == nil {
		p.seen = make(map[string]bool, count)
	} else {
		clear(p.seen)
	}
	oldItems := p.items
	p.items = append(p.items[:0], staticCompletionItemsForLanguage(language)...)
	for _, item := range p.items {
		p.seen[item.Text] = true
	}
	checks := 0
	for _, c := range chunks {
		for _, word := range c.words {
			if checks&255 == 0 {
				if err := ctx.Err(); err != nil {
					p.reset()
					return nil, err
				}
			}
			checks++
			if !p.seen[word] {
				p.seen[word] = true
				p.items = append(p.items, CompletionItem{Text: word, Kind: CikVariable, Detail: "identifier"})
			}
		}
	}
	if err := ctx.Err(); err != nil {
		p.reset()
		return nil, err
	}
	if len(p.items) < len(oldItems) {
		clear(oldItems[len(p.items):]) // do not pin removed identifier strings in spare capacity
	}
	if len(p.items) == 0 {
		p.items = nil // preserve the complete scanner's empty plain-text result
	}
	p.text, p.language, p.chunks, p.valid, p.disabledAbove = text, language, chunks, true, 0
	return p.items, nil
}

func completionLexicalDifference(ctx context.Context, old, text string) (start, oldEnd, newEnd int, err error) {
	for start < len(old) && start < len(text) && old[start] == text[start] {
		if start&4095 == 0 {
			if err = ctx.Err(); err != nil {
				return
			}
		}
		start++
	}
	oldEnd, newEnd = len(old), len(text)
	for oldEnd > start && newEnd > start && old[oldEnd-1] == text[newEnd-1] {
		if newEnd&4095 == 0 {
			if err = ctx.Err(); err != nil {
				return
			}
		}
		oldEnd--
		newEnd--
	}
	return
}

func completionLexicalChunks(ctx context.Context, text string, begin, end int) ([]completionLexicalChunk, bool, error) {
	var chunks []completionLexicalChunk
	var words []string
	seen := make(map[string]bool)
	start, chunkStart, nextCheck, count := -1, begin, begin, 0
	add := func(end int) bool {
		if start < 0 {
			return true
		}
		word := text[start:end]
		start = -1
		if len(word) >= 2 && !seen[word] {
			word = strings.Clone(word)
			words = append(words, word)
			seen[word] = true
			count++
		}
		return count <= completionLexicalMaxWords
	}
	for pos := begin; pos < end; {
		if pos >= nextCheck {
			if err := ctx.Err(); err != nil {
				return nil, false, err
			}
			nextCheck = pos + 4096
		}
		r, width := utf8.DecodeRuneInString(text[pos:end])
		if start >= 0 && !isIdentPart(r) && !add(pos) {
			return nil, false, nil
		}
		if start < 0 && isIdentStart(r) {
			start = pos
		}
		pos += width
		if pos-chunkStart >= completionLexicalChunkBytes && !isIdentPart(r) {
			chunks = append(chunks, completionLexicalChunk{pos, words})
			words = nil
			clear(seen)
			chunkStart = pos
		}
	}
	if !add(end) {
		return nil, false, nil
	}
	if chunkStart < end {
		chunks = append(chunks, completionLexicalChunk{end, words})
	}
	return chunks, true, nil
}
