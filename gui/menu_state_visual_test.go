package gui

import "testing"

func TestPopupCheckedStateDoesNotMaskKeyboardHighlight(t *testing.T) {
	oldMode, oldHover := CurrentThemeMode(), mouseHoverWidget
	t.Cleanup(func() { SetThemeMode(oldMode); mouseHoverWidget = oldHover })
	for _, mode := range []ThemeMode{ThemeLight, ThemeDark} {
		SetThemeMode(mode)
		m := NewPopupMenu()
		a := m.AddButton1("Properties", nil)
		a.Action().SetChecked(true)
		a.SetSize(180, 28)
		setMouseHoverWidget(nil)
		checked := newThemeFillRecorder()
		checked.fill = Theme().MenuBGColor
		Theme().DrawButton(checked, a)
		setMouseHoverWidget(a)
		active := newThemeFillRecorder()
		active.fill = Theme().MenuBGColor
		Theme().DrawButton(active, a)
		if checked.fill == active.fill {
			t.Fatal("checked row and active keyboard target have identical fills")
		}
		if contrastRatio(checked.ink, checked.fill) < 4.5 || contrastRatio(active.ink, active.fill) < 4.5 {
			t.Fatal("menu label contrast regressed")
		}
	}
}

type menuTickRecorder struct {
	*shortcutRecorder
	points  [][2]float64
	strokes int
}

func (r *menuTickRecorder) MoveTo(x, y float64) { r.points = append(r.points, [2]float64{x, y}) }
func (r *menuTickRecorder) LineTo(x, y float64) { r.points = append(r.points, [2]float64{x, y}) }
func (r *menuTickRecorder) Stroke()             { r.strokes++ }

func TestPopupCheckmarkDoesNotOverlapLabelShortcutOrSubmenu(t *testing.T) {
	oldHover := mouseHoverWidget
	t.Cleanup(func() { mouseHoverWidget = oldHover })
	setMouseHoverWidget(nil)
	m := NewPopupMenu()
	a := m.AddButton1("Current panel", nil)
	a.SetShortcutText("Ctrl+Shift+P")
	a.Action().SetChecked(true)
	hints := a.SizeHints()
	a.SetSize(hints.Width, hints.Height)
	r := &menuTickRecorder{shortcutRecorder: &shortcutRecorder{focusRingRecorder: newFocusRingRecorder()}}
	Theme().DrawButton(r, a)
	if r.strokes != 1 || len(r.points) != 3 || len(r.texts) != 2 {
		t.Fatalf("missing distinct tick: strokes=%d points=%v texts=%v", r.strokes, r.points, r.texts)
	}
	hint := r.texts[1]
	end := hint.x + Theme().Font.TextExtents(hint.text).Width
	for _, p := range r.points {
		if p[0] < end+4 || p[0] >= hints.Width || p[1] < 1 || p[1] >= hints.Height {
			t.Fatal("tick overlaps text or escapes row", p, end, hints)
		}
	}

	// Plain labels also retain a visible gap at their natural minimum width.
	a.SetShortcutText("")
	hints = a.SizeHints()
	a.SetSize(hints.Width, hints.Height)
	r.points = nil
	r.texts = nil
	r.strokes = 0
	Theme().DrawButton(r, a)
	label := r.texts[0]
	end = label.x + Theme().Font.TextExtents(label.text).Width
	for _, p := range r.points {
		if p[0] < end+4 {
			t.Fatal("checkmark touches label", p, end)
		}
	}
	a.SetSubPopup(NewPopupMenu())
	r.points = nil
	r.strokes = 0
	Theme().DrawButton(r, a)
	if r.strokes != 0 || len(r.points) != 4 {
		t.Fatal("checkmark painted over submenu arrow")
	}
}
