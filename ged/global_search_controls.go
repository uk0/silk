package ged

import (
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
	"strings"
)

type globalSearchControls struct {
	panel                                   *GlobalSearchPanel
	query, replacement, include, exclude    *globalSearchEdit
	search, stop, preview, apply            *gui.Button
	regex, matchCase, wholeWord             *gui.CheckBox
	title, typesLabel, excludeLabel, status *gui.Label
	syncing, tips                           bool
	bottom                                  float64
}
type globalSearchEdit struct {
	gui.Edit
	panel *GlobalSearchPanel
	kind  int
}

func (e *globalSearchEdit) OnKeyDown(key int, repeat bool) {
	switch key {
	case gui.KeyEsc:
		e.panel.CancelSearch()
	case gui.KeyEnter:
		if e.kind == 1 {
			e.panel.PreviewReplacements()
		} else {
			e.panel.Search(e.panel.controls.query.Text())
		}
	default:
		e.Edit.OnKeyDown(key, repeat)
	}
}
func (p *GlobalSearchPanel) ensureControls() {
	if p.controls != nil {
		return
	}
	c := &globalSearchControls{panel: p}
	p.controls = c
	edit := func(kind int) *globalSearchEdit {
		e := &globalSearchEdit{panel: p, kind: kind}
		e.Init(e)
		e.SetParent(p)
		e.SetPadding(gui.Padding{L: 4, R: 4, T: 2, B: 2})
		e.SetFont(paint.NewFont(gui.Theme().Font.Family(), 11, false, false))
		return e
	}
	c.query, c.replacement, c.include, c.exclude = edit(0), edit(1), edit(2), edit(3)
	c.include.SetText(strings.Join(p.include, ", "))
	c.exclude.SetText(strings.Join(p.exclude, ", "))
	c.query.SigTextEdited(func(_ interface{}, s string) {
		if c.syncing {
			return
		}
		p.query = s
		p.queryRunes = []rune(s)
		p.queryCursor = len(p.queryRunes)
		p.invalidateSearch()
	})
	c.replacement.SigTextEdited(func(_ interface{}, s string) {
		if c.syncing {
			return
		}
		p.replaceRunes = []rune(s)
		p.replaceCursor = len(p.replaceRunes)
		p.preview = nil
		p.notice = ""
		if p.searching && p.previewing {
			p.startSearch(false)
		}
		p.syncControls()
		p.Update()
	})
	c.include.SigTextEdited(func(_ interface{}, s string) {
		if !c.syncing {
			p.include = splitSearchFilters(s, true)
			p.invalidateSearch()
		}
	})
	c.exclude.SigTextEdited(func(_ interface{}, s string) {
		if !c.syncing {
			p.exclude = splitSearchFilters(s, false)
			p.invalidateSearch()
		}
	})
	button := func(text string, fn func()) *gui.Button {
		b := gui.NewButton1(text, nil)
		b.SetParent(p)
		b.Action().BindFunc0(fn)
		return b
	}
	c.search = button("Search", func() { p.Search(c.query.Text()) })
	c.stop = button("Cancel", p.CancelSearch)
	c.preview = button("Preview", p.PreviewReplacements)
	c.apply = button("Apply", p.ApplyPreview)
	check := func(text string) *gui.CheckBox { b := gui.NewCheckBox(); b.SetText(text); b.SetParent(p); return b }
	c.matchCase, c.wholeWord, c.regex = check("Aa"), check("Word"), check(".*")
	changed := func(bool) {
		if c.syncing {
			return
		}
		p.optCaseSense = c.matchCase.IsChecked()
		p.optWholeWord = c.wholeWord.IsChecked()
		p.optRegex = c.regex.IsChecked()
		p.Search(c.query.Text())
	}
	c.matchCase.SigCheck(changed)
	c.wholeWord.SigCheck(changed)
	c.regex.SigCheck(changed)
	label := func(text string) *gui.Label {
		l := gui.NewLabel(text)
		l.SetParent(p)
		l.SetFont(paint.NewFont(gui.Theme().Font.Family(), 10, false, false))
		return l
	}
	c.title = label("Find in files (disk)")
	c.typesLabel = label("Types")
	c.excludeLabel = label("Exclude")
	c.status = label("")
	p.syncControls()
}
func splitSearchFilters(s string, types bool) []string {
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' || r == ' ' || r == '\t' })
	if types {
		for i := range parts {
			parts[i] = strings.TrimPrefix(parts[i], "*")
		}
	}
	return parts
}
func (p *GlobalSearchPanel) syncControls() {
	c := p.controls
	if c == nil {
		return
	}
	c.syncing = true
	if c.query.Text() != p.query {
		c.query.SetText(p.query)
	}
	if c.replacement.Text() != string(p.replaceRunes) {
		c.replacement.SetText(string(p.replaceRunes))
	}
	c.regex.SetChecked(p.optRegex)
	c.matchCase.SetChecked(p.optCaseSense)
	c.wholeWord.SetChecked(p.optWholeWord)
	for _, w := range []gui.IWidget{c.query, c.replacement, c.include, c.exclude, c.regex, c.matchCase, c.wholeWord, c.search} {
		w.SetEnabled(!p.applying)
	}
	c.stop.SetEnabled(!p.applying && (p.searching || p.preview != nil || p.searchReady))
	c.preview.SetEnabled(!p.applying && !p.searching && p.searchReady && p.report.Complete() && len(p.results) > 0)
	c.apply.SetEnabled(!p.applying && !p.searching && p.preview != nil && len(p.preview) > 0)
	c.status.SetText(p.statusText())
	c.syncing = false
	c.tooltips(p.IsVisible() && p.Window() != nil && !p.closed)
	if c.tips {
		gui.SetToolTip(c.status, p.statusText())
	}
}
func (c *globalSearchControls) tooltips(live bool) {
	if c.tips == live {
		return
	}
	c.tips = live
	tips := map[gui.IWidget]string{c.query: "Search disk text; press Enter or Search. Unsaved buffers are not scanned.", c.replacement: "Replacement; regex accepts $1, ${name} and $$. Preview before Apply.", c.include: "Extensions separated by commas: .go, .txt (empty = all text files)", c.exclude: "Basename/segment globs separated by commas: *_test.go, generated", c.regex: "Go/RE2 regex, line-local anchors; zero-width hits are not listed", c.matchCase: "Match case", c.wholeWord: "Unicode whole word", c.preview: "Read-only replacement preview. Click a result file for its diff.", c.apply: "Commit the exact preview; reject changed files and unsaved buffers", c.stop: "Cancel pending computation or discard the preview", c.search: "Search current workspace on disk", c.status: c.panel.statusText()}
	for w, text := range tips {
		if !live {
			text = ""
		}
		gui.SetToolTip(w, text)
	}
}
func (p *GlobalSearchPanel) Layout() {
	p.ensureControls()
	c := p.controls
	w := p.Width()
	if w <= 0 {
		w = 300
	}
	put := func(v gui.IWidget, x, y, width float64) { v.SetPos(x, y); v.SetSize(max(0, width), 24) }
	stopW := min(c.stop.SizeHints().Width, max(0, w-12))
	put(c.stop, w-stopW-6, 2, stopW)
	put(c.title, 6, 2, max(0, w-stopW-18))
	searchW := min(c.search.SizeHints().Width, max(0, w-12))
	put(c.query, 6, 30, w-searchW-18)
	put(c.search, w-searchW-6, 30, searchW)
	previewW, applyW := c.preview.SizeHints().Width, c.apply.SizeHints().Width
	y := 58.0
	if w >= previewW+applyW+100 {
		put(c.replacement, 6, y, w-previewW-applyW-24)
		put(c.preview, w-previewW-applyW-12, y, previewW)
		put(c.apply, w-applyW-6, y, applyW)
	} else {
		put(c.replacement, 6, y, w-12)
		y += 28
		put(c.preview, 6, y, min(previewW, w-12))
		put(c.apply, previewW+12, y, min(applyW, max(0, w-previewW-18)))
	}
	y += 28
	x := 6.0
	for _, v := range []*gui.CheckBox{c.matchCase, c.wholeWord, c.regex} {
		width := v.SizeHints().Width
		if x > 6 && x+width > w-6 {
			x = 6
			y += 28
		}
		put(v, x, y, min(width, max(0, w-12)))
		x += width + 6
	}
	y += 28
	put(c.typesLabel, 6, y, 44)
	put(c.include, 54, y, w-60)
	y += 28
	put(c.excludeLabel, 6, y, 44)
	put(c.exclude, 54, y, w-60)
	y += 28
	put(c.status, 6, y, w-12)
	c.bottom = y + 28
}
