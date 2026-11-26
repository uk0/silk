package gui

import (
	"strings"
	"testing"

	"github.com/uk0/silk/paint"
)

func BenchmarkSearchBoxHotPath(b *testing.B) {
	font := Theme().Font
	for _, tc := range []struct {
		name, text string
	}{
		{"short", "pressure-hi warning"},
		{"unicode", strings.Repeat("告警 valve 🟩 ", 16)},
	} {
		b.Run(tc.name+"/hit-test", func(b *testing.B) {
			x := font.TextExtents(tc.text).XAdvance + 1
			b.ReportAllocs()
			for b.Loop() {
				searchBoxCaretIndex(tc.text, font, x)
			}
		})
		b.Run(tc.name+"/paint-selection", func(b *testing.B) {
			s := NewSearchBox()
			s.SetSize(350, 30)
			s.SetText(tc.text)
			s.OnFocusIn()
			s.moveCaret(3, false)
			s.moveCaret(len([]rune(tc.text)), true)
			pix := paint.NewPixmap(350, 30)
			defer pix.Destroy()
			g := pix.NewPainter()
			b.ReportAllocs()
			for b.Loop() {
				s.Draw(g)
			}
		})
	}
}

func TestSearchBoxCaretPrefixesMatchRuneShaping(t *testing.T) {
	font := Theme().Font
	for _, text := range []string{"", "AV a ", "管道 🟢 ", "e\u0301 and 👨‍👩‍👧", "A\xff\xff中\xfd", "A\x00B"} {
		runes := []rune(text)
		advance := make([]float64, len(runes)+1)
		for n := 1; n <= len(runes); n++ {
			advance[n] = font.TextExtents(string(runes[:n])).XAdvance
		}
		for n := -1; n <= len(runes)+1; n++ {
			if got, want := searchBoxCaretX(text, font, n), advance[max(0, min(n, len(runes)))]; got != want {
				t.Fatalf("prefix %q %d: got %v, want %v", text, n, got, want)
			}
		}
		for n := 1; n <= len(runes); n++ {
			mid := (advance[n-1] + advance[n]) * 0.5
			for _, x := range []float64{mid - 0.01, mid, mid + 0.01, advance[n], advance[len(runes)] + 1} {
				want := 0
				if x > 0 {
					for want < len(runes) && x >= (advance[want]+advance[want+1])*0.5 {
						want++
					}
				}
				if got := searchBoxCaretIndex(text, font, x); got != want {
					t.Fatalf("hit %q at %v: got %d, want %d", text, x, got, want)
				}
			}
		}
	}
}
