package gui

import (
	"context"
	"errors"
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

func TestEditorFindEngineUnicodeAndBytes(t *testing.T) {
	cases := []struct {
		text, query string
		opts        EditorFindOptions
		want        []findMatch
	}{
		{"Foo foo FOO", "foo", EditorFindOptions{}, []findMatch{{0, 0, 3}, {0, 4, 7}, {0, 8, 11}}},
		{"Foo foo FOO", "foo", EditorFindOptions{MatchCase: true}, []findMatch{{0, 4, 7}}},
		{"猫foo foo foo_ foo2 _foo foo猫 foo\u0301 foo!", "foo", EditorFindOptions{WholeWord: true}, []findMatch{{0, 5, 8}, {0, 34, 37}}},
		{"Σσς Sſs KKk", "σ", EditorFindOptions{}, []findMatch{{0, 0, 1}, {0, 1, 2}, {0, 2, 3}}},
		{"Sſs", "s", EditorFindOptions{}, []findMatch{{0, 0, 1}, {0, 1, 2}, {0, 2, 3}}},
		{"KKk", "k", EditorFindOptions{}, []findMatch{{0, 0, 1}, {0, 1, 2}, {0, 2, 3}}},
		{"界foo\n🙂FOO", "foo", EditorFindOptions{}, []findMatch{{0, 1, 4}, {1, 1, 4}}},
		{"\xff�\xfe", "�", EditorFindOptions{}, []findMatch{{0, 1, 2}}},
		{"\xff�\xfe", "\xfe", EditorFindOptions{}, []findMatch{{0, 2, 3}}},
		{"aaaaa", "aa", EditorFindOptions{}, []findMatch{{0, 0, 2}, {0, 2, 4}}},
	}
	for _, tt := range cases {
		t.Run(tt.text+tt.query, func(t *testing.T) {
			r := computeEditorFind(context.Background(), editorFindRequest{text: tt.text, query: tt.query, options: tt.opts})
			if r.err != nil || !reflect.DeepEqual(r.matches, tt.want) {
				t.Fatalf("got %v err=%v want %v", r.matches, r.err, tt.want)
			}
		})
	}
}
func TestEditorFindEngineMatchesNaiveUnicodeOracle(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	alphabet := []rune("abABΣσςSſsKKk界🙂 _\n")
	for n := 0; n < 1000; n++ {
		runes := make([]rune, rng.Intn(120))
		for i := range runes {
			runes[i] = alphabet[rng.Intn(len(alphabet))]
		}
		qr := make([]rune, rng.Intn(6)+1)
		for i := range qr {
			qr[i] = alphabet[rng.Intn(len(alphabet)-1)]
		}
		query := string(qr)
		opts := EditorFindOptions{MatchCase: rng.Intn(2) == 0, WholeWord: rng.Intn(2) == 0}
		var want []findMatch
		for line, text := range strings.Split(string(runes), "\n") {
			rr := []rune(text)
			for c := 0; c+len(qr) <= len(rr); c++ {
				chunk := string(rr[c : c+len(qr)])
				equal := chunk == query
				if !opts.MatchCase {
					equal = strings.EqualFold(chunk, query)
				}
				if !equal || (opts.WholeWord && ((c > 0 && findWordRune(rr[c-1])) || (c+len(qr) < len(rr) && findWordRune(rr[c+len(qr)])))) {
					continue
				}
				want = append(want, findMatch{line, c, c + len(qr)})
				c += len(qr) - 1
			}
		}
		got := computeEditorFind(context.Background(), editorFindRequest{text: string(runes), query: query, options: opts})
		if got.err != nil || !reflect.DeepEqual(got.matches, want) {
			t.Fatalf("%q query %q opts%v got%v want%v err%v", string(runes), query, opts, got.matches, want, got.err)
		}
	}
}
func TestEditorFindEngineLimitsAndAtomicReplacement(t *testing.T) {
	ctx := context.Background()
	r := computeEditorFind(ctx, editorFindRequest{text: strings.Repeat("a ", editorFindHitLimit+3), query: "a"})
	if r.count != editorFindHitLimit+3 || len(r.matches) != editorFindHitLimit {
		t.Fatal(r.count, len(r.matches))
	}
	r = computeEditorFind(ctx, editorFindRequest{text: strings.Repeat("a ", editorFindHitLimit+3), query: "a", replacement: "🙂", replace: true})
	if r.err != nil || r.count != editorFindHitLimit+3 || r.text != strings.Repeat("🙂 ", editorFindHitLimit+3) {
		t.Fatal(r.err, r.count)
	}
	r = computeEditorFind(ctx, editorFindRequest{text: "\xffΣ界ς\xfe", query: "σ", replacement: "🙂\n", replace: true})
	if r.err != nil || r.text != "\xff🙂\n界🙂\n\xfe" || r.count != 2 {
		t.Fatal(r.err, r.text, r.count)
	}
	for _, q := range []string{"\n", "a\rb", strings.Repeat("a", editorFindQueryLimit+1)} {
		r = computeEditorFind(ctx, editorFindRequest{text: "abc", query: q, replace: true})
		if r.err == nil || r.count != 0 || r.text != "" {
			t.Fatal("invalid query accepted")
		}
	}
	// Refuse before copying a too-large replacement. One allocation, no partial result.
	r = computeEditorFind(ctx, editorFindRequest{text: "a", query: "a", replacement: strings.Repeat("x", editorFindReplacementLimit+1), replace: true})
	if !errors.Is(r.err, ErrEditorFindReplacementTooLarge) || r.text != "" || r.count != 0 {
		t.Fatal(r.err, r.count)
	}
	c, cancel := context.WithCancel(ctx)
	cancel()
	r = computeEditorFind(c, editorFindRequest{text: "abc", query: "a"})
	if !errors.Is(r.err, context.Canceled) {
		t.Fatal(r.err)
	}
}
