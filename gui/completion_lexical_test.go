package gui

import (
	"context"
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"
	"unsafe"
)

func completionLexicalFixture(n int) string {
	var s strings.Builder
	s.WriteString("package main\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&s, "func value%05d(x int) int { return x + %d }\n", i, i)
	}
	s.WriteString("func Edit(){ _ = \"alpha beta gamma delta\" }\n")
	return s.String()
}

func TestCompletionLexicalOwnedWordsAndRetiredTail(t *testing.T) {
	var p completionLexicalIndex
	text := completionLexicalFixture(500)
	checkCompletionLexical(t, &p, text, "plain")
	base := uintptr(unsafe.Pointer(unsafe.StringData(text)))
	for _, chunk := range p.chunks {
		for _, word := range chunk.words {
			ptr := uintptr(unsafe.Pointer(unsafe.StringData(word)))
			if ptr >= base && ptr < base+uintptr(len(text)) {
				t.Fatal("identifier pins document backing storage")
			}
		}
	}
	old := p.items
	checkCompletionLexical(t, &p, "", "plain")
	for _, item := range old {
		if item.Text != "" {
			t.Fatal("spare candidate capacity pins retired strings")
		}
	}
	for _, language := range []string{"plain", "go", "json", "plain"} {
		for _, text := range []string{"", "x", " 123 ", "alpha", "", "名"} {
			checkCompletionLexical(t, &p, text, language)
		}
	}
	p.reset()
	if p.valid || p.chunks != nil || p.items != nil || p.seen != nil || p.text != "" {
		t.Fatal("reset retained payload")
	}
}

func checkCompletionLexical(t *testing.T, p *completionLexicalIndex, text, language string) {
	t.Helper()
	got, err := p.scan(context.Background(), text, language)
	want, wantErr := scanCompletionItemsForLanguage(context.Background(), text, language)
	if err != nil || wantErr != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("lexical candidate mismatch language=%q bytes=%d got=%d want=%d err=%v", language, len(text), len(got), len(want), err)
	}
}

func TestCompletionLexicalRepeatedEditsMatchFullScanner(t *testing.T) {
	rng := rand.New(rand.NewSource(85811))
	text := completionLexicalFixture(1100)
	var p completionLexicalIndex
	words := []string{"alpha", "新名称", "", "123_thing", "\xff", "\xe4", "\xb8\xad", "\r\n", "\n", " world ", " ", "\x00", "😀"}
	for i := 0; i < 400; i++ {
		checkCompletionLexical(t, &p, text, []string{"go", "json", "plain"}[i%3])
		a := rng.Intn(len(text) + 1)
		b := min(len(text), a+rng.Intn(12))
		text = text[:a] + words[rng.Intn(len(words))] + text[b:]
	}
}

func TestCompletionLexicalSeamsAndDuplicateOrder(t *testing.T) {
	for _, text := range []string{
		strings.Repeat(" ", completionLexicalChunkBytes-1) + "\xe4 aa tail",
		strings.Repeat("alpha ", 2000) + "世界 名称\r\nlast __123 1name",
		"alpha beta alpha gamma beta delta\xffomega 名称 𐐀x",
		"", strings.Repeat("x", completionLexicalChunkBytes*3),
	} {
		var p completionLexicalIndex
		for _, pos := range []int{0, 1, completionLexicalChunkBytes - 2, completionLexicalChunkBytes - 1, completionLexicalChunkBytes, completionLexicalChunkBytes + 1, len(text)} {
			pos = min(pos, len(text))
			for _, v := range []string{"", "\xb8\xad", "\xe4", "word", "\n", "_123名", "\x80", " "} {
				checkCompletionLexical(t, &p, text, "go")
				checkCompletionLexical(t, &p, text[:pos]+v+text[pos:], "go")
				if pos < len(text) {
					checkCompletionLexical(t, &p, text[:pos]+v+text[pos+1:], "go")
				}
			}
		}
	}
}

func TestCompletionLexicalBudgetsAndCancellation(t *testing.T) {
	var p completionLexicalIndex
	checkCompletionLexical(t, &p, strings.Repeat("word ", completionLexicalMaxBytes/5+1), "go")
	if p.valid || p.text != "" || p.chunks != nil {
		t.Fatal("oversized source retained")
	}
	var s strings.Builder
	for i := 0; i <= completionLexicalMaxWords; i++ {
		fmt.Fprintf(&s, "v%d ", i)
	}
	checkCompletionLexical(t, &p, s.String(), "json")
	if p.valid || p.text != "" || p.chunks != nil || p.disabledAbove == 0 {
		t.Fatal("word budget did not retire sidecar")
	}
	checkCompletionLexical(t, &p, s.String()+" extra", "go")
	checkCompletionLexical(t, &p, "alpha beta", "go")
	if !p.valid || p.disabledAbove != 0 {
		t.Fatal("shrunk input did not recover")
	}
	for _, after := range []int{1, 2, 3, 4, 5, 12, 25, 50} {
		p.reset()
		checkCompletionLexical(t, &p, completionLexicalFixture(1300), "go")
		ctx := &completionCancelAfter{Context: context.Background(), after: after}
		_, _ = p.scan(ctx, "prefix "+completionLexicalFixture(1300), "go")
		checkCompletionLexical(t, &p, "new "+completionLexicalFixture(1200), "go")
	}
}

func FuzzCompletionLexicalEdits(f *testing.F) {
	for _, s := range [][]byte{{0, 1, 2, 3, 4, 5}, {1, 255, 0, 128, 0, 0}, {2, 3, 4, 5, 6, 7, 8, 9}} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, script []byte) {
		if len(script) == 0 || len(script) > 128 {
			return
		}
		text := completionLexicalFixture(300)
		var p completionLexicalIndex
		for i := 0; i+3 < len(script); i += 4 {
			checkCompletionLexical(t, &p, text, []string{"go", "json", "text"}[int(script[i])%3])
			a := (int(script[i+1])<<8 | int(script[i+2])) % (len(text) + 1)
			b := min(len(text), a+int(script[i+3])%16)
			v := []string{"", "alpha", "\xe4", "\xb8\xad", "世界", "123_thing", "\n", "\xff", "x", " "}[int(script[i])%10]
			text = text[:a] + v + text[b:]
		}
		checkCompletionLexical(t, &p, text, "go")
	})
}

func BenchmarkCompletionLexicalBodyEdits(b *testing.B) {
	a := completionLexicalFixture(5000)
	c := a[:len(a)-3] + "extra " + a[len(a)-3:]
	b.Run("full", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			scanCompletionItemsForLanguage(context.Background(), a, "go")
			scanCompletionItemsForLanguage(context.Background(), c, "go")
		}
	})
	b.Run("chunks", func(b *testing.B) {
		var p completionLexicalIndex
		p.scan(context.Background(), a, "go")
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			p.scan(context.Background(), a, "go")
			p.scan(context.Background(), c, "go")
		}
	})
}
