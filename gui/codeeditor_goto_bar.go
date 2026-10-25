package gui

import (
	"fmt"
	"strings"

	"github.com/uk0/silk/paint"
)

type editorGotoBar struct {
	Widget
	editor        *CodeEditor
	query         *editorGotoEdit
	accept, close *editorGotoButton
	status        *Label
	tipsLive      bool
}
type editorGotoEdit struct {
	Edit
	bar *editorGotoBar
}
type editorGotoButton struct {
	Button
	bar *editorGotoBar
}

// Auxiliary input must not let a host Undo/format/rename mutate the source
// document behind it. Ordinary field clipboard/navigation stays in Edit.
func gotoOwnsDocumentShortcut(key int, mods uint8) bool {
	action := mods&ModAction != 0
	if action {
		switch key {
		case 'Z', 'Y', 'I', 'J', 'K', 'D', '.':
			return true
		}
	}
	return key == KeyF2 || key == KeyF12 || key == KeyEnter && mods&ModAlt != 0
}
func (f *editorGotoEdit) HandleShortcut(key int, mods uint8) bool {
	if (mods&ModAction != 0) && key == 'G' {
		f.bar.editor.ShowGoToLine()
		return true
	}
	return gotoOwnsDocumentShortcut(key, mods)
}
func (b *editorGotoButton) HandleShortcut(key int, mods uint8) bool {
	return b.bar.query.HandleShortcut(key, mods)
}
func (f *editorGotoEdit) OnKeyDown(key int, repeat bool) {
	switch key {
	case KeyEsc:
		f.bar.editor.HideGoToLine()
		return
	case KeyEnter:
		f.bar.editor.gotoLineAccept()
		return
	}
	f.Edit.OnKeyDown(key, repeat)
}
func (b *editorGotoButton) OnKeyDown(key int, repeat bool) {
	if key == KeyEsc {
		b.bar.editor.HideGoToLine()
		return
	}
	b.Button.OnKeyDown(key, repeat)
}
func (e *CodeEditor) ensureGoToBar() {
	if e.gotoBar != nil {
		return
	}
	b := &editorGotoBar{editor: e}
	b.Init(b)
	b.SetParent(e.Self())
	e.gotoBar = b
	b.query = &editorGotoEdit{bar: b}
	b.query.Init(b.query)
	b.query.SetParent(b)
	b.query.SetFont(paint.NewFont(defaultFontFamily(), 12, false, false))
	b.query.SetPadding(Padding{L: 5, R: 5, T: 2, B: 2})
	b.query.SetMaxLength(64)
	b.query.SetValidator(ValidatorFunc(func(s string) error {
		_, err := e.ResolveGoToLocation(s)
		return err
	}))
	button := func(label, icon string, fn func()) *editorGotoButton {
		v := &editorGotoButton{bar: b}
		v.Init(v)
		v.SetParent(b)
		v.SetText(label)
		v.SetIconName(icon)
		v.Action().BindFunc0(fn)
		return v
	}
	b.accept = button("Go", "navigate-forward", e.gotoLineAccept)
	b.accept.SetTextVisible(true)
	b.close = button("Close", "close", e.HideGoToLine)
	b.close.SetTextVisible(false)
	b.status = NewLabel("")
	b.status.SetParent(b)
	b.status.SetFocusPolicy(NoFocus)
	b.status.SetFont(paint.NewFont(defaultFontFamily(), 11, false, false))
	b.query.SigTextChanged(func(_ interface{}, s string) { e.gotoLineText = s; e.syncGoToBar() })
}
func (e *CodeEditor) syncGoToBar() {
	b := e.gotoBar
	if b == nil || !e.gotoLineActive {
		return
	}
	p, err := e.ResolveGoToLocation(b.query.Text())
	status := fmt.Sprintf("Line %d, column %d", p.Line+1, p.RuneColumn+1)
	if err != nil {
		status = strings.TrimPrefix(err.Error(), ErrEditorLocation.Error()+": ")
	}
	if b.query.Text() == "" {
		status = fmt.Sprintf("line[:column], lines 1-%d", len(e.lines))
	}
	b.status.SetText(status)
	b.query.revalidate()
	b.accept.SetEnabled(err == nil)
	b.accept.Action().SetEnabled(err == nil)
	b.SetVisible(true)
	b.tooltips(e.findVisible() && e.Window() != nil)
	if b.tipsLive {
		SetToolTip(b.status, status)
	}
	b.Update()
}
func (b *editorGotoBar) tooltips(live bool) {
	if b.tipsLive == live {
		return
	}
	b.tipsLive = live
	for w, s := range map[IWidget]string{b.query: "One-based line or line:column; columns count Unicode code points, not visual tab cells or UTF-16 units.", b.accept: "Go to the validated location (Enter)", b.close: "Cancel without moving the caret (Esc)", b.status: b.status.Text()} {
		if !live {
			s = ""
		}
		SetToolTip(w, s)
	}
}
func (b *editorGotoBar) Draw(g paint.Painter) {
	pal := b.editor.editorColors()
	w, h := b.Size()
	g.SetBrush1(pal.gutter)
	g.Rectangle(0, 0, w, h)
	g.Fill()
	g.SetPen1(pal.gutterBorder, 1)
	g.Line(0, h-1, w, h-1)
	g.Stroke()
}
func (e *CodeEditor) layoutGoToBar() {
	b := e.gotoBar
	if b == nil || !e.gotoLineActive {
		return
	}
	w, h := e.Size()
	w = max(0, w)
	top := 0.0
	if e.findActive {
		top = e.findBarHeight
	}
	row := max(30, b.query.SizeHints().Height+6, b.accept.SizeHints().Height+6, b.close.SizeHints().Height+6)
	gw, cw := max(48, b.accept.SizeHints().Width), max(24, b.close.SizeHints().Width)
	rows := 2.0
	if w >= 560 {
		rows = 1
	} else if w < gw+cw+120 {
		rows = 3
	}
	e.gotoBarHeight = min(rows*row, max(0, h-top-e.statusBarHeight-e.breadcrumbHeight))
	b.SetBounds(0, top, w, e.gotoBarHeight)
	place := func(v IWidget, x, y, vw float64) {
		x = max(0, min(x, w))
		y = max(0, min(y, e.gotoBarHeight))
		v.SetBounds(x, y, max(0, min(vw, w-x)), max(0, min(row-6, e.gotoBarHeight-y)))
	}
	if rows == 1 {
		place(b.query, 6, 3, 144)
		place(b.accept, 156, 3, gw)
		place(b.close, 162+gw, 3, cw)
		x := 168 + gw + cw
		place(b.status, x, 3, w-x-6)
	} else if rows == 2 {
		input := max(0, w-gw-cw-24)
		place(b.query, 6, 3, input)
		place(b.accept, 12+input, 3, gw)
		place(b.close, 18+input+gw, 3, cw)
		place(b.status, 6, row+3, w-12)
	} else {
		place(b.query, 6, 3, w-12)
		goWidth := min(gw, max(0, (w-18)*.65))
		closeWidth := min(cw, max(0, w-goWidth-18))
		place(b.accept, 6, row+3, goWidth)
		place(b.close, 12+goWidth, row+3, closeWidth)
		place(b.status, 6, 2*row+3, w-12)
	}
}
