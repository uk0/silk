package gui

import (
	"github.com/uk0/silk/paint"
)

// The bar and every control belong to the editor's widget tree. No independent
// native windows, custom text editing, or unbounded text drawing are involved.
type editorFindBar struct {
	Widget
	editor                                 *CodeEditor
	query, replacement                     *editorFindEdit
	prev, next, toggle, close, one, all    *editorFindButton
	matchCase, wholeWord, regex, selection *editorFindCheck
	status                                 *Label
	syncing, tipsLive                      bool
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
	b.regex = check(".*")
	b.selection = check("Selection")
	b.selection.SigCheck(func(on bool) {
		if !b.syncing {
			_ = e.SetFindInSelection(on)
		}
	})
	changed := func(bool) {
		if !b.syncing {
			e.SetFindOptions(EditorFindOptions{MatchCase: b.matchCase.IsChecked(), WholeWord: b.wholeWord.IsChecked(), Regex: b.regex.IsChecked()})
		}
	}
	b.matchCase.SigCheck(changed)
	b.wholeWord.SigCheck(changed)
	b.regex.SigCheck(changed)
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
	b.regex.SetChecked(e.findState.options.Regex)
	b.selection.SetChecked(e.findState.scope.enabled)
	b.status.SetText(e.findStatusText())
	ready := e.findRangesCurrent() && len(e.findMatches) > 0
	b.prev.SetEnabled(ready)
	b.next.SetEnabled(ready)
	b.one.SetEnabled(ready)
	b.all.SetEnabled(ready)
	b.syncing = false
	b.SetVisible(e.findActive)
	b.tooltips(e.findActive && e.findVisible() && e.Window() != nil)
	if b.tipsLive {
		SetToolTip(b.status, e.findStatusText()+"\n"+e.findScopeStatus())
	}
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
	tips := map[IWidget]string{b.query: "Find text (enable .* for a Go regular expression)", b.replacement: "Replacement text; regex mode supports $1, ${name} and $$", b.selection: "Freeze selection from Find opening; local regex anchors / word edges. Unrelated edits require a new selection.", b.regex: "Regular expression (Go/RE2; no look-around or backreferences)", b.status: b.editor.findStatusText(), b.prev: "Previous match (Shift+F3)", b.next: "Next match (F3)", b.matchCase: "Match case", b.wholeWord: "Whole word (Unicode letters, digits, marks and underscore)", b.toggle: "Show or hide replacement", b.close: "Close find (Esc)", b.one: "Replace current (Enter in replacement)", b.all: "Replace all (Shift+Enter in replacement)"}
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
	// Wrap complete measured controls. This also handles narrow panes and fonts
	// whose checkbox padding is substantially wider than their label text.
	type control struct {
		widget IWidget
		width  float64
	}
	options := []control{{b.matchCase, b.matchCase.SizeHints().Width}, {b.wholeWord, b.wholeWord.SizeHints().Width}, {b.regex, b.regex.SizeHints().Width}, {b.selection, b.selection.SizeHints().Width}}
	pw, nw := b.prev.SizeHints().Width, b.next.SizeHints().Width
	tw, xw := b.toggle.SizeHints().Width, b.close.SizeHints().Width
	ow, aw := b.one.SizeHints().Width, b.all.SizeHints().Width
	optionW := 0.0
	for _, v := range options {
		optionW += v.width + 6
	}
	statusW := 150.0
	tail := pw + nw + optionW + statusW + tw + xw + 6*6
	wide := w >= tail+160
	medium := w >= pw+nw+optionW+80+4*6
	rows := 1.0
	xw = min(xw, max(0, w-12))
	tw = min(tw, max(0, w-xw-18))
	placeFind(b.close, w-xw-6, 0, xw)
	placeFind(b.toggle, w-xw-tw-12, 0, tw)
	if wide {
		input := w - tail
		placeFind(b.query, 6, 0, input)
		x := input + 12
		controls := []control{{b.prev, pw}, {b.next, nw}}
		controls = append(controls, options...)
		controls = append(controls, control{b.status, statusW})
		for _, v := range controls {
			placeFind(v.widget, x, 0, v.width)
			x += v.width + 6
		}
	} else {
		rows = 2
		placeFind(b.query, 6, 0, max(0, w-xw-tw-24))
		placeFind(b.prev, 6, 30, min(pw, max(0, w-12)))
		placeFind(b.next, pw+12, 30, min(nw, max(0, w-pw-18)))
		x := pw + nw + 18
		if medium {
			for _, v := range options {
				placeFind(v.widget, x, 30, v.width)
				x += v.width + 6
			}
			placeFind(b.status, x, 30, max(0, w-x-6))
		} else {
			placeFind(b.status, x, 30, max(0, w-x-6))
			x = 6
			y := 60.0
			for _, v := range options {
				if x > 6 && x+v.width > w-6 {
					x = 6
					y += 30
				}
				placeFind(v.widget, x, y, min(v.width, max(0, w-12)))
				x += v.width + 6
			}
			rows = y/30 + 1
		}
	}
	e.findBarHeight = rows * codeEditorFindRowHeight
	b.replacement.SetVisible(e.replaceVisible)
	b.one.SetVisible(e.replaceVisible)
	b.all.SetVisible(e.replaceVisible)
	if e.replaceVisible {
		y := e.findBarHeight
		e.findBarHeight += 30
		if w >= ow+aw+120+4*6 {
			placeFind(b.replacement, 6, y, w-ow-aw-24)
			placeFind(b.one, w-ow-aw-12, y, ow)
			placeFind(b.all, w-aw-6, y, aw)
		} else {
			e.findBarHeight += 30
			placeFind(b.replacement, 6, y, max(0, w-12))
			placeFind(b.one, 6, y+30, min(ow, max(0, w-12)))
			placeFind(b.all, ow+12, y+30, min(aw, max(0, w-ow-18)))
		}
	}
	b.SetSize(w, e.findBarHeight)
}
