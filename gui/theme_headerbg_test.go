package gui

import (
	"testing"

	"github.com/uk0/silk/paint"
)

// headerBgRecorder is a no-op paint.Painter (it embeds a nil Painter) that
// records the brush colour of the last fill so drawHeaderBg can be exercised
// without a GLFW window or a real render target. Only the handful of methods
// drawHeaderBg (and the pixmapFace placeholder branch) reach are overridden;
// any other call would hit the nil embedded Painter, and drawHeaderBg never
// makes one. Mirrors the nopPainter pattern used by the other gui draw tests.
type headerBgRecorder struct {
	paint.Painter
	lastBrush paint.Color
	fills     int
}

func (r *headerBgRecorder) SetBrush1(c paint.Color)          { r.lastBrush = c }
func (r *headerBgRecorder) Rectangle(x, y, w, h float64)     {}
func (r *headerBgRecorder) Fill()                            { r.fills++ }
func (r *headerBgRecorder) FillPreserve()                    { r.fills++ }
func (r *headerBgRecorder) Stroke()                          {}
func (r *headerBgRecorder) SetPen1(c paint.Color, w float64) {}

// TestThemeDrawHeaderBgNilFaceNoPanic locks the regression fixed here: the
// default theme leaves ButtonPushedFace nil (header rendering is programmatic),
// so Table/HeaderView draw used to call Draw on a nil *pixmapFace and panic.
// drawHeaderBg must instead fall back to a flat FormColor fill.
func TestThemeDrawHeaderBgNilFaceNoPanic(t *testing.T) {
	th := Theme()
	if th.ButtonPushedFace != nil {
		t.Skip("active theme configures a header face; nil-fallback path not applicable")
	}
	rec := &headerBgRecorder{}
	th.drawHeaderBg(rec, 80, 24) // must not panic on the nil face
	if rec.fills == 0 {
		t.Fatalf("drawHeaderBg did not fill the header background on the nil-face path")
	}
	if rec.lastBrush != th.FormColor {
		t.Errorf("fallback brush = %+v, want FormColor %+v", rec.lastBrush, th.FormColor)
	}
}

// TestThemeFormColorOpaqueFallback: the fallback fill colour must be opaque so
// headers are not painted transparent when no face is configured.
func TestThemeFormColorOpaqueFallback(t *testing.T) {
	if a := Theme().FormColor.A; a == 0 {
		t.Fatalf("Theme().FormColor is fully transparent (A=0); header fallback would be invisible")
	}
}

// TestThemeDrawHeaderBgUsesFaceWhenSet: when a face IS configured, drawHeaderBg
// routes to it instead of the FormColor fallback. An empty pixmapFace (nil
// brushes) draws the placeholder grey, distinct from FormColor, which proves the
// non-nil branch was taken. Uses a locally constructed theme so the shared
// singleton is not mutated.
func TestThemeDrawHeaderBgUsesFaceWhenSet(t *testing.T) {
	th := &defaultTheme{
		FormColor:        Theme().FormColor, // any non-grey sentinel; the empty face draws grey
		ButtonPushedFace: &pixmapFace{},     // nil brushes -> Draw paints placeholder grey
	}
	rec := &headerBgRecorder{}
	th.drawHeaderBg(rec, 40, 20) // must not panic; must route to the face
	if rec.lastBrush == th.FormColor {
		t.Errorf("drawHeaderBg used the FormColor fallback though a face was set")
	}
}
