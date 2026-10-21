package main

import (
	"context"
	"encoding/json"
	"strings"
	"unicode"
	"unicode/utf16"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

var lspDocuments = map[string]*lspDocument{}
var lspSessionGeneration uint64
var lspStartCancel context.CancelFunc

func ideLSPCapabilities() json.RawMessage {
	var c map[string]any
	_ = json.Unmarshal(core.DefaultClientCapabilities(), &c)
	td := c["textDocument"].(map[string]any)
	td["completion"].(map[string]any)["completionItem"].(map[string]any)["snippetSupport"] = false
	td["hover"].(map[string]any)["contentFormat"] = []string{"plaintext"}
	return mustLSPJSON(c)
}
func mustLSPJSON(v any) json.RawMessage { b, _ := json.Marshal(v); return b }

func openLSPDocument(path, text string) {
	if globalLSP == nil || !isGoFile(path) {
		return
	}
	lspVersions[path] = 1
	var previous <-chan struct{}
	if old := lspDocuments[path]; old != nil {
		old.close()
		previous = old.done
	}
	client := globalLSP
	ed := openEditors[path]
	epoch := lspSessionGeneration
	var doc *lspDocument
	doc = newLSPDocument(client, fileURIOf(path), text, previous, func(s lspSnapshot, items []core.LSPCompletionItem) {
		gui.Post(func() {
			if globalLSP != client || lspSessionGeneration != epoch || lspDocuments[path] != doc || openEditors[path] != ed || ed == nil || lspVersions[path] != s.version || ed.CursorLine() != s.line || ed.CursorUTF16Col() != s.col {
				return
			}
			out := make([]gui.ExternalCompletion, 0, min(len(items), 200))
			for _, it := range items[:min(len(items), 200)] {
				item := it
				out = append(out, gui.ExternalCompletion{Label: item.Label, Detail: item.Detail, Insert: item.InsertText, Apply: func(target *gui.CodeEditor) {
					if target != ed || globalLSP != client || lspDocuments[path] != doc || target.Text() != s.text || target.CursorLine() != s.line || target.CursorUTF16Col() != s.col {
						return
					}
					if err := applyLSPCompletion(target, s, item); err != nil {
						silkideToast(err.Error(), gui.ToastWarning)
					}
				}})
			}
			ed.SetExternalCompletions(out) // empty response MUST clear stale candidates
			documentHighlightViaLSP(ed, path, s.line, s.col)
			if ed.CompletionVisible() {
				ed.TriggerCompletion()
			}
		})
	})
	lspDocuments[path] = doc
}
func updateLSPDocument(path string, ed *gui.CodeEditor, text string) {
	if globalLSP == nil || !isGoFile(path) {
		return
	}
	d := lspDocuments[path]
	if d == nil {
		openLSPDocument(path, text)
		d = lspDocuments[path]
	}
	lspVersions[path]++
	s := lspSnapshot{text: text, version: lspVersions[path]}
	if ed != nil {
		lspHoverGen++
		lspSigGen++
		lspHighlightGen[path]++
		ed.SetOccurrenceHighlights(nil)
		ed.ClearExternalCompletions()
		s.line = ed.CursorLine()
		s.col = ed.CursorUTF16Col()
		// Newline/paste/undo still synchronize, but do not request completion for
		// punctuation or whitespace. Automatic popup policy remains in the editor.
		lines := strings.Split(text, "\n")
		if s.line < len(lines) {
			r := []rune(lines[s.line])
			col := ed.CursorCol()
			if col > 0 && col <= len(r) {
				ch := r[col-1]
				s.complete = ch == '.' || ch == '_' || unicode.IsLetter(ch) || unicode.IsDigit(ch)
			}
		}
	}
	d.update(s)
}
func flushLSPDocument(d *lspDocument) bool {
	if d == nil {
		return true
	}
	_, err := d.sync()
	return err == nil
}

func closeLSPDocument(path string) {
	if d := lspDocuments[path]; d != nil {
		d.close()
		go func() {
			<-d.done
			gui.Post(func() {
				if lspDocuments[path] == d {
					delete(lspDocuments, path)
				}
			})
		}()
	}
	delete(lspCompletionGen, path)
	lspHoverGen++
	lspSigGen++
}
func retireLSPDocuments() {
	for path, d := range lspDocuments {
		d.close()
		delete(lspDocuments, path)
	}
	lspHoverGen++
	lspSigGen++
	for _, ed := range openEditors {
		if ed != nil {
			ed.ClearExternalCompletions()
			ed.SetErrors(nil)
			ed.SetOccurrenceHighlights(nil)
		}
	}
}

// Apply the provider's UTF-16 replacement and auto-import edits together, with
// one undo entry. Never replace the editor's guessed prefix when textEdit exists.
func applyLSPCompletion(ed *gui.CodeEditor, s lspSnapshot, it core.LSPCompletionItem) error {
	if ed.Text() != s.text {
		return nil
	}
	primary := it.TextEdit
	if primary == nil {
		text := it.InsertText
		if text == "" {
			text = it.Label
		}
		col := s.col
		line := strings.Split(s.text, "\n")[s.line]
		r := []rune(line)
		n := ed.CursorCol()
		for n > 0 && (unicode.IsLetter(r[n-1]) || unicode.IsDigit(r[n-1]) || r[n-1] == '_') {
			n--
			col -= utf16.RuneLen(r[n])
		}
		primary = &core.LSPTextEdit{Range: core.LSPRange{Start: core.LSPPosition{Line: s.line, Character: col}, End: core.LSPPosition{Line: s.line, Character: s.col}}, NewText: text}
	}
	// IDE explicitly negotiates plain text. Do not insert uninterpreted snippet
	// placeholders if a non-conforming server sends them anyway.
	if it.InsertTextFormat == 2 {
		return nil
	}
	edits := append([]core.LSPTextEdit{*primary}, it.AdditionalTextEdits...)
	text, err := core.ApplyTextEdits(s.text, edits)
	if err != nil {
		return err
	}
	start := lspPositionOffset(s.text, primary.Range.Start)
	cursor := start + len(primary.NewText)
	for _, e := range it.AdditionalTextEdits {
		a := lspPositionOffset(s.text, e.Range.Start)
		b := lspPositionOffset(s.text, e.Range.End)
		if a <= start {
			cursor += len(e.NewText) - (b - a)
		}
	}
	cursor = max(0, min(cursor, len(text)))
	prefix := text[:cursor]
	line := strings.Count(prefix, "\n")
	col := len([]rune(prefix[strings.LastIndex(prefix, "\n")+1:]))
	ed.ReplaceAllTextAt(text, line, col)
	return nil
}
func lspPositionOffset(text string, p core.LSPPosition) int {
	off := 0
	for range p.Line {
		i := strings.IndexByte(text[off:], '\n')
		if i < 0 {
			return len(text)
		}
		off += i + 1
	}
	units := 0
	for i, r := range text[off:] {
		if r == '\n' || units >= p.Character {
			return off + i
		}
		units += utf16.RuneLen(r)
	}
	return len(text)
}

func requestLSPCompletion(path string, ed *gui.CodeEditor) {
	if globalLSP == nil || ed == nil || !isGoFile(path) {
		return
	}
	if lspDocuments[path] == nil {
		openLSPDocument(path, ed.Text())
	}
	lspVersions[path]++
	lspDocuments[path].update(lspSnapshot{text: ed.Text(), version: lspVersions[path], line: ed.CursorLine(), col: ed.CursorUTF16Col(), complete: true})
}
