package gui

import (
	"strings"
	"testing"
)

func TestLanguageOverlayPreservesUnicodeTabsAndRejectsStale(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("\t//silk:expand 中文(X)\n")
	rev := e.TextRevision()
	spans := []SyntaxHighlight{{0, 1, 14, SyntaxKeyword}, {0, 15, 17, SyntaxFunction}}
	if !e.SetSyntaxHighlights(rev, spans) {
		t.Fatal("valid snapshot rejected")
	}
	spans[0].Kind = SyntaxComment
	tokens, _ := e.tokenizeLineCached(0, e.lines[0], false)
	var joined strings.Builder
	found := false
	for _, tok := range tokens {
		joined.WriteString(tok.text)
		if tok.typ == tokKeyword {
			found = true
		}
	}
	if !found || joined.String() != e.lines[0] {
		t.Fatalf("overlay damaged source: %q %+v", joined.String(), tokens)
	}
	e.ReplaceAllText(e.Text() + "// edit")
	if e.SetSyntaxHighlights(rev, spans) {
		t.Fatal("stale snapshot accepted")
	}
	tokens, _ = e.tokenizeLineCached(0, e.lines[0], false)
	for _, tok := range tokens {
		if tok.typ == tokKeyword {
			t.Fatal("cached stale overlay survived edit")
		}
	}
}
func TestAdditionalLanguageSymbolsCopiedAndInvalidated(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("package p\n//silk:macro Decl()\n")
	rev := e.TextRevision()
	s := []CodeSymbol{{Name: "Decl", Kind: SymMacro, Line: 1}}
	if !e.SetAdditionalSymbols(rev, s) {
		t.Fatal("rejected")
	}
	s[0].Name = "mutated"
	if got := e.ParseSymbols(); len(got) != 1 || got[0].Name != "Decl" {
		t.Fatal(got)
	}
	e.ReplaceAllText("package changed\n//silk:macro Other()\n")
	if e.SetAdditionalSymbols(rev, s) || len(e.ParseSymbols()) != 0 {
		t.Fatal("stale outline")
	}
}
