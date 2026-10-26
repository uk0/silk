package gui

import (
	"math"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/uk0/silk/paint"
)

// SetEditable switches between free-text entry (the default) and selecting an
// existing item. Disabling editing detaches the editor and shows the committed
// item; arbitrary draft text is not a selection. Re-enabling creates the default
// editor. Custom editors may still be installed with SetEditWidget.
func (c *ComboBox) SetEditable(editable bool) {
	if editable == c.IsEditable() {
		return
	}
	focused := c.HasFocus()
	if editable {
		c.SetEditWidget(newComboEdit(c))
		c.edit.SetText(c.ActiveItem().Text)
	} else {
		c.SetEditWidget(nil)
	}
	if focused {
		if c.edit != nil {
			c.edit.SetFocus()
		} else {
			c.SetFocus()
		}
	}
	c.Update()
}

// The editable composite has one automatic Tab stop, on its actual editor.
func (c *ComboBox) tabFocusProxy() IWidget { return c.edit }

type comboEdit struct {
	Edit
	combo *ComboBox
}

func newComboEdit(c *ComboBox) *comboEdit {
	e := &comboEdit{combo: c}
	e.Edit.Init(e)
	e.SetPadding(Padding{})
	e.SetNoFrame(true)
	return e
}
func (e *comboEdit) OnKeyDown(key int, repeat bool) {
	if e.combo != nil && e.Parent() == e.combo.Self() {
		if !e.combo.IsEnabled() {
			return
		}
		// Home/End and selection shortcuts still belong to the text editor.
		if key == KeyUp || key == KeyDown || key == KeyPageUp || key == KeyPageDown || key == KeyF4 {
			if e.combo.handleComboKey(key, repeat) {
				return
			}
		}
	}
	e.Edit.OnKeyDown(key, repeat)
}
func (e *comboEdit) OnSystemKeyDown(key int, repeat bool) bool {
	return e.combo != nil && e.Parent() == e.combo.Self() && e.combo.OnSystemKeyDown(key, repeat)
}
func (c *ComboBox) OnSystemKeyDown(key int, repeat bool) bool {
	// Windows Alt+F4/Alt+Space and AltGr must remain native.
	if currentMods() != ModAlt || IsKeyDown(KeyCtrl) || (key != KeyDown && key != KeyUp) {
		return false
	}
	return c.handleComboKey(key, repeat)
}
func (c *ComboBox) handleComboKey(key int, repeat bool) bool {
	if !c.IsEnabled() || !c.IsVisible() {
		return true
	}
	mods := currentMods()
	if mods == ModAlt && !IsKeyDown(KeyCtrl) && (key == KeyDown || key == KeyUp) {
		if !repeat {
			if key == KeyDown {
				c.ShowSubPopup()
			} else {
				c.HideSubPopup()
			}
		}
		return true
	}
	if mods != 0 || IsKeyDown(KeyCtrl) {
		return false
	}
	if key == KeyF4 {
		if !repeat {
			c.ToggleSubPopup()
		}
		return true
	}
	open := c.IsSubPopupVisible()
	if key == KeySpace && !open && !c.IsEditable() {
		if !repeat {
			c.ShowSubPopup()
		}
		return true
	}
	switch key {
	case KeyEsc:
		if open {
			c.HideSubPopup()
		}
		return true
	case KeyEnter, KeySpace:
		if open {
			c.onListSubmit()
		}
		return true
	case KeyUp, KeyDown, KeyHome, KeyEnd, KeyPageUp, KeyPageDown:
		idx := c.activeIndex
		if open {
			idx = c.sub.activeIndex
		}
		switch key {
		case KeyDown:
			idx++
		case KeyUp:
			idx--
		case KeyHome:
			idx = 0
		case KeyEnd:
			idx = c.Count() - 1
		case KeyPageDown:
			idx = pageStepIndex(idx, c.Count(), c.sub.rowsPerPage(), 1)
		case KeyPageUp:
			idx = pageStepIndex(idx, c.Count(), c.sub.rowsPerPage(), -1)
		}
		c.previewOrSelect(clampIndex(idx, c.Count()))
		return true
	}
	return false
}
func (c *ComboBox) previewOrSelect(idx int) {
	if idx < 0 {
		return
	}
	if c.IsSubPopupVisible() {
		c.sub.SetActiveIndex(idx)
	} else {
		c.setActiveIndex(idx)
	}
}
func (c *ComboBox) OnTextInput(text string) {
	if !c.IsEditable() {
		c.searchPrefix(text)
	}
}
func (c *ComboBox) searchPrefix(text string) {
	// Text input is already committed by the platform (including IME/AltGr).
	// Do not consult modifiers from an outer, still-running modal shortcut.
	if !c.IsEnabled() || !c.IsVisible() {
		return
	}
	text = strings.TrimSpace(text)
	if text == "" || !utf8.ValidString(text) {
		return
	}
	for _, r := range text {
		if unicode.IsControl(r) {
			return
		}
	}
	start := c.activeIndex
	if c.IsSubPopupVisible() {
		start = c.sub.activeIndex
	}
	for off := 1; off <= c.Count(); off++ {
		idx := (start + off) % c.Count()
		if comboPrefixFold(c.Item(idx).Text, text) {
			c.previewOrSelect(idx)
			return
		}
	}
}

// Rune-wise Unicode simple folding, with no per-item lowercase/slice allocation.
func comboPrefixFold(text, prefix string) bool {
	for _, want := range prefix {
		got, n := utf8.DecodeRuneInString(text)
		if n == 0 {
			return false
		}
		text = text[n:]
		if got != want {
			matched := false
			for f := unicode.SimpleFold(got); f != got; f = unicode.SimpleFold(f) {
				if f == want {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		}
	}
	return true
}
func (c *ComboBox) preparePopup() {
	c.sub.activeIndex = c.activeIndex
	c.sub.hoverIndex = -1
	c.sub.isLeftDown = false
	icons := false
	for _, item := range c.sub.items {
		if item.Icon != nil {
			icons = true
			break
		}
	}
	c.sub.SetIconVisible(icons)
}
func (p *comboPopup) OnKeyDown(key int, repeat bool) {
	if p.combo != nil {
		p.combo.handleComboKey(key, repeat)
	}
}
func (p *comboPopup) OnSystemKeyDown(key int, repeat bool) bool {
	return p.combo != nil && p.combo.OnSystemKeyDown(key, repeat)
}
func (p *comboPopup) OnTextInput(text string) {
	if p.combo != nil {
		p.combo.searchPrefix(text)
	}
}
func (p *comboPopup) OnMouseMove(x, y float64) {
	p.ListWidget.OnMouseMove(x, y)
	if p.combo != nil && p.combo.IsEnabled() {
		if row, _ := p.HitTest(x, y); row >= 0 {
			p.SetActiveIndex(row)
		}
	}
}
func (p *comboPopup) OnLeftUp(x, y float64) {
	row, col := p.HitTest(x, y)
	if row < 0 || col < 0 || p.downRow < 0 || !p.isLeftDown {
		p.isLeftDown = false
		p.downRow, p.downCol = -1, -1
		return
	}
	p.SetActiveIndex(row)
	p.ListWidget.OnLeftUp(x, y)
}
func (p *comboPopup) OnHide() {
	if p.combo != nil {
		p.activeIndex = p.combo.activeIndex
	}
	p.hoverIndex = -1
	p.isLeftDown = false
}

type comboLabelCache struct {
	text, fitted, family string
	font                 paint.Font
	width                float64
	size                 int
	bold, italic         bool
	bearing              float64
}

func (c *ComboBox) fittedSelectionLabel(width float64) (string, float64) {
	f := Theme().Font
	text := c.ActiveItem().Text
	k := &c.labelCache
	if k.text != text || k.font != f || k.width != width || k.size != f.Size() || k.family != f.Family() || k.bold != f.Bold() || k.italic != f.Italic() {
		*k = comboLabelCache{text: text, font: f, width: width, size: f.Size(), family: f.Family(), bold: f.Bold(), italic: f.Italic()}
		k.fitted = ElideText(f, text, width-2)
		k.bearing = math.Floor(f.TextExtents(k.fitted).XBearing)
	}
	return k.fitted, k.bearing
}
func (c *ComboBox) drawSelectionLabel(g paint.Painter) {
	r := c.ClientRect()
	if r.Width <= 0 || r.Height <= 0 {
		return
	}
	f := Theme().Font
	fe := f.FontExtents()
	ink := Theme().TextColor
	if !c.IsEnabled() {
		ink = paint.CurrentIconPalette().Disabled
	}
	g.Save()
	g.Rectangle1(r)
	g.Clip()
	if icon := c.ActiveItem().Icon; icon != nil {
		size := math.Min(Theme().IconSize, math.Min(r.Width, r.Height))
		g.DrawIcon1(icon, r.X, r.Y+(r.Height-size)*.5, size, !c.IsEnabled())
		r.X += size + 4
		r.Width = math.Max(0, r.Width-size-4)
	}
	text, bearing := c.fittedSelectionLabel(r.Width)
	g.SetFont(f)
	g.SetBrush1(ink)
	g.DrawText1(r.X+1-bearing, r.Y+(r.Height-fe.Height)*.5+fe.Ascent, text)
	g.Restore()
}

// A popup is non-activating natively, but must own logical input. Otherwise
// Escape is routed to its modal owner and cancels the whole settings dialog.
func (p *comboPopup) OnShow() { p.SetFocus() }

// Tab cancels the preview and leaves the composite field, not its scrollbar.
func (p *comboPopup) WantsTab(reverse bool) bool {
	c := p.combo
	if c == nil || !c.IsSubPopupVisible() {
		return false
	}
	owner := c.OwnerWindow()
	c.HideSubPopup()
	if owner != nil && owner.widget != nil {
		var current IWidget = c.Self()
		if c.edit != nil && (!reverse || !isTabFocusable(c.Self())) {
			current = c.edit
		}
		if next := nextFocusable(owner.widget, current, !reverse); next != nil {
			next.SetFocus()
		}
	}
	return true
}
