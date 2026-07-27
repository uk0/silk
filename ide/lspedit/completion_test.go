package lspedit

import (
	"errors"
	"testing"
)

// The buffer every completion test works on: an unimported fmt call with the
// caret after a two-character prefix, which is exactly the case that needs an
// additionalTextEdits import to compile.
const bufSrc = "package main\n\nfunc main() {\n\tfmt.Pr\n}\n"

// caret sits right after "fmt.Pr" on line 3 ("\tfmt.Pr" is 7 UTF-16 units).
var caret = Position{Line: 3, Character: 7}

func TestResolveCompletionTextEditWins(t *testing.T) {
	// The server replaces "Pr" itself; InsertText must be ignored (LSP rule),
	// otherwise the buffer ends up with "fmt.PrPRINTLN".
	item := CompletionItem{
		Label:      "Println",
		InsertText: "PRINTLN",
		TextEdit: &TextEdit{
			Range:   Range{Start: Position{3, 5}, End: Position{3, 7}},
			NewText: "Println",
		},
	}
	app, err := ResolveCompletion(bufSrc, caret, item)
	if err != nil {
		t.Fatalf("ResolveCompletion: %v", err)
	}
	if app.Primary != *item.TextEdit {
		t.Fatalf("primary = %+v, want the item's textEdit %+v", app.Primary, *item.TextEdit)
	}
	got, err := app.Apply(bufSrc)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	want := "package main\n\nfunc main() {\n\tfmt.Println\n}\n"
	if got != want {
		t.Fatalf("applied text = %q, want %q", got, want)
	}
}

func TestResolveCompletionPlainInsertReplacesPrefix(t *testing.T) {
	// No textEdit: the insertion has to consume the identifier prefix left of
	// the caret, not append to it.
	app, err := ResolveCompletion(bufSrc, caret, CompletionItem{Label: "Println", InsertText: "Println"})
	if err != nil {
		t.Fatalf("ResolveCompletion: %v", err)
	}
	wantRange := Range{Start: Position{3, 5}, End: Position{3, 7}}
	if app.Primary.Range != wantRange {
		t.Fatalf("primary range = %+v, want %+v (the typed \"Pr\")", app.Primary.Range, wantRange)
	}
	got, err := app.Apply(bufSrc)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	want := "package main\n\nfunc main() {\n\tfmt.Println\n}\n"
	if got != want {
		t.Fatalf("applied text = %q, want %q", got, want)
	}
}

func TestResolveCompletionLabelFallback(t *testing.T) {
	app, err := ResolveCompletion(bufSrc, caret, CompletionItem{Label: "Printf"})
	if err != nil {
		t.Fatalf("ResolveCompletion: %v", err)
	}
	if app.Primary.NewText != "Printf" {
		t.Fatalf("NewText = %q, want the label %q", app.Primary.NewText, "Printf")
	}
	// The prefix stops at the dot, so only "Pr" is replaced.
	if app.Primary.Range.Start != (Position{3, 5}) {
		t.Fatalf("prefix start = %+v, want line 3 char 5", app.Primary.Range.Start)
	}
}

func TestResolveCompletionPrefixEmptyAtDot(t *testing.T) {
	// Caret directly after "fmt." — no prefix typed yet, so the primary edit is
	// a pure insertion.
	app, err := ResolveCompletion(bufSrc, Position{3, 5}, CompletionItem{Label: "Println"})
	if err != nil {
		t.Fatalf("ResolveCompletion: %v", err)
	}
	if app.Primary.Range.Start != app.Primary.Range.End {
		t.Fatalf("range = %+v, want an empty insertion range", app.Primary.Range)
	}
	got, err := app.Apply(bufSrc)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if want := "package main\n\nfunc main() {\n\tfmt.PrintlnPr\n}\n"; got != want {
		t.Fatalf("applied text = %q, want %q", got, want)
	}
}

func TestResolveCompletionAdditionalTextEditsOffsets(t *testing.T) {
	// gopls' auto-import shape: replace the prefix on line 3 AND add an import
	// line at the top. The import edit sits BEFORE the primary one, so a
	// sequential application would resolve the primary range against a shifted
	// document; both must be resolved against the original buffer.
	imp := TextEdit{
		Range:   Range{Start: Position{1, 0}, End: Position{1, 0}},
		NewText: "import \"fmt\"\n",
	}
	item := CompletionItem{
		Label: "Println",
		TextEdit: &TextEdit{
			Range:   Range{Start: Position{3, 5}, End: Position{3, 7}},
			NewText: "Println",
		},
		AdditionalTextEdits: []TextEdit{imp},
	}
	app, err := ResolveCompletion(bufSrc, caret, item)
	if err != nil {
		t.Fatalf("ResolveCompletion: %v", err)
	}
	if len(app.Edits()) != 2 {
		t.Fatalf("Edits() = %d edits, want the primary plus the import", len(app.Edits()))
	}
	got, err := app.Apply(bufSrc)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	want := "package main\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println\n}\n"
	if got != want {
		t.Fatalf("applied text = %q, want %q", got, want)
	}

	// Guard the reason this matters: applying the import first and then the
	// primary edit against the NEW text corrupts line 3.
	stepwise, err := ApplyEdits(bufSrc, []TextEdit{imp})
	if err != nil {
		t.Fatalf("ApplyEdits(import): %v", err)
	}
	stepwise, err = ApplyEdits(stepwise, []TextEdit{*item.TextEdit})
	if err != nil {
		t.Fatalf("ApplyEdits(primary): %v", err)
	}
	if stepwise == want {
		t.Fatal("sequential application produced the same text; the offset test is not exercising drift")
	}
}

func TestResolveCompletionAdditionalEditsWithoutTextEdit(t *testing.T) {
	// Same import, but the item only carries insertText: the additional edit
	// must survive the plain-insert path too.
	item := CompletionItem{
		Label:      "Println",
		InsertText: "Println",
		AdditionalTextEdits: []TextEdit{{
			Range:   Range{Start: Position{1, 0}, End: Position{1, 0}},
			NewText: "import \"fmt\"\n",
		}},
	}
	app, err := ResolveCompletion(bufSrc, caret, item)
	if err != nil {
		t.Fatalf("ResolveCompletion: %v", err)
	}
	got, err := app.Apply(bufSrc)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	want := "package main\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println\n}\n"
	if got != want {
		t.Fatalf("applied text = %q, want %q", got, want)
	}
}

func TestResolveCompletionSnippetBody(t *testing.T) {
	body := "Println(${1:a})$0"
	item := CompletionItem{
		Label:     "Println",
		IsSnippet: true,
		TextEdit: &TextEdit{
			Range:   Range{Start: Position{3, 5}, End: Position{3, 7}},
			NewText: body,
		},
	}
	app, err := ResolveCompletion(bufSrc, caret, item)
	if err != nil {
		t.Fatalf("ResolveCompletion: %v", err)
	}
	if !app.Snippet {
		t.Fatal("Snippet = false, want true")
	}
	if app.SnippetBody != body {
		t.Fatalf("SnippetBody = %q, want the template %q unexpanded", app.SnippetBody, body)
	}
	if app.Primary.NewText != body {
		t.Fatalf("Primary.NewText = %q, want the template %q", app.Primary.NewText, body)
	}
	if app.Primary.Range != item.TextEdit.Range {
		t.Fatalf("Primary.Range = %+v, want the snippet target %+v", app.Primary.Range, item.TextEdit.Range)
	}
}

func TestResolveCompletionSnippetKeepsAdditionalEdits(t *testing.T) {
	item := CompletionItem{
		Label:      "Println",
		IsSnippet:  true,
		InsertText: "Println($1)$0",
		AdditionalTextEdits: []TextEdit{{
			Range:   Range{Start: Position{1, 0}, End: Position{1, 0}},
			NewText: "import \"fmt\"\n",
		}},
	}
	app, err := ResolveCompletion(bufSrc, caret, item)
	if err != nil {
		t.Fatalf("ResolveCompletion: %v", err)
	}
	if app.SnippetBody != "Println($1)$0" {
		t.Fatalf("SnippetBody = %q", app.SnippetBody)
	}
	if len(app.Additional) != 1 || app.Additional[0].NewText != "import \"fmt\"\n" {
		t.Fatalf("Additional = %+v, want the import edit", app.Additional)
	}
}

func TestResolveCompletionErrors(t *testing.T) {
	if _, err := ResolveCompletion(bufSrc, Position{Line: 99}, CompletionItem{Label: "x"}); !errors.Is(err, ErrInvalidRange) {
		t.Fatalf("cursor past end of document: err = %v, want ErrInvalidRange", err)
	}
	if _, err := ResolveCompletion(bufSrc, caret, CompletionItem{}); !errors.Is(err, ErrEmptyItem) {
		t.Fatalf("empty item: err = %v, want ErrEmptyItem", err)
	}
	// An additional edit overlapping the primary one must be rejected, not
	// applied into a corrupted buffer.
	overlapping := CompletionItem{
		Label: "Println",
		TextEdit: &TextEdit{
			Range:   Range{Start: Position{3, 5}, End: Position{3, 7}},
			NewText: "Println",
		},
		AdditionalTextEdits: []TextEdit{{
			Range:   Range{Start: Position{3, 4}, End: Position{3, 6}},
			NewText: "X",
		}},
	}
	if _, err := ResolveCompletion(bufSrc, caret, overlapping); !errors.Is(err, ErrOverlap) {
		t.Fatalf("overlapping additional edit: err = %v, want ErrOverlap", err)
	}
	// A textEdit range outside the document is rejected up front.
	bad := CompletionItem{TextEdit: &TextEdit{
		Range:   Range{Start: Position{3, 5}, End: Position{3, 99}},
		NewText: "x",
	}}
	if _, err := ResolveCompletion(bufSrc, caret, bad); !errors.Is(err, ErrInvalidRange) {
		t.Fatalf("textEdit past end of line: err = %v, want ErrInvalidRange", err)
	}
}

func TestApplyEditsOrderIndependent(t *testing.T) {
	src := "one\ntwo\nthree\n"
	a := TextEdit{Range: Range{Position{0, 0}, Position{0, 3}}, NewText: "ONE"}
	b := TextEdit{Range: Range{Position{2, 0}, Position{2, 5}}, NewText: "THREE"}
	want := "ONE\ntwo\nTHREE\n"
	for _, edits := range [][]TextEdit{{a, b}, {b, a}} {
		got, err := ApplyEdits(src, edits)
		if err != nil {
			t.Fatalf("ApplyEdits: %v", err)
		}
		if got != want {
			t.Fatalf("ApplyEdits = %q, want %q", got, want)
		}
	}
}

func TestApplyEditsUTF16Columns(t *testing.T) {
	// "a" + U+1F642 (two UTF-16 units, four bytes) + "b".
	src := "a\U0001F642b\n"
	got, err := ApplyEdits(src, []TextEdit{{
		Range:   Range{Start: Position{0, 3}, End: Position{0, 4}},
		NewText: "B",
	}})
	if err != nil {
		t.Fatalf("ApplyEdits: %v", err)
	}
	if want := "a\U0001F642B\n"; got != want {
		t.Fatalf("ApplyEdits = %q, want %q", got, want)
	}
	// Character 2 lands inside the surrogate pair: refuse rather than cut the
	// rune in half.
	if _, err := ApplyEdits(src, []TextEdit{{
		Range: Range{Start: Position{0, 2}, End: Position{0, 3}},
	}}); !errors.Is(err, ErrInvalidRange) {
		t.Fatalf("mid-surrogate column: err = %v, want ErrInvalidRange", err)
	}
	// Character past the end of the line is out of range, not clamped.
	if _, err := ApplyEdits(src, []TextEdit{{
		Range: Range{Start: Position{0, 4}, End: Position{0, 9}},
	}}); !errors.Is(err, ErrInvalidRange) {
		t.Fatalf("column past end of line: err = %v, want ErrInvalidRange", err)
	}
	// The end of the last line is a valid insertion point.
	if _, err := ApplyEdits(src, []TextEdit{{
		Range:   Range{Start: Position{1, 0}, End: Position{1, 0}},
		NewText: "tail",
	}}); err != nil {
		t.Fatalf("insertion at end of document: %v", err)
	}
}

func TestApplyEditsRejectsInvertedRange(t *testing.T) {
	src := "one\ntwo\n"
	_, err := ApplyEdits(src, []TextEdit{{
		Range: Range{Start: Position{1, 2}, End: Position{0, 1}},
	}})
	if !errors.Is(err, ErrInvalidRange) {
		t.Fatalf("err = %v, want ErrInvalidRange", err)
	}
}

func TestApplyEditsTouchingRangesAllowed(t *testing.T) {
	// Half-open spans that meet are not an overlap: [0,3) then [3,3).
	src := "one\n"
	got, err := ApplyEdits(src, []TextEdit{
		{Range: Range{Position{0, 0}, Position{0, 3}}, NewText: "1"},
		{Range: Range{Position{0, 3}, Position{0, 3}}, NewText: "!"},
	})
	if err != nil {
		t.Fatalf("ApplyEdits: %v", err)
	}
	if want := "1!\n"; got != want {
		t.Fatalf("ApplyEdits = %q, want %q", got, want)
	}
}
