package main

import (
	"unicode"
	"unicode/utf8"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

// One current-line snapshot per response, not a document edit or line scan per
// candidate. Opaque SDK callbacks are never filtered by their display label.
type lspCompletionPrefix struct {
	text, word              string
	line, endByte, endUTF16 int
	valid                   bool
}

func lspCompletionPrefixAt(ed *gui.CodeEditor, s lspSnapshot) lspCompletionPrefix {
	if ed == nil || ed.Text() != s.text {
		return lspCompletionPrefix{}
	}
	p, ok := ed.CursorTextPosition()
	if !ok || p.Line != s.line || p.UTF16Column != s.col {
		return lspCompletionPrefix{}
	}
	line, _ := ed.LineText(p.Line)
	start := p.ByteColumn
	for start > 0 {
		r, n := utf8.DecodeLastRuneInString(line[:start])
		if r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			break
		}
		start -= n
	}
	return lspCompletionPrefix{text: line, word: line[start:p.ByteColumn], line: p.Line, endByte: p.ByteColumn, endUTF16: p.UTF16Column, valid: true}
}

// Only suppress edits proven to leave BOTH text and caret unchanged. Imports,
// snippets, edits to other ranges and edits advancing over an existing suffix
// remain available. Invalid/unknown metadata is left to normal validation.
func (p lspCompletionPrefix) unchanged(item core.LSPCompletionItem) bool {
	if !p.valid || len(item.AdditionalTextEdits) != 0 || item.InsertTextFormat == 2 {
		return false
	}
	if item.TextEdit == nil {
		text := item.InsertText
		if text == "" {
			text = item.Label
		}
		return text == p.word
	}
	edit := item.TextEdit
	if edit.Range.Start.Line != p.line || edit.Range.End.Line != p.line || edit.Range.End.Character != p.endUTF16 || edit.Range.Start.Character < 0 || edit.Range.Start.Character > p.endUTF16 {
		return false
	}
	text := edit.NewText
	if len(text) > p.endByte || !utf8.ValidString(text) || p.text[p.endByte-len(text):p.endByte] != text {
		return false
	}
	units := 0
	for _, r := range text {
		units++
		if r > 0xffff {
			units++
		}
	}
	return p.endUTF16-edit.Range.Start.Character == units
}

// Refresh user intent even if the current local list has no useful rows. A
// dismissed request stays dismissed; SetExternalCompletions does not open it.
func refreshLSPCompletionItems(ed *gui.CodeEditor, items []gui.ExternalCompletion) {
	active := ed.CompletionActive()
	ed.SetExternalCompletions(items)
	ed.SetCompletionStatus("")
	if len(items) == 0 {
		ed.SetCompletionStatus("gopls: no matches; local suggestions")
	}
	if active {
		ed.TriggerCompletion()
	}
}
