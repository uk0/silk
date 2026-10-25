package gui

import (
	"strings"
	"testing"
)

func jsonEditor(t *testing.T, text string) *CodeEditor {
	t.Helper()
	e := NewCodeEditor()
	// No filesystem/git work is needed to test the document dialect.
	e.filePath = "settings.JSON"
	e.SetText(text)
	t.Cleanup(e.CancelCompletion)
	if e.Language() != "json" || e.languageLabel() != "JSON" || e.usesGoSyntax() {
		t.Fatalf("JSON must have its own dialect, got %q / %q", e.Language(), e.languageLabel())
	}
	return e
}

func tokenSource(tokens []token) string {
	var b strings.Builder
	for _, tok := range tokens {
		b.WriteString(tok.text)
	}
	return b.String()
}

func TestJSONEditorTokensPreserveSourceAndDistinguishKeys(t *testing.T) {
	source := "\t{\"名称🙂\": \"quoted \\\" value\", \"path\": \"C:\\\\temp\", \"ok\": true, \"items\": [null, false, -1.25e+3]}\r"
	e := jsonEditor(t, source)
	got := e.tokensForLine(0)
	if tokenSource(got) != source {
		t.Fatal("tokenization altered source bytes")
	}
	wants := map[string]tokenType{`"名称🙂"`: tokType, `"ok"`: tokType, `"quoted \" value"`: tokString, "true": tokKeyword, "null": tokKeyword, "false": tokKeyword, "-1.25e+3": tokNumber}
	for text, kind := range wants {
		found := false
		for _, tok := range got {
			if tok.text == text && tok.typ == kind {
				found = true
			}
		}
		if !found {
			t.Errorf("missing %q with kind %d: %+v", text, kind, got)
		}
	}
	if !tokensEqual(got, e.tokensForLine(0)) {
		t.Fatal("cache differs from first draw")
	}
	if len(e.goLineStates.lines) != 0 {
		t.Fatal("JSON should not scan Go prefix state")
	}
}

func TestJSONEditorNumbersAndKeywordsAreWholeTokens(t *testing.T) {
	for _, s := range []string{"0", "-0", "123", "-123", "0.5", "1e3", "1E-3", "-0.5e+12"} {
		e := jsonEditor(t, s)
		got := e.tokensForLine(0)
		if len(got) != 1 || got[0].typ != tokNumber || got[0].text != s {
			t.Errorf("valid number %q: %+v", s, got)
		}
	}
	for _, s := range []string{"01", "-01", "+1", ".5", "1.", "1e", "1e+", "--1", "0x10", "1_000", "1i", "NaN", "Infinity", "trueish", "falsehood", "NULL", "return", "func", "nil", "中true"} {
		e := jsonEditor(t, s)
		for _, tok := range e.tokensForLine(0) {
			if tok.typ != tokNormal {
				t.Errorf("invalid JSON atom %q colored as %d", s, tok.typ)
			}
		}
	}
}

func TestJSONEditorIncompleteStringsDoNotPoisonFollowingLines(t *testing.T) {
	for _, source := range []string{"\"unclosed\\\ntrue", "\"/*not Go*/`\\u1234\nfalse", "\"\xff🙂\"\nnull", "\"escaped\\\"quote\"\n0"} {
		e := jsonEditor(t, source)
		for i, line := range e.lines {
			got := e.tokensForLine(i)
			if tokenSource(got) != line {
				t.Fatalf("line %d bytes changed", i)
			}
			if i == 1 && (len(got) != 1 || got[0].typ != tokKeyword && got[0].typ != tokNumber) {
				t.Fatal("previous invalid string affected next line", got)
			}
		}
	}
}

func TestJSONEditorCacheTracksLanguageAndEdits(t *testing.T) {
	e := jsonEditor(t, `{"x": true}`)
	base := e.tokensForLine(0)
	e.MarkSaved()
	rev := e.TextRevision()
	e.filePath = "settings.txt" // cache key must distinguish non-Go dialects too
	for _, tok := range e.tokensForLine(0) {
		if tok.typ != tokNormal {
			t.Fatal("JSON cache leaked into plain text")
		}
	}
	e.filePath = "settings.json"
	if !tokensEqual(base, e.tokensForLine(0)) {
		t.Fatal("JSON cache did not return")
	}
	if e.TextRevision() != rev || e.Modified() {
		t.Fatal("coloring mutated document")
	}
	e.ReplaceAllText(`{"x": false}`)
	if tokenSource(e.tokensForLine(0)) != e.Text() {
		t.Fatal("stale tokens after same-line edit")
	}
	e.Undo()
	if !tokensEqual(base, e.tokensForLine(0)) || e.Modified() {
		t.Fatal("undo lost source or cache")
	}
}

func TestJSONEditorDoesNotEnableGoFeatures(t *testing.T) {
	e := jsonEditor(t, "package p\nfunc Fake() {}\nret\niferr")
	if len(e.ParseSymbols()) != 0 || len(e.FoldRegions()) != 0 {
		t.Fatal("invented Go outline/folds")
	}
	e.ScrollToLineCol(2, 3)
	e.TriggerCompletion()
	if findItem(e.completion.filtered, "return") != nil {
		t.Fatal("Go keyword completion leaked")
	}
	e.ScrollToLineCol(3, 5)
	if e.tryExpandSnippet() || e.tryExpandSnippetAtCursor() {
		t.Fatal("Go snippet changed JSON")
	}
}

func TestJSONEditorExplicitOverlayAndLongLineFallback(t *testing.T) {
	e := jsonEditor(t, `{"x": true}`)
	if !e.SetSyntaxHighlights(e.TextRevision(), []SyntaxHighlight{{Line: 0, StartCol: 1, EndCol: 4, Kind: SyntaxFunction}}) {
		t.Fatal("overlay rejected")
	}
	found := false
	for _, tok := range e.tokensForLine(0) {
		found = found || tok.text == `"x"` && tok.typ == tokFunction
	}
	if !found {
		t.Fatal("explicit provider overlay ignored")
	}
	e.ReplaceAllText(strings.Repeat(`{"x":true}`, maxHighlightLineLength))
	got := e.tokensForLine(0)
	if len(got) != 1 || got[0].typ != tokNormal || got[0].text != e.Text() {
		t.Fatal("unbounded tokenization of long line")
	}
}

func BenchmarkJSONEditorCachedLine(b *testing.B) {
	e := NewCodeEditor()
	e.filePath = "settings.json"
	e.SetText(`{"name":"中🙂", "count":-1.25e3, "enabled":true}`)
	e.tokensForLine(0)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		e.tokensForLine(0)
	}
}

func TestJSONEditorStableTokenCacheDoesNotAllocate(t *testing.T) {
	e := jsonEditor(t, `{"name":"中🙂", "ready":true}`)
	e.tokensForLine(0)
	if n := testing.AllocsPerRun(100, func() { e.tokensForLine(0) }); n != 0 {
		t.Fatalf("stable JSON line allocated %v", n)
	}
	for _, size := range []int{maxHighlightLineLength - 1, maxHighlightLineLength} {
		text := "true" + strings.Repeat(" ", size-4)
		e.SetText(text)
		got := e.tokensForLine(0)
		if size < maxHighlightLineLength && got[0].typ != tokKeyword {
			t.Fatal("premature long-line fallback")
		}
		if size == maxHighlightLineLength && (len(got) != 1 || got[0].typ != tokNormal) {
			t.Fatal("missing exact-boundary fallback")
		}
	}
}
