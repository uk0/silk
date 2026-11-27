package gui

import (
	"math"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/uk0/silk/paint"
)

func TestSignatureTooltipMapsWhitespaceAndRejectsSplitUnicode(t *testing.T) {
	input := "𐐀f(a\r\n\t名称 string)"
	at := strings.Index(input, "名称")
	part := "名称 string"
	text, pos := normalizeSignatureTip(input, signatureTipSpan{at, at + len(part)})
	if text != "𐐀f(a\n    名称 string)" || text[pos.start:pos.end] != part {
		t.Fatalf("text=%q span=%v", text, pos)
	}
	_, invalid := normalizeSignatureTip(input, signatureTipSpan{1, at})
	if invalid != (signatureTipSpan{}) {
		t.Fatal("mid-rune boundary was accepted")
	}
}

func TestSignatureTooltipWrapsAndKeepsActiveParameter(t *testing.T) {
	input := "func 𐐀f(" + strings.Repeat("a int, ", 55) + "目标 string)"
	at := strings.Index(input, "目标")
	l := layoutSignatureTip(input, signatureTipSpan{at, len(input) - 1}, 125, 70)
	if l.size.Width > 125 || l.size.Height > 70 || len(l.rows) > 4 {
		t.Fatalf("layout exceeds owner: %+v", l.size)
	}
	found := false
	for _, row := range l.rows {
		if !utf8.ValidString(row.text) {
			t.Fatal("wrap split a UTF-8 rune")
		}
		if row.emphasisW > 0 && strings.Contains(row.text, "目标") {
			found = true
		}
	}
	if !found {
		t.Fatalf("active parameter scrolled out of hint: %+v", l.rows)
	}
}

func TestSignatureTooltipPaintEmphasisOnlyOnActiveParameter(t *testing.T) {
	text := "f(count int, label string)"
	begin := strings.Index(text, "label")
	l := layoutSignatureTip(text, signatureTipSpan{begin, len(text) - 1}, 360, 240)
	if len(l.rows) != 1 || l.rows[0].emphasisW <= 0 {
		t.Fatalf("even short signature did not produce one highlighted row: %+v", l.rows)
	}
	px := paint.NewPixmap(int(math.Ceil(l.size.Width)), int(math.Ceil(l.size.Height)))
	defer paint.CloseSurface(px)
	g := px.NewPainter()
	defer paint.ClosePainter(g)
	l.draw(g)
	px.Flush()
	img, err := px.Image()
	if err != nil {
		t.Fatal(err)
	}
	shape := l.rows[0]
	marker := func(x, y int) bool {
		r, g, b, a := img.At(x, y).RGBA()
		return r>>8 == 34 && g>>8 == 82 && b>>8 == 130 && a == 65535
	}
	x := int(math.Ceil(signatureTipPadX + shape.emphasisX + 2))
	if !marker(x, 5) || marker(10, 5) {
		t.Fatal("accent not confined to active parameter")
	}
}

func TestClosingArgumentListRetiresOnlyEditorHint(t *testing.T) {
	root, win, ed := editorToolTipFixture(t)
	t.Cleanup(HideToolTip)
	ed.SetText("f()")
	ed.ScrollToLineCol(0, 2)
	replaceExplicitToolTip(newOwnedToolTip(hoverToolTipAnchor{win, 50, 48}, ed, "f(value string)"))
	ed.signatureHintVisible = true
	ed.OnTextInput(")")
	if ed.signatureHintVisible || IsToolTipVisible() || ed.Text() != "f()" {
		t.Fatal("overtyping a closing paren retained the hint or edited an existing closer")
	}
	other := testTooltipWidget()
	other.SetParent(root)
	replaceExplicitToolTip(newOwnedToolTip(hoverToolTipAnchor{win, 57, 50}, other, "another owner"))
	ed.OnTextInput(")")
	if currentTooltip.Load() == nil || currentTooltip.Load().owner != other {
		t.Fatal("closing the editor signature hid a different control's tooltip")
	}
}
