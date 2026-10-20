package main

import (
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/paint"
	"math"
)

type widgetPalette struct {
	gui.Widget
	list   *ged.WidgetList
	search *gui.SearchBox
}

func (p *widgetPalette) Title() string    { return i18n.T("Widgets") }
func (p *widgetPalette) Icon() paint.Icon { return paint.LoadIcon("window") }
func (p *widgetPalette) SizeHints() gui.SizeHints {
	return gui.SizeHints{Width: 220, Height: 400, Policy: gui.GrowHorizontal | gui.GrowVertical}
}
func (p *widgetPalette) Layout() {
	w, h := p.Size()
	searchH := math.Min(36, math.Max(0, h))
	p.search.SetBounds(math.Min(4, math.Max(0, w)), math.Min(4, searchH), math.Max(0, w-8), math.Max(0, searchH-8))
	p.list.SetBounds(0, searchH, math.Max(0, w), math.Max(0, h-searchH))
}

// Keep the drag source itself intact and give its existing filter a visible
// search field. A single managed parent reserves space above the scroll list.
func buildWidgetPalette(list *ged.WidgetList) *widgetPalette {
	box := &widgetPalette{list: list}
	box.Init(box)
	search := gui.NewSearchBox()
	box.search = search
	search.SetPlaceholder(i18n.T("Filter widgets..."))
	search.SigTextChanged(list.SetFilter)
	gui.SetToolTip(search, i18n.T("Search by widget or factory name"))
	search.SetParent(box)
	list.SetParent(box)
	return box
}
