package gui

import "testing"

// Keyboard / text-input routing tests for SearchBox, NumberInput and Button.
//
// The window dispatches keys via target.(IEventKeyDown).OnKeyDown(key, repeat)
// and typed characters via target.(IEventTextInput).OnTextInput(string) (see
// gui/window_glfw.go onKey / onChar). These widgets previously declared
// OnKeyDown(key, mods int) and OnChar(rune) — signatures that satisfy neither
// interface, so every keystroke and character was silently dropped. The tests
// below drive the corrected handlers directly and assert observable state; no
// GLFW window is created (Update() is a no-op without a host window).

// Compile-time proof the widgets satisfy the exact interfaces the window
// dispatches. A signature regression breaks the build here.
var (
	_ IEventKeyDown   = (*SearchBox)(nil)
	_ IEventTextInput = (*SearchBox)(nil)
	_ IEventKeyDown   = (*NumberInput)(nil)
	_ IEventTextInput = (*NumberInput)(nil)
	_ IEventKeyDown   = (*Button)(nil)
)

// TestKeyboardInputInterfaceRouting mirrors the window's dispatch-site type
// assertions. Unless each succeeds at runtime, onKey / onChar drop the event.
func TestKeyboardInputInterfaceRouting(t *testing.T) {
	if _, ok := interface{}(NewSearchBox()).(IEventKeyDown); !ok {
		t.Error("SearchBox must implement IEventKeyDown")
	}
	if _, ok := interface{}(NewSearchBox()).(IEventTextInput); !ok {
		t.Error("SearchBox must implement IEventTextInput")
	}
	if _, ok := interface{}(NewNumberInput()).(IEventKeyDown); !ok {
		t.Error("NumberInput must implement IEventKeyDown")
	}
	if _, ok := interface{}(NewNumberInput()).(IEventTextInput); !ok {
		t.Error("NumberInput must implement IEventTextInput")
	}
	if _, ok := interface{}(NewButton()).(IEventKeyDown); !ok {
		t.Error("Button must implement IEventKeyDown")
	}
}

// TestSearchBoxKeyboardInput drives typed text (OnTextInput), caret navigation
// and editing (OnKeyDown), and the Enter->search commit through the corrected
// handlers, asserting the visible text, the fired query and the search value.
func TestSearchBoxKeyboardInput(t *testing.T) {
	sb := NewSearchBox()

	var lastQuery string
	changes := 0
	sb.SigTextChanged(func(s string) {
		lastQuery = s
		changes++
	})

	// Typing appends at the caret and fires the change (search filter) each time.
	sb.OnTextInput("h")
	sb.OnTextInput("i")
	if sb.Text() != "hi" {
		t.Fatalf("after typing: Text()=%q want %q", sb.Text(), "hi")
	}
	if lastQuery != "hi" || changes != 2 {
		t.Fatalf("change callback: lastQuery=%q changes=%d want %q/2", lastQuery, changes, "hi")
	}

	// Home moves the caret to the start; the next character inserts there.
	sb.OnKeyDown(KeyHome, false)
	sb.OnTextInput("X")
	if sb.Text() != "Xhi" {
		t.Fatalf("after Home+insert: Text()=%q want %q", sb.Text(), "Xhi")
	}

	// End moves the caret to the tail; Backspace deletes the rune before it.
	sb.OnKeyDown(KeyEnd, false)
	sb.OnKeyDown(KeyBackSpace, false)
	if sb.Text() != "Xh" {
		t.Fatalf("after End+Backspace: Text()=%q want %q", sb.Text(), "Xh")
	}
	if lastQuery != "Xh" {
		t.Fatalf("backspace should fire change: lastQuery=%q want %q", lastQuery, "Xh")
	}

	// Enter commits the current text to the search callback.
	var searched string
	searchFired := 0
	sb.SigSearch(func(s string) {
		searched = s
		searchFired++
	})
	sb.OnKeyDown(KeyEnter, false)
	if searched != "Xh" || searchFired != 1 {
		t.Fatalf("Enter->search: searched=%q fired=%d want %q/1", searched, searchFired, "Xh")
	}
}

// TestNumberInputKeyboardInput covers the two OnKeyDown modes (arrow stepping
// while not editing, Enter commit while editing), the OnTextInput numeric
// filter, and that OnTextInput is inert when the field is not being edited.
func TestNumberInputKeyboardInput(t *testing.T) {
	// Not editing: Up/Down arrows step the value through OnKeyDown.
	ni := NewNumberInput()
	ni.SetRange(0, 100)
	ni.SetStep(1)
	ni.SetValue(5)
	ni.OnKeyDown(KeyUp, false)
	if ni.Value() != 6 {
		t.Fatalf("KeyUp step: Value()=%v want 6", ni.Value())
	}
	ni.OnKeyDown(KeyDown, false)
	if ni.Value() != 5 {
		t.Fatalf("KeyDown step: Value()=%v want 5", ni.Value())
	}

	// Not editing: typed characters are ignored (no edit buffer to append to).
	ni.OnTextInput("9")
	if ni.editText != "" {
		t.Fatalf("OnTextInput while not editing must be inert, editText=%q", ni.editText)
	}

	// Editing: typed digits accumulate, non-numeric runes are filtered, Enter
	// commits the parsed value.
	ni2 := NewNumberInput()
	ni2.SetRange(0, 1000)
	ni2.editing = true
	ni2.editText = ""
	ni2.OnTextInput("4")
	ni2.OnTextInput("x") // filtered: not a digit / dot / minus
	ni2.OnTextInput("2")
	if ni2.editText != "42" {
		t.Fatalf("editing buffer: editText=%q want %q", ni2.editText, "42")
	}
	ni2.OnKeyDown(KeyEnter, false)
	if ni2.editing {
		t.Fatal("Enter should commit and leave editing mode")
	}
	if ni2.Value() != 42 {
		t.Fatalf("Enter commit: Value()=%v want 42", ni2.Value())
	}
}

// TestButtonKeyboardActivation verifies Enter and Space fire the button's
// action (the same emit path a mouse click uses), a disabled button ignores
// keys, and a non-activation key is a no-op.
func TestButtonKeyboardActivation(t *testing.T) {
	btn := NewButton1("ok", nil)
	fired := 0
	btn.Action().BindFunc0(func() { fired++ })

	btn.OnKeyDown(KeyEnter, false)
	if fired != 1 {
		t.Fatalf("Enter should activate: fired=%d want 1", fired)
	}
	btn.OnKeyDown(KeySpace, false)
	if fired != 2 {
		t.Fatalf("Space should activate: fired=%d want 2", fired)
	}

	// Non-activation key does nothing.
	btn.OnKeyDown(KeyLeft, false)
	if fired != 2 {
		t.Fatalf("KeyLeft should not activate: fired=%d want 2", fired)
	}

	// Disabled button ignores keys.
	btn.SetEnabled(false)
	btn.OnKeyDown(KeyEnter, false)
	if fired != 2 {
		t.Fatalf("disabled button must ignore Enter: fired=%d want 2", fired)
	}
}

// TestButtonJoinsFocusChain verifies that implementing IEventKeyDown opts the
// button into the Tab focus chain via focus.go's AutoFocus heuristic. Before
// the fix Button implemented no key interface and was skipped.
func TestButtonJoinsFocusChain(t *testing.T) {
	btn := NewButton1("ok", nil)
	if !isTabFocusable(btn) {
		t.Fatal("a visible, enabled Button implementing IEventKeyDown should be Tab-focusable")
	}
	// NoFocus still excludes it explicitly.
	btn.NakedWidget().SetFocusPolicy(NoFocus)
	if isTabFocusable(btn) {
		t.Fatal("NoFocus must exclude the button from the focus chain")
	}
}
