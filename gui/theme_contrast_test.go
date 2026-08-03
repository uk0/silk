package gui

import (
	"math"
	"testing"

	"github.com/uk0/silk/paint"
)

// WCAG 2.x contrast maths. Kept here rather than in paint/ because only the
// theme tests need it: they must assert "this ink is legible on the fill the
// theme actually paints under it", which channel-distance (see
// colorChannelDelta) cannot express — #FFFFFF and #E4E4E7 are 27 apart per
// channel yet 1.27:1 in contrast, i.e. invisible.

func srgbChannel(v uint8) float64 {
	c := float64(v) / 255
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

func relLuminance(c paint.Color) float64 {
	return 0.2126*srgbChannel(c.R) + 0.7152*srgbChannel(c.G) + 0.0722*srgbChannel(c.B)
}

// contrastRatio is the WCAG ratio between two opaque colours: 1.0 identical,
// 21.0 black on white. 4.5 is the AA threshold for normal-size body text.
func contrastRatio(a, b paint.Color) float64 {
	la, lb := relLuminance(a), relLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// themeFillRecorder captures the brush of the last fill and the last pen, so a
// theme Draw* method can be asserted without a GLFW window or a render target.
// Only the methods the theme draw paths actually reach are overridden; the
// embedded nopPainter (spinner_test.go) covers the rest.
type themeFillRecorder struct {
	paint.Painter
	brush paint.Color
	fill  paint.Color
	ink   paint.Color
	pen   paint.Color
	penW  float64
	fills int
	texts int
	font  paint.Font
}

func (r *themeFillRecorder) Arc(xc, yc, radius, angle1, angle2 float64) {}
func (r *themeFillRecorder) LineTo(x, y float64)                        {}
func (r *themeFillRecorder) MoveTo(x, y float64)                        {}
func (r *themeFillRecorder) Rectangle(x, y, w, h float64)               {}
func (r *themeFillRecorder) Translate(tx, ty float64)                   {}
func (r *themeFillRecorder) Stroke()                                    {}
func (r *themeFillRecorder) SetBrush1(c paint.Color)                    { r.brush = c }
func (r *themeFillRecorder) Fill()                                      { r.fill = r.brush; r.fills++ }
func (r *themeFillRecorder) FillPreserve()                              { r.fill = r.brush; r.fills++ }
func (r *themeFillRecorder) SetPen1(c paint.Color, w float64)           { r.pen, r.penW = c, w }
func (r *themeFillRecorder) SetFont(f paint.Font)                       { r.font = f }
func (r *themeFillRecorder) Font() paint.Font                           { return r.font }
func (r *themeFillRecorder) DrawText(text string)                       { r.ink = r.brush; r.texts++ }

func newThemeFillRecorder() *themeFillRecorder {
	return &themeFillRecorder{Painter: nopPainter{}}
}

// TestThemeDrawEditFrameFillFollowsThemeSurface locks two defects that shared
// one cause — DrawEditFrame filling a hardcoded opaque white:
//
//   - the fill covered the read-only tint every caller (Edit, TextArea,
//     PathEdit, TreeView) paints just before calling it, so the read-only /
//     disabled state was never actually rendered; and
//   - in dark mode it put a white field under TextColor (zinc-200), i.e. text
//     at 1.27:1 — unreadable.
//
// The fill must come from the theme: ViewBGColor normally, FormColor when the
// caller reports read-only.
func TestThemeDrawEditFrameFillFollowsThemeSurface(t *testing.T) {
	orig := CurrentThemeMode()
	t.Cleanup(func() { SetThemeMode(orig) })

	for _, tc := range []struct {
		mode ThemeMode
		name string
	}{{ThemeLight, "light"}, {ThemeDark, "dark"}} {
		SetThemeMode(tc.mode)
		th := Theme()
		for _, readonly := range []bool{false, true} {
			rec := newThemeFillRecorder()
			th.DrawEditFrame(rec, 0, 0, 120, 24, false, false, readonly)

			want := th.ViewBGColor
			if readonly {
				want = th.FormColor
			}
			if rec.fills == 0 {
				t.Fatalf("%s readonly=%v: DrawEditFrame painted no field background", tc.name, readonly)
			}
			if rec.fill != want {
				t.Errorf("%s readonly=%v: field fill = %v, want theme surface %v",
					tc.name, readonly, rec.fill, want)
			}
			if r := contrastRatio(th.TextColor, rec.fill); r < 4.5 {
				t.Errorf("%s readonly=%v: TextColor %v on field fill %v = %.2f:1, want >= 4.5:1",
					tc.name, readonly, th.TextColor, rec.fill, r)
			}
		}
	}
}

// TestThemeDrawEditFrameShowsFocusWhenReadOnly: read-only is not the same as
// unfocusable. isTabFocusable only drops hidden / disabled / NoFocus widgets,
// so Tab lands on a read-only Edit or TextArea (TextArea.SetReadOnly even
// documents that it still takes focus, for copying). Suppressing the ring for
// readonly therefore left a keyboard user with no idea where focus was.
func TestThemeDrawEditFrameShowsFocusWhenReadOnly(t *testing.T) {
	e := NewEdit()
	e.SetReadOnly(true)
	if !isTabFocusable(e.Self()) {
		t.Fatalf("premise gone: a read-only Edit is no longer in the Tab focus chain")
	}

	th := Theme()
	editable := newThemeFillRecorder()
	th.DrawEditFrame(editable, 0, 0, 120, 24, true, false, false)

	ro := newThemeFillRecorder()
	th.DrawEditFrame(ro, 0, 0, 120, 24, true, false, true)

	idle := newThemeFillRecorder()
	th.DrawEditFrame(idle, 0, 0, 120, 24, false, false, true)

	if ro.pen != editable.pen || ro.penW != editable.penW {
		t.Errorf("focused read-only frame stroked %v @%v, want the same focus ring as an editable field (%v @%v)",
			ro.pen, ro.penW, editable.pen, editable.penW)
	}
	if ro.pen == idle.pen && ro.penW == idle.penW {
		t.Errorf("focused read-only frame is stroked exactly like the unfocused one (%v @%v): focus is invisible",
			idle.pen, idle.penW)
	}
}

// TestThemeButtonLabelReadableOnItsOwnFill drives DrawButton per state and
// asserts the label is legible on whatever that state fills behind it. It
// locks the dark-mode hover regression: the hover fill was a hardcoded
// blue-50, so hovering a button in dark mode painted zinc-200 text at 1.17:1
// and the label vanished under the pointer.
//
// The disabled state is deliberately out of the table: disabled text sits
// below 4.5:1 by design in both modes (WCAG exempts inactive controls), so
// asserting it here would only force a blind tweak of the light palette.
func TestThemeButtonLabelReadableOnItsOwnFill(t *testing.T) {
	orig := CurrentThemeMode()
	t.Cleanup(func() { SetThemeMode(orig) })

	states := []struct {
		name  string
		setup func(*Button)
	}{
		{"normal", func(b *Button) {}},
		{"hover", func(b *Button) { b.pushed = true }}, // IsPushed||IsHover -> hover face
	}

	for _, tc := range []struct {
		mode ThemeMode
		name string
	}{{ThemeLight, "light"}, {ThemeDark, "dark"}} {
		SetThemeMode(tc.mode)
		th := Theme()
		for _, st := range states {
			b := NewButton1("确定", nil)
			b.SetSize(80, 28)
			st.setup(b)

			rec := newThemeFillRecorder()
			// Seed the fill with the chrome the button sits on: the enabled
			// idle state paints no face at all, so "no fill" means the form
			// colour is what shows through behind the label.
			rec.fill = th.FormColor
			th.DrawButton(rec, b)

			if rec.texts == 0 {
				t.Fatalf("%s/%s: DrawButton drew no label", tc.name, st.name)
			}
			if r := contrastRatio(rec.ink, rec.fill); r < 4.5 {
				t.Errorf("%s/%s: label %v on fill %v = %.2f:1, want >= 4.5:1",
					tc.name, st.name, rec.ink, rec.fill, r)
			}
		}
	}
}
