package gui

import (
	"context"
	"errors"
	"math/rand"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestEditorRegexStdlibOracle(t *testing.T) {
	patterns := []string{`a`, `a|ab`, `ab|a`, `a*`, `a*?`, `a?`, `a+`, `.*`, `(?s).*`, `.*?`, `(?m)^.*$`, `\A|a`, `^|$`, `\b`, `\B`, `\z`, `(?:\A)?a`, `(\A)|(a)`, `(?P<name>a)|(b)`, `a(b*)`, `[\pL\pM]+`, `a\nb`, `(?:a|\n)*`, `(?m)^`, `(?m)$`, `(?m)\A|^b`, `(a?)(b*)`, `\x{FFFD}`, `(?i)k|s`, `a{0,2}`, `(?:\b|$)`, `(?:ab)?`, `(?m:^)a?`}
	texts := []string{"", "a", "aaa", "aba ab a\nb\n", "\r\na\r\nb", "中a中 a中\n中b\n", "\xffa\xfe\n", "x\u0301 a", "KkſS"}
	rng := rand.New(rand.NewSource(37))
	for i := 0; i < 80; i++ {
		var s strings.Builder
		alphabet := []rune("aab \n中\u0301_")
		for j := 0; j < rng.Intn(60); j++ {
			s.WriteRune(alphabet[rng.Intn(len(alphabet))])
		}
		texts = append(texts, s.String())
	}
	for _, pattern := range patterns {
		for _, text := range texts {
			re, err := compileEditorRegex(context.Background(), pattern, EditorFindOptions{Regex: true, MatchCase: true})
			if err != nil {
				t.Fatal(pattern, err)
			}
			var got [][]int
			err = re.scan(context.Background(), text, func(m []int) error { got = append(got, m); return nil })
			want := regexp.MustCompile(pattern).FindAllStringSubmatchIndex(text, -1)
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("pattern %q text %q: got %v (%v) want %v", pattern, text, got, err, want)
			}
		}
	}
}
func TestEditorRegexReplacementOracle(t *testing.T) {
	for _, pattern := range []string{`(?P<name>a)|(b)`, `(?P<x>a)|(?P<x>b)`, `(?P<01>a)`, `(a?)(b*)`, `(?m)^`, `\b`, `(?s).+`, `(?P<1234567890>a)`} {
		for _, text := range []string{"", "ab ba\na", "中a\xffb"} {
			for _, replacement := range []string{`$0`, `$1`, `${1}x`, `$1x`, `$$`, `$name/$2`, `${name}`, `$unknown`, `$`, `${}`, `${foo`, `$01`, `$1234567890`, `$x`, `[$0]$2$1`, "中\n${1}"} {
				got := computeEditorFind(context.Background(), editorFindRequest{text: text, query: pattern, replacement: replacement, options: EditorFindOptions{Regex: true, MatchCase: true}, replace: true})
				want := regexp.MustCompile(pattern).ReplaceAllString(text, replacement)
				if got.count == 0 && got.err == nil {
					got.text = text
				}
				if got.err != nil || got.text != want {
					t.Fatalf("%q %q %q: got %q err%v want%q", pattern, text, replacement, got.text, got.err, want)
				}
			}
		}
	}
}
func TestEditorRegexWholeWordAndSpans(t *testing.T) {
	for _, tc := range []struct {
		text, query string
		count       int
	}{{"ab ab", "a|ab", 2}, {"foo foo food _foo foo中 foo\u0301", "foo", 2}, {"中 中文 中", "中", 2}, {"ab\nab", "(?m)^ab$", 2}, {"a a", "\\Aa|a", 2}, {"a a", "\\Aa", 1}} {
		r := computeEditorFind(context.Background(), editorFindRequest{text: tc.text, query: tc.query, options: EditorFindOptions{Regex: true, MatchCase: true, WholeWord: true}})
		if r.err != nil || r.count != tc.count {
			t.Fatal(tc, r.err, r.count, r.spans)
		}
	}
	r := computeEditorFind(context.Background(), editorFindRequest{text: "x中\n🙂a\nz", query: "中\\n🙂", options: EditorFindOptions{Regex: true}})
	if r.err != nil || !reflect.DeepEqual(r.spans, []editorFindSpan{{1, 9, 1, 1}}) || !reflect.DeepEqual(r.matches, []findMatch{{0, 1, 1}}) {
		t.Fatal(r)
	}
}
func TestEditorRegexLimitsCancellationAndFullReplacement(t *testing.T) {
	for _, q := range []string{strings.Repeat("x", editorRegexQueryLimit+1), strings.Repeat("()", editorRegexCaptureLimit+1), "(?:" + strings.Repeat("x", 100) + "){1000}"} {
		r := computeEditorFind(context.Background(), editorFindRequest{text: "safe", query: q, options: EditorFindOptions{Regex: true}})
		if !errors.Is(r.err, ErrEditorRegexComplexity) {
			t.Fatal("missing complexity limit", r.err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if r := computeEditorFind(ctx, editorFindRequest{text: "a", query: "a", options: EditorFindOptions{Regex: true}}); !errors.Is(r.err, context.Canceled) {
		t.Fatal(r)
	}
	text := strings.Repeat("a ", editorFindHitLimit+2)
	req := editorFindRequest{text: text, query: "a", options: EditorFindOptions{Regex: true}}
	r := computeEditorFind(context.Background(), req)
	if r.err != nil || r.count != editorFindHitLimit+2 || len(r.matches) != editorFindHitLimit || len(r.spans) != editorFindHitLimit {
		t.Fatal("bounded retain", r.err, r.count, len(r.matches))
	}
	req.replace = true
	req.replacement = "中"
	r = computeEditorFind(context.Background(), req)
	if r.err != nil || r.text != strings.ReplaceAll(text, "a", "中") {
		t.Fatal("full replacement", r.err, r.count)
	}
	req.text = "a"
	req.query = "("
	r = computeEditorFind(context.Background(), req)
	if r.err == nil || r.text != "" || r.count != 0 {
		t.Fatal("partial result", r)
	}
}
func TestEditorRegexReaderCancellationNeverAcceptsFalseEOF(t *testing.T) {
	ctx := &cancelAfterContext{remaining: 3}
	re, err := compileEditorRegex(context.Background(), `a*$`, EditorFindOptions{Regex: true})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	err = re.scan(ctx, strings.Repeat("a", 100000), func([]int) error { count++; return nil })
	if !errors.Is(err, context.Canceled) || count != 0 {
		t.Fatal(err, count)
	}
}

type cancelAfterContext struct{ remaining int }

func (c *cancelAfterContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *cancelAfterContext) Done() <-chan struct{}       { return nil }
func (c *cancelAfterContext) Err() error {
	c.remaining--
	if c.remaining < 0 {
		return context.Canceled
	}
	return nil
}
func (c *cancelAfterContext) Value(any) any { return nil }

func TestEditorRegexGeneratedPatternOracle(t *testing.T) {
	rng := rand.New(rand.NewSource(912))
	atoms := []string{`a`, `b`, `中`, `\A`, `\z`, `\b`, `\B`, `(?m:^)`, `(?m:$)`, `(?s:.)`, `[ab]`, `(?:)`}
	texts := []string{"", "ab ab", "aaabbb", "中a中b\na", " a \nb\n", "\xffa\xfe\n"}
	for i := 0; i < 600; i++ {
		a, b := atoms[rng.Intn(len(atoms))], atoms[rng.Intn(len(atoms))]
		patterns := []string{"(" + a + ")?(" + b + ")*", "(?:" + a + "|" + b + ")+?", "(" + a + ")(" + b + ")|b", "(?:" + a + "|" + b + "){0,3}", "(?i:" + a + b + ")"}
		query := patterns[rng.Intn(len(patterns))]
		re, err := compileEditorRegex(context.Background(), query, EditorFindOptions{Regex: true, MatchCase: true})
		if err != nil {
			t.Fatal(err)
		}
		for _, text := range texts {
			var got [][]int
			err := re.scan(context.Background(), text, func(m []int) error { got = append(got, m); return nil })
			want := regexp.MustCompile(query).FindAllStringSubmatchIndex(text, -1)
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("%q %q: got%v want%v err%v", query, text, got, want, err)
			}
		}
	}
}
func TestEditorRegexSingleTargetAndTemplateLimits(t *testing.T) {
	req := editorFindRequest{text: "ab", query: "a", options: EditorFindOptions{Regex: true}, replace: true, one: true, target: editorFindSpan{start: 1, end: 2}}
	r := computeEditorFind(context.Background(), req)
	if !errors.Is(r.err, ErrEditorFindStaleMatch) || r.text != "" {
		t.Fatal(r)
	}
	req.one = false
	req.replacement = strings.Repeat("$", editorFindQueryLimit+1)
	r = computeEditorFind(context.Background(), req)
	if r.err == nil || r.text != "" || r.count != 0 {
		t.Fatal("template cap missing")
	}
}
func BenchmarkEditorRegex100K(b *testing.B) {
	req := editorFindRequest{text: strings.Repeat("foo line 00000000\n", 100000), query: `(?P<word>foo) line [0-9]+`, options: EditorFindOptions{Regex: true}}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := computeEditorFind(context.Background(), req)
		if r.err != nil || r.count != 100000 {
			b.Fatal(r.err, r.count)
		}
	}
}
