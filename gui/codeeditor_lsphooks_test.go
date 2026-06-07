package gui

import (
	"testing"
)

// ---------------------------------------------------------------------------
// LSP host hooks: SigHoverRequested + SigSignatureRequested
//
// These are host-driven signals. The editor only reports WHERE the user is
// (hover identifier / signature trigger) and lets the host (silkide) run the
// async gopls RPC and display the result. The editor never fetches or shows
// hover / signature text itself, so these tests assert the editor fires the
// callbacks with the right position — not any LSP behaviour. All paths here
// are GL-free: posFromXY / wordBoundsAt / OnTextInput run headless, exactly as
// the breakpoint and completer tests already drive them.
// ---------------------------------------------------------------------------

// isSignatureTrigger is a pure predicate: only "(" (call start) and ","
// (next argument) request signature help; ")" closes the call and must NOT.
func TestIsSignatureTrigger(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"(", true},
		{",", true},
		{")", false},
		{"a", false},
		{"", false},
		{"()", false}, // multi-char is not a single trigger keystroke
		{"(,", false}, // multi-char
		{" ", false},  // space
		{".", false},  // dot is a completion trigger, not signature
	}
	for _, c := range cases {
		if got := isSignatureTrigger(c.in); got != c.want {
			t.Errorf("isSignatureTrigger(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// SigSignatureRequested must fire on a "(" trigger with the cursor position
// AFTER the insert, and must NOT fire on an ordinary identifier rune.
func TestSigSignatureRequestedFires(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("foo")
	// Park the cursor at end of "foo" (line 0, col 3) so the "(" inserts there.
	e.cursorLine = 0
	e.cursorCol = 3

	var fired bool
	var gotLine, gotCol int
	e.SigSignatureRequested(func(line, col int) {
		fired = true
		gotLine, gotCol = line, col
	})

	// Typing "(" is a signature trigger.
	e.OnTextInput("(")
	if !fired {
		t.Fatalf("SigSignatureRequested did not fire on \"(\"")
	}
	// Cursor reported AFTER the insert: "foo(" -> col 4 on line 0.
	if gotLine != 0 || gotCol != 4 {
		t.Errorf("signature callback got (line=%d, col=%d), want (0, 4)", gotLine, gotCol)
	}

	// Typing an ordinary letter must NOT fire the signature signal.
	fired = false
	e.OnTextInput("a")
	if fired {
		t.Errorf("SigSignatureRequested fired on \"a\" (only \"(\" and \",\" should trigger)")
	}

	// A comma (next-argument trigger) fires too.
	fired = false
	e.OnTextInput(",")
	if !fired {
		t.Errorf("SigSignatureRequested did not fire on \",\"")
	}
}

// With no callback registered the signature path is a no-op and must not panic.
func TestSigSignatureRequestedNilSafe(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("x")
	e.cursorLine = 0
	e.cursorCol = 1
	// No SigSignatureRequested call — cbSignatureRequested stays nil.
	e.OnTextInput("(") // must not panic
}

// SigHoverRequested stores the callback (registration check).
func TestSigHoverRequestedRegistration(t *testing.T) {
	e := NewCodeEditor()
	if e.cbHoverRequested != nil {
		t.Fatalf("new editor should have no hover callback")
	}
	e.SigHoverRequested(func(line, col int, gx, gy float64) {})
	if e.cbHoverRequested == nil {
		t.Errorf("SigHoverRequested did not store the callback")
	}
}

// maybeFireHover fires once when the mouse settles over an identifier in the
// text area, fires AGAIN only when the identifier changes (word-start col),
// and not while the mouse stays within the same word. This mirrors the
// error-tooltip's fire-on-change gating and is driven headless via the
// editor's own font metrics (same y->line math as posFromXY).
func TestSigHoverRequestedFiresOnIdentifierChange(t *testing.T) {
	e := NewCodeEditor()
	e.SetSize(400, 300)
	// Two identifiers on line 0 with a space between them.
	e.SetText("alpha beta")

	lh := e.font.FontExtents().Height + 2
	topOff := e.topOffset()
	yLine0 := topOff + lh/2 // vertical centre of line 0

	// xForCol returns a click X near the centre of the given column, matching
	// posFromXY's xOff = x - gutterW - 10 + scrollX (scrollX == 0).
	xForCol := func(col int) float64 {
		runes := []rune(e.lines[0])
		if col > len(runes) {
			col = len(runes)
		}
		prev := e.measureText(string(runes[:col]))
		next := prev
		if col < len(runes) {
			next = e.measureText(string(runes[:col+1]))
		}
		return e.gutterW + 10 + (prev+next)/2
	}

	type hit struct {
		line, col int
		gx, gy    float64
	}
	var hits []hit
	e.SigHoverRequested(func(line, col int, gx, gy float64) {
		hits = append(hits, hit{line, col, gx, gy})
	})

	// Sanity: our chosen (x, y) over "alpha" resolves to an identifier there.
	if line, col, ok := e.hoverIdentAt(xForCol(1), yLine0); !ok || line != 0 || col != 0 {
		t.Fatalf("hoverIdentAt over 'alpha' = (line=%d, col=%d, ok=%v), want (0, 0, true)", line, col, ok)
	}

	// 1) Settle on "alpha" (col 0..5): fires once at word-start col 0.
	e.maybeFireHover(xForCol(1), yLine0)
	if len(hits) != 1 {
		t.Fatalf("first hover over 'alpha' should fire once, got %d events", len(hits))
	}
	if hits[0].line != 0 || hits[0].col != 0 {
		t.Errorf("hover event = (line=%d, col=%d), want (0, 0)", hits[0].line, hits[0].col)
	}

	// 2) Move WITHIN "alpha" (col 3): same identifier -> no new event.
	e.maybeFireHover(xForCol(3), yLine0)
	if len(hits) != 1 {
		t.Errorf("moving within 'alpha' must not re-fire; got %d events", len(hits))
	}

	// 3) Move onto "beta" (starts at col 6): a NEW identifier -> one more event.
	e.maybeFireHover(xForCol(7), yLine0)
	if len(hits) != 2 {
		t.Fatalf("moving onto 'beta' should fire again, got %d events", len(hits))
	}
	if hits[1].col != 6 {
		t.Errorf("second hover event word-start col = %d, want 6 ('beta')", hits[1].col)
	}
}

// maybeFireHover must NOT fire in the gutter, off any identifier, while the
// completion popup is visible, or when no callback is registered.
func TestSigHoverRequestedSuppression(t *testing.T) {
	e := NewCodeEditor()
	e.SetSize(400, 300)
	e.SetText("alpha beta")

	lh := e.font.FontExtents().Height + 2
	topOff := e.topOffset()
	yLine0 := topOff + lh/2

	var fired int
	e.SigHoverRequested(func(line, col int, gx, gy float64) { fired++ })

	// Gutter (x < gutterW): never an identifier.
	e.maybeFireHover(e.gutterW/2, yLine0)
	if fired != 0 {
		t.Errorf("hover fired in the gutter, want suppressed")
	}

	// Over the space between words: x at the left edge of the space glyph
	// resolves to col 5 (the ' '), which is not an identifier rune.
	if _, _, ok := e.hoverIdentAt(e.gutterW+10+e.measureText("alpha"), yLine0); ok {
		t.Fatalf("hoverIdentAt over the inter-word space reported an identifier")
	}
	spaceX := e.gutterW + 10 + e.measureText("alpha")
	e.hoverReqLine, e.hoverReqCol = -1, -1
	e.maybeFireHover(spaceX, yLine0)
	if fired != 0 {
		t.Errorf("hover fired over whitespace, want suppressed")
	}

	// Completion popup visible -> suppressed even over an identifier.
	e.completion = NewCompletionPopup(e)
	e.completion.visible = true
	e.hoverReqLine, e.hoverReqCol = -1, -1
	textX := e.gutterW + 12 // inside "alpha"
	e.maybeFireHover(textX, yLine0)
	if fired != 0 {
		t.Errorf("hover fired while completion popup visible, want suppressed")
	}
	e.completion.visible = false

	// No callback registered -> no-op, no panic.
	e2 := NewCodeEditor()
	e2.SetSize(400, 300)
	e2.SetText("alpha")
	e2.maybeFireHover(e2.gutterW+12, topOff+lh/2)
}
