package gui

import (
	"math"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/uk0/silk/paint"
)

type labelPaintRecorder struct {
	*elideRecorder
	x, y      float64
	positions [][2]float64
	clip      [4]float64
	saves     int
	restores  int
	clips     int
}

func newLabelPaintRecorder() *labelPaintRecorder {
	return &labelPaintRecorder{elideRecorder: newElideRecorder()}
}
func (p *labelPaintRecorder) Save() int                    { p.saves++; return p.saves }
func (p *labelPaintRecorder) Restore() int                 { p.restores++; return p.saves - p.restores }
func (p *labelPaintRecorder) Clip()                        { p.clips++ }
func (p *labelPaintRecorder) Rectangle(x, y, w, h float64) { p.clip = [4]float64{x, y, w, h} }
func (p *labelPaintRecorder) Translate(x, y float64)       { p.x += x; p.y += y }
func (p *labelPaintRecorder) DrawText(s string) {
	p.texts = append(p.texts, s)
	p.positions = append(p.positions, [2]float64{p.x, p.y})
}

func TestLabelDrawExplicitLinesAndAlignment(t *testing.T) {
	for _, wrap := range []bool{false, true} {
		l := NewLabel("Git unavailable\r\n\nRetry after installing Git\rEnd")
		l.SetWrap(wrap)
		l.SetBounds(0, 0, 500, 200)
		for _, align := range []TextAlign{AlignLeft, AlignCenter, AlignRight} {
			l.SetAlign(align)
			p := newLabelPaintRecorder()
			l.Draw(p)
			if strings.Join(p.texts, "|") != "Git unavailable|Retry after installing Git|End" {
				t.Fatalf("draw did not split newlines: %q", p.texts)
			}
			if len(l.lines) != 4 || l.SizeHints().Height != 4*l.lineHeight {
				t.Fatalf("blank line or multiline hint lost: %+v", l.SizeHints())
			}
			for i, s := range p.texts {
				e := l.effectiveFont().TextExtents(s)
				left := p.positions[i][0] + e.XBearing
				want := 0.0
				if align == AlignCenter {
					want = (500 - e.Width) / 2
				}
				if align == AlignRight {
					want = 500 - e.Width
				}
				if math.Abs(left-want) > 1e-6 {
					t.Fatalf("alignment %v: got %v want %v", align, left, want)
				}
			}
			if p.positions[1][1]-p.positions[0][1] != 2*l.lineHeight || p.saves != 1 || p.restores != 1 || p.clips != 1 || p.clip != [4]float64{0, 0, 500, 200} {
				t.Fatalf("invalid line spacing or clip: %+v", p)
			}
		}
	}
}

func TestLabelWrapUnicodeResizeAndVisibleRows(t *testing.T) {
	text := strings.Repeat("中文 abcdefghijklmnop ", 80)
	l := NewLabel(text)
	l.SetWrap(true)
	l.SetBounds(0, 0, 96, 5000)
	l.SizeHints()
	var rebuilt strings.Builder
	for _, row := range l.lines {
		if !utf8.ValidString(row.text) || strings.ContainsAny(row.text, "\r\n") {
			t.Fatalf("invalid row %q", row.text)
		}
		rebuilt.WriteString(row.text)
		if row.ext.Width > 97 {
			t.Fatalf("row exceeds width: %q %.2f", row.text, row.ext.Width)
		}
	}
	if rebuilt.String() != text {
		t.Fatal("wrapping lost text")
	}
	narrow := len(l.lines)
	l.SetBounds(0, 0, 400, 25)
	p := newLabelPaintRecorder()
	l.Draw(p)
	if len(l.lines) >= narrow {
		t.Fatal("resize reused stale wrapping")
	}
	if len(p.texts) > int(math.Ceil(25/l.lineHeight)) || len(p.texts) == 0 {
		t.Fatalf("painted offscreen rows: %d", len(p.texts))
	}
	if p.positions[0][1] != l.lineAscent {
		t.Fatal("overflow hid the first line")
	}
	l.SetWrap(false)
	l.SizeHints()
	if len(l.lines) != 1 {
		t.Fatal("wrap toggle reused layout")
	}
	l.SetBounds(0, 0, 0, 0)
	p = newLabelPaintRecorder()
	l.Draw(p)
	if len(p.texts) != 0 || p.saves != 0 {
		t.Fatal("zero sized label painted")
	}
}

type labelCountingFont struct {
	paint.Font
	measurements int
}

func (f *labelCountingFont) TextExtents(s string) *paint.TextExtents {
	f.measurements++
	return f.Font.TextExtents(s)
}
func (f *labelCountingFont) FontExtents() *paint.FontExtents {
	f.measurements++
	return f.Font.FontExtents()
}

func TestLabelLayoutCacheInvalidation(t *testing.T) {
	f := &labelCountingFont{Font: Theme().Font.Dup()}
	l := NewLabel("first line\nsecond line long enough to wrap")
	l.SetFont(f)
	l.SetWrap(true)
	l.SetBounds(0, 0, 100, 200)
	p := newLabelPaintRecorder()
	l.Draw(p)
	n := f.measurements
	for i := 0; i < 100; i++ {
		p.texts = p.texts[:0]
		p.positions = p.positions[:0]
		l.Draw(p)
		l.SizeHints()
	}
	if f.measurements != n {
		t.Fatalf("repaint reshaped unchanged text: %d -> %d", n, f.measurements)
	}
	check := func(change func()) {
		t.Helper()
		n = f.measurements
		change()
		l.SizeHints()
		if f.measurements <= n {
			t.Fatal("stale layout cache")
		}
	}
	check(func() { l.SetText("changed\nlabel") })
	check(func() { l.SetBounds(0, 0, 50, 200) })
	check(func() { l.SetWrap(false) })
	check(func() { f.SetSize(f.Size() + 4) })
	check(func() { f.SetBold(!f.Bold()) })
	check(func() { f.SetItalic(!f.Italic()) })
	replacement := &labelCountingFont{Font: Theme().Font.Dup()}
	l.SetFont(replacement)
	l.SizeHints()
	if replacement.measurements == 0 || l.hintFont != replacement {
		t.Fatal("font replacement reused stale layout")
	}
	l.SetText("")
	if h := l.SizeHints(); h.Width != 0 || h.Height <= 0 {
		t.Fatalf("empty label hints: %+v", h)
	}
}

func BenchmarkLabelCachedDraw(b *testing.B) {
	l := NewLabel(strings.Repeat("Git is optional. 中文提示 ", 40))
	l.SetWrap(true)
	l.SetBounds(0, 0, 500, 120)
	p := newLabelPaintRecorder()
	l.Draw(p)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.texts = p.texts[:0]
		p.positions = p.positions[:0]
		l.Draw(p)
	}
}

func BenchmarkLabelWrapLayout(b *testing.B) {
	l := NewLabel(strings.Repeat("longtoken中文 ", 4096))
	l.SetWrap(true)
	l.SetBounds(0, 0, 160, 120)
	b.ReportAllocs()
	b.SetBytes(int64(len(l.text)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.hintsValid = false
		l.SizeHints()
	}
}
