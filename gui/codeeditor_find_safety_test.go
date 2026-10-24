package gui

import (
	"strings"
	"testing"
)

func TestFindReplaceDoesNotUseStaleDocumentRanges(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("foo safe")
	e.findActive = true
	e.findText = "foo"
	e.replaceText = "X"
	e.findUpdateMatches()
	e.ReplaceAllText("bar safe")
	e.replaceCurrent()
	if e.Text() != "bar safe" {
		t.Fatalf("stale search result replaced unrelated text: %q", e.Text())
	}
}

func BenchmarkFindLargeLiteral(b *testing.B) {
	text := strings.Repeat("foo line 00000000\n", 100000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if len(findMatches(text, "foo", false)) != 100000 {
			b.Fatal("bad match count")
		}
	}
}
