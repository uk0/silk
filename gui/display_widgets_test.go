package gui

import (
	"math"
	"testing"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
)

// factoryWidget builds a widget the way the designer and the .tdoc loader
// build it — core factory plus reflected Init — rather than through NewXxx.
func factoryWidget(t *testing.T, name string) IWidget {
	t.Helper()
	f := core.FindFactory(name)
	if f == nil {
		t.Fatalf("no factory registered for %q", name)
	}
	iw, ok := f.New().(IWidget)
	if !ok {
		t.Fatalf("factory %q did not produce an IWidget", name)
	}
	return iw
}

// TestFactoryTagIsVisible: a Tag dragged off the widget palette comes from
// the factory, so colours set only in NewTag leave it fully transparent.
func TestFactoryTagIsVisible(t *testing.T) {
	tag := factoryWidget(t, "gui.Tag").(*Tag)
	if tag.Color().A == 0 {
		t.Errorf("factory Tag pill colour is %v — transparent, nothing renders", tag.Color())
	}
	if tag.textColor.A == 0 {
		t.Errorf("factory Tag text colour is %v — transparent, the label never appears", tag.textColor)
	}
}

// TestFactoryBadgeIsVisible: same for Badge, whose maxCount also drives
// displayText — a zero cap turns every count into "0+".
func TestFactoryBadgeIsVisible(t *testing.T) {
	b := factoryWidget(t, "gui.Badge").(*Badge)
	if b.color.A == 0 {
		t.Errorf("factory Badge colour is %v — transparent", b.color)
	}
	if b.MaxCount() != 99 {
		t.Errorf("factory Badge MaxCount = %d, want 99", b.MaxCount())
	}
	b.SetCount(5)
	if got := b.displayText(); got != "5" {
		t.Errorf("factory Badge displayText for count 5 = %q, want \"5\"", got)
	}
}

// TestFactoryAvatarIsVisible: a zero size makes SizeHints 0x0 and collapses
// the whole avatar to a point.
func TestFactoryAvatarIsVisible(t *testing.T) {
	a := factoryWidget(t, "gui.Avatar").(*Avatar)
	if a.AvatarSize() <= 0 {
		t.Errorf("factory Avatar size = %v, want the 40 NewAvatar uses", a.AvatarSize())
	}
	if a.bgColor.A == 0 {
		t.Errorf("factory Avatar background is %v — transparent", a.bgColor)
	}
}

// TestFactoryLinkIsVisible: Link paints its text with this.color, so a zero
// colour draws the text fully transparent.
func TestFactoryLinkIsVisible(t *testing.T) {
	l := factoryWidget(t, "gui.Link").(*Link)
	if l.Color().A == 0 {
		t.Errorf("factory Link colour is %v — transparent text", l.Color())
	}
}

// TestFactoryProgressBarIsVisible: track and fill colours both come from
// NewProgressBar, so a factory bar paints a transparent track under a
// transparent fill.
func TestFactoryProgressBarIsVisible(t *testing.T) {
	p := factoryWidget(t, "gui.ProgressBar").(*ProgressBar)
	if p.BarColor().A == 0 {
		t.Errorf("factory ProgressBar fill colour is %v — transparent", p.BarColor())
	}
	if p.bgColor.A == 0 {
		t.Errorf("factory ProgressBar track colour is %v — transparent", p.bgColor)
	}
}

// TestFactoryRatingDrawsStars: maxStars is set in NewRating only, so the
// factory Rating's draw loop runs zero times and SetValue clamps every
// value to 0.
func TestFactoryRatingDrawsStars(t *testing.T) {
	r := factoryWidget(t, "gui.Rating").(*Rating)
	if r.MaxStars() != 5 {
		t.Errorf("factory Rating MaxStars = %d, want 5", r.MaxStars())
	}
	r.SetValue(3)
	if r.Value() != 3 {
		t.Errorf("factory Rating Value after SetValue(3) = %d, want 3", r.Value())
	}
	r.SetSize(96, 20)
	rec := newDisplayRecorder()
	r.Draw(rec)
	if len(rec.arcs) == 0 {
		t.Error("factory Rating drew no stars")
	}
}

// TestFactoryRatingHoverStartsUnset: hoverValue must start at -1. At 0 the
// "is hovered" test (hoverValue >= 0) is already true before the mouse
// arrives, so the widget paints an empty hover preview instead of its value.
func TestFactoryRatingHoverStartsUnset(t *testing.T) {
	r := factoryWidget(t, "gui.Rating").(*Rating)
	if r.hoverValue != -1 {
		t.Errorf("factory Rating hoverValue = %d, want -1 (no hover)", r.hoverValue)
	}
}

// TestFactorySpinnerDrawsDots: Draw bails on dotCount < 3 and the factory
// leaves it at 0, so a factory spinner renders nothing even when busy.
func TestFactorySpinnerDrawsDots(t *testing.T) {
	s := factoryWidget(t, "gui.Spinner").(*Spinner)
	if s.DotCount() != 8 {
		t.Errorf("factory Spinner DotCount = %d, want 8", s.DotCount())
	}
	if s.CycleDuration() != time.Second {
		t.Errorf("factory Spinner CycleDuration = %v, want 1s", s.CycleDuration())
	}
	if s.Color().A == 0 {
		t.Errorf("factory Spinner colour is %v — transparent dots", s.Color())
	}
	s.SetSize(24, 24)
	s.SetBusy(true)
	defer s.SetBusy(false)
	rec := newDisplayRecorder()
	s.Draw(rec)
	if len(rec.arcs) == 0 {
		t.Error("busy factory Spinner drew no dots")
	}
}

// TestRatingHitTestMatchesPaintedStars: Draw shrinks the stars to fit the
// widget, but hitTestStar measured with the fixed full-size constants — so
// on any widget narrower or shorter than the size hint a click selected a
// different star than the one under the cursor.
func TestRatingHitTestMatchesPaintedStars(t *testing.T) {
	sizes := []struct{ w, h float64 }{
		{96, 20},  // natural size
		{48, 20},  // half width — the radius shrinks
		{96, 12},  // short — height caps the radius even at full width
		{200, 20}, // roomier than needed — the radius stays capped
	}
	for _, s := range sizes {
		r := NewRating()
		r.SetSize(s.w, s.h)

		rec := newDisplayRecorder()
		r.Draw(rec)
		if len(rec.arcs) != r.MaxStars() {
			t.Fatalf("size %vx%v: drew %d stars, want %d", s.w, s.h, len(rec.arcs), r.MaxStars())
		}
		for i, a := range rec.arcs {
			if got := r.hitTestStar(a.x); got != i+1 {
				t.Errorf("size %vx%v: click at painted centre of star %d (x=%v) hit %d",
					s.w, s.h, i+1, a.x, got)
			}
		}
	}
}

// TestRatingStarRadiusNeverNegative: a widget shorter than the 4px of
// vertical padding drove the radius negative, and that reaches cairo as an
// Arc with a negative radius.
func TestRatingStarRadiusNeverNegative(t *testing.T) {
	for _, h := range []float64{0, 1, 3, 4} {
		r := NewRating()
		r.SetSize(80, h)
		rec := newDisplayRecorder()
		r.Draw(rec)
		for i, a := range rec.arcs {
			if a.r < 0 {
				t.Errorf("height %v: star %d painted with radius %v", h, i, a.r)
			}
		}
	}
}

// TestLinkHoverBlueDoesNotWrap: the hover tint added 40 to the blue channel
// in uint8, so the default link blue (B=244) wrapped to 28 and the hovered
// link went dark instead of bright. The guard meant to catch that compared a
// uint8 against 255 and could never fire.
func TestLinkHoverBlueDoesNotWrap(t *testing.T) {
	l := NewLink("链接", "https://example.com")
	l.SetSize(80, 20)

	rec := newDisplayRecorder()
	l.Draw(rec)
	if len(rec.brushes) == 0 {
		t.Fatal("Link.Draw set no brush")
	}
	rest := rec.brushes[0]

	l.OnMouseEnter()
	rec = newDisplayRecorder()
	l.Draw(rec)
	if len(rec.brushes) == 0 {
		t.Fatal("hovered Link.Draw set no brush")
	}
	hover := rec.brushes[0]

	if hover.B < rest.B {
		t.Errorf("hover blue %d is below the resting blue %d — the channel wrapped", hover.B, rest.B)
	}
	if hover.R != rest.R || hover.G != rest.G || hover.A != rest.A {
		t.Errorf("hover changed more than the blue channel: %v -> %v", rest, hover)
	}

	// A colour with headroom still gains the full 40, so the clamp cannot be
	// a blanket "blue = 255" that flattens every link to one hover tint.
	l.SetColor(paint.Color{10, 20, 30, 255})
	rec = newDisplayRecorder()
	l.Draw(rec)
	if got := rec.brushes[0].B; got != 70 {
		t.Errorf("hover blue for B=30 = %d, want 70", got)
	}
}

// TestBadgeMarkerPaintsAboveContent: the framework paints a widget, then its
// children, then its overlay. Badge.Layout hands the content the whole
// widget rect, so a marker painted in Draw ends up underneath it.
func TestBadgeMarkerPaintsAboveContent(t *testing.T) {
	b := NewBadge()
	b.SetCount(5)
	b.SetSize(40, 40)

	rec := newDisplayRecorder()
	b.Draw(rec)
	if rec.fills != 0 {
		t.Errorf("Badge.Draw painted %d fills; the marker must come after the content child", rec.fills)
	}

	ov, ok := interface{}(b).(IDrawOverlay)
	if !ok {
		t.Fatal("Badge does not implement IDrawOverlay, so its marker cannot paint above content")
	}
	rec = newDisplayRecorder()
	ov.DrawOverlay(rec)
	if rec.fills == 0 {
		t.Error("Badge.DrawOverlay painted nothing")
	}
	if rec.texts != 1 {
		t.Errorf("Badge.DrawOverlay drew %d text runs, want 1 (the count)", rec.texts)
	}
}

// TestTagTextClippedBeforeCloseButton: SizeHints reserves 18px on the right
// for the cross, but Draw placed the label with no bound at all — a tag
// sized under its hint ran the text straight through the cross.
func TestTagTextClippedBeforeCloseButton(t *testing.T) {
	tag := NewTag("一个非常非常长的标签文本")
	tag.SetCloseable(true)
	const w, h = 60.0, 24.0
	tag.SetSize(w, h)

	rec := newDisplayRecorder()
	tag.Draw(rec)
	if len(rec.clips) == 0 {
		t.Fatal("Tag.Draw pushed no clip, so the label can overrun the close cross")
	}
	right := rec.clips[0].x + rec.clips[0].w
	if limit := w - 18.0; right > limit+1e-9 {
		t.Errorf("text clip reaches x=%v, past the close-cross slot at x=%v", right, limit)
	}
}

// TestTagTextClipMatchesPaddingWhenNotCloseable: with no cross the label owns
// everything up to the 8px right padding SizeHints reserves, and no less.
func TestTagTextClipMatchesPaddingWhenNotCloseable(t *testing.T) {
	tag := NewTag("标签")
	const w, h = 60.0, 24.0
	tag.SetSize(w, h)

	rec := newDisplayRecorder()
	tag.Draw(rec)
	if len(rec.clips) == 0 {
		t.Fatal("Tag.Draw pushed no clip")
	}
	right := rec.clips[0].x + rec.clips[0].w
	if want := w - 8.0; math.Abs(right-want) > 1e-9 {
		t.Errorf("text clip right edge = %v, want %v", right, want)
	}
}

// TestTagTextClipNeverNegative: a tag narrower than its own padding plus the
// cross slot produced a negative clip width, which cairo reads as a
// rectangle running backwards from the text origin.
func TestTagTextClipNeverNegative(t *testing.T) {
	tag := NewTag("标签")
	tag.SetCloseable(true)
	tag.SetSize(10, 24)

	rec := newDisplayRecorder()
	tag.Draw(rec)
	if len(rec.clips) == 0 {
		t.Fatal("Tag.Draw pushed no clip")
	}
	if rec.clips[0].w < 0 {
		t.Errorf("text clip width = %v", rec.clips[0].w)
	}
}

// TestAvatarSizeNeverNegative: 大小 is an editable property, and a negative
// one reaches SizeHints as a negative extent and Draw as a negative radius.
func TestAvatarSizeNeverNegative(t *testing.T) {
	a := NewAvatar()
	a.SetAvatarSize(-10)
	if a.AvatarSize() < 0 {
		t.Errorf("AvatarSize after SetAvatarSize(-10) = %v", a.AvatarSize())
	}
	if h := a.SizeHints(); h.Width < 0 || h.Height < 0 {
		t.Errorf("SizeHints = %vx%v after a negative size", h.Width, h.Height)
	}
}

// --- recorder ---

type recRect struct{ x, y, w, h float64 }
type recArc struct{ x, y, r float64 }

// displayRecorder captures the draw ops the display widgets issue. It embeds
// nopPainter (spinner_test.go) so an op none of these widgets used before
// panics here rather than passing silently.
type displayRecorder struct {
	paint.Painter
	fills         int
	fillPreserves int
	texts         int
	arcs          []recArc
	brushes       []paint.Color
	clips         []recRect
	pending       recRect
	state         int
}

func newDisplayRecorder() *displayRecorder {
	return &displayRecorder{Painter: nopPainter{}}
}

func (r *displayRecorder) Save() int                    { r.state++; return r.state }
func (r *displayRecorder) Restore() int                 { r.state--; return r.state }
func (r *displayRecorder) CurrentState() int            { return r.state }
func (r *displayRecorder) Rectangle(x, y, w, h float64) { r.pending = recRect{x, y, w, h} }
func (r *displayRecorder) Clip()                        { r.clips = append(r.clips, r.pending) }
func (r *displayRecorder) MoveTo(x, y float64)          {}
func (r *displayRecorder) LineTo(x, y float64)          {}
func (r *displayRecorder) Arc(xc, yc, radius, a1, a2 float64) {
	r.arcs = append(r.arcs, recArc{xc, yc, radius})
}
func (r *displayRecorder) SetBrush1(c paint.Color)              { r.brushes = append(r.brushes, c) }
func (r *displayRecorder) SetPen1(c paint.Color, width float64) {}
func (r *displayRecorder) Fill()                                { r.fills++ }
func (r *displayRecorder) FillPreserve()                        { r.fillPreserves++ }
func (r *displayRecorder) Stroke()                              {}
func (r *displayRecorder) SetFont(f paint.Font)                 {}
func (r *displayRecorder) Font() paint.Font                     { return Theme().Font }
func (r *displayRecorder) Translate(tx, ty float64)             {}
func (r *displayRecorder) DrawText(s string)                    { r.texts++ }
func (r *displayRecorder) DrawText1(x, y float64, s string)     { r.texts++ }
