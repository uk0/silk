package gui

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/uk0/silk/paint"
)

func TestTextBlockFallbackMatchesMeasuredAdvance(t *testing.T) {
	for _, text := range []string{"A中🙂B", "中🙂 apple🍎 后缀", "中文🚀abcdef", ""} {
		var b TextBlock
		b.SetFont(paint.NewFont(Theme().Font.Family(), 17, false, false))
		b.SetText(text)
		b.Layout(600)
		x, _ := b.PosToPoint(b.RunesCount())
		want := b.Font().TextExtents(text).XAdvance
		if math.Abs(x-want) > .001 {
			t.Errorf("%q caret advance=%g measured=%g", text, x, want)
		}
	}
}

func TestTextBlockFractionalHitTesting(t *testing.T) {
	var b TextBlock
	b.SetText(strings.Repeat("im", 40))
	b.Layout(1000)
	for pos := 1; pos < b.RunesCount(); pos++ {
		x, _ := b.PosToPoint(pos)
		if got := b.PointToPos(x, .5*b.RowHeight()); got != pos {
			t.Fatalf("caret %d x=%g hits %d", pos, x, got)
		}
	}
}

func TestTextBlockNewlineAndEmptyGeometry(t *testing.T) {
	for _, text := range []string{"", "a\n", "a\r", "a\r\n", "a\n\r", "\r\n\n", "first\nlast"} {
		var b TextBlock
		// Queries before the first layout must never index rows[-1].
		b.SetText(text)
		b.PosToPoint(100)
		b.PosToRowCol(100)
		b.PointToPos(100, 100)
		b.SetMultiLine(true)
		b.Layout(100)
		if len(b.rows) == 0 {
			t.Fatal("no empty row")
		}
		for i := range b.rows {
			row := &b.rows[i]
			pos := b.PointToPos(1e9, (float64(i)+.5)*b.RowHeight())
			if pos != row.begin+len(row.text) {
				t.Fatalf("%q row %d far-right click jumped line: %d", text, i, pos)
			}
			if row.caretX(100000) > 1000 {
				t.Fatal("synthetic newline polluted geometry")
			}
		}
		if strings.HasSuffix(text, "\r") || strings.HasSuffix(text, "\n") {
			last := b.rows[len(b.rows)-1]
			if len(last.text) != 0 || last.begin != b.RunesCount() {
				t.Fatalf("terminal newline lost final empty row: %q %+v", text, last)
			}
		}
	}
}

func TestTextBlockWrappedFontRunsAndCaret(t *testing.T) {
	var b TextBlock
	b.SetWrap(true)
	b.SetText(strings.Repeat("中🙂 a🍎", 64))
	for _, width := range []float64{1, 32, 180} {
		b.Layout(width)
		end := 0
		for i := range b.rows {
			row := &b.rows[i]
			if row.begin != end || len(row.carets) != len(row.text)+1 {
				t.Fatal("wrap gap")
			}
			count := 0
			for _, run := range row.runs {
				count += len(run.Glyphs)
				for _, g := range run.Glyphs {
					if g.A != 0 {
						t.Fatal("glyph ABI overwritten")
					}
				}
			}
			if count != len(row.text) {
				t.Fatal("font run dropped or repeated while wrapping")
			}
			for c := 0; c < len(row.text); c++ {
				x := row.caretX(c)
				if got := b.PointToPos(x, (float64(i)+.5)*b.RowHeight()); got != row.begin+c {
					t.Fatalf("wrapped hit=%d want=%d", got, row.begin+c)
				}
			}
			end = row.end
		}
		if end != len(b.text) {
			t.Fatal("lost sentinel")
		}
	}
}

func TestEditFallbackInputMethodCaretMatchesLayout(t *testing.T) {
	e := NewEdit()
	e.SetSize(120, 36)
	e.SetText("中🙂 apple🍎 后缀")
	for pos := 0; pos <= e.RunesCount(); pos++ {
		e.SetCaretPos(pos)
		x, _ := e.PosToPoint(pos)
		r := e.InputMethodCursorRect()
		if math.Abs(r.X-(e.padding.L+x-e.ScrollX())) > .001 {
			t.Fatalf("IME detached at %d: %+v x=%g", pos, r, x)
		}
		assertEditCaretVisible(t, e)
	}
}

func BenchmarkTextBlockFallbackWrap(b *testing.B) {
	for _, n := range []int{1000, 10000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			var block TextBlock
			block.SetWrap(true)
			block.SetText(strings.Repeat("中a🙂", n))
			block.Layout(32)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				block.Layout(32)
			}
		})
	}
}

func BenchmarkEditFallbackRetainedDraw(b *testing.B) {
	e := NewEdit()
	e.SetSize(600, 36)
	e.SetText("中🙂 apple🍎 后缀")
	p := paint.NewPixmap(600, 36)
	defer paint.CloseSurface(p)
	g := p.NewPainter()
	defer paint.ClosePainter(g)
	e.Draw(g)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.Save()
		e.Draw(g)
		g.Restore()
	}
}

func TestTextBlockRetainedRowPixels(t *testing.T) {
	for _, scale := range []float64{1, 1.5, 2} {
		for _, width := range []float64{35, 260} {
			var block TextBlock
			block.SetWrap(true)
			block.SetText("A中🙂B apple🍎 后缀")
			block.Layout(width)
			p, ref := paint.NewPixmap(600, 600), paint.NewPixmap(600, 600)
			g, want := p.NewPainter(), ref.NewPainter()
			for _, pen := range []paint.Painter{g, want} {
				pen.Scale(scale, scale)
				pen.SetFont(block.Font())
				pen.SetBrush1(paint.Color{A: 255})
			}
			for i := range block.rows {
				row := &block.rows[i]
				baseline := float64(i)*block.RowHeight() + block.fontExtents().Ascent
				row.draw(g, baseline)
				want.DrawText1(0, baseline, string(row.text))
			}
			p.Flush()
			ref.Flush()
			a, err := p.Image()
			if err != nil {
				t.Fatal(err)
			}
			b, err := ref.Image()
			if err != nil {
				t.Fatal(err)
			}
			different := false
			for y := 0; y < 600 && !different; y++ {
				for x := 0; x < 600; x++ {
					if a.At(x, y) != b.At(x, y) {
						different = true
						break
					}
				}
			}
			paint.ClosePainter(g)
			paint.ClosePainter(want)
			paint.CloseSurface(p)
			paint.CloseSurface(ref)
			if different {
				t.Fatalf("wrapped retained pixels differ: width=%g scale=%g", width, scale)
			}
		}
	}
}
