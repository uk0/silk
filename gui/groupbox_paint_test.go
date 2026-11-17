package gui

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/paint"
)

type groupBoxArcRecorder struct {
	paint.Painter
	sweeps []float64
}

func (p *groupBoxArcRecorder) Arc(x, y, r, a, b float64) {
	p.sweeps = append(p.sweeps, math.Mod(b-a+4*math.Pi, 2*math.Pi))
	p.Painter.Arc(x, y, r, a, b)
}

func (p *groupBoxArcRecorder) ArcNegative(x, y, r, a, b float64) {
	p.sweeps = append(p.sweeps, -math.Mod(a-b+4*math.Pi, 2*math.Pi))
	p.Painter.ArcNegative(x, y, r, a, b)
}

type groupBoxTextRecorder struct{ *labelPaintRecorder }

func (p *groupBoxTextRecorder) ArcNegative(float64, float64, float64, float64, float64) {}
func (p *groupBoxTextRecorder) Font() paint.Font                                        { return Theme().Font }

func newGroupBoxTextRecorder() *groupBoxTextRecorder {
	return &groupBoxTextRecorder{newLabelPaintRecorder()}
}

func TestGroupBoxBorderQuarterArcs(t *testing.T) {
	group := NewGroupBox("")
	group.SetSize(180, 100)
	pm := paint.NewPixmap(200, 120)
	defer paint.CloseSurface(pm)
	g := pm.NewPainter()
	defer paint.ClosePainter(g)
	recorder := &groupBoxArcRecorder{Painter: g}
	group.Draw(recorder)
	if len(recorder.sweeps) != 4 {
		t.Fatalf("corner count = %d, want 4", len(recorder.sweeps))
	}
	for i, sweep := range recorder.sweeps {
		if math.Abs(sweep+math.Pi/2) > 1e-8 {
			t.Errorf("corner %d sweeps %.2f degrees, want -90", i, sweep*180/math.Pi)
		}
	}
}

func TestGroupBoxBorderNoInteriorCurl(t *testing.T) {
	for _, scale := range []float64{1, 1.25, 2, 3} {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			group := NewGroupBox("")
			group.SetSize(180, 100)
			pm := paint.NewPixmap(int(math.Ceil(196*scale)), int(math.Ceil(116*scale)))
			defer paint.CloseSurface(pm)
			g := pm.NewPainter()
			defer paint.ClosePainter(g)
			g.Scale(scale, scale)
			g.Translate(8, 8)
			group.Draw(g)
			img, err := pm.Image()
			if err != nil {
				t.Fatal(err)
			}
			// Above and inside each bottom corner: the proper exterior quarter
			// cannot leave ink here, whereas the old 270-degree loop did.
			for _, x0 := range []float64{11, 166} {
				for y := int(math.Ceil(100 * scale)); y < int(math.Floor(103*scale)); y++ {
					for x := int(math.Ceil((8 + x0) * scale)); x < int(math.Floor((11+x0)*scale)); x++ {
						if _, _, _, a := img.At(x, y).RGBA(); a != 0 {
							t.Fatalf("interior curl at (%d,%d), scale %g", x, y, scale)
						}
					}
				}
			}
		})
	}
}

func TestGroupBoxBorderAlignedInsideBounds(t *testing.T) {
	group := NewGroupBox("")
	group.SetSize(100, 60)
	pm := paint.NewPixmap(120, 80)
	defer paint.CloseSurface(pm)
	g := pm.NewPainter()
	defer paint.ClosePainter(g)
	g.Translate(8, 8)
	group.Draw(g)
	img, err := pm.Image()
	if err != nil {
		t.Fatal(err)
	}
	for _, point := range [][2]int{{14, 40}, {101, 40}, {50, 67}, {50, 8 + int(group.titleHeight()*0.5)}} {
		_, _, _, a := img.At(point[0], point[1]).RGBA()
		if a != uint32(Theme().BorderColor.A)*257 {
			t.Errorf("aligned border at %v: alpha %d, want full border coverage", point, a)
		}
	}
	if _, _, _, a := img.At(50, 68).RGBA(); a != 0 {
		t.Fatal("bottom stroke escaped the assigned height")
	}
}

func TestGroupBoxPaintRestoresCallerGeometryAndFont(t *testing.T) {
	group := NewGroupBox("Frame")
	group.SetSize(150, 80)
	pm := paint.NewPixmap(240, 180)
	defer paint.CloseSurface(pm)
	g := pm.NewPainter()
	defer paint.ClosePainter(g)
	g.Rectangle(20, 10, 180, 140)
	g.Clip()
	g.Translate(40, 25)
	font := paint.NewFont("", 23, true, false)
	g.SetFont(font)
	beforeClip, beforeDepth := g.ClipBounds1(), g.CurrentState()
	var beforeMatrix, afterMatrix geom.Mat3x2
	g.GetMatrix(&beforeMatrix)
	group.Draw(g)
	g.GetMatrix(&afterMatrix)
	if g.Font() != font || g.ClipBounds1() != beforeClip || beforeMatrix != afterMatrix || g.CurrentState() != beforeDepth {
		t.Fatal("caption leaked font, clip, transform or save depth into its caller")
	}
}

func TestGroupBoxCaptionKeepsParentBackground(t *testing.T) {
	for _, title := range []string{"", "Frame", "中文😀"} {
		t.Run(title, func(t *testing.T) {
			group := NewGroupBox(title)
			group.SetSize(180, 80)
			pm := paint.NewPixmap(180, 80)
			defer paint.CloseSurface(pm)
			g := pm.NewPainter()
			defer paint.ClosePainter(g)
			g.SetBrush1(paint.Color{R: 27, G: 63, B: 91, A: 255})
			g.Rectangle(0, 0, 180, 80)
			g.Fill()
			group.Draw(g)
			img, err := pm.Image()
			if err != nil {
				t.Fatal(err)
			}
			r, green, b, a := img.At(12, 0).RGBA()
			if r != 27*257 || green != 63*257 || b != 91*257 || a != 65535 {
				t.Fatal("caption erased a different parent background with FormColor")
			}
		})
	}
}

func TestGroupBoxCaptionFitsAssignedWidth(t *testing.T) {
	title := strings.Repeat("Control pressure ", 5) + "中文😀"
	group := NewGroupBox(title)
	group.SetSize(150, 80)
	p := newGroupBoxTextRecorder()
	group.Draw(p)
	if len(p.texts) != 1 || p.texts[0] == title || !strings.Contains(p.texts[0], "…") ||
		!utf8.ValidString(p.texts[0]) || Theme().Font.TextExtents(p.texts[0]).Width > 124.5 {
		t.Fatalf("caption did not fit its allocation: %q", p.texts)
	}
	if group.Title() != title {
		t.Fatal("paint-only fitting changed the authored title")
	}
}

func TestGroupBoxLongTitleParticipatesInHints(t *testing.T) {
	title := strings.Repeat("Wide title ", 20)
	group := NewGroupBox(title)
	need := Theme().Font.TextExtents(title).Width + 2*(group.borderPadding()+6)
	if got := group.SizeHints().Width; got < need {
		t.Fatalf("preferred width %.2f cannot contain title requiring %.2f", got, need)
	}
}

func TestGroupBoxTitleSetterNotifiesLayout(t *testing.T) {
	resetUIQueue()
	t.Cleanup(resetUIQueue)
	root := newInvalidationVBox()
	root.SetSize(300, 300)
	group := NewGroupBox("Short")
	root.AddWidget(group)
	drainLayoutTest(t)
	root.calls = 0
	group.SetTitle("Runtime header changed")
	drainLayoutTest(t)
	if root.calls == 0 {
		t.Fatal("title change did not admit its owning layout")
	}
	root.calls = 0
	group.SetTitle(group.Title())
	drainLayoutTest(t)
	if root.calls != 0 {
		t.Fatal("unchanged title created new layout work")
	}
}

func TestGroupBoxCheckableHeaderNotifiesLayout(t *testing.T) {
	resetUIQueue()
	t.Cleanup(resetUIQueue)
	theme := Theme()
	previousSize := theme.IconSize
	theme.IconSize = 40
	t.Cleanup(func() { theme.IconSize = previousSize })
	root := newInvalidationVBox()
	root.SetSize(300, 300)
	group := NewGroupBox("Frame")
	root.AddWidget(group)
	drainLayoutTest(t)
	root.calls = 0
	group.SetCheckable(true)
	drainLayoutTest(t)
	_, y, size := group.titleCheckRect()
	if root.calls == 0 || y < 0 || y+size > group.titleHeight() {
		t.Fatalf("header not relaid out/contained: layouts=%d y=%g size=%g band=%g", root.calls, y, size, group.titleHeight())
	}
}

type groupBoxMetricFont struct {
	paint.Font
	textCalls, fontCalls int
}

func (f *groupBoxMetricFont) TextExtents(text string) *paint.TextExtents {
	f.textCalls++
	return f.Font.TextExtents(text)
}

func (f *groupBoxMetricFont) FontExtents() *paint.FontExtents {
	f.fontCalls++
	return f.Font.FontExtents()
}

func TestGroupBoxCaptionMetricsWarmCache(t *testing.T) {
	theme := Theme()
	previous := theme.Font
	font := &groupBoxMetricFont{Font: previous.Dup()}
	theme.Font = font
	t.Cleanup(func() { theme.Font = previous })
	group := NewGroupBox(strings.Repeat("Cached header ", 8))
	group.SetSize(250, 100)
	p := newGroupBoxTextRecorder()
	group.Draw(p)
	group.SizeHintsForWidth(250)
	textCalls, fontCalls := font.textCalls, font.fontCalls
	for i := 0; i < 64; i++ {
		group.Draw(p)
		group.SizeHints()
		group.SizeHintsForWidth(250)
	}
	if font.textCalls != textCalls || font.fontCalls != fontCalls {
		t.Fatalf("warm title was remeasured: text +%d, font +%d", font.textCalls-textCalls, font.fontCalls-fontCalls)
	}
	font.SetSize(font.Size() + 1)
	group.Draw(p)
	group.SizeHints()
	if font.textCalls == textCalls || font.fontCalls == fontCalls {
		t.Fatal("in-place mutable font did not refresh title metrics")
	}
}

func TestGroupBoxInvalidSizeDrawIsQuiet(t *testing.T) {
	for _, size := range [][2]float64{{0, 100}, {100, 0}, {-1, 10}, {math.NaN(), 10}, {10, math.Inf(1)}} {
		group := NewGroupBox("Nothing to paint")
		group.SetSize(size[0], size[1])
		// A nil delegate panics on every Painter method: invalid dimensions
		// must return before touching painter state or its native backend.
		group.Draw(&groupBoxArcRecorder{})
	}
}

func TestGroupBoxPaintContainsConstrainedInk(t *testing.T) {
	for _, size := range [][2]float64{{1, 1}, {8, 8}, {12, 24}, {18, 12}, {30, 40}, {100, 80}} {
		for _, checkable := range []bool{false, true} {
			t.Run(fmt.Sprintf("%gx%g/checkable-%t", size[0], size[1], checkable), func(t *testing.T) {
				group := NewGroupBox(strings.Repeat("Wide title 中文😀 ", 8))
				group.SetSize(size[0], size[1])
				group.SetCheckable(checkable)
				pm := paint.NewPixmap(200, 110)
				defer paint.CloseSurface(pm)
				g := pm.NewPainter()
				defer paint.ClosePainter(g)
				g.Translate(8, 8)
				group.Draw(g)
				img, err := pm.Image()
				if err != nil {
					t.Fatal(err)
				}
				for y := 0; y < 110; y++ {
					for x := 0; x < 200; x++ {
						if x >= 8 && float64(x) < 8+size[0] && y >= 8 && float64(y) < 8+size[1] {
							continue
						}
						if _, _, _, a := img.At(x, y).RGBA(); a != 0 {
							t.Fatalf("ink escaped its allocation at (%d,%d)", x, y)
						}
					}
				}
			})
		}
	}
}

type nonComparableGroupBoxFont struct {
	paint.Font
	tags []int
}

type nestedGroupBoxMetricFont struct{ paint.Font }

func TestGroupBoxNestedNonComparableMetricFont(t *testing.T) {
	theme := Theme()
	previous := theme.Font
	// The outer struct type is comparable, but its interface field contains
	// a noncomparable value. Only a value-level check can safely compare it.
	theme.Font = nestedGroupBoxMetricFont{nonComparableGroupBoxFont{Font: previous.Dup(), tags: []int{1}}}
	t.Cleanup(func() { theme.Font = previous })
	group := NewGroupBox("Nested custom metrics")
	for i := 0; i < 3; i++ {
		if hints := group.SizeHints(); hints.Width <= 0 || hints.Height <= 0 {
			t.Fatalf("invalid nested custom-font hints: %+v", hints)
		}
	}
}

func TestGroupBoxCustomNonComparableMetricFont(t *testing.T) {
	theme := Theme()
	previous := theme.Font
	theme.Font = nonComparableGroupBoxFont{Font: previous.Dup(), tags: []int{1}}
	t.Cleanup(func() { theme.Font = previous })
	group := NewGroupBox("Custom metrics")
	for i := 0; i < 3; i++ {
		if hints := group.SizeHints(); hints.Width <= 0 || hints.Height <= 0 {
			t.Fatalf("invalid custom-font hints: %+v", hints)
		}
	}
}

type tallGroupBoxMetricFont struct{ paint.Font }

func (f *tallGroupBoxMetricFont) TextExtents(text string) *paint.TextExtents {
	ext := *f.Font.TextExtents(text)
	if text != "" {
		ext.YBearing, ext.Height = -28, 38
	}
	return &ext
}

func TestGroupBoxCaptionReservesFallbackInkHeight(t *testing.T) {
	theme := Theme()
	previous := theme.Font
	theme.Font = &tallGroupBoxMetricFont{Font: previous.Dup()}
	t.Cleanup(func() { theme.Font = previous })
	group := NewGroupBox("Tall fallback")
	group.SetSize(300, 100)
	p := newGroupBoxTextRecorder()
	group.Draw(p)
	if len(p.positions) != 1 {
		t.Fatal("missing caption position")
	}
	inkTop := p.positions[0][1] - 28
	if inkTop < 0 || inkTop+38 > group.titleHeight() {
		t.Fatalf("fallback ink exceeds header band: top=%g band=%g", inkTop, group.titleHeight())
	}
}

func TestGroupBoxKeyboardFocusCueClipsToHeader(t *testing.T) {
	isolateEnabledTest(t)
	group := NewGroupBox(strings.Repeat("Options 中文 ", 8))
	group.SetCheckable(true)
	group.SetSize(100, 70)
	base := paint.NewPixmap(130, 100)
	defer paint.CloseSurface(base)
	before := base.NewPainter()
	before.Translate(10, 10)
	group.Draw(before)
	paint.ClosePainter(before)
	baseImage, err := base.Image()
	if err != nil {
		t.Fatal(err)
	}
	group.SetFocus()
	focus := paint.NewPixmap(130, 100)
	defer paint.CloseSurface(focus)
	after := focus.NewPainter()
	after.Translate(10, 10)
	group.Draw(after)
	paint.ClosePainter(after)
	focusImage, err := focus.Image()
	if err != nil {
		t.Fatal(err)
	}
	different := 0
	for y := 0; y < 100; y++ {
		for x := 0; x < 130; x++ {
			if focusImage.At(x, y) != baseImage.At(x, y) {
				different++
				if x < 10 || x >= 110 || y < 10 || float64(y) >= 10+group.titleHeight() {
					t.Fatalf("header focus cue escaped allocation at %d,%d", x, y)
				}
			}
		}
	}
	if different == 0 {
		t.Fatal("keyboard focus has no visible header cue")
	}
}
