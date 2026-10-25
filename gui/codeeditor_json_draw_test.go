package gui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/uk0/silk/paint"
)

const jsonDrawFixture = `{
  "name": "SilkIDE 中文🙂",
  "enabled": true,
  "missing": null,
  "retry": 12345,
  "delay": -1.25e+3,
  "path": "C:\\projects\\silk",
  "items": ["editor", "designer", false]
}`

func TestJSONEditorDrawThemesAndScales(t *testing.T) {
	mode := CurrentThemeMode()
	t.Cleanup(func() { SetThemeMode(mode) })
	for _, theme := range []ThemeMode{ThemeLight, ThemeDark} {
		SetThemeMode(theme)
		for _, scale := range []int{1, 2} {
			t.Run(fmt.Sprintf("theme%d-%dx", theme, scale), func(t *testing.T) {
				e := jsonEditor(t, jsonDrawFixture)
				e.SetSize(1000, 500)
				r := &editorLexicalRecorder{fragments: map[string]paint.Color{}}
				e.Draw(r)
				for text, typ := range map[string]tokenType{`"name"`: tokType, `"SilkIDE 中文🙂"`: tokString, "true": tokKeyword, "null": tokKeyword, "12345": tokNumber, "-1.25e+3": tokNumber} {
					if got, ok := r.fragments[text]; !ok || got != e.editorColors().tokens[typ] {
						t.Fatalf("wrong draw color for %q: %v present=%v", text, got, ok)
					}
				}
				if _, ok := r.fragments["JSON"]; !ok {
					t.Fatal("language absent from status bar")
				}
				if _, ok := r.fragments["Tabs: 4"]; !ok {
					t.Fatal("indentation policy absent from wide status bar")
				}
				pix := paint.NewPixmap(1000*scale, 500*scale)
				g := pix.NewPainter()
				defer paint.ClosePainter(g)
				g.Scale(float64(scale), float64(scale))
				e.Draw(g)
				img, err := pix.Image()
				if err != nil {
					t.Fatal(err)
				}
				r8, g8, b8, a8 := img.At(700*scale, 300*scale).RGBA()
				bg := e.editorColors().bg
				if byte(r8>>8) != bg.R || byte(g8>>8) != bg.G || byte(b8>>8) != bg.B || a8 != 0xffff {
					t.Fatal("wrong raster background")
				}
				if dir := os.Getenv("SILK_JSON_QA_DIR"); dir != "" {
					if err := os.MkdirAll(dir, 0755); err != nil {
						t.Fatal(err)
					}
					if err := pix.WritePNG(filepath.Join(dir, fmt.Sprintf("json-theme%d-%dx.png", theme, scale))); err != nil {
						t.Fatal(err)
					}
				}
			})
		}
	}
}

func TestJSONEditorLargeAsyncProvidersAndLiterals(t *testing.T) {
	e, posts := asyncSymbolsEditor(t)
	e.filePath = "settings.json"
	e.resetDocumentLanguage()
	if _, ready := e.SymbolsSnapshot(); ready {
		t.Fatal("fixture did not exercise background symbols")
	}
	waitCompletionPost(t, posts)()
	waitSymbolWorker(t, e.symbolWorker)
	if symbols, ready := e.SymbolsSnapshot(); !ready || len(symbols) != 0 {
		t.Fatal("background parser invented Go symbols", symbols)
	}
	e.SetText("tru\n" + strings.Repeat("  \"item\": 0,\n", 4000))
	e.ScrollToLineCol(0, 3)
	e.completion = NewCompletionPopup(e)
	e.completion.indexPost = func(f func()) { posts <- f }
	t.Cleanup(func() {
		w := e.completion.indexWorker
		e.CancelCompletion()
		if w != nil {
			waitCompletionWorker(t, w)
		}
	})
	e.TriggerCompletion()
	waitCompletionPost(t, posts)()
	if findItem(e.completion.filtered, "true") == nil || findItem(e.completion.items, "return") != nil {
		t.Fatal("wrong JSON asynchronous completion candidates")
	}
}

func TestJSONEditorModeSwitchPreservesHistoryAndDropsOldProjections(t *testing.T) {
	e := jsonEditor(t, `{"x": true}`)
	// Exercise the real path-change API without leaking unrelated Git work into
	// the process-wide UI queue used by other tests. Git delivery is tested by
	// the dedicated service tests; this fixture only checks document modes.
	e.gitService = newGitStatusService(1, func(context.Context, string) (map[int]GitLineStatus, error) {
		return nil, nil
	}, func(func()) {})
	t.Cleanup(func() {
		e.CancelGitStatus()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := e.gitService.shutdown(ctx); err != nil {
			t.Fatal(err)
		}
	})
	e.MarkSaved()
	e.ReplaceAllText(`{"x": false}`)
	rev := e.TextRevision()
	e.SetSyntaxHighlights(rev, []SyntaxHighlight{{Line: 0, StartCol: 1, EndCol: 4, Kind: SyntaxFunction}})
	e.SetFilePath(filepath.Join(t.TempDir(), "notes.txt"))
	for _, tok := range e.tokensForLine(0) {
		if tok.typ != tokNormal {
			t.Fatal("stale provider after mode switch")
		}
	}
	e.SetFilePath(filepath.Join(t.TempDir(), "notes.json"))
	if e.TextRevision() != rev || !e.Modified() {
		t.Fatal("mode switch changed buffer")
	}
	e.Undo()
	if e.Text() != `{"x": true}` || e.Modified() {
		t.Fatal("mode switch lost undo")
	}
	e.FormatCode()
	if e.Text() != `{"x": true}` {
		t.Fatal("Go formatter changed JSON")
	}
	e.SetSize(1000, 500)
	vis := e.visibleLineIndices()
	segments := e.minimapSegments(e.minimapLayout(), vis)
	found := false
	for _, seg := range segments {
		found = found || seg.typ == tokType
	}
	if !found {
		t.Fatal("minimap omitted JSON key color")
	}
}

func FuzzJSONLinePreservesBytes(f *testing.F) {
	for _, s := range []string{jsonDrawFixture, `"x\"": true`, "\xff\x00", strings.Repeat("x", maxHighlightLineLength), ""} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if got := tokenSource(tokenizeJSONLine(s)); got != s {
			t.Fatal("lexer changed bytes")
		}
	})
}
func FuzzJSONNumberMatchesDecoder(f *testing.F) {
	for _, s := range []string{"0", "-1.25e+3", "01", "1e", "-0", "1 ", " 1", "1e9999", ""} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		starts := len(s) > 0 && (s[0] == '-' || s[0] >= '0' && s[0] <= '9')
		want := starts && strings.TrimSpace(s) == s && json.Valid([]byte(s))
		if jsonNumber(s) != want {
			t.Fatalf("number %q got %v want %v", s, jsonNumber(s), want)
		}
	})
}

func BenchmarkJSONEditorScrolledDraw(b *testing.B) {
	for _, n := range []int{1000, 100000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			e := NewCodeEditor()
			e.filePath = "settings.json"
			e.SetSize(1000, 700)
			e.SetText("[\n" + strings.Repeat(`{"name":"item", "ok":true},`+"\n", n) + "null\n]")
			e.ScrollToLineCol(n, 0)
			e.ParseSymbols()
			g := editorNoopPainter{}
			e.Draw(g)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				e.Draw(g)
			}
		})
	}
}
