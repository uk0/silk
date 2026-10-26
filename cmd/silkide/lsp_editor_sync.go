package main

import (
	"context"
	"encoding/json"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

var lspDocuments = map[string]*lspDocument{}
var lspSessionGeneration uint64
var lspStartCancel context.CancelFunc

func ideLSPCapabilities() json.RawMessage {
	var c map[string]any
	_ = json.Unmarshal(core.DefaultClientCapabilities(), &c)
	c["window"] = map[string]any{"workDoneProgress": true}
	td := c["textDocument"].(map[string]any)
	td["completion"].(map[string]any)["completionItem"].(map[string]any)["snippetSupport"] = false
	td["hover"].(map[string]any)["contentFormat"] = []string{"plaintext"}
	return mustLSPJSON(c)
}
func mustLSPJSON(v any) json.RawMessage { b, _ := json.Marshal(v); return b }

func openLSPDocument(path, text string) {
	openLSPDocumentAfter(path, text, nil)
}

// previous orders renamed documents after the old URI didClose without blocking the UI.
func openLSPDocumentAfter(path, text string, previous <-chan struct{}) {
	if globalLSP == nil || !isGoFile(path) || fileRenameAffects(path) {
		return
	}
	lspVersions[path] = 1
	if old := lspDocuments[path]; old != nil {
		old.close()
		if previous == nil {
			previous = old.done
		}
	}
	client := globalLSP
	ed := openEditors[path]
	epoch := lspSessionGeneration
	var doc *lspDocument
	doc = newLSPDocument(client, fileURIOf(path), text, previous, func(s lspSnapshot, items []core.LSPCompletionItem, err error) {
		gui.Post(func() {
			if globalLSP != client || lspSessionGeneration != epoch || lspDocuments[path] != doc || openEditors[path] != ed || ed == nil || lspVersions[path] != s.version || ed.CursorLine() != s.line || ed.CursorUTF16Col() != s.col {
				return
			}
			if err != nil {
				ed.SetCompletionStatus(lspCompletionStatus(err))
				return
			}
			prefix := lspCompletionPrefixAt(ed, s)
			out := make([]gui.ExternalCompletion, 0, min(len(items), 200))
			for _, it := range items[:min(len(items), 200)] {
				item := it
				if prefix.unchanged(item) {
					continue
				}
				out = append(out, gui.ExternalCompletion{Label: item.Label, Detail: item.Detail, Insert: item.InsertText, Source: "gopls", Apply: func(target *gui.CodeEditor) {
					if target != ed || globalLSP != client || lspDocuments[path] != doc || target.Text() != s.text || target.CursorLine() != s.line || target.CursorUTF16Col() != s.col {
						return
					}
					if err := applyLSPCompletion(target, s, item); err != nil {
						silkideToast(err.Error(), gui.ToastWarning)
					}
				}})
			}
			refreshLSPCompletionItems(ed, out)
			documentHighlightViaLSP(ed, path, s.line, s.col)
		})
	})
	lspDocuments[path] = doc
}
func updateLSPDocument(path string, ed *gui.CodeEditor, text string) {
	if globalLSP == nil || !isGoFile(path) || fileRenameAffects(path) {
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
		// The notification text and caret must belong to the same buffer. A host
		// sending another snapshot still synchronizes text, but cannot request a
		// completion at coordinates from an unrelated editor revision.
		if ed.Text() == text {
			if p, ok := ed.CursorTextPosition(); ok {
				s.line, s.col = p.Line, p.UTF16Column
				// Newline/paste/undo still synchronize. Automatic popup policy stays in
				// the editor; no document split or whole-line rune copy on this hot path.
				ch := p.PreviousRune
				s.complete = p.HasPreviousRune && (ch == '.' || ch == '_' || unicode.IsLetter(ch) || unicode.IsDigit(ch))
			}
		}
	}
	if ed != nil && s.complete {
		ed.SetCompletionStatus("gopls: waiting for completion...")
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
	symbolNavigation.stop()
	if globalReferencesPanel != nil {
		globalReferencesPanel.SetLoading(false)
	}
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
		p, ok := ed.CursorTextPosition()
		if !ok || p.Line != s.line || p.UTF16Column != s.col {
			return nil // stale/malformed fallback snapshot: never panic or edit another position
		}
		line, _ := ed.LineText(p.Line)
		col, start := p.UTF16Column, p.ByteColumn
		for start > 0 {
			ch, width := utf8.DecodeLastRuneInString(line[:start])
			if ch != '_' && !unicode.IsLetter(ch) && !unicode.IsDigit(ch) {
				break
			}
			start -= width
			col -= utf16.RuneLen(ch)
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
	if globalLSP == nil || ed == nil || !isGoFile(path) || fileRenameAffects(path) {
		return
	}
	if lspDocuments[path] == nil {
		openLSPDocument(path, ed.Text())
	}
	if lspDocuments[path] == nil {
		return
	}
	ed.SetCompletionStatus("gopls: waiting for completion...")
	lspVersions[path]++
	lspDocuments[path].update(lspSnapshot{text: ed.Text(), version: lspVersions[path], line: ed.CursorLine(), col: ed.CursorUTF16Col(), complete: true})
}
