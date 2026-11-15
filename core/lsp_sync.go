package core

import (
	"encoding/json"
	"strings"
	"unicode/utf8"
)

// LSPContentChange replaces Range in the previously synchronized document.
// A nil Range is a whole-document replacement. Positions count UTF-16 units;
// RangeLength is deliberately omitted (deprecated by the LSP specification).
type LSPContentChange struct {
	Range *LSPRange `json:"range,omitempty"`
	Text  string    `json:"text"`
}

type lspChangedDocument struct {
	URI     string `json:"uri"`
	Version int    `json:"version"`
}
type lspContentChangeParams struct {
	TextDocument   lspChangedDocument `json:"textDocument"`
	ContentChanges []LSPContentChange `json:"contentChanges"`
}

// SupportsIncrementalSync reports explicit initialize-time UTF-16 incremental
// sync support. Omitted/invalid capabilities or another position encoding do
// not authorize range updates. Dynamic registration is not advertised.
func (c *LSPClient) SupportsIncrementalSync() bool {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()
	return c.incrementalSync
}

func lspSupportsIncrementalSync(raw json.RawMessage) bool {
	// Only exact, explicit protocol keys authorize an optimization. Unlike
	// struct decoding, a map does not accept case-insensitive misspellings.
	var caps map[string]json.RawMessage
	if json.Unmarshal(raw, &caps) != nil {
		return false
	}
	if encoded, ok := caps["positionEncoding"]; ok {
		var encoding string
		if json.Unmarshal(encoded, &encoding) != nil || encoding != "utf-16" {
			return false
		}
	}
	var kind int
	if json.Unmarshal(caps["textDocumentSync"], &kind) != nil {
		var options map[string]json.RawMessage
		if json.Unmarshal(caps["textDocumentSync"], &options) != nil || json.Unmarshal(options["change"], &kind) != nil {
			return false
		}
	}
	return kind == 2
}

// DidChangeFrom uses a compact change only when the server explicitly accepts
// UTF-16 incremental sync. Otherwise it preserves DidChange's full-text wire
// format. previous must be the LAST SUCCESSFULLY SENT text, not the previous UI
// edit: debounced/coalesced edits can skip many local revisions. The host owns
// open/change/close ordering; this client does not retain per-document source.
// Do not call on a UI thread: framing/pipe writes can block, as with DidChange.
func (c *LSPClient) DidChangeFrom(uri string, version int, previous, text string) error {
	change := LSPContentChange{Text: text}
	if c.SupportsIncrementalSync() {
		change = ComputeLSPTextChange(previous, text)
	}
	return c.sendContentChange(uri, version, change)
}

func (c *LSPClient) sendContentChange(uri string, version int, change LSPContentChange) error {
	return c.SendNotification("textDocument/didChange", lspContentChangeParams{
		TextDocument:   lspChangedDocument{URI: uri, Version: version},
		ContentChanges: []LSPContentChange{change},
	})
}

// ComputeLSPTextChange returns ONE exact UTF-16 edit from previous to text.
// It coalesces disjoint edits into a containing replacement, never cuts a rune
// or CRLF, and falls back to a full replacement for invalid UTF-8, bare CR or
// when the range overhead would erase most savings. Inputs are immutable; the
// result may reference text's storage. Capability/ordering checks belong to
// the caller (normally DidChangeFrom). No document-sized buffers are built.
func ComputeLSPTextChange(previous, text string) LSPContentChange {
	full := LSPContentChange{Text: text}
	if !validIncrementalText(previous) || !validIncrementalText(text) {
		return full
	}
	if previous == text {
		if len(text) < 96 {
			return full
		}
		return LSPContentChange{Range: &LSPRange{}}
	}
	start := 0
	limit := min(len(previous), len(text))
	// Chunk equality uses the runtime's optimized memory comparison, without
	// unsafe loads/alignment assumptions on any supported architecture.
	for start+64 <= limit && previous[start:start+64] == text[start:start+64] {
		start += 64
	}
	for start < limit && previous[start] == text[start] {
		start++
	}
	oldEnd, newEnd := len(previous), len(text)
	for oldEnd-start >= 64 && newEnd-start >= 64 && previous[oldEnd-64:oldEnd] == text[newEnd-64:newEnd] {
		oldEnd -= 64
		newEnd -= 64
	}
	for oldEnd > start && newEnd > start && previous[oldEnd-1] == text[newEnd-1] {
		oldEnd--
		newEnd--
	}
	for start > 0 && (!lspChangeBoundary(previous, start) || !lspChangeBoundary(text, start)) {
		start--
	}
	for !lspChangeBoundary(previous, oldEnd) || !lspChangeBoundary(text, newEnd) {
		oldEnd++
		newEnd++
	}
	if len(text)-(newEnd-start) < 96 {
		return full
	}
	from := advanceLSPChangePosition(previous[:start], LSPPosition{})
	to := advanceLSPChangePosition(previous[start:oldEnd], from)
	return LSPContentChange{Range: &LSPRange{Start: from, End: to}, Text: text[start:newEnd]}
}

func validIncrementalText(text string) bool {
	if !utf8.ValidString(text) {
		return false
	}
	for offset := 0; offset < len(text); {
		i := strings.IndexByte(text[offset:], '\r')
		if i < 0 {
			return true
		}
		offset += i
		if offset+1 >= len(text) || text[offset+1] != '\n' {
			return false
		}
		offset += 2
	}
	return true
}
func lspChangeBoundary(text string, offset int) bool {
	if offset == 0 || offset == len(text) {
		return true
	}
	return utf8.RuneStart(text[offset]) && !(text[offset-1] == '\r' && text[offset] == '\n')
}
func advanceLSPChangePosition(text string, p LSPPosition) LSPPosition {
	// All preceding complete lines affect only Line, not the final UTF-16
	// column. Avoid decoding every rune of a large unchanged document prefix.
	if lines := strings.Count(text, "\n"); lines > 0 {
		p.Line += lines
		p.Character = 0
		text = text[strings.LastIndexByte(text, '\n')+1:]
	}
	for _, r := range text {
		p.Character++
		if r > 0xffff {
			p.Character++
		}
	}
	return p
}
