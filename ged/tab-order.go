package ged

import (
	"fmt"
	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
	"math"
	"sort"
)

// TabOrderMode provides a visual tab order editor that overlays numbered
// circles on each focusable widget. When active, clicking a widget assigns it
// the next number in the tab sequence. This is similar to Qt Creator's tab
// order editing mode.
type TabOrderMode struct {
	active   bool
	scene    *GedScene
	order    []graph.IItem
	nextNum  int
	assigned map[graph.IItem]int
}

// NewTabOrderMode creates a new tab order editor for the given scene.
func NewTabOrderMode(scene *GedScene) *TabOrderMode {
	p := new(TabOrderMode)
	p.scene = scene
	p.assigned = make(map[graph.IItem]int)
	return p
}

// IsActive returns whether tab order editing mode is active.
func (this *TabOrderMode) IsActive() bool {
	return this.active
}

// Enter activates tab order editing mode. The existing order is preserved;
// if none exists yet, items are sorted top-to-bottom, left-to-right.
func (this *TabOrderMode) Enter() {
	this.active = true
	if len(this.order) == 0 {
		this.Reset()
	}
	this.nextNum = len(this.order) + 1
	this.rebuildAssigned()
}

// Exit deactivates tab order editing mode.
func (this *TabOrderMode) Exit() {
	this.active = false
}

// Reset resets the tab order to the default: top-to-bottom, left-to-right.
func (this *TabOrderMode) Reset() {
	this.order = nil
	this.assigned = make(map[graph.IItem]int)
	if this.scene == nil {
		return
	}
	children := this.scene.Children()
	// Collect focusable widgets
	var focusable []graph.IItem
	for _, c := range children {
		if _, ok := c.(*FakeWidget); ok {
			focusable = append(focusable, c)
		}
	}
	// Sort by Y first, then by X (top-to-bottom, left-to-right)
	sort.Slice(focusable, func(i, j int) bool {
		yi := focusable[i].Y()
		yj := focusable[j].Y()
		if math.Abs(yi-yj) > 2 { // tolerance for same row
			return yi < yj
		}
		return focusable[i].X() < focusable[j].X()
	})
	this.order = focusable
	this.nextNum = len(focusable) + 1
	this.rebuildAssigned()
}

// rebuildAssigned populates the lookup map from the order slice.
func (this *TabOrderMode) rebuildAssigned() {
	this.assigned = make(map[graph.IItem]int)
	for i, item := range this.order {
		this.assigned[item] = i + 1
	}
}

// Order returns the current tab order list.
func (this *TabOrderMode) Order() []graph.IItem {
	return this.order
}

// OnLeftDown handles clicks during tab order editing. Clicking a widget
// assigns it the next tab number. If all widgets are already assigned,
// clicking starts a new sequence.
func (this *TabOrderMode) OnLeftDown(x, y float64) bool {
	if !this.active || this.scene == nil {
		return false
	}

	// Find the widget under the click
	children := this.scene.Children()
	for _, c := range children {
		cx, cy, cw, ch := c.Bounds()
		if x >= cx && x <= cx+cw && y >= cy && y <= cy+ch {
			// Check if already assigned
			if _, exists := this.assigned[c]; exists {
				// Already assigned; restart the sequence
				this.order = nil
				this.assigned = make(map[graph.IItem]int)
				this.nextNum = 1
			}
			this.order = append(this.order, c)
			this.assigned[c] = this.nextNum
			this.nextNum++
			return true
		}
	}
	return false
}

// DrawOverlay renders the tab order numbers as blue circles on each widget.
// This should be called after the scene is drawn.
func (this *TabOrderMode) DrawOverlay(g paint.Painter) {
	if !this.active || this.scene == nil {
		return
	}

	circleFont := paint.NewFont(gui.Theme().Font.Family(), 11, true, false)
	radius := 10.0

	children := this.scene.Children()
	for _, c := range children {
		num, ok := this.assigned[c]
		if !ok {
			// Unassigned widget: draw gray circle with "?"
			cx, cy := c.X(), c.Y()
			centerX := cx + radius + 2
			centerY := cy + radius + 2

			// Gray circle background
			g.SetBrush1(paint.Color{180, 180, 190, 200})
			g.Arc(centerX, centerY, radius, 0, 2*math.Pi)
			g.Fill()

			// Question mark
			g.SetBrush1(paint.Color{255, 255, 255, 255})
			g.SetFont(circleFont)
			g.DrawText1(centerX-3, centerY+4, "?")
			continue
		}

		cx, cy := c.X(), c.Y()
		centerX := cx + radius + 2
		centerY := cy + radius + 2

		// Blue circle background
		g.SetBrush1(paint.Color{41, 98, 215, 230})
		g.Arc(centerX, centerY, radius, 0, 2*math.Pi)
		g.Fill()

		// White circle border
		g.SetPen1(paint.Color{255, 255, 255, 255}, 1.5)
		g.Arc(centerX, centerY, radius, 0, 2*math.Pi)
		g.Stroke()

		// Number text
		numStr := fmt.Sprintf("%d", num)
		g.SetBrush1(paint.Color{255, 255, 255, 255})
		g.SetFont(circleFont)
		ext := circleFont.TextExtents(numStr)
		tx := centerX - ext.Width*0.5 - ext.XBearing
		ty := centerY - ext.Height*0.5 - ext.YBearing
		g.DrawText1(tx, ty, numStr)
	}

	// Draw instructional text at the top
	g.SetBrush1(paint.Color{41, 98, 215, 200})
	g.Rectangle(0, -18, 200, 16)
	g.Fill()
	g.SetBrush1(paint.Color{255, 255, 255, 255})
	g.SetFont(paint.NewFont(gui.Theme().Font.Family(), 11, false, false))
	g.DrawText1(4, -6, "Tab Order Mode - Click widgets to set order")
}
