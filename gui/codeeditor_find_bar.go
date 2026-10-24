package gui

import (
	"github.com/uk0/silk/paint"
)

// The bar and every control belong to the editor's widget tree. No independent
// native windows, custom text editing, or unbounded text drawing are involved.
type editorFindBar struct {
	Widget
	editor                              *CodeEditor
	query, replacement                  *editorFindEdit
	prev, next, toggle, close, one, all *editorFindButton
	matchCase, wholeWord                *editorFindCheck
	status                              *Label
	syncing, tipsLive                   bool
}
type editorFindEdit struct {
	Edit
	bar         *editorFindBar
	replacement bool
}
type editorFindButton struct {
	Button
	bar *editorFindBar
}
type editorFindCheck struct {
	CheckBox
	bar *editorFindBar
}

func (b *editorFindBar) key(key int, shift bool) bool {
	e := b.editor
	switch {
	case key == KeyEsc:
		e.closeFindBar()
	case key == KeyF3:
		if shift {
			e.findPrev()
		} else {
			e.findNext()
		}
	case key == 'F' && (IsKeyDown(KeyCtrl) || isActionModifier()):
		e.closeFindBar()
	case key == 'H' && (IsKeyDown(KeyCtrl) || isActionModifier()):
		e.toggleReplaceRow()
	default:
		return false
	}
	return true
}
func (f *editorFindEdit) OnKeyDown(key int, repeat bool) {
	f.key(key, repeat, IsKeyDown(KeyShift))
}
func (f *editorFindEdit) key(key int, repeat, shift bool) {
	e := f.bar.editor
	if f.bar.key(key, shift) {
		return
	}
	if key == KeyEnter {
		if f.replacement {
			if shift {
				e.replaceAll()
			} else {
				e.replaceCurrent()
			}
		} else if shift {
			e.findPrev()
		} else {
			e.findNext()
		}
		return
	}
	f.Edit.OnKeyDown(key, repeat)
}
func (f *editorFindEdit) OnFocusChanged(n, o IWidget) {
	if n == f {
		f.bar.editor.replaceFocused = f.replacement
		f.bar.editor.findState.documentFocus = false
	}
	f.Widget.OnFocusChanged(n, o)
}
func (b *editorFindButton) OnKeyDown(key int, repeat bool) {
	if !b.bar.key(key, IsKeyDown(KeyShift)) {
		b.Button.OnKeyDown(key, repeat)
	}
}
func (c *editorFindCheck) OnKeyDown(key int, repeat bool) {
	if !c.bar.key(key, IsKeyDown(KeyShift)) {
		c.CheckBox.OnKeyDown(key, repeat)
	}
}
func (e *CodeEditor) ensureFindBar() {
	if e.findBar != nil {
		e.syncFindBar()
		return
	}
	b := &editorFindBar{editor: e}
	b.Init(b)
	b.SetParent(e.Self())
	e.findBar = b
	edit := func(replace bool) *editorFindEdit {
		f := &editorFindEdit{bar: b, replacement: replace}
		f.Init(f)
		f.SetParent(b)
		f.SetFont(paint.NewFont(defaultFontFamily(), 12, false, false))
		f.SetPadding(Padding{L: 5, R: 5, T: 2, B: 2})
		f.SigTextEdited(func(_ interface{}, s string) {
			if replace {
				e.replaceText = s
				e.replaceCursor = f.CaretPos()
				if e.findState.pending && e.findState.request.replace {
					e.findUpdateMatches()
				}
			} else {
				e.SetFindQuery(s)
			}
		})
		return f
	}
	b.query = edit(false)
	b.replacement = edit(true)
	button := func(text, icon string, fn func()) *editorFindButton {
		v := &editorFindButton{bar: b}
		v.Init(v)
		v.SetParent(b)
		v.SetText(text)
		if icon != "" {
			v.SetIconName(icon)
			v.SetTextVisible(false)
		}
		v.Action().BindFunc0(fn)
		return v
	}
	b.prev = button("Previous", "navigate-back", e.findPrev)
	b.next = button("Next", "navigate-forward", e.findNext)
	b.toggle = button("Replace…", "", e.toggleReplaceRow)
	b.close = button("Close", "close", e.closeFindBar)
	b.one = button("Replace", "", e.replaceCurrent)
	b.all = button("Replace all", "", func() { e.replaceAll() })
	check := func(text string) *editorFindCheck {
		v := &editorFindCheck{bar: b}
		v.Init(v)
		v.SetParent(b)
		v.SetText(text)
		return v
	}
	b.matchCase = check("Aa")
	b.wholeWord = check("Word")
	changed := func(bool) {
		if !b.syncing {
			e.SetFindOptions(EditorFindOptions{b.matchCase.IsChecked(), b.wholeWord.IsChecked()})
		}
	}
	b.matchCase.SigCheck(changed)
	b.wholeWord.SigCheck(changed)
	b.status = NewLabel("")
	b.status.SetParent(b)
	b.status.SetFont(paint.NewFont(defaultFontFamily(), 11, false, false))
	b.status.SetFocusPolicy(NoFocus)
	e.syncFindBar()
}
func (e *CodeEditor) syncFindBar() {
	b := e.findBar
	if b == nil {
		return
	}
	b.syncing = true
	if b.query.Text() != e.findText {
		b.query.SetText(e.findText)
		b.query.SetCaretPos(e.findCursor)
	}
	if b.replacement.Text() != e.replaceText {
		b.replacement.SetText(e.replaceText)
		b.replacement.SetCaretPos(e.replaceCursor)
	}
	b.matchCase.SetChecked(e.findState.options.MatchCase)
	b.wholeWord.SetChecked(e.findState.options.WholeWord)
	b.status.SetText(e.findStatusText())
	ready := e.findRangesCurrent() && len(e.findMatches) > 0
	b.prev.SetEnabled(ready)
	b.next.SetEnabled(ready)
	b.one.SetEnabled(ready)
	b.all.SetEnabled(!e.findState.pending && e.findState.err == nil && e.findState.count > 0)
	b.syncing = false
	b.SetVisible(e.findActive)
	b.tooltips(e.findActive && e.Window() != nil)
}
func (e *CodeEditor) focusFindInput(replace, selectAll bool) {
	e.ensureFindBar()
	e.replaceFocused = replace
	e.findState.documentFocus = false
	f := e.findBar.query
	if replace {
		f = e.findBar.replacement
	}
	f.SetFocus()
	if selectAll {
		f.SelectAll()
	}
}
func (b *editorFindBar) tooltips(live bool) {
	if b.tipsLive == live {
		return
	}
	b.tipsLive = live
	tips := map[IWidget]string{b.query: "Find literal text", b.replacement: "Replacement text", b.prev: "Previous match (Shift+F3)", b.next: "Next match (F3)", b.matchCase: "Match case", b.wholeWord: "Whole word (Unicode letters, digits, marks and underscore)", b.toggle: "Show or hide replacement", b.close: "Close find (Esc)", b.one: "Replace current (Enter in replacement)", b.all: "Replace all (Shift+Enter in replacement)"}
	for w, s := range tips {
		if !live {
			s = ""
		}
		SetToolTip(w, s)
	}
}
func (b *editorFindBar) Draw(g paint.Painter) {
	pal := b.editor.editorColors()
	w, h := b.Size()
	g.SetBrush1(pal.gutter)
	g.Rectangle(0, 0, w, h)
	g.Fill()
	g.SetPen1(pal.gutterBorder, 1)
	g.Line(0, h-1, w, h-1)
	g.Stroke()
}
func placeFind(w IWidget, x, y, width float64) { w.SetPos(x, y+3); w.SetSize(max(0, width), 24) }

// Layout wraps complete controls instead of painting inputs over their peers.
// A not-yet-sized model uses the wide layout; the native editor always has width.
func (e *CodeEditor) layoutFindBar() {
	b := e.findBar
	if b == nil || !e.findActive {
		return
	}
	w, _ := e.Size()
	if w <= 0 {
		w = 1200
	}
	b.SetPos(0, 0)
	// Honor actual theme metrics, not ASCII character-count estimates. In
	// particular checkbox labels start AFTER both margins and their icon.
	pw, nw := b.prev.SizeHints().Width, b.next.SizeHints().Width
	cw, ww := b.matchCase.SizeHints().Width, b.wholeWord.SizeHints().Width
	tw, xw := b.toggle.SizeHints().Width, b.close.SizeHints().Width
	ow, aw := b.one.SizeHints().Width, b.all.SizeHints().Width
	statusW := 150.0
	tail := pw + nw + cw + ww + statusW + tw + xw + 8*6
	wide := w >= tail+160
	medium := w >= pw+nw+cw+ww+80+6*6
	rows := 1.0
	if !wide {
		rows = 2
	}
	if !medium {
		rows = 3
	}
	e.findBarHeight = rows * codeEditorFindRowHeight
	splitReplace := w < ow+aw+120+4*6
	if e.replaceVisible {
		e.findBarHeight += codeEditorFindRowHeight
		if splitReplace {
			e.findBarHeight += codeEditorFindRowHeight
		}
	}
	b.SetSize(w, e.findBarHeight)
	// Extremely small panes clip rather than sharing the same hit rectangle.
	xw = min(xw, max(0, w-12))
	tw = min(tw, max(0, w-xw-18))
	placeFind(b.close, w-xw-6, 0, xw)
	placeFind(b.toggle, w-xw-tw-12, 0, tw)
	if wide {
		input := w - tail
		placeFind(b.query, 6, 0, input)
		x := input + 12
		for _, v := range []struct {
			widget IWidget
			width  float64
		}{{b.prev, pw}, {b.next, nw}, {b.matchCase, cw}, {b.wholeWord, ww}, {b.status, statusW}} {
			placeFind(v.widget, x, 0, v.width)
			x += v.width + 6
		}
	} else {
		placeFind(b.query, 6, 0, max(0, w-xw-tw-24))
		placeFind(b.prev, 6, 30, pw)
		placeFind(b.next, pw+12, 30, nw)
		x := pw + nw + 18
		if medium {
			placeFind(b.matchCase, x, 30, cw)
			x += cw + 6
			placeFind(b.wholeWord, x, 30, ww)
			x += ww + 6
			placeFind(b.status, x, 30, max(0, w-x-6))
		} else {
			placeFind(b.status, x, 30, max(0, w-x-6))
			placeFind(b.matchCase, 6, 60, cw)
			placeFind(b.wholeWord, cw+12, 60, min(ww, max(0, w-cw-18)))
		}
	}
	b.replacement.SetVisible(e.replaceVisible)
	b.one.SetVisible(e.replaceVisible)
	b.all.SetVisible(e.replaceVisible)
	if e.replaceVisible {
		y := rows * codeEditorFindRowHeight
		if !splitReplace {
			placeFind(b.replacement, 6, y, w-ow-aw-24)
			placeFind(b.one, w-ow-aw-12, y, ow)
			placeFind(b.all, w-aw-6, y, aw)
		} else {
			placeFind(b.replacement, 6, y, max(0, w-12))
			placeFind(b.one, 6, y+30, min(ow, max(0, w-12)))
			placeFind(b.all, ow+12, y+30, min(aw, max(0, w-ow-18)))
		}
	}
}
