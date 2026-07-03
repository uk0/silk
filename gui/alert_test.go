package gui

import (
	"testing"

	"github.com/uk0/silk/paint"
)

// TestNewAlertDefaults locks in the construction-time state: the level and
// message come from the constructor, the title is empty and the close button
// is off until a caller opts in.
func TestNewAlertDefaults(t *testing.T) {
	a := NewAlert(AlertWarning, "disk almost full")
	if a.Level() != AlertWarning {
		t.Errorf("Level() = %d, want AlertWarning", a.Level())
	}
	if a.Message() != "disk almost full" {
		t.Errorf("Message() = %q, want %q", a.Message(), "disk almost full")
	}
	if a.Title() != "" {
		t.Errorf("Title() = %q, want empty", a.Title())
	}
	if a.IsCloseable() {
		t.Errorf("new alert should not be closeable by default")
	}
}

// TestAlertSettersUpdateState verifies each setter updates the corresponding
// field and that mutating state never fires the close callback (only a user
// click on × should).
func TestAlertSettersUpdateState(t *testing.T) {
	a := NewAlert(AlertInfo, "hello")
	fired := false
	a.SigClose(func() { fired = true })

	a.SetLevel(AlertError)
	if a.Level() != AlertError {
		t.Errorf("SetLevel: Level() = %d, want AlertError", a.Level())
	}
	a.SetMessage("goodbye")
	if a.Message() != "goodbye" {
		t.Errorf("SetMessage: Message() = %q, want %q", a.Message(), "goodbye")
	}
	a.SetTitle("Heads up")
	if a.Title() != "Heads up" {
		t.Errorf("SetTitle: Title() = %q, want %q", a.Title(), "Heads up")
	}
	a.SetCloseable(true)
	if !a.IsCloseable() {
		t.Errorf("SetCloseable(true): IsCloseable() = false")
	}

	if fired {
		t.Errorf("setters must not fire SigClose")
	}
}

// TestAlertCloseHitFiresAndHides: clicking the × (top-right) on a closeable
// alert must hide the widget and fire SigClose exactly once.
func TestAlertCloseHitFiresAndHides(t *testing.T) {
	a := NewAlert(AlertSuccess, "saved")
	a.SetCloseable(true)
	calls := 0
	a.SigClose(func() { calls++ })

	w, _ := a.SizeHints().Width, a.SizeHints().Height
	a.SetSize(w, a.SizeHints().Height)

	// Click squarely on the close glyph centre.
	cx := w - alertPadding - alertCloseR
	cy := alertPadding + alertCloseR
	a.OnLeftDown(cx, cy)

	if calls != 1 {
		t.Errorf("close click fired SigClose %d times, want 1", calls)
	}
	if a.IsVisible() {
		t.Errorf("alert should be hidden after close click")
	}
}

// TestAlertClickOutsideCloseIsNoop: a click in the message area must not fire
// the callback or hide the alert.
func TestAlertClickOutsideCloseIsNoop(t *testing.T) {
	a := NewAlert(AlertInfo, "informational message")
	a.SetCloseable(true)
	fired := false
	a.SigClose(func() { fired = true })
	a.SetSize(a.SizeHints().Width, a.SizeHints().Height)

	// Click near the left text area, well away from the × corner.
	a.OnLeftDown(alertBarW+alertPadding, a.SizeHints().Height/2)

	if fired {
		t.Errorf("click outside × must not fire SigClose")
	}
	if !a.IsVisible() {
		t.Errorf("click outside × must not hide the alert")
	}
}

// TestAlertNotCloseableIgnoresClick: when the alert is not closeable, even a
// click at the corner must be inert.
func TestAlertNotCloseableIgnoresClick(t *testing.T) {
	a := NewAlert(AlertError, "error")
	fired := false
	a.SigClose(func() { fired = true })
	a.SetSize(a.SizeHints().Width, a.SizeHints().Height)

	w, _ := a.Size()
	a.OnLeftDown(w-alertPadding-alertCloseR, alertPadding+alertCloseR)

	if fired {
		t.Errorf("non-closeable alert must not fire SigClose on click")
	}
	if !a.IsVisible() {
		t.Errorf("non-closeable alert must stay visible")
	}
}

// TestAlertSizeHintsNonZero: every level/title/closeable combination must
// report a positive footprint so a layout never collapses the banner.
func TestAlertSizeHintsNonZero(t *testing.T) {
	cases := []*Alert{
		NewAlert(AlertInfo, "short"),
		NewAlert(AlertSuccess, "a much longer success message body"),
	}
	cases[1].SetTitle("Title")
	cases[1].SetCloseable(true)

	for i, a := range cases {
		h := a.SizeHints()
		if h.Width <= 0 || h.Height <= 0 {
			t.Errorf("case %d: SizeHints = %v, want positive width/height", i, h)
		}
	}

	// Adding a title must not shrink the height below the title-less variant.
	plain := NewAlert(AlertInfo, "body")
	titled := NewAlert(AlertInfo, "body")
	titled.SetTitle("Heads up")
	if titled.SizeHints().Height <= plain.SizeHints().Height {
		t.Errorf("titled alert height %.1f should exceed plain %.1f",
			titled.SizeHints().Height, plain.SizeHints().Height)
	}
}

// TestAlertDrawDoesNotPanic exercises Draw for every level (with and without
// title/close) against a nil-safe recording painter — no GL context needed.
// Models the spinner_test nopPainter pattern.
func TestAlertDrawDoesNotPanic(t *testing.T) {
	for lvl := AlertInfo; lvl <= AlertError; lvl++ {
		a := NewAlert(lvl, "message")
		a.SetSize(a.SizeHints().Width, a.SizeHints().Height)
		a.Draw(alertNopPainter{}) // must not panic

		a.SetTitle("Title")
		a.SetCloseable(true)
		a.SetSize(a.SizeHints().Width, a.SizeHints().Height)
		a.Draw(alertNopPainter{}) // must not panic
	}
}

// alertNopPainter satisfies paint.Painter with no-op stubs (embeds a nil
// Painter) so Draw can run without a render target. Only the methods Draw
// actually calls need behaviour; the embedded nil supplies the rest, which
// Draw never reaches.
type alertNopPainter struct{ paint.Painter }

func (alertNopPainter) Save() int                                  { return 0 }
func (alertNopPainter) Restore() int                               { return 0 }
func (alertNopPainter) Arc(xc, yc, radius, angle1, angle2 float64) {}
func (alertNopPainter) Rectangle(x, y, w, h float64)               {}
func (alertNopPainter) MoveTo(x, y float64)                        {}
func (alertNopPainter) LineTo(x, y float64)                        {}
func (alertNopPainter) Fill()                                      {}
func (alertNopPainter) Stroke()                                    {}
func (alertNopPainter) SetBrush1(c paint.Color)                    {}
func (alertNopPainter) SetPen1(c paint.Color, width float64)       {}
func (alertNopPainter) SetFont(f paint.Font)                       {}
func (alertNopPainter) DrawText1(x, y float64, text string)        {}
