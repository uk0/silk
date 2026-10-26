package gui

import (
	"math"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/uk0/silk/paint"
)

// elideRecorder captures the strings a Draw actually paints, so the message a
// status bar or a toast puts on screen can be measured without a window. Embeds
// nopPainter (spinner_test.go) so a draw op these widgets did not use before
// panics here rather than passing silently.
type elideRecorder struct {
	paint.Painter
	texts []string
}

func newElideRecorder() *elideRecorder { return &elideRecorder{Painter: nopPainter{}} }

func (r *elideRecorder) DrawText(s string)                    { r.texts = append(r.texts, s) }
func (r *elideRecorder) Rectangle(x, y, w, h float64)         {}
func (r *elideRecorder) Line(x1, y1, x2, y2 float64)          {}
func (r *elideRecorder) MoveTo(x, y float64)                  {}
func (r *elideRecorder) LineTo(x, y float64)                  {}
func (r *elideRecorder) Arc(xc, yc, radius, a1, a2 float64)   {}
func (r *elideRecorder) SetBrush1(c paint.Color)              {}
func (r *elideRecorder) SetPen1(c paint.Color, width float64) {}
func (r *elideRecorder) Fill()                                {}
func (r *elideRecorder) Stroke()                              {}
func (r *elideRecorder) SetFont(f paint.Font)                 {}
func (r *elideRecorder) Translate(tx, ty float64)             {}

// TestElideToWidthKeepsFittingString: a string already inside the budget must
// come back byte-identical — the common case is every short status message,
// and appending an ellipsis to those would be a visible regression.
func TestElideToWidthKeepsFittingString(t *testing.T) {
	f := Theme().Font
	s := "已保存 main.go"
	max := f.TextExtents(s).Width // exactly the boundary: <= max still fits
	if got := elideToWidth(f, s, max); got != s {
		t.Errorf("elideToWidth cut a string that fits: got %q, want %q", got, s)
	}
}

// TestElideToWidthResultFitsBudget: the cut string must both end in the
// ellipsis and measure within the budget. Measuring is the half that matters —
// an implementation that chops runes and appends "…" without re-measuring
// still overflows by the width of the ellipsis.
func TestElideToWidthResultFitsBudget(t *testing.T) {
	f := Theme().Font
	s := "构建失败: /Users/dev/silk/ged/generated/very_long_form_name.go:128"
	max := 120.0
	got := elideToWidth(f, s, max)
	if got == s {
		t.Fatalf("elideToWidth returned the full string for a %.3f px budget", max)
	}
	if !strings.HasSuffix(got, elideMark) {
		t.Errorf("elided %q does not end in %q", got, elideMark)
	}
	if w := f.TextExtents(got).Width; w > max {
		t.Errorf("elided %q measures %.3f px, over the %.3f px budget", got, w, max)
	}
}

// TestElideToWidthRuneSafe: the cut may only land on a rune boundary, so a
// Chinese message stays valid UTF-8 and the kept part is a real prefix of the
// input rather than half of a three-byte rune.
func TestElideToWidthRuneSafe(t *testing.T) {
	f := Theme().Font
	s := strings.Repeat("控件", 40)
	got := elideToWidth(f, s, 60)
	if !utf8.ValidString(got) {
		t.Fatalf("elideToWidth split a multi-byte rune: %q is not valid UTF-8", got)
	}
	body := strings.TrimSuffix(got, elideMark)
	if body == "" {
		t.Fatalf("elideToWidth kept nothing of %q at a 60 px budget", s[:12])
	}
	if !strings.HasPrefix(s, body) {
		t.Errorf("elided body %q is not a prefix of the input", body)
	}
}

// TestElideToWidthNoRoomForEllipsis: below the width of the ellipsis itself
// there is nothing honest to draw, so the caller gets "" and paints nothing.
func TestElideToWidthNoRoomForEllipsis(t *testing.T) {
	f := Theme().Font
	max := f.TextExtents(elideMark).Width - 1
	if got := elideToWidth(f, "构建失败", max); got != "" {
		t.Errorf("elideToWidth returned %q for a %.3f px budget that cannot hold %q", got, max, elideMark)
	}
}

// TestStatusBarDrawElidesMessageToPermanentEdge: the permanent widgets are
// children, so they paint after the bar itself and StatusIconLabel fills no
// background — a long build-error path drawn unbounded runs straight through
// the branch cell. Draw must stop the message at the edge Layout reserved.
func TestStatusBarDrawElidesMessageToPermanentEdge(t *testing.T) {
	th := Theme()
	m := th.ButtonMargin
	const barW = 400.0

	sb := NewStatusBar()
	sb.SetBounds(0, 0, barW, 24)
	sb.AddIconLabel("git-branch", "feature/elide-status-message")
	msg := "构建失败: " + strings.Repeat("gui/generated/very/long/", 6) + "form.go"
	sb.SetMessage(msg)
	sb.Layout()

	if sb.permanentLeft <= 0 {
		t.Fatalf("Layout recorded no permanent-widget edge; there is no bound to test")
	}
	limit := sb.permanentLeft - m.L
	// The permanent widget has to be wide enough that the bar-wide bound and
	// the permanent-edge bound differ, otherwise eliding to the wrong one
	// would still pass.
	wide := elideToWidth(th.Font, msg, barW-m.L-m.R)
	if w := th.Font.TextExtents(wide).Width; w <= limit {
		t.Fatalf("test is not discriminating: a bar-wide elision measures %.3f px, already inside the %.3f px permanent edge", w, limit)
	}

	rec := newElideRecorder()
	sb.Draw(rec)
	if len(rec.texts) != 1 {
		t.Fatalf("StatusBar.Draw painted %d strings, want 1 (the message)", len(rec.texts))
	}
	got := rec.texts[0]
	if len(got) >= len(msg) {
		t.Errorf("StatusBar.Draw painted %d bytes of a %d byte message: it was not cut", len(got), len(msg))
	}
	if w := th.Font.TextExtents(got).Width; w > limit {
		t.Errorf("StatusBar painted %q at %.3f px wide; the permanent widgets start at x=%.3f, leaving %.3f px",
			got, w, sb.permanentLeft, limit)
	}
}

// TestStatusBarDrawElidesMessageToBarWidth: a bar with no permanent widget is
// the default shape, and there the message is bounded only by the bar's own
// margins. Without that bound a long path is drawn past the right edge, where
// the widget clip slices it mid-glyph — so the fallback has to cut too, not
// just the permanent-edge case.
func TestStatusBarDrawElidesMessageToBarWidth(t *testing.T) {
	th := Theme()
	m := th.ButtonMargin
	const barW = 240.0

	sb := NewStatusBar()
	sb.SetBounds(0, 0, barW, 24)
	msg := "构建失败: " + strings.Repeat("gui/generated/very/long/", 6) + "form.go"
	sb.SetMessage(msg)
	sb.Layout()

	if sb.permanentLeft != 0 {
		t.Fatalf("a bare status bar recorded a permanent edge at x=%.3f; this test needs the no-widget path", sb.permanentLeft)
	}
	limit := barW - m.L - m.R
	if w := th.Font.TextExtents(msg).Width; w <= limit {
		t.Fatalf("test is not discriminating: the message measures %.3f px, already inside the %.3f px bar", w, limit)
	}

	rec := newElideRecorder()
	sb.Draw(rec)
	if len(rec.texts) != 1 {
		t.Fatalf("StatusBar.Draw painted %d strings, want 1 (the message)", len(rec.texts))
	}
	got := rec.texts[0]
	if got == msg {
		t.Fatalf("StatusBar.Draw painted the whole %.3f px message on a %.0f px bar: with no permanent widget nothing bounded it",
			th.Font.TextExtents(msg).Width, barW)
	}
	if w := th.Font.TextExtents(got).Width; w > limit {
		t.Errorf("bare StatusBar painted %q at %.3f px wide, past the %.3f px its own margins leave inside a %.0f px bar",
			got, w, limit, barW)
	}
}

// TestStatusBarLayoutClearsPermanentEdge: with nothing on the right the
// message may use the whole bar, so Layout must not leave a stale edge behind
// after the last permanent widget is removed.
func TestStatusBarLayoutClearsPermanentEdge(t *testing.T) {
	sb := NewStatusBar()
	sb.SetBounds(0, 0, 400, 24)
	cell := sb.AddIconLabel("git-branch", "main")
	if sb.permanentLeft <= 0 {
		t.Fatalf("Layout recorded no edge for a visible permanent widget")
	}
	sb.RemovePermanentWidget(cell)
	if sb.permanentLeft != 0 {
		t.Errorf("permanentLeft = %.3f after removing the last permanent widget, want 0", sb.permanentLeft)
	}
}

// TestToastDrawElidesMessageBeforeCloseButton: the toast is a fixed 320x48 and
// the close button is painted after the text at w-20, taking clicks from w-30
// on. A message drawn unbounded runs under the ✕ and is sliced mid-glyph by the
// widget clip, so Draw has to cut it to the text column.
func TestToastDrawElidesMessageBeforeCloseButton(t *testing.T) {
	const (
		w        = 320.0
		h        = 48.0
		accentW  = 4.0 // the left accent bar toastEntry.Draw starts the text after
		textLeft = accentW + 32.0
	)
	limit := w - 40.0 - textLeft

	te := new(toastEntry)
	te.Init(te)
	te.level = ToastSuccess
	te.message = "已保存 " + strings.Repeat("generated_form_", 4) + "designer.go"
	te.SetSize(w, h)

	rec := newElideRecorder()
	te.Draw(rec)
	if len(rec.texts) != 1 {
		t.Fatalf("toastEntry.Draw painted %d strings, want 1 (the message)", len(rec.texts))
	}
	got := rec.texts[0]
	if got == te.message {
		t.Errorf("toastEntry.Draw painted the whole message; it cannot fit %.3f px", limit)
	}
	msgFont := paint.NewFont(Theme().Font.Family(), 13, false, false)
	if tw := msgFont.TextExtents(got).Width; tw > limit {
		t.Errorf("toast painted %q at %.3f px wide, past the %.3f px text column and under the close button",
			got, tw, limit)
	}
}

func TestElideTextPublicBoundaryContract(t *testing.T) {
	f := Theme().Font
	for _, w := range []float64{-1, 0, math.NaN()} {
		if ElideText(f, "text", w) != "" {
			t.Fatal("invalid width paints text")
		}
	}
	if ElideText(nil, "text", 100) != "" {
		t.Fatal("nil font paints text")
	}
	for _, text := range []string{"", "Title", "控件🙂 Unicode"} {
		if got := ElideText(f, text, math.Inf(1)); got != text {
			t.Fatalf("unlimited width lost %q", text)
		}
		for _, w := range []float64{1, 15, 50, 120} {
			got := ElideText(f, text, w)
			if !utf8.ValidString(got) || f.TextExtents(got).Width > w {
				t.Fatalf("text %q exceeds %g", got, w)
			}
		}
	}
}
