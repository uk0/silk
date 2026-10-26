package main

import (
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

func TestLSPCompletionIntentNoopProof(t *testing.T) {
	ed := gui.NewCodeEditor()
	ed.SetText("中🙂 UniqueQAVerbSuffix\r\nnext")
	ed.ScrollToLineCol(0, 15) // after the 12-character word, before Suffix
	s := lspSnapshot{text: ed.Text(), line: 0, col: 16}
	p := lspCompletionPrefixAt(ed, s)
	makeEdit := func(start, end int, text string) *core.LSPTextEdit {
		return &core.LSPTextEdit{Range: core.LSPRange{Start: core.LSPPosition{Line: 0, Character: start}, End: core.LSPPosition{Line: 0, Character: end}}, NewText: text}
	}
	for _, tc := range []struct {
		name string
		item core.LSPCompletionItem
		noop bool
	}{
		{"fallback", core.LSPCompletionItem{Label: "UniqueQAVerb"}, true},
		{"insert", core.LSPCompletionItem{Label: "display", InsertText: "UniqueQAVerb"}, true},
		{"range", core.LSPCompletionItem{Label: "word", TextEdit: makeEdit(4, 16, "UniqueQAVerb")}, true},
		{"empty", core.LSPCompletionItem{Label: "word", TextEdit: makeEdit(16, 16, "")}, true},
		{"case", core.LSPCompletionItem{Label: "uniqueqaverb"}, false},
		{"call", core.LSPCompletionItem{Label: "UniqueQAVerb", InsertText: "UniqueQAVerb()"}, false},
		{"import", core.LSPCompletionItem{Label: "UniqueQAVerb", AdditionalTextEdits: []core.LSPTextEdit{{NewText: "import"}}}, false},
		{"snippet", core.LSPCompletionItem{Label: "UniqueQAVerb", InsertTextFormat: 2}, false},
		{"advance", core.LSPCompletionItem{Label: "word", TextEdit: makeEdit(4, 22, "UniqueQAVerbSuffix")}, false},
		{"invalid_start", core.LSPCompletionItem{Label: "word", TextEdit: makeEdit(5, 16, "UniqueQAVerb")}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := p.unchanged(tc.item); got != tc.noop {
				t.Fatalf("noop=%v want=%v context=%+v", got, tc.noop, p)
			}
		})
	}
	ed.ScrollToLineCol(0, 0)
	if lspCompletionPrefixAt(ed, s).unchanged(core.LSPCompletionItem{Label: "UniqueQAVerb"}) {
		t.Fatal("stale context used")
	}
}

func TestLSPCompletionIntentRefreshesEmptyButNotDismissedRequest(t *testing.T) {
	ed := gui.NewCodeEditor()
	t.Cleanup(ed.CancelCompletion)
	ed.SetText("UniqueQAVerb")
	ed.ScrollToLineCol(0, 12)
	ed.TriggerCompletion()
	if ed.CompletionVisible() || !ed.CompletionActive() {
		t.Fatal("empty result lost request intent")
	}
	// The same SDK sequence used by the LSP UI callback must deliver even when
	// local candidates are empty, but must not reopen after Escape/navigation.
	refresh := func() {
		refreshLSPCompletionItems(ed, []gui.ExternalCompletion{{Label: "UniqueQAVerbOther", Apply: func(*gui.CodeEditor) {}}})
	}
	refresh()
	if !ed.CompletionVisible() {
		t.Fatal("provider result not shown")
	}
	ed.OnKeyDown(gui.KeyEsc, false)
	refresh()
	if ed.CompletionVisible() || ed.CompletionActive() {
		t.Fatal("dismissed provider request reopened")
	}
	ed.ClearExternalCompletions()
	ed.TriggerCompletion()
	ed.ScrollToLineCol(0, 12)
	refresh()
	if ed.CompletionVisible() || ed.CompletionActive() {
		t.Fatal("navigation did not retire request")
	}
}

func BenchmarkLSPCompletionNoopProof(b *testing.B) {
	text := "中🙂 UniqueQAVerb"
	p := lspCompletionPrefix{text: text, word: "UniqueQAVerb", line: 0, endByte: len(text), endUTF16: 16, valid: true}
	item := core.LSPCompletionItem{TextEdit: &core.LSPTextEdit{Range: core.LSPRange{Start: core.LSPPosition{Character: 4}, End: core.LSPPosition{Character: 16}}, NewText: "UniqueQAVerb"}}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !p.unchanged(item) {
			b.Fatal("not a no-op")
		}
	}
}
