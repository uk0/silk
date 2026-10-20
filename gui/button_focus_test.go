package gui

import (
	"fmt"
	"testing"

	"github.com/uk0/silk/paint"
)

// TestButtonFocusRingStates pins the ring's state table. Both terms have to
// carry their own row: a picker that forgot `focused` still rings the dialog
// default, and one that forgot `isDefault` still rings on Tab, so only the two
// single-term rows below can tell either omission apart from the real rule.
func TestButtonFocusRingStates(t *testing.T) {
	cases := []struct {
		name      string
		focused   bool
		isDefault bool
		enabled   bool
		want      bool
	}{
		{"focused only", true, false, true, true},
		{"default only", false, true, true, true},
		{"focused and default", true, true, true, true},
		{"idle", false, false, true, false},
		{"disabled focused", true, false, false, false},
		{"disabled default", false, true, false, false},
		{"disabled focused default", true, true, false, false},
		{"disabled idle", false, false, false, false},
	}
	for _, tc := range cases {
		got := buttonFocusRing(tc.focused, tc.isDefault, tc.enabled)
		if got != tc.want {
			t.Errorf("%s: buttonFocusRing(focused=%v, isDefault=%v, enabled=%v) = %v, want %v",
				tc.name, tc.focused, tc.isDefault, tc.enabled, got, tc.want)
		}
	}
}

// focusRingRecorder is a no-op paint.Painter (embedding nopPainter for the
// methods the button path never reaches) that keeps every stroke's pen and
// width. Counting strokes rather than keeping only the last one is what lets
// the assertions say "exactly one accent ring": the state faces stroke their
// own 1px border first, and a second ring would otherwise hide behind the
// first. Mirrors the recorder pattern in theme_headerbg_test.go.
type focusRingRecorder struct {
	paint.Painter
	brush   paint.Color
	pen     paint.Color
	penW    float64
	font    paint.Font
	strokes []strokeOp
}

type strokeOp struct {
	pen   paint.Color
	width float64
}

func (r *focusRingRecorder) Arc(xc, yc, radius, angle1, angle2 float64) {}
func (r *focusRingRecorder) LineTo(x, y float64)                        {}
func (r *focusRingRecorder) MoveTo(x, y float64)                        {}
func (r *focusRingRecorder) Rectangle(x, y, w, h float64)               {}
func (r *focusRingRecorder) Translate(tx, ty float64)                   {}
func (r *focusRingRecorder) SetBrush1(c paint.Color)                    { r.brush = c }
func (r *focusRingRecorder) SetBrush(paint.Brush)                       {}
func (r *focusRingRecorder) Fill()                                      {}
func (r *focusRingRecorder) FillPreserve()                              {}
func (r *focusRingRecorder) SetPen1(c paint.Color, w float64)           { r.pen, r.penW = c, w }
func (r *focusRingRecorder) Stroke() {
	r.strokes = append(r.strokes, strokeOp{r.pen, r.penW})
}
func (r *focusRingRecorder) SetFont(f paint.Font) { r.font = f }
func (r *focusRingRecorder) Font() paint.Font     { return r.font }
func (r *focusRingRecorder) DrawText(text string) {}

func newFocusRingRecorder() *focusRingRecorder {
	return &focusRingRecorder{Painter: nopPainter{}}
}

// rings counts the strokes that look like the focus ring: accent pen at 2px.
// Every other stroke DrawButton makes is a 1px state border.
func (r *focusRingRecorder) rings(accent paint.Color) int {
	n := 0
	for _, s := range r.strokes {
		if s.pen == accent && s.width == 2 {
			n++
		}
	}
	return n
}

// TestButtonDrawFocusRing drives the real theme DrawButton and asserts the ring
// actually reaches the painter. Button joins the Tab chain through the
// AutoFocus heuristic (it implements IEventKeyDown), but DrawButton used to read
// only disabled/pressed/hover — so Tab moved focus through a dialog with nothing
// on screen changing, and Enter then fired whichever button silently held it.
func TestButtonDrawFocusRing(t *testing.T) {
	th := Theme()
	accent := th.Accent
	b := NewButton1("确定", nil)
	b.SetSize(80, 28)
	// focusWidget is a package global; leaving it pointing at a local button
	// would leak into the next test.
	t.Cleanup(func() { focusWidget = nil })

	draw := func() int {
		rec := newFocusRingRecorder()
		th.DrawButton(rec, b)
		return rec.rings(accent)
	}

	if n := draw(); n != 0 {
		t.Errorf("idle button: %d accent ring(s) @2px, want 0", n)
	}

	b.SetFocus()
	if !b.HasFocus() {
		t.Fatalf("premise gone: SetFocus did not take on a windowless button")
	}
	if n := draw(); n != 1 {
		t.Errorf("focused button: %d accent ring(s) @2px, want exactly 1 — Tab is invisible", n)
	}

	focusWidget = nil
	b.SetDefault(true)
	if b.HasFocus() {
		t.Fatalf("premise gone: focus was not cleared, the default case is not isolated")
	}
	if n := draw(); n != 1 {
		t.Errorf("default button: %d accent ring(s) @2px, want exactly 1 — the Enter target is invisible", n)
	}

	b.SetEnabled(false)
	b.SetFocus()
	if n := draw(); n != 0 {
		t.Errorf("disabled focused/default button: %d accent ring(s) @2px, want 0", n)
	}
}

// flaggedDefaults returns every button of dlg wearing the isDefault flag — the
// set the theme would ring. The invariant behind all the dialog default tests
// is that it holds exactly the one button defaultButton() activates.
func flaggedDefaults(dlg *Dialog) []*Button {
	var out []*Button
	for _, b := range dlg.buttons {
		if b.IsDefault() {
			out = append(out, b)
		}
	}
	return out
}

// btnText names a button in a failure message, tolerating a missing default.
func btnText(b *Button) string {
	if b == nil {
		return "<none>"
	}
	return b.Text()
}

// assertSoleDefault pins the invariant at one point in a dialog's life: exactly
// one button flagged, it is want, and defaultButton() agrees with the flag.
// step names the state so a failure says which call left the ring behind.
func assertSoleDefault(t *testing.T, dlg *Dialog, want *Button, step string) {
	t.Helper()
	got := flaggedDefaults(dlg)
	if len(got) != 1 {
		names := make([]string, len(got))
		for i, b := range got {
			names[i] = btnText(b)
		}
		t.Fatalf("%s: %d buttons flagged default %q, want exactly 1 (%q)",
			step, len(got), names, btnText(want))
	}
	if got[0] != want {
		t.Errorf("%s: flagged %q, want %q — the ring sits on a button Enter does not activate",
			step, btnText(got[0]), btnText(want))
	}
	if def := dlg.defaultButton(); got[0] != def {
		t.Errorf("%s: flagged %q but defaultButton() activates %q",
			step, btnText(got[0]), btnText(def))
	}
}

// TestDialogMarksExactlyOneDefaultButton: defaultButton() already knew which
// button Enter activates, and nothing painted it. AddButton / SetDefaultButton
// must keep that answer mirrored onto exactly one button's isDefault flag —
// leaving the previous default flagged would ring two buttons, which reads as
// two Enter targets.
func TestDialogMarksExactlyOneDefaultButton(t *testing.T) {
	dlg := NewDialog("t", nil)
	ok := dlg.AddButton("OK", DialogOK)
	cancel := dlg.AddButton("Cancel", DialogCancel)

	assertSoleDefault(t, dlg, ok, "after AddButton OK+Cancel")

	dlg.SetDefaultButton(cancel)

	assertSoleDefault(t, dlg, cancel, "after SetDefaultButton(Cancel)")
}

// dialogAdd is one AddButton call in an arrangement fixture.
type dialogAdd struct {
	text   string
	result DialogResult
}

// TestDialogDefaultFlagFollowsAddOrder walks the arrangements an OK-first
// fixture cannot reach: the affirmative button arriving last, and a set where
// the implicit default has to move twice before it lands. The flag is asserted
// after every AddButton, not only at the end — markDefaultButton's own comment
// promises the last-added button carries the default until an OK/Yes shows up,
// so a dialog whose answer never changes cannot tell whether the flag was
// computed over the button set the caller just added to or the one before it.
func TestDialogDefaultFlagFollowsAddOrder(t *testing.T) {
	cases := []struct {
		name string
		adds []dialogAdd
		// want[i] is the text of the button that must be default once adds[i]
		// has been added.
		want []string
	}{
		{
			name: "cancel then ok",
			adds: []dialogAdd{{"Cancel", DialogCancel}, {"OK", DialogOK}},
			want: []string{"Cancel", "OK"},
		},
		{
			name: "no then cancel then yes",
			adds: []dialogAdd{{"No", DialogNo}, {"Cancel", DialogCancel}, {"Yes", DialogYes}},
			want: []string{"No", "Cancel", "Yes"},
		},
		{
			name: "ok first keeps it through two more",
			adds: []dialogAdd{{"OK", DialogOK}, {"No", DialogNo}, {"Cancel", DialogCancel}},
			want: []string{"OK", "OK", "OK"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dlg := NewDialog("t", nil)
			byText := make(map[string]*Button, len(tc.adds))
			for i, a := range tc.adds {
				byText[a.text] = dlg.AddButton(a.text, a.result)
				assertSoleDefault(t, dlg, byText[tc.want[i]],
					fmt.Sprintf("after AddButton(%q), %d button(s) added", a.text, i+1))
			}
		})
	}
}

// TestDialogSetDefaultButtonAfterButtonsExist covers the override arriving once
// the whole button set is already built, and the revert back to the implicit
// heuristic. Both re-mark over every button, so the flag has to leave the
// button it was on and land on exactly the new answer.
func TestDialogSetDefaultButtonAfterButtonsExist(t *testing.T) {
	dlg := NewDialog("t", nil)
	no := dlg.AddButton("No", DialogNo)
	dlg.AddButton("Cancel", DialogCancel)
	yes := dlg.AddButton("Yes", DialogYes)

	assertSoleDefault(t, dlg, yes, "three buttons, implicit default")

	dlg.SetDefaultButton(no)
	assertSoleDefault(t, dlg, no, "after SetDefaultButton(No)")

	dlg.SetDefaultButton(nil)
	assertSoleDefault(t, dlg, yes, "after SetDefaultButton(nil), back to the implicit default")
}
