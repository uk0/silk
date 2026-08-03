package gui

import "testing"

// TestSearchBoxClickPlacesCaret: clicking inside the text must move the caret
// to the clicked character boundary. The boundaries are the pen advances of
// the successive prefixes — the very offsets Draw paints the caret at — so a
// click at boundary n has to come back as cursorPos == n for every n. The
// sample contains spaces because a prefix ending in one has no ink past the
// previous glyph: measuring with the ink box instead of the advance puts the
// caret several pixels left of where the next character starts.
func TestSearchBoxClickPlacesCaret(t *testing.T) {
	const text = "a b c"
	sb := NewSearchBox()
	sb.SetSize(300, 26)
	sb.SetText(text)

	f := Theme().Font
	n := len([]rune(text))
	for i := 0; i <= n; i++ {
		sb.cursorPos = 0
		x := searchBoxTextX + searchBoxCaretX(text, f, i)
		sb.OnLeftDown(x, 13)
		if sb.cursorPos != i {
			t.Errorf("click at boundary %d (x=%.3f): cursorPos = %d, want %d",
				i, x, sb.cursorPos, i)
		}
	}
}

// TestSearchBoxClickSnapsToNearestBoundary: a click in the left half of a
// glyph puts the caret before it, one in the right half after it, and a click
// past the end of the text parks the caret at the end.
func TestSearchBoxClickSnapsToNearestBoundary(t *testing.T) {
	const text = "hello"
	sb := NewSearchBox()
	sb.SetSize(300, 26)
	sb.SetText(text)

	f := Theme().Font
	x1 := searchBoxCaretX(text, f, 1)
	x2 := searchBoxCaretX(text, f, 2)

	sb.OnLeftDown(searchBoxTextX+x1+(x2-x1)*0.2, 13)
	if sb.cursorPos != 1 {
		t.Errorf("click 20%% into the 2nd glyph: cursorPos = %d, want 1", sb.cursorPos)
	}
	sb.OnLeftDown(searchBoxTextX+x1+(x2-x1)*0.8, 13)
	if sb.cursorPos != 2 {
		t.Errorf("click 80%% into the 2nd glyph: cursorPos = %d, want 2", sb.cursorPos)
	}
	sb.OnLeftDown(searchBoxTextX+x2, 13)
	if sb.cursorPos != 2 {
		t.Errorf("click on the boundary after the 2nd glyph: cursorPos = %d, want 2", sb.cursorPos)
	}
	// Left of the text origin, and far to the right of the last glyph.
	sb.OnLeftDown(1, 13)
	if sb.cursorPos != 0 {
		t.Errorf("click left of the text: cursorPos = %d, want 0", sb.cursorPos)
	}
	sb.OnLeftDown(150, 13)
	if sb.cursorPos != 5 {
		t.Errorf("click past the text: cursorPos = %d, want 5", sb.cursorPos)
	}
}

// TestSearchBoxDisabledIgnoresInput: a disabled search box must ignore typed
// characters, editing keys and the clear button. Widget-level enablement is
// not enforced by the event dispatcher — each widget checks it itself.
func TestSearchBoxDisabledIgnoresInput(t *testing.T) {
	sb := NewSearchBox()
	sb.SetSize(200, 26)
	sb.SetText("keep")
	sb.SetEnabled(false)

	// Each case restarts from the same text so a typed character followed by a
	// backspace cannot cancel out and hide a live handler.
	cases := []struct {
		name string
		act  func()
	}{
		{"OnTextInput", func() { sb.OnTextInput("x") }},
		{"Backspace", func() { sb.OnKeyDown(KeyBackSpace, false) }},
		{"Delete", func() { sb.cursorPos = 0; sb.OnKeyDown(KeyDelete, false) }},
		// The clear button lives in the right-hand 24px strip.
		{"clear-button click", func() { sb.OnLeftDown(190, 13) }},
	}
	for _, c := range cases {
		sb.SetText("keep")
		c.act()
		if sb.Text() != "keep" {
			t.Errorf("disabled %s: Text() = %q, want %q", c.name, sb.Text(), "keep")
		}
	}
	// Enter must not fire the search callback either.
	fired := 0
	sb.SigSearch(func(string) { fired++ })
	sb.OnKeyDown(KeyEnter, false)
	if fired != 0 {
		t.Errorf("disabled Enter fired the search callback %d times, want 0", fired)
	}
}
