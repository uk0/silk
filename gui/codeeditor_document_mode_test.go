package gui

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/uk0/silk/paint"
)

type editorStatusRecorder struct {
	paint.Painter
	labels []string
}

func (*editorStatusRecorder) SetBrush1(paint.Color)                        {}
func (*editorStatusRecorder) SetPen1(paint.Color, float64)                 {}
func (*editorStatusRecorder) Rectangle(float64, float64, float64, float64) {}
func (*editorStatusRecorder) Line(float64, float64, float64, float64)      {}
func (*editorStatusRecorder) Fill()                                        {}
func (*editorStatusRecorder) Stroke()                                      {}
func (*editorStatusRecorder) SetFont(paint.Font)                           {}
func (r *editorStatusRecorder) DrawText1(_, _ float64, text string) {
	r.labels = append(r.labels, text)
}

func TestEditorStatusUsesLoadedBufferImmediately(t *testing.T) {
	e := NewCodeEditor()
	text := "你好😀\n" + strings.Repeat("item\n", 100000)
	e.SetText(text)
	r := &editorStatusRecorder{}
	e.drawStatusBar(r, 1000, 500, 20)
	want := fmt.Sprintf("%d chars, %d lines", utf8.RuneCountInString(text), strings.Count(text, "\n")+1)
	if !strings.Contains(strings.Join(r.labels, "|"), want) {
		t.Fatalf("first draw: %v, want %q", r.labels, want)
	}
}

func TestPlainTextDoesNotInventGoSyntaxSymbolsOrCandidates(t *testing.T) {
	e := NewCodeEditor()
	e.filePath = "notes.txt"
	e.SetSize(1000, 500)
	e.SetText("ret\nfunc NotGo() {\n // ordinary prose\n}\n")
	t.Run("tokens", func(t *testing.T) {
		tokens, block := e.tokenizeLineCached(1, e.lines[1], false)
		for _, token := range tokens {
			if token.typ != tokNormal {
				t.Fatalf("plain text colored as Go: %+v", tokens)
			}
		}
		if block {
			t.Fatal("plain text entered Go block comment")
		}
	})
	t.Run("symbols", func(t *testing.T) {
		e.SetText("package p\nfunc NotGo() {}\n")
		if got := e.ParseSymbols(); len(got) != 0 {
			t.Fatalf("invented Go symbols: %+v", got)
		}
	})
	t.Run("completion", func(t *testing.T) {
		e.SetText("ret\nRemoteWord\n")
		e.ScrollToLineCol(0, 3)
		e.TriggerCompletion()
		defer e.CancelCompletion()
		if findItem(e.completion.filtered, "return") != nil {
			t.Fatal("Go keyword leaked into plain text")
		}
	})
	t.Run("status", func(t *testing.T) {
		r := &editorStatusRecorder{}
		e.drawStatusBar(r, 1000, 500, 20)
		if !strings.Contains(strings.Join(r.labels, "|"), "Plain Text") {
			t.Fatal("wrong language label", r.labels)
		}
	})
}

func TestEditorLanguagePathDetection(t *testing.T) {
	for _, tc := range []struct{ path, language, label string }{
		{"", "go", "Go"}, {"main.go", "go", "Go"}, {"MAIN.GO", "go", "Go"},
		{"main.silkgo", "silkgo", "SilkGo"}, {"MAIN.SILKGO", "silkgo", "SilkGo"},
		{"notes.txt", "plaintext", "Plain Text"}, {"config.json", "plaintext", "Plain Text"},
		{"README", "plaintext", "Plain Text"}, {"go.mod", "plaintext", "Plain Text"},
		{"script.go.txt", "plaintext", "Plain Text"},
	} {
		e := NewCodeEditor()
		e.filePath = tc.path
		if e.Language() != tc.language || e.languageLabel() != tc.label {
			t.Fatal(tc, e.Language(), e.languageLabel())
		}
	}
}

func TestEditorLanguageChangeInvalidatesWithoutEditing(t *testing.T) {
	e := NewCodeEditor()
	defer e.CancelGitStatus()
	defer e.CancelCompletion()
	e.SetSize(1000, 500)
	source := "package p\nfunc Sample() {\n println(1)\n}\n"
	e.SetText(source)
	e.MarkSaved()
	_, _ = e.tokenizeLineCached(1, e.lines[1], false)
	if len(e.ParseSymbols()) == 0 {
		t.Fatal("Go fixture has no symbols")
	}
	e.ToggleFold(1)
	revision := e.TextRevision()
	changed := 0
	e.SigChanged(func(string) { changed++ })
	path := filepath.Join(t.TempDir(), "notes.txt")
	e.SetFilePath(path)
	if len(e.FoldRegions()) != 0 || len(e.ParseSymbols()) != 0 || e.IsFolded(1) {
		t.Fatal("old Go projections survived")
	}
	if len(e.visibleLineIndices()) != len(e.lines) {
		t.Fatal("hidden Go fold survived")
	}
	if e.TextRevision() != revision || e.Text() != source || changed != 0 || e.Modified() {
		t.Fatal("filename changed the document")
	}
	e.SetFilePath(path + ".go")
	if len(e.ParseSymbols()) == 0 || len(e.FoldRegions()) == 0 {
		t.Fatal("switching back did not restore Go projections")
	}
}

func TestPlainTextKeepsExplicitProvidersButNotDefaultGoSnippets(t *testing.T) {
	e := NewCodeEditor()
	e.filePath = "notes.txt"
	e.SetSize(800, 500)
	e.SetText("iferr")
	e.ScrollToLineCol(0, 5)
	if e.tryExpandSnippetAtCursor() || e.tryExpandSnippet() {
		t.Fatal("default Go expansion changed plain text")
	}
	e.SetSnippets(NewGoSnippetSet()) // explicitly chosen by the SDK caller
	if !e.tryExpandSnippetAtCursor() {
		t.Fatal("explicit snippet set was suppressed")
	}
	e.SetText("provider")
	e.SetSyntaxHighlights(e.TextRevision(), []SyntaxHighlight{{Line: 0, StartCol: 0, EndCol: 8, Kind: SyntaxKeyword}})
	tokens, _ := e.tokenizeLineCached(0, e.lines[0], false)
	if len(tokens) != 1 || tokens[0].typ != tokKeyword {
		t.Fatal("explicit highlighting was suppressed", tokens)
	}
	e.SetAdditionalSymbols(e.TextRevision(), []CodeSymbol{{Name: "provider", Kind: SymVar, Line: 0}})
	if got := e.ParseSymbols(); len(got) != 1 || got[0].Name != "provider" {
		t.Fatal("explicit symbols were suppressed", got)
	}
}

func TestCompletionLanguageChangeRejectsQueuedOldIndex(t *testing.T) {
	e, posts := indexedCompletionEditor(t)
	e.SetText("ret\n" + completionLargeSource(3000) + "ReturnLocal\n")
	e.ScrollToLineCol(0, 3)
	e.TriggerCompletion()
	oldDelivery := waitCompletionPost(t, posts)
	e.SetFilePath(filepath.Join(t.TempDir(), "notes.txt"))
	defer e.CancelGitStatus()
	if e.completion.items != nil || e.completion.filtered != nil {
		t.Fatal("cancel retained old candidates/snapshot")
	}
	e.TriggerCompletion()
	oldDelivery()
	waitCompletionPost(t, posts)()
	if !e.CompletionVisible() || findItem(e.completion.filtered, "ReturnLocal") == nil || findItem(e.completion.filtered, "return") != nil {
		t.Fatal("wrong dialect index", e.completion.filtered)
	}
}

func BenchmarkDocumentSymbolDialect(b *testing.B) {
	var text strings.Builder
	text.WriteString("package p\n")
	for i := 0; i < 8000; i++ {
		fmt.Fprintf(&text, "func Function%d() {}\n", i)
	}
	for _, path := range []string{"source.go", "notes.txt"} {
		b.Run(path, func(b *testing.B) {
			e := NewCodeEditor()
			e.filePath = path
			e.SetText(text.String())
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				e.symbolCache.valid = false
				e.cachedSymbols()
			}
		})
	}
}
