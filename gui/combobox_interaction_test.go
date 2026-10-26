package gui

import (
	"fmt"
	"github.com/uk0/silk/paint"
	"strings"
	"testing"
)

func TestComboSelectionModePaintsSelectedValue(t *testing.T) {
	c := newTestCombo("Dark", "Light")
	c.SetActiveIndex(1)
	c.SetEditWidget(nil)
	c.SetSize(180, 32)
	p := paint.NewPixmap(180, 32)
	g := &listPresentationInkRecorder{Painter: p.NewPainter()}
	c.Draw(g)
	if len(g.texts) != 1 || g.texts[0] != "Light" {
		t.Fatalf("selection-only combo is blank: %v", g.texts)
	}
}

func TestComboPopupKeyboardSelectionIsVisible(t *testing.T) {
	c := newTestCombo("Dark", "Light")
	if !c.sub.IsSelectionVisible() {
		t.Fatal("keyboard selection has no highlight")
	}
	c.sub.SetSize(180, 80)
	c.sub.SetVisible(true)
	c.sub.OnKeyDown(KeyEnd, false)
	if c.sub.ActiveIndex() != 1 || c.ActiveIndex() != -1 {
		t.Fatal("navigation committed instead of previewing")
	}
}

func TestComboEditableModesAndProgrammaticText(t *testing.T) {
	c := newTestCombo("Dark", "Light")
	c.SetActiveIndex(0)
	e := c.EditWidget()
	e.SetText("uncommitted")
	changes := 0
	c.SigSelectionChanged(func(interface{}, int) { changes++ })
	c.SetEditable(false)
	if c.IsEditable() || e.Parent() != nil || c.Text() != "Dark" || changes != 0 {
		t.Fatal("mode switch committed draft or retained detached editor")
	}
	c.SetText("Light")
	c.SetText("unknown")
	if c.Text() != "Light" || c.ActiveIndex() != 1 || changes != 1 {
		t.Fatal("selection-only text must match an existing item")
	}
	c.SetEditable(true)
	if c.EditWidget() == e || c.Text() != "Light" || changes != 1 {
		t.Fatal("default editor not recreated from committed value")
	}
	c.EditWidget().SetText("free text")
	if c.Text() != "free text" || c.ActiveIndex() != 1 {
		t.Fatal("editable mode lost free-text contract")
	}
}

func TestComboPopupPreviewScrollCancelAndSubmit(t *testing.T) {
	old := focusWidget
	t.Cleanup(func() { focusWidget = old })
	c := NewComboBox()
	c.SetEditable(false)
	for i := 0; i < 100; i++ {
		c.Append(ListItem{Text: fmt.Sprintf("Item %03d", i)})
	}
	c.SetActiveIndex(0)
	changes, submits := 0, 0
	c.SigSelectionChanged(func(interface{}, int) {
		changes++
		if c.IsSubPopupVisible() {
			t.Error("callback ran before old popup closed")
		}
	})
	c.SigSubmit(func(interface{}) { submits++ })
	c.sub.SetSize(180, 100)
	c.preparePopup()
	c.sub.SetVisible(true)
	c.sub.OnKeyDown(KeyEnd, false)
	if c.sub.ActiveIndex() != 99 || c.sub.ScrollY() <= 0 || c.ActiveIndex() != 0 {
		t.Fatal("last preview not visible or already committed")
	}
	c.sub.OnKeyDown(KeyEsc, false)
	if c.sub.ActiveIndex() != 0 || c.IsSubPopupVisible() || changes != 0 || submits != 0 {
		t.Fatal("cancel retained stale preview or fired callbacks")
	}
	c.preparePopup()
	c.sub.SetVisible(true)
	c.sub.OnTextInput("Item 099")
	if c.ActiveIndex() != 0 || c.sub.ActiveIndex() != 99 {
		t.Fatal("native text did not preview matching item")
	}
	c.sub.OnKeyDown(KeyEnter, false)
	c.sub.OnKeyDown(KeyEnter, false)
	if c.ActiveIndex() != 99 || changes != 1 || submits != 1 || c.IsSubPopupVisible() {
		t.Fatal("submit was missing or duplicated")
	}
	c.preparePopup()
	c.sub.SetVisible(true)
	c.sub.OnKeyDown(KeyHome, false)
	c.SigSubmit(func(interface{}) { c.sub.SetVisible(true) })
	c.sub.OnKeyDown(KeyEnter, false)
	if !c.IsSubPopupVisible() {
		t.Fatal("old submit dismissed callback-created popup")
	}
	c.sub.SetVisible(false)
}

func TestComboUnicodeTextAndKeyEventsDoNotDoubleCycle(t *testing.T) {
	c := newTestCombo("Apple", "Apricot", "简体中文", "😀 emoji", "Kelvin", "Σigma")
	c.SetEditable(false)
	for _, tc := range []struct {
		text string
		want int
	}{{"a", 0}, {"A", 1}, {"简体", 2}, {"😀", 3}, {"kel", 4}, {"σ", 5}, {"no-match", 5}, {"\xff", 5}} {
		c.OnKeyDown('A', false) // native key-down must not consume the character too.
		before := c.ActiveIndex()
		c.OnTextInput(tc.text)
		if c.ActiveIndex() != tc.want {
			t.Fatalf("%q after %d -> %d, want %d", tc.text, before, c.ActiveIndex(), tc.want)
		}
	}
	c.SetEnabled(false)
	c.OnTextInput("a")
	if c.ActiveIndex() != 5 {
		t.Fatal("disabled type-ahead changed selection")
	}
}

func TestComboDefaultEditorRoutesArrowsButKeepsCaretKeys(t *testing.T) {
	c := newTestCombo("Alpha", "Beta", "Gamma")
	c.SetActiveIndex(0)
	e := c.EditWidget().(*comboEdit)
	e.OnKeyDown(KeyDown, false)
	if c.ActiveIndex() != 1 || e.Text() != "Beta" {
		t.Fatal("editor swallowed combo navigation")
	}
	e.OnKeyDown(KeyHome, false)
	if c.ActiveIndex() != 1 || e.CaretPos() != 0 {
		t.Fatal("Home changed selection instead of text caret")
	}
	e.OnKeyDown(KeyEnd, false)
	if c.ActiveIndex() != 1 || e.CaretPos() != 4 {
		t.Fatal("End changed selection instead of text caret")
	}
	for _, key := range []int{KeyF1, KeyF2, KeyF5, KeyF12} {
		c.OnKeyDown(key, false)
		if c.ActiveIndex() != 1 {
			t.Fatal("function key searched as printable ASCII")
		}
	}
	if c.OnSystemKeyDown(KeyF4, false) || e.OnSystemKeyDown(KeySpace, false) {
		t.Fatal("system window commands consumed")
	}
}

func TestComboPopupMouseAndBlankArea(t *testing.T) {
	old := focusWidget
	t.Cleanup(func() { focusWidget = old })
	c := newTestCombo("One", "Two")
	c.SetEditable(false)
	c.SetActiveIndex(0)
	c.sub.SetSize(180, 140)
	c.preparePopup()
	c.sub.SetVisible(true)
	x, y := 30.0, c.sub.padding.T+1.5*c.sub.RowHeight()
	c.sub.OnMouseMove(x, y)
	if c.sub.ActiveIndex() != 1 || c.ActiveIndex() != 0 {
		t.Fatal("hover did not preview")
	}
	c.sub.OnLeftDown(x, 120)
	c.sub.OnLeftUp(x, 120)
	if !c.IsSubPopupVisible() || c.ActiveIndex() != 0 {
		t.Fatal("blank area committed old row")
	}
	c.sub.OnLeftDown(x, c.sub.padding.T+5)
	c.sub.OnMouseMove(x, y)
	c.sub.OnLeftUp(x, y)
	if c.IsSubPopupVisible() || c.ActiveIndex() != 1 {
		t.Fatal("drag release did not commit visible row")
	}
}

func TestComboPopupPreparationAndLabelCache(t *testing.T) {
	c := newTestCombo(strings.Repeat("中文 Long label ", 40), "Two")
	c.SetActiveIndex(0)
	c.SetEditable(false)
	c.preparePopup()
	if c.sub.IsIconVisible() {
		t.Fatal("empty icon column wastes label space")
	}
	c.SetItem(1, ListItem{Text: "Two", Icon: Theme().CheckedIcon})
	c.preparePopup()
	if !c.sub.IsIconVisible() {
		t.Fatal("real icons hidden")
	}
	for _, w := range []float64{0, 4, 50, 180} {
		c.SetSize(w, 30)
		r := c.ClientRect()
		if r.Width < 0 || r.Height < 0 {
			t.Fatal("negative editor bounds")
		}
		text, _ := c.fittedSelectionLabel(r.Width)
		if Theme().Font.TextExtents(text).Width > r.Width {
			t.Fatal("selection label exceeds viewport")
		}
	}
	c.SetText("Two")
	text, _ := c.fittedSelectionLabel(180)
	if text != "Two" {
		t.Fatal("cached label stale after selection")
	}
}

func BenchmarkComboSelectionLabelWarmCache(b *testing.B) {
	c := newTestCombo(strings.Repeat("Long label 中文 ", 100))
	c.SetActiveIndex(0)
	c.SetEditable(false)
	c.fittedSelectionLabel(140)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.fittedSelectionLabel(140)
	}
}
func BenchmarkComboUnicodeSearch1000(b *testing.B) {
	c := NewComboBox()
	c.SetEditable(false)
	for i := 0; i < 1000; i++ {
		c.Append(ListItem{Text: fmt.Sprintf("Item %d 中文", i)})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.OnTextInput("missing")
	}
}

func TestComboSelectionInkStaysInsideEditorArea(t *testing.T) {
	mode := CurrentThemeMode()
	t.Cleanup(func() { SetThemeMode(mode) })
	for _, theme := range []ThemeMode{ThemeDark, ThemeLight} {
		SetThemeMode(theme)
		for _, enabled := range []bool{false, true} {
			c := newTestCombo(strings.Repeat("W", 80))
			c.SetEditable(false)
			c.SetActiveIndex(0)
			c.SetEnabled(enabled)
			c.SetSize(180, 32)
			render := func() paint.Pixmap {
				p := paint.NewPixmap(180, 32)
				g := p.NewPainter()
				c.Draw(g)
				paint.ClosePainter(g)
				p.Flush()
				return p
			}
			full, _ := render().Image()
			c.SetActiveIndex(-1)
			empty, _ := render().Image()
			r := c.ClientRect()
			for y := 0; y < 32; y++ {
				for x := 0; x < 180; x++ {
					if float64(x) >= r.X && float64(x) < r.Right() && float64(y) >= r.Y && float64(y) < r.Bottom() {
						continue
					}
					if full.At(x, y) != empty.At(x, y) {
						t.Fatalf("theme=%v enabled=%v text escaped at %d,%d", theme, enabled, x, y)
					}
				}
			}
		}
	}
}

type comboFocusSpy struct {
	ComboBox
	updates int
}

func (c *comboFocusSpy) Update() { c.updates++ }
func TestComboSelectionFocusRepaintsOnTabTransitions(t *testing.T) {
	old := focusWidget
	t.Cleanup(func() { focusWidget = old })
	c := new(comboFocusSpy)
	c.ComboBox.Init(c)
	c.SetEditable(false)
	before := c.updates
	c.SetFocus()
	if c.updates <= before {
		t.Fatal("focus enter did not repaint selection-only frame")
	}
	before = c.updates
	NewEdit().SetFocus()
	if c.updates <= before {
		t.Fatal("focus leave retained stale frame")
	}
}

func TestComboEditableHasSingleAutomaticTabStop(t *testing.T) {
	root := NewForm()
	previous, next := NewEdit(), NewEdit()
	previous.SetParent(root)
	c := NewComboBox()
	c.SetParent(root)
	next.SetParent(root)
	for _, editable := range []bool{true, false, true} {
		c.SetEditable(editable)
		var target IWidget = c
		if editable {
			target = c.EditWidget()
		}
		if got := nextFocusable(root, previous, true); got != target {
			t.Fatalf("editable=%v Tab landed on %T, want %T", editable, got, target)
		}
		if got := nextFocusable(root, next, false); got != target {
			t.Fatalf("editable=%v reverse Tab landed on %T", editable, got)
		}
		if nextFocusable(root, target, true) != next || nextFocusable(root, target, false) != previous {
			t.Fatal("composite added an extra Tab stop")
		}
	}
	// An explicit policy still takes precedence over the automatic proxy.
	c.SetFocusPolicy(TabFocus)
	if nextFocusable(root, previous, true) != c {
		t.Fatal("explicit TabFocus ignored")
	}
}
