package gui

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/uk0/silk/paint"
)

func TestCompletionRowLongDetailDoesNotOverlapLabel(t *testing.T) {
	for _, size := range []int{8, 18, 40} {
		font := paint.NewFont("monospace", size, false, false)
		for _, width := range []float64{80, 300, 480} {
			item := CompletionItem{Text: "Println", Detail: strings.Repeat("func(中文...any) (n int, err error)\n", 8)}
			label, detail, x := completionRowText(font, item, width)
			if !utf8.ValidString(label+detail) || (label != "" && 24+completionTextWidth(font, label) > width-8) {
				t.Fatalf("label outside popup: size=%d width=%v label=%q", size, width, label)
			}
			if strings.ContainsAny(detail, "\n\r\t") || (detail != "" && (x < 24+font.TextExtents(label).XAdvance+16 || x+font.TextExtents(detail).XAdvance > width-7)) {
				t.Fatalf("size=%d width=%v label=%q detail=%q x=%v", size, width, label, detail, x)
			}
			if detail != "" && x+completionTextWidth(font, detail) > width-8 {
				t.Fatalf("detail ink outside popup: size=%d width=%v detail=%q", size, width, detail)
			}
		}
	}
}

func TestProviderCompletionPrecedesTypedFragment(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("fmt.Pr")
	e.ScrollToLineCol(0, 6)
	called := false
	e.SetExternalCompletions([]ExternalCompletion{{Label: "Println", Apply: func(*CodeEditor) { called = true }}})
	e.TriggerCompletion()
	if !e.CompletionVisible() || e.completion.filtered[0].Text != "Println" {
		t.Fatalf("typed fragment outranks semantic candidate: %+v", e.completion.filtered)
	}
	e.completion.Accept(e)
	if !called {
		t.Fatal("provider edit not applied")
	}
	e.ClearExternalCompletions()
	e.TriggerCompletion()
	for _, item := range e.completion.filtered {
		if item.apply != nil {
			t.Fatal("stale provider action survived clear")
		}
	}
}

func TestProviderCompletionPrecedesKeywordsAfterDot(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("fmt.")
	e.ScrollToLineCol(0, 4)
	e.SetExternalCompletions([]ExternalCompletion{{Label: "Println", Apply: func(*CodeEditor) {}}})
	e.TriggerCompletion()
	if e.completion.filtered[0].Text != "Println" {
		t.Fatal("unrelated local keyword selected before provider candidate")
	}
}
