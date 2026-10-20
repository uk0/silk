package gui

import (
	"math"
	"testing"
)

type shortcutTextOp struct {
	text string
	x, y float64
}
type shortcutRecorder struct {
	*focusRingRecorder
	x, y  float64
	texts []shortcutTextOp
}

func (r *shortcutRecorder) Translate(x, y float64) { r.x += x; r.y += y }
func (r *shortcutRecorder) DrawText(text string) {
	r.texts = append(r.texts, shortcutTextOp{text, r.x, r.y})
}

func TestPopupButtonShortcutColumn(t *testing.T) {
	menu := NewPopupMenu()
	b := menu.AddButton1("Save", nil)
	before := b.SizeHints()
	b.SetShortcutText("Ctrl+Shift+S")
	after := b.SizeHints()
	if after.Width <= before.Width || after.Height != before.Height {
		t.Fatalf("hint does not reserve its column: before=%+v after=%+v", before, after)
	}
	b.SetSize(after.Width, after.Height)
	r := &shortcutRecorder{focusRingRecorder: newFocusRingRecorder()}
	Theme().DrawButton(r, b)
	if len(r.texts) != 2 || r.texts[0].text != "Save" || r.texts[1].text != b.ShortcutText() {
		t.Fatalf("missing label/hint: %+v", r.texts)
	}
	font := Theme().Font
	label, hint := r.texts[0], r.texts[1]
	if label.x+font.TextExtents(label.text).XAdvance >= hint.x {
		t.Fatalf("shortcut overlaps label: %+v", r.texts)
	}
	if hint.x+font.TextExtents(hint.text).Width > after.Width || math.Abs(label.y-hint.y) > 1e-9 {
		t.Fatalf("hint escapes bounds/baseline: %+v", r.texts)
	}
	b.SetShortcutText("")
	if b.SizeHints() != before {
		t.Fatal("clearing shortcut leaves cached width")
	}
	b.Detach()
	b.SetShortcutText("Ctrl+S")
	r.texts = nil
	Theme().DrawButton(r, b)
	if len(r.texts) != 1 {
		t.Fatal("popup-only hint drawn on ordinary button")
	}
}
