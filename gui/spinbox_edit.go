package gui

import (
	"math"
	"strconv"

	"github.com/uk0/silk/geom"
)

// The text field is a logical child, never a second native window. Reusing Edit
// gives this composite selection, clipboard, bounded undo and IME routing.
type spinBoxEdit struct {
	Edit
	spin *SpinBox
}

func newSpinBoxEdit(s *SpinBox) *spinBoxEdit {
	e := &spinBoxEdit{spin: s}
	e.Edit.Init(e)
	e.SetNoFrame(true)
	e.SetPadding(Padding{})
	e.SetRedrawParent(true)
	e.SetValidator(spinIntegerValidator{})
	return e
}

// Reject oversized drafts rather than truncating a pasted number into another value.
type spinIntegerValidator struct{}

func (spinIntegerValidator) Validate(text string) State {
	if len(text) > 64 {
		return Invalid
	}
	return NewIntValidator(int64(math.MinInt), int64(math.MaxInt)).Validate(text)
}

func (s *SpinBox) tabFocusProxy() IWidget { return s.editor }
func (s *SpinBox) SetFocus() {
	if s.editor != nil {
		s.editor.SetFocus()
	} else {
		s.Widget.SetFocus()
	}
}
func (s *SpinBox) HasFocus() bool {
	return s.Widget.HasFocus() || s.editor != nil && s.editor.HasFocus()
}
func (s *SpinBox) IsHover() bool { return s.Widget.IsHover() || s.editor != nil && s.editor.IsHover() }
func (s *SpinBox) SelectAll() {
	if s.editor != nil {
		s.editor.SelectAll()
		s.Update()
	}
}

// Text returns the draft number without the non-editable suffix.
func (s *SpinBox) Text() string {
	if s.editor != nil {
		return s.editor.Text()
	}
	return strconv.Itoa(s.value)
}
func (s *SpinBox) OnTextInput(text string) {
	if s.editor != nil {
		s.editor.OnTextInput(text)
	}
}
func (s *SpinBox) IsReadOnly() bool { return s.editor != nil && s.editor.IsReadOnly() }
func (s *SpinBox) SetReadOnly(b bool) {
	s.CancelEdit()
	if s.editor != nil {
		s.editor.SetReadOnly(b)
	}
	s.Update()
}
func (s *SpinBox) SetEnabled(b bool) {
	if !b {
		s.CancelEdit()
	}
	s.Widget.SetEnabled(b)
	if s.editor != nil {
		s.editor.SetEnabled(b)
	}
	s.Update()
}
func (s *SpinBox) OnHide() { s.CancelEdit() }
func (s *SpinBox) syncEditor() bool {
	if s.editor == nil {
		return false
	}
	text := strconv.Itoa(s.value)
	if s.editor.Text() == text {
		s.editor.CancelPendingPaste()
		s.editor.ClearUndoHistory()
		return false
	}
	s.editor.SetText(text)
	return true
}

// CancelEdit discards the draft and field-local history, leaving Value unchanged.
func (s *SpinBox) CancelEdit() { s.syncEditor(); s.Update() }

// Commit accepts a complete int and clamps it to Range. Incomplete input is
// restored to Value and returns false. Notifications fire once, only on change.
func (s *SpinBox) Commit() bool {
	if s.editor == nil {
		return false
	}
	if !s.IsEnabled() || s.IsReadOnly() {
		s.CancelEdit()
		return false
	}
	v, err := strconv.Atoi(s.editor.Text())
	if err != nil {
		s.CancelEdit()
		return false
	}
	s.SetValue(v)
	return true
}
func (s *SpinBox) stepBy(delta int) {
	if !s.IsEnabled() || s.IsReadOnly() {
		return
	}
	// Resolve the draft and the requested step in one transaction/callback.
	v := s.value
	if parsed, err := strconv.Atoi(s.Text()); err == nil {
		v = max(s.min, min(s.max, parsed))
	}
	s.SetValue(spinStepped(v, delta, s.min, s.max))
	s.SelectAll()
}
func (e *spinBoxEdit) available() bool {
	return e.spin != nil && e.Parent() == e.spin.Self() && editContextAvailable(&e.Edit)
}
func (e *spinBoxEdit) OnTextInput(s string) {
	if e.available() {
		e.Edit.OnTextInput(s)
	}
}
func (e *spinBoxEdit) OnMouseWheel(x, y, z float64) {
	if e.available() {
		e.spin.OnMouseWheel(x, y, z)
	}
}
func (e *spinBoxEdit) OnKeyDown(key int, repeat bool) {
	if !e.available() {
		return
	}
	s := e.spin
	if currentMods() == 0 {
		switch key {
		case KeyEnter:
			s.Commit()
			s.SelectAll()
			return
		case KeyEsc:
			s.CancelEdit()
			s.SelectAll()
			return
		case KeyUp:
			s.stepBy(s.Step())
			return
		case KeyDown:
			s.stepBy(-s.Step())
			return
		case KeyPageUp:
			s.stepBy(s.PageStep())
			return
		case KeyPageDown:
			s.stepBy(-s.PageStep())
			return
		// Retain the SDK's established plain Home/End range navigation.
		// Shift+Home/End and other editing shortcuts stay with the Edit.
		case KeyHome:
			if !s.IsReadOnly() {
				s.SetValue(s.Min())
				s.SelectAll()
			}
			return
		case KeyEnd:
			if !s.IsReadOnly() {
				s.SetValue(s.Max())
				s.SelectAll()
			}
			return
		}
	}
	e.Edit.OnKeyDown(key, repeat)
}
func (e *spinBoxEdit) OnFocusChanged(n, o IWidget) {
	if o == e && n != e {
		// Opening our context menu is not an editing completion boundary.
		inside := false
		for p := n; p != nil; p = p.Parent() {
			if p == e {
				inside = true
				break
			}
		}
		if !inside {
			if e.available() {
				e.spin.Commit()
			} else {
				e.spin.CancelEdit()
			}
		}
	}
	e.Edit.OnFocusChanged(n, o)
	e.spin.Update()
}
func (s *SpinBox) suffixRect() geom.Rect {
	w, h := s.Size()
	w, h = math.Max(0, w), math.Max(0, h)
	available := math.Max(0, w-s.buttonWidth()-8)
	width := 0.0
	if s.suffix != "" {
		width = math.Min(Theme().Font.TextExtents(s.suffix).Width, math.Max(0, available-32))
	}
	return geom.Rect{X: math.Min(w, 4+available-width), Width: width, Height: h}
}
func (s *SpinBox) Layout() {
	s.layoutTheme = ThemeRev()
	if s.editor == nil {
		return
	}
	w, h := s.Size()
	w, h = math.Max(0, w), math.Max(0, h)
	suffix := s.suffixRect()
	textH := math.Min(math.Max(0, h-4), Theme().Font.FontExtents().Height)
	s.editor.SetBounds(math.Min(4, w), (h-textH)/2, math.Max(0, suffix.X-math.Min(4, w)), textH)
}
