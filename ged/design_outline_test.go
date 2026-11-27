package ged

import (
	"testing"

	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

// Record only the scene decoration. Real widget painting (including the Card
// title) happens on its own pixmap and is not routed through this painter.
type containerOutlineProbe struct {
	paint.Painter
	lines   [][2][2]float64
	start   [2]float64
	text    []string
	strokes int
}

func (p *containerOutlineProbe) MoveTo(x, y float64) {
	p.start = [2]float64{x, y}
	p.Painter.MoveTo(x, y)
}

func (p *containerOutlineProbe) LineTo(x, y float64) {
	end := [2]float64{x, y}
	p.lines = append(p.lines, [2][2]float64{p.start, end})
	p.start = end
	p.Painter.LineTo(x, y)
}

func (p *containerOutlineProbe) DrawText1(x, y float64, text string) {
	p.text = append(p.text, text)
	p.Painter.DrawText1(x, y, text)
}

func (p *containerOutlineProbe) Stroke() {
	p.strokes++
	p.Painter.Stroke()
}

func TestContainerOutlineDoesNotPaintOverWidgetChrome(t *testing.T) {
	for _, factory := range []string{"gui.Card", "gui.GridLayout", "gui.GroupBox", "gui.TabWidget"} {
		t.Run(factory, func(t *testing.T) {
			f, err := NewFakeWidgetFromFactory(factory)
			if err != nil {
				t.Fatal(err)
			}
			if card, ok := f.Widget().(*gui.Card); ok {
				card.SetTitle("Drive panel")
			}
			const w, h = 42.0, 25.0
			f.SetBounds(9, 7, w, h)
			target := paint.NewPixmap(240, 160)
			defer paint.CloseSurface(target)
			g := target.NewPainter()
			defer paint.ClosePainter(g)
			g.Scale(3.78, 3.78)
			p := &containerOutlineProbe{Painter: g}
			before := p.CurrentState()
			f.DrawSelf(p)
			if p.CurrentState() != before {
				t.Error("outline leaked painter state")
			}
			if len(p.text) != 0 {
				t.Errorf("canvas painted a duplicate type caption over the widget: %q", p.text)
			}
			if len(p.lines) < 4 || p.strokes != 2 {
				t.Fatalf("expected a dashed outline and one batched decoration stroke, got %d segments, %d strokes", len(p.lines), p.strokes)
			}
			for _, line := range p.lines {
				a, b := line[0], line[1]
				if (a[0] == b[0]) == (a[1] == b[1]) {
					t.Errorf("invalid outline segment: %v", line)
				}
				for _, point := range line {
					x, y := point[0], point[1]
					if !(0 < x && x < w && 0 < y && y < h &&
						(x <= 0.5 || x >= w-0.5 || y <= 0.5 || y >= h-0.5)) {
						t.Errorf("outline segment is outside the inset perimeter: %v", line)
					}
				}
			}
		})
	}
}

func TestContainerOutlineBatchesLargeBoundsAndSkipsTinyBounds(t *testing.T) {
	target := paint.NewPixmap(32, 32)
	defer paint.CloseSurface(target)
	g := target.NewPainter()
	defer paint.ClosePainter(g)

	p := &containerOutlineProbe{Painter: g}
	drawDesignContainerOutline(p, 0.6, 0.6)
	if p.strokes != 0 || len(p.lines) != 0 {
		t.Fatal("container too small for the inset should not get an outline")
	}
	drawDesignContainerOutline(p, 1e5, 1e5)
	if p.strokes != 1 || len(p.lines) != 4*256 {
		t.Fatalf("unbounded or missing outline work: %d strokes, %d segments", p.strokes, len(p.lines))
	}
}
