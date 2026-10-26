package gui

import (
	"github.com/uk0/silk/paint"
	"image"
	"math"
	"strings"
	"testing"
	"unicode/utf8"
)

func assertEditCaretVisible(t *testing.T, e *Edit) {
	t.Helper()
	x, _ := e.PosToPoint(e.CaretPos())
	w, _ := e.ViewportSize()
	sx := e.ScrollX()
	if sx < 0 || x < sx-.01 || x+2 > sx+w+.01 {
		t.Fatalf("caret=%g scroll=%g viewport=%g text=%q", x, sx, w, e.Text())
	}
	if !e.ml && e.ScrollY() != 0 {
		t.Fatal("single line scrolled vertically")
	}
}

func TestEditHorizontalCaretReveal(t *testing.T) {
	for _, text := range []string{"66.12345678901234", strings.Repeat("中文🙂abcdef", 4)} {
		t.Run(text, func(t *testing.T) {
			e := NewEdit()
			e.SetSize(78, 30)
			e.SetText(text)
			e.OnKeyDown(KeyEnd, false)
			assertEditCaretVisible(t, e)
			if e.ScrollX() == 0 || e.HorzScrollBar().IsVisible() {
				t.Fatal("single-line scroll range missing or scrollbar visible")
			}
			carets := &e.rows[0].carets[0]
			for i := e.RunesCount(); i >= 0; i-- {
				e.SetCaretPos(i)
				assertEditCaretVisible(t, e)
			}
			if e.ScrollX() != 0 {
				t.Fatal("Home did not reset scrolling")
			}
			if carets != &e.rows[0].carets[0] {
				t.Fatal("caret navigation reshaped text")
			}
			e.SetSelection(0, e.RunesCount())
			assertEditCaretVisible(t, e)
			e.OnTextInput("abcdefghijklmnopqrst")
			assertEditCaretVisible(t, e)
			e.moveCaret(e.CaretPos()-3, true)
			assertEditCaretVisible(t, e)
			e.pasteString("中文🙂abcdefghijk")
			assertEditCaretVisible(t, e)
			e.SetReadOnly(true)
			e.OnKeyDown(KeyHome, false)
			assertEditCaretVisible(t, e)
			e.OnKeyDown(KeyEnd, false)
			assertEditCaretVisible(t, e)
			e.SetReadOnly(false)
			e.SetSize(600, 30)
			if e.ScrollX() != 0 {
				t.Fatal("widening did not reset scroll")
			}
			assertEditCaretVisible(t, e)
			e.SetSize(60, 30)
			assertEditCaretVisible(t, e)
			e.SetFont(paint.NewFont(e.Font().Family(), 20, false, false))
			assertEditCaretVisible(t, e)
			e.SetText("x")
			assertEditCaretVisible(t, e)
			if e.CaretPos() != 1 || e.ScrollX() != 0 {
				t.Fatal("shorter value left stale selection or scroll")
			}
			e.SelectAll()
			e.OnKeyDown(KeyBackSpace, false)
			assertEditCaretVisible(t, e)
			if e.ScrollX() != 0 || e.Text() != "" {
				t.Fatal("delete did not reset scrolling")
			}
		})
	}
}

func TestEditHorizontalScrollMouseHitAndLayout(t *testing.T) {
	e := NewEdit()
	e.SetSize(85, 30)
	e.SetText(strings.Repeat("abcdef", 10))
	e.SetCaretPos(e.RunesCount())
	assertEditCaretVisible(t, e)
	sx := e.ScrollX()
	x := e.padding.L + 20
	y := e.padding.T + 2
	want := e.PointToPos(x+sx-e.padding.L, 2)
	e.OnLeftDown(x, y)
	e.OnLeftUp(x, y)
	if e.CaretPos() != want {
		t.Fatalf("mouse hit=%d want=%d", e.CaretPos(), want)
	}
	e.SetSize(0, 0)
	e.SetText("")
	e.SetCaretPos(999)
	if math.IsNaN(e.ScrollX()) || math.IsInf(e.ScrollX(), 0) {
		t.Fatal("degenerate viewport")
	}
	e.SetSize(85, 50)
	e.SetMultiLine(true)
	e.SetWrap(false)
	e.SetText("short\n" + strings.Repeat("long", 30))
	e.SetCaretPos(e.RunesCount())
	assertEditCaretVisible(t, e)
	e.SetCaretPos(0)
	assertEditCaretVisible(t, e)
	e.SetWrap(true)
	e.SetCaretPos(e.RunesCount())
	assertEditCaretVisible(t, e)
}

func TestEditValidatorUsesRuneSelection(t *testing.T) {
	for _, paste := range []bool{false, true} {
		e := NewEdit()
		e.SetSize(80, 30)
		e.SetText("中文🙂abcdef")
		e.SetValidator(validUTF8Validator{})
		e.SetSelection(1, 3)
		if paste {
			e.pasteString("新🙂")
		} else {
			e.OnTextInput("新🙂")
		}
		if e.Text() != "中新🙂abcdef" || !e.IsValid() {
			t.Fatalf("rune replacement rejected or corrupted: %q", e.Text())
		}
		assertEditCaretVisible(t, e)
	}
}

// Invalid, not Intermediate: a byte-sliced candidate used to reject valid
// Chinese/emoji replacements even though TextBlock.Replace is rune-safe.
type validUTF8Validator struct{}

func (validUTF8Validator) Validate(s string) State {
	if utf8.ValidString(s) {
		return Acceptable
	}
	return Invalid
}

func TestEditHorizontalDrawDoesNotCoverPadding(t *testing.T) {
	old := CurrentThemeMode()
	t.Cleanup(func() { SetThemeMode(old) })
	for _, mode := range []ThemeMode{ThemeDark, ThemeLight} {
		SetThemeMode(mode)
		e := NewEdit()
		e.SetSize(90, 32)
		e.SetText(strings.Repeat("WW中文", 20))
		e.SetFocus()
		e.SelectAll()
		render := func() image.Image {
			p := paint.NewPixmap(90, 32)
			g := p.NewPainter()
			e.Draw(g)
			p.Flush()
			img, err := p.Image()
			if err != nil {
				t.Fatal(err)
			}
			return img
		}
		full := render()
		e.SetText("")
		empty := render()
		for y := 0; y < 32; y++ {
			for x := 0; x < 90; x++ {
				if x >= int(e.padding.L) && x < 90-int(e.padding.R) {
					continue
				}
				if full.At(x, y) != empty.At(x, y) {
					t.Fatalf("scrolled glyph/selection painted padding at %d,%d", x, y)
				}
			}
		}
	}
}

func BenchmarkEditHorizontalCaret(b *testing.B) {
	e := NewEdit()
	e.SetSize(90, 32)
	e.SetText(strings.Repeat("中abcdef", 2000))
	n := e.RunesCount()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.SetCaretPos(i % n)
	}
}
