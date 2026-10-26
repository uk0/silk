package main

import (
	"fmt"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
	"strings"
)

func init() {
	i18n.AddMany("zh-CN", map[string]string{
		"Search commands. Up/Down selects, Enter runs, Escape closes.": "搜索命令；上下键选择，回车执行，Escape 关闭。",
		"No matching commands. Try a shorter search.":                  "没有匹配的命令，请缩短搜索内容。",
		"Commands: %d":                        "命令：%d",
		"Unavailable in the current context.": "当前上下文不可用。",
	})
}

// A bounded, keyboard-focusable description viewport: selected commands may
// carry long localized explanations, so elision in rows must not hide access
// to the original text. Reuses Edit's Unicode selection/copy and wrapping.
type paletteDetails struct{ gui.Edit }

func newPaletteDetails() *paletteDetails {
	d := new(paletteDetails)
	d.Edit.Init(d)
	d.SetReadOnly(true)
	d.SetMultiLine(true)
	d.SetWrap(true)
	return d
}
func (d *paletteDetails) SizeHints() gui.SizeHints {
	return gui.SizeHints{Width: 100, Height: 4*d.RowHeight() + gui.Theme().EditPadding.T + gui.Theme().EditPadding.B, Policy: gui.GrowHorizontal}
}
func paletteDescription(c paletteCommand) string {
	parts := []string{c.Name}
	if !c.available() {
		parts = append(parts, i18n.T("Unavailable in the current context."))
	}
	if c.Hint != "" {
		parts = append(parts, c.Hint)
	}
	return strings.Join(parts, "\n")
}
func updatePaletteDetails(list *gui.ListWidget, details *paletteDetails, count *gui.Label) {
	count.SetText(fmt.Sprintf(i18n.T("Commands: %d"), list.Count()))
	text := i18n.T("No matching commands. Try a shorter search.")
	if idx := list.ActiveIndex(); idx >= 0 && idx < list.Count() {
		if c, ok := list.Item(idx).Data.(paletteCommand); ok {
			text = paletteDescription(c)
		}
	}
	details.SetText(text)
	details.SetCaretPos(0)
	gui.SetToolTip(details, text)
}

func paletteRowPresentation(c paletteCommand) gui.ListItemPresentation {
	p := gui.ListItemPresentation{SecondaryText: c.Hint}
	if !c.available() {
		p.TrailingText = i18n.T("Unavailable")
		p.Muted = true
	}
	return p
}
