package gui

import (
	"fmt"
	"silk/paint"
	"sync"
	"time"
)

// PerfStats is the process-wide sampler for paint / layout / frame timing.
// It's designed to be toggled on-demand with F12: when invisible, the
// per-frame accounting is still cheap (a few atomics) but the overlay does
// no drawing.
type PerfStats struct {
	mu         sync.Mutex
	frameCount int       // frames accumulated in the current 1s window
	lastTime   time.Time // start of the current 1s window
	fps        float64   // FPS computed at the end of the last window

	// Running averages over the last ~60 samples, stored as exponential
	// moving averages so a single outlier doesn't distort the reading.
	paintTime  time.Duration
	layoutTime time.Duration

	// widgetCount is updated by the painter during each frame.
	widgetCount int

	visible bool
}

// GlobalPerfStats is the shared stats singleton. Code elsewhere references
// it rather than instantiating a new PerfStats — F12 toggles this one.
var GlobalPerfStats = &PerfStats{lastTime: time.Now()}

// Toggle flips the overlay on or off and forces a repaint.
func (s *PerfStats) Toggle() {
	s.mu.Lock()
	s.visible = !s.visible
	s.mu.Unlock()
	// Force all visible windows to repaint so the overlay appears/disappears
	// immediately. We don't know which window the user is on, so mark all
	// visible ones dirty. Implementation is platform-specific; on Windows
	// this is currently a no-op (the overlay still appears on the next
	// paint triggered by user interaction).
	markAllWindowsDirty()
}

// IsVisible reports whether the overlay is currently shown.
func (s *PerfStats) IsVisible() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.visible
}

// SetVisible sets the overlay visibility explicitly. Useful for tests.
func (s *PerfStats) SetVisible(v bool) {
	s.mu.Lock()
	s.visible = v
	s.mu.Unlock()
}

// RecordFrame should be called once per frame from the main loop. When the
// 1-second window elapses, the accumulated frame count becomes the reported
// FPS and the window resets. Safe to call even when the overlay is hidden.
func (s *PerfStats) RecordFrame() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.frameCount++
	if s.lastTime.IsZero() {
		s.lastTime = time.Now()
		return
	}
	elapsed := time.Since(s.lastTime)
	if elapsed >= time.Second {
		s.fps = float64(s.frameCount) / elapsed.Seconds()
		s.frameCount = 0
		s.lastTime = time.Now()
	}
}

// RecordPaint folds a paint duration into the rolling average. Call this
// once per frame, wrapping the widget-tree draw pass.
func (s *PerfStats) RecordPaint(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.paintTime == 0 {
		s.paintTime = d
	} else {
		// Exponential moving average with alpha=0.2.
		s.paintTime = (s.paintTime*4 + d) / 5
	}
}

// RecordLayout records a layout-pass duration into the rolling average.
func (s *PerfStats) RecordLayout(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.layoutTime == 0 {
		s.layoutTime = d
	} else {
		s.layoutTime = (s.layoutTime*4 + d) / 5
	}
}

// SetWidgetCount publishes the most recent widget count (computed during
// the last paint pass).
func (s *PerfStats) SetWidgetCount(n int) {
	s.mu.Lock()
	s.widgetCount = n
	s.mu.Unlock()
}

// snapshot returns a read-only copy of the current counters.
func (s *PerfStats) snapshot() (fps float64, paint, layout time.Duration, widgets int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.fps, s.paintTime, s.layoutTime, s.widgetCount
}

// Draw paints the overlay in the top-right corner of the surface. When the
// overlay is hidden, this is a no-op. Colors are hard-coded (dark bg with
// green monospaced text) so the overlay is legible regardless of theme.
func (s *PerfStats) Draw(g paint.Painter, w, h float64) {
	if !s.IsVisible() {
		return
	}
	fps, pt, lt, wc := s.snapshot()

	const (
		padX = 12.0
		padY = 8.0
		gap  = 12.0 // margin from window edge
	)

	font := paint.NewFont("Menlo", 11, false, false)
	fe := font.FontExtents()
	lh := fe.Height + 2

	// Count animations at draw time (cheap — a length read).
	animCount := len(animManager.animations) + len(animManager.groups)

	lines := []string{
		"┌─ Performance ─┐",
		fmt.Sprintf("│ FPS: %-7.1f │", fps),
		fmt.Sprintf("│ Paint: %-5.1fms │", float64(pt.Microseconds())/1000.0),
		fmt.Sprintf("│ Layout: %-4.1fms │", float64(lt.Microseconds())/1000.0),
		fmt.Sprintf("│ Animations: %-2d │", animCount),
		fmt.Sprintf("│ Widgets: %-5d │", wc),
		"└───────────────┘",
	}

	// Measure the widest line for the background rect.
	g.SetFont(font)
	maxW := 0.0
	for _, ln := range lines {
		ext := font.TextExtents(ln)
		if ext.XAdvance > maxW {
			maxW = ext.XAdvance
		}
	}

	boxW := maxW + padX*2
	boxH := float64(len(lines))*lh + padY*2
	boxX := w - boxW - gap
	boxY := gap

	// Semi-transparent dark background.
	g.SetBrush1(paint.Color{R: 0, G: 0, B: 0, A: 190})
	g.Rectangle(boxX, boxY, boxW, boxH)
	g.Fill()

	// Subtle green border.
	g.SetPen1(paint.Color{R: 60, G: 200, B: 110, A: 220}, 1)
	g.Rectangle(boxX, boxY, boxW, boxH)
	g.Stroke()

	// Green monospace text.
	g.SetBrush1(paint.Color{R: 90, G: 220, B: 120, A: 255})
	tx := boxX + padX
	ty := boxY + padY + fe.Ascent
	for _, ln := range lines {
		g.DrawText1(tx, ty, ln)
		ty += lh
	}
}

// CountWidgets walks the widget tree rooted at iw and returns the total
// count (including iw itself). Used by the perf overlay to display a
// live widget count.
func CountWidgets(iw IWidget) int {
	if iw == nil {
		return 0
	}
	n := 1
	for _, ch := range iw.Children() {
		n += CountWidgets(ch)
	}
	return n
}
