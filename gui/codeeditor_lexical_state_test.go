package gui

import (
	"math/rand"
	"slices"
	"strings"
	"testing"
	"unsafe"
)

func TestGoLineStateDelimiters(t *testing.T) {
	for _, tc := range []struct {
		name, line string
		in, out    goLexState
	}{
		{"normal", "func main() {}", goLexNormal, goLexNormal},
		{"open_block", "/* 中文 `", goLexNormal, goLexBlock},
		{"close_block", "*/ var s = `", goLexBlock, goLexRaw},
		{"open_raw", "var s = `/*", goLexNormal, goLexRaw},
		{"raw_comment", "// /* 中文😀", goLexRaw, goLexRaw},
		{"close_raw", "` /* x", goLexRaw, goLexBlock},
		{"line_comment", "// ` /*", goLexNormal, goLexNormal},
		{"escaped_quote", `"x\"/*" '\'' // ` + "`", goLexNormal, goLexNormal},
		{"invalid_quote", `"/*`, goLexNormal, goLexNormal},
		{"crlf_raw", "`\r", goLexNormal, goLexRaw},
		{"empty_raw", "", goLexRaw, goLexRaw},
		{"empty_block", "", goLexBlock, goLexBlock},
		{"long_closing_block", strings.Repeat("x", maxHighlightLineLength) + "*/", goLexBlock, goLexNormal},
		{"long_closing_raw", strings.Repeat("x", maxHighlightLineLength) + "` /*", goLexRaw, goLexBlock},
		{"long_opening_raw", strings.Repeat("x", maxHighlightLineLength) + "`", goLexNormal, goLexRaw},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := scanGoLineState(tc.line, tc.in); got != tc.out {
				t.Fatalf("scan=%v want=%v", got, tc.out)
			}
			_, got := tokenizeGoLine(tc.line, tc.in)
			if got != tc.out {
				t.Fatalf("tokenize=%v want=%v", got, tc.out)
			}
		})
	}
}

func TestGoStateScannerMatchesTokenizer(t *testing.T) {
	r := rand.New(rand.NewSource(51023))
	alphabet := []rune("abc / * ` \" ' \\ \t\r中文😀_09")
	for i := 0; i < 3000; i++ {
		runes := make([]rune, r.Intn(100))
		for j := range runes {
			runes[j] = alphabet[r.Intn(len(alphabet))]
		}
		line := string(runes)
		for state := goLexNormal; state <= goLexRaw; state++ {
			want := scanGoLineState(line, state)
			tokens, got := tokenizeGoLine(line, state)
			if got != want {
				t.Fatalf("%q in=%v: tokenize=%v scan=%v", line, state, got, want)
			}
			var text strings.Builder
			for _, tok := range tokens {
				text.WriteString(tok.text)
			}
			if text.String() != line {
				t.Fatalf("tokens lost bytes for %q", line)
			}
		}
	}
}

func assertGoLineStates(t *testing.T, c *goLineStateCache, lines []string) {
	t.Helper()
	if len(c.lines) != len(lines) {
		t.Fatalf("state length %d != %d", len(c.lines), len(lines))
	}
	state := goLexNormal
	for i, line := range lines {
		next := scanGoLineState(line, state)
		if got := c.lines[i]; got.text != line || got.in != state || got.out != next {
			t.Fatalf("line %d %q: %+v want in=%v out=%v", i, line, got, state, next)
		}
		state = next
	}
}

func TestGoLineStateIncrementalPropagation(t *testing.T) {
	lines := []string{"package p", "var s = `", "/* still raw", "`", "var End = 1", "// tail"}
	var c goLineStateCache
	if n := c.update(lines); n != len(lines) {
		t.Fatal("initial scan", n)
	}
	if n := c.update(lines); n != 0 {
		t.Fatal("unchanged scan", n)
	}
	lines[2] = "other /* raw text"
	if n := c.update(lines); n != 1 {
		t.Fatal("local edit scanned suffix", n)
	}
	lines[1] = "/*"
	if n := c.update(lines); n != 5 {
		t.Fatal("missing propagation", n)
	}
	assertGoLineStates(t, &c, lines)
	lines[3] = "*/"
	if n := c.update(lines); n != 3 {
		t.Fatal("missing closing propagation", n)
	}
	assertGoLineStates(t, &c, lines)
	lines = slices.Insert(lines, 2, "new comment body")
	if n := c.update(lines); n != 1 {
		t.Fatal("insert scanned unchanged suffix", n)
	}
	assertGoLineStates(t, &c, lines)
	lines = slices.Delete(lines, 2, 3)
	if n := c.update(lines); n != 0 {
		t.Fatal("delete rescanned converged suffix", n)
	}
	assertGoLineStates(t, &c, lines)
}

func TestGoLineStateRandomEdits(t *testing.T) {
	r := rand.New(rand.NewSource(51024))
	var c goLineStateCache
	lines := []string{"package p"}
	corpus := []string{"", "/*", "*/", "`", "var x = `foo", "` // end", "// /* `", "func f() {}", "中文😀", `"escaped\"/*"`, strings.Repeat("x", maxHighlightLineLength) + "*/"}
	c.update(lines)
	for i := 0; i < 2000; i++ {
		switch r.Intn(3) {
		case 0:
			lines = slices.Insert(lines, r.Intn(len(lines)+1), corpus[r.Intn(len(corpus))])
		case 1:
			if len(lines) > 0 {
				j := r.Intn(len(lines))
				lines = slices.Delete(lines, j, j+1)
			}
		case 2:
			if len(lines) > 0 {
				lines[r.Intn(len(lines))] = corpus[r.Intn(len(corpus))]
			}
		}
		c.update(lines)
		assertGoLineStates(t, &c, lines)
	}
}

func TestGoLineStateRebindsStorageAndClearsTail(t *testing.T) {
	lines := strings.Split("prefix\n/*\nbody\n*/\nsuffix", "\n")
	var c goLineStateCache
	c.update(lines)
	replacement := strings.Split(strings.Clone(strings.Join(lines, "\n")), "\n")
	replacement[2] = "changed"
	c.update(replacement)
	for i, line := range replacement {
		if unsafe.StringData(c.lines[i].text) != unsafe.StringData(line) {
			t.Fatal("old storage retained", i)
		}
	}
	old := c.lines
	c.update(replacement[:1])
	for _, record := range old[1:] {
		if record != (goLineState{}) {
			t.Fatal("deleted tail retained", record)
		}
	}
	c.update(nil)
	if len(c.lines) != 0 {
		t.Fatal("empty document retains records")
	}
}

func TestEditorGoLineStatesFollowEditUndoRedoAndDialect(t *testing.T) {
	e := NewCodeEditor()
	defer e.CancelGitStatus()
	e.SetSize(1000, 700)
	original := "package p\nvar s = `\n/* raw\n`\nfunc After() {}\n"
	e.SetText(original)
	assertGoLineStates(t, &e.goLineStates, e.lines)
	updated := strings.Replace(original, "/* raw", "raw */", 1)
	e.ReplaceAllText(updated)
	assertGoLineStates(t, &e.goLineStates, e.lines)
	e.Undo()
	if e.Text() != original {
		t.Fatal("undo text")
	}
	assertGoLineStates(t, &e.goLineStates, e.lines)
	e.Redo()
	if e.Text() != updated {
		t.Fatal("redo text")
	}
	assertGoLineStates(t, &e.goLineStates, e.lines)
	e.SetFilePath("lexical.txt")
	if len(e.goLineStates.lines) != 0 {
		t.Fatal("plain text kept Go state")
	}
	e.SetFilePath("lexical.silkgo")
	assertGoLineStates(t, &e.goLineStates, e.lines)
	e.SetText(strings.Clone(e.Text()))
	assertGoLineStates(t, &e.goLineStates, e.lines)
}

func TestGoTokenCacheKeysRawState(t *testing.T) {
	e := NewCodeEditor()
	line := "func fake /*"
	for _, state := range []goLexState{goLexNormal, goLexRaw, goLexBlock, goLexNormal, goLexRaw} {
		want, next := tokenizeGoLine(line, state)
		got, gotNext := e.tokenizeGoLineCached(0, line, state)
		if !tokensEqual(want, got) || next != gotNext {
			t.Fatal("cache state collision", state)
		}
	}
}

func TestGoLineStateStablePaintDoesNotRebuild(t *testing.T) {
	e := NewCodeEditor()
	e.SetSize(1000, 700)
	e.SetText("package p\n" + strings.Repeat("// stable\n", 10000) + "var End = 1\n")
	e.ScrollToLineCol(10001, 0)
	e.Draw(editorNoopPainter{})
	before := slices.Clone(e.goLineStates.lines)
	first := &e.goLineStates.lines[0]
	for i := 0; i < 3; i++ {
		e.Draw(editorNoopPainter{})
	}
	if first != &e.goLineStates.lines[0] || !slices.Equal(before, e.goLineStates.lines) {
		t.Fatal("paint changed lexical index")
	}
	if n := testing.AllocsPerRun(100, func() { e.tokensForLine(10001) }); n != 0 {
		t.Fatal("warm token lookup allocates", n)
	}
	if n := testing.AllocsPerRun(100, func() { scanGoLineState("` /* raw //", goLexNormal) }); n != 0 {
		t.Fatal("state scanner allocates", n)
	}
}
