package prop

import (
	"github.com/uk0/silk/gui"
	"testing"
)

type sessionOwner struct{ stack *gui.UndoStack }

func (o *sessionOwner) PushCommand(c gui.ICommand) { o.stack.Push(c) }
func sessionInt(t *testing.T) (*IntEdit, *PropertySheet, *gui.UndoStack, *int) {
	t.Helper()
	v := -42
	o := &sessionOwner{gui.NewUndoStack("property-session")}
	obj := &fakeObj{enum: func(l IPropertyList) { l.AddProperty("N", func() int { return v }, func(n int) { v = n }) }}
	s := bindSheet([]interface{}{obj}, o)
	return s.propMap["N"].Control().(*IntEdit), s, o.stack, &v
}
func TestPropertyTypingSessionUndoRedo(t *testing.T) {
	e, s, stack, v := sessionInt(t)
	e.SelectAll()
	for _, ch := range []string{"-", "5", "7"} {
		e.OnTextInput(ch)
	}
	if *v != -57 {
		t.Fatal("live preview stopped updating", *v)
	}
	e.OnSubmit(e.Text())
	if stack.Count() != 1 {
		t.Fatalf("one field edit created %d commands", stack.Count())
	}
	stack.Undo()
	s.RefreshValues()
	if *v != -42 || e.Text() != "-42" {
		t.Fatal("undo did not restore original", *v, e.Text())
	}
	stack.Redo()
	s.RefreshValues()
	if *v != -57 || e.Text() != "-57" {
		t.Fatal("redo did not restore final", *v, e.Text())
	}
}
func TestPropertyTextAndFloatTypingSessions(t *testing.T) {
	t.Run("text", func(t *testing.T) {
		v := "original"
		o := &sessionOwner{gui.NewUndoStack("text")}
		obj := &fakeObj{enum: func(l IPropertyList) { l.AddProperty("Text", func() string { return v }, func(n string) { v = n }) }}
		s := bindSheet([]interface{}{obj}, o)
		e := s.propMap["Text"].Control().(*TextEdit)
		e.SelectAll()
		for _, ch := range []string{"S", "i", "l", "k", " IDE"} {
			e.OnTextInput(ch)
		}
		e.OnSubmit(e.Text())
		e.Deactivate()
		if v != "Silk IDE" || o.stack.Count() != 1 {
			t.Fatal(v, o.stack.Count())
		}
		o.stack.Undo()
		if v != "original" {
			t.Fatal(v)
		}
		o.stack.Redo()
		if v != "Silk IDE" {
			t.Fatal(v)
		}
	})
	t.Run("float", func(t *testing.T) {
		v := 12.5
		o := &sessionOwner{gui.NewUndoStack("float")}
		obj := &fakeObj{enum: func(l IPropertyList) { l.AddProperty("F", func() float64 { return v }, func(n float64) { v = n }) }}
		s := bindSheet([]interface{}{obj}, o)
		e := s.propMap["F"].Control().(*FloatEdit)
		e.SelectAll()
		for _, ch := range []string{"2", ".", "7", "5"} {
			e.OnTextInput(ch)
		}
		e.OnSubmit(e.Text())
		e.Deactivate()
		if v != 2.75 || o.stack.Count() != 1 {
			t.Fatal(v, o.stack.Count())
		}
		o.stack.Undo()
		if v != 12.5 {
			t.Fatal(v)
		}
		o.stack.Redo()
		if v != 2.75 {
			t.Fatal(v)
		}
	})
}
func TestPropertyTypingSessionRestoresMixedSelection(t *testing.T) {
	a, b := -42, 8
	o := &sessionOwner{gui.NewUndoStack("mixed")}
	obj := func(v *int) *fakeObj {
		return &fakeObj{enum: func(l IPropertyList) { l.AddProperty("N", func() int { return *v }, func(n int) { *v = n }) }}
	}
	s := bindSheet([]interface{}{obj(&a), obj(&b)}, o)
	e := s.propMap["N"].Control().(*IntEdit)
	e.Deactivate()
	for _, ch := range []string{"-", "5", "7"} {
		e.OnTextInput(ch)
	}
	e.Deactivate()
	if a != -57 || b != -57 || o.stack.Count() != 1 {
		t.Fatal(a, b, o.stack.Count())
	}
	o.stack.Undo()
	s.RefreshValues()
	if a != -42 || b != 8 || e.Text() != "" {
		t.Fatal("mixed undo", a, b, e.Text())
	}
	o.stack.Redo()
	s.RefreshValues()
	if a != -57 || b != -57 {
		t.Fatal(a, b)
	}
}
func TestPropertyTypingSessionCommitAndFocusBoundaries(t *testing.T) {
	for _, boundary := range []string{"enter", "blur", "refresh", "hide", "disable", "readOnly"} {
		t.Run(boundary, func(t *testing.T) {
			e, s, stack, v := sessionInt(t)
			e.SetFocus()
			e.SelectAll()
			e.OnTextInput("1")
			switch boundary {
			case "enter":
				e.OnSubmit(e.Text())
			case "blur":
				gui.NewEdit().SetFocus()
			case "refresh":
				s.RefreshValues()
			case "hide":
				e.SetVisible(false)
				e.SetVisible(true)
			case "disable":
				e.SetEnabled(false)
				e.SetEnabled(true)
			case "readOnly":
				e.SetReadOnly(true)
				e.SetReadOnly(false)
			}
			e.SelectAll()
			e.OnTextInput("2")
			e.OnTextInput("3")
			e.OnSubmit(e.Text())
			if *v != 23 || stack.Count() != 2 {
				t.Fatal(*v, stack.Count())
			}
			stack.Undo()
			if *v != 1 {
				t.Fatal("boundary merged", *v)
			}
			stack.Undo()
			if *v != -42 {
				t.Fatal(*v)
			}
		})
	}
}
func TestPropertyTypingSessionPreservesSaveAndBranch(t *testing.T) {
	e, _, stack, v := sessionInt(t)
	e.SelectAll()
	e.OnTextInput("1")
	stack.SetClean()
	e.OnTextInput("2")
	if *v != 12 || stack.Count() != 2 || stack.IsClean() {
		t.Fatal("save boundary", *v, stack.Count())
	}
	stack.Undo()
	if *v != 1 || !stack.IsClean() {
		t.Fatal("saved state unreachable")
	}
	stack.Redo()
	e.SelectAll()
	e.OnTextInput("3")
	e.OnTextInput("4")
	if stack.Count() != 3 {
		t.Fatal("merged across undo/redo", stack.Count())
	}
	stack.Undo()
	if *v != 12 {
		t.Fatal(*v)
	}
	e.SelectAll()
	e.OnTextInput("5")
	e.OnTextInput("6")
	if stack.CanRedo() || stack.Count() != 3 || *v != 56 {
		t.Fatal("branch", *v, stack.Count())
	}
	stack.Undo()
	if *v != 12 {
		t.Fatal("branch lost its baseline", *v)
	}
}
func TestPropertyTypingSessionRejectsStaleAndNoOpChanges(t *testing.T) {
	e, s, stack, v := sessionInt(t)
	e.OnEdited("-42")
	e.Deactivate()
	if stack.Count() != 0 {
		t.Fatal("no-op history")
	}
	e.SelectAll()
	e.OnTextInput("1")
	*v = 70 // external model update, even before the host refreshes the row
	e.OnTextInput("2")
	if *v != 12 || stack.Count() != 2 {
		t.Fatal("external value was absorbed", *v, stack.Count())
	}
	stack.Undo()
	if *v != 70 {
		t.Fatal(*v)
	}
	s.Clear(nil)
	e.OnEdited("99")
	e.Deactivate()
	if *v != 70 {
		t.Fatal("retired editor wrote stale binding")
	}
}

func TestPropertyTypingSessionRepeatedSteps(t *testing.T) {
	e, _, stack, v := sessionInt(t)
	e.OnKeyDown(gui.KeyUp, false)
	e.OnKeyDown(gui.KeyUp, true)
	e.OnKeyDown(gui.KeyUp, true)
	e.OnKeyUp(gui.KeyUp)
	if *v != -39 || stack.Count() != 1 {
		t.Fatal(*v, stack.Count())
	}
	e.OnKeyDown(gui.KeyUp, false)
	e.OnKeyUp(gui.KeyUp)
	if *v != -38 || stack.Count() != 2 {
		t.Fatal(*v, stack.Count())
	}
	stack.Undo()
	if *v != -39 {
		t.Fatal(*v)
	}
	stack.Undo()
	if *v != -42 {
		t.Fatal(*v)
	}
}
func TestPropertyTypingSessionUsesNormalizedReadback(t *testing.T) {
	v := 3
	o := &sessionOwner{gui.NewUndoStack("normalized")}
	obj := &fakeObj{enum: func(l IPropertyList) {
		l.AddProperty("N", func() int { return v }, func(n int) { v = max(0, min(10, n)) })
	}}
	s := bindSheet([]interface{}{obj}, o)
	e := s.propMap["N"].Control().(*IntEdit)
	e.SelectAll()
	e.OnTextInput("9")
	e.OnTextInput("9")
	e.OnTextInput("9")
	e.OnSubmit(e.Text())
	if v != 10 || e.Text() != "10" || o.stack.Count() != 1 {
		t.Fatal(v, e.Text(), o.stack.Count())
	}
	o.stack.Undo()
	if v != 3 {
		t.Fatal(v)
	}
	o.stack.Redo()
	if v != 10 {
		t.Fatal(v)
	}
}
func TestPropertyTypingSessionContextMenuAndDisabledParent(t *testing.T) {
	e, s, stack, v := sessionInt(t)
	e.SetFocus()
	e.SelectAll()
	e.OnTextInput("1")
	menu := gui.NewEdit()
	menu.SetParent(e)
	menu.SetFocus()
	e.SetFocus()
	e.OnTextInput("2")
	if *v != 12 || stack.Count() != 1 {
		t.Fatal("owned menu split a field edit", *v, stack.Count())
	}
	s.SetEnabled(false)
	e.OnEdited("99")
	if *v != 12 {
		t.Fatal("disabled ancestor accepted a write")
	}
	s.SetEnabled(true)
	s.SetVisible(false)
	e.OnEdited("99")
	if *v != 12 {
		t.Fatal("hidden ancestor accepted a write")
	}
}
func TestPropertyTypingSessionDoesNotMergeSeparateRows(t *testing.T) {
	a, b := 1, 2
	o := &sessionOwner{gui.NewUndoStack("rows")}
	obj := &fakeObj{enum: func(l IPropertyList) {
		l.AddProperty("A", func() int { return a }, func(n int) { a = n })
		l.AddProperty("B", func() int { return b }, func(n int) { b = n })
	}}
	s := bindSheet([]interface{}{obj}, o)
	ea := s.propMap["A"].Control().(*IntEdit)
	eb := s.propMap["B"].Control().(*IntEdit)
	ea.SetFocus()
	ea.SelectAll()
	ea.OnTextInput("3")
	ea.OnTextInput("4")
	eb.SetFocus()
	eb.SelectAll()
	eb.OnTextInput("5")
	eb.OnTextInput("6")
	eb.Deactivate()
	if a != 34 || b != 56 || o.stack.Count() != 2 {
		t.Fatal(a, b, o.stack.Count())
	}
	o.stack.Undo()
	if a != 34 || b != 2 {
		t.Fatal(a, b)
	}
	o.stack.Undo()
	if a != 1 || b != 2 {
		t.Fatal(a, b)
	}
}
func TestPropertyTypingSessionRefreshDoesNotReenterSetter(t *testing.T) {
	v := "original"
	calls := 0
	o := &sessionOwner{gui.NewUndoStack("refresh")}
	obj := &fakeObj{enum: func(l IPropertyList) {
		l.AddProperty("Text", func() string { return v }, func(n string) { v = n; calls++ })
	}}
	s := bindSheet([]interface{}{obj}, o)
	e := s.propMap["Text"].Control().(*TextEdit)
	for i := 0; i < 3; i++ {
		s.RefreshValues()
		e.SetFocus()
		gui.NewEdit().SetFocus()
		e.Deactivate()
	}
	if calls != 0 || o.stack.Count() != 0 {
		t.Fatal("viewing pushed duplicate writes", calls, o.stack.Count())
	}
	e.SelectAll()
	for i := 0; i < 200; i++ {
		e.OnTextInput("x")
	}
	e.Deactivate()
	if calls != 200 || o.stack.Count() != 1 {
		t.Fatal("live writes or bounded history lost", calls, o.stack.Count())
	}
	o.stack.Undo()
	if v != "original" {
		t.Fatal(v)
	}
}
