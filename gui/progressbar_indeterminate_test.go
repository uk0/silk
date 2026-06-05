package gui

import (
	"math"
	"testing"

	"silk/paint"
)

const floatTol = 1e-9

// TestMarqueeOffsetPingPong locks the core sweep math: at phase 0 the
// chunk hugs the left edge, at the half-cycle it hugs the right edge,
// and a full cycle brings it back to the left — a symmetric ping-pong.
// trackWidth=100, chunkWidth=30 → span (max left x) = 70.
func TestMarqueeOffsetPingPong(t *testing.T) {
	const cycle, track, chunk = 1.2, 100.0, 30.0
	span := track - chunk // 70

	// Phase 0: left edge.
	if got := marqueeOffset(0, cycle, track, chunk); math.Abs(got-0) > floatTol {
		t.Errorf("offset at phase 0 = %v, want 0", got)
	}
	// Half-cycle: right edge (trackWidth - chunkWidth).
	if got := marqueeOffset(cycle/2, cycle, track, chunk); math.Abs(got-span) > floatTol {
		t.Errorf("offset at half-cycle = %v, want %v", got, span)
	}
	// Full cycle: back to the left edge.
	if got := marqueeOffset(cycle, cycle, track, chunk); math.Abs(got-0) > floatTol {
		t.Errorf("offset at full cycle = %v, want 0 (ping-pong)", got)
	}
	// Next half-cycle reaches the right edge again — periodicity holds
	// past the first cycle.
	if got := marqueeOffset(cycle*1.5, cycle, track, chunk); math.Abs(got-span) > floatTol {
		t.Errorf("offset at 1.5 cycles = %v, want %v", got, span)
	}
}

// TestMarqueeOffsetSymmetric verifies the sweep out equals the sweep
// back: the position a quarter into the cycle (mid travel to the right)
// equals the position three quarters in (mid travel back to the left).
func TestMarqueeOffsetSymmetric(t *testing.T) {
	const cycle, track, chunk = 1.2, 100.0, 30.0
	out := marqueeOffset(cycle*0.25, cycle, track, chunk)
	back := marqueeOffset(cycle*0.75, cycle, track, chunk)
	if math.Abs(out-back) > floatTol {
		t.Errorf("sweep asymmetric: quarter=%v threequarter=%v", out, back)
	}
	// At a quarter cycle the triangle wave is at 0.5, so x = 0.5*span.
	if want := (track - chunk) * 0.5; math.Abs(out-want) > floatTol {
		t.Errorf("offset at quarter cycle = %v, want %v", out, want)
	}
}

// TestMarqueeOffsetClampsToTrack guarantees the chunk never overruns
// the track regardless of phase, and that degenerate geometry (chunk
// wider than the track, or a non-positive cycle) yields 0 instead of a
// negative or NaN x.
func TestMarqueeOffsetClampsToTrack(t *testing.T) {
	const cycle, track, chunk = 1.2, 100.0, 30.0
	span := track - chunk
	// Sample densely across two cycles; x must stay within [0, span].
	for i := 0; i <= 200; i++ {
		ph := float64(i) / 100.0 * cycle // 0 .. 2 cycles
		x := marqueeOffset(ph, cycle, track, chunk)
		if x < 0 || x > span+floatTol {
			t.Fatalf("offset %v out of [0,%v] at elapsed=%v", x, span, ph)
		}
	}
	// Chunk wider than the track → span <= 0 → 0.
	if got := marqueeOffset(0.3, cycle, 30, 100); got != 0 {
		t.Errorf("chunk wider than track: got %v, want 0", got)
	}
	// Non-positive cycle must not divide by zero.
	if got := marqueeOffset(0.3, 0, track, chunk); got != 0 {
		t.Errorf("zero cycle: got %v, want 0", got)
	}
}

// TestSetIndeterminateTogglesAnimation drives the public API. Turning
// busy on must arm a running heartbeat Animation (so the window keeps
// ticking) and flip IsIndeterminate(); turning it off must clear the
// heartbeat. We assert on the bar's own anim pointer/state — mirroring
// spinner_test.go — because HasActiveAnimations() is global state that
// other tests can pollute.
func TestSetIndeterminateTogglesAnimation(t *testing.T) {
	p := NewProgressBar()
	defer p.SetIndeterminate(false) // never leak a looping anim

	if p.IsIndeterminate() {
		t.Fatalf("new ProgressBar should not start indeterminate")
	}
	if p.anim != nil {
		t.Fatalf("determinate ProgressBar must have no heartbeat anim")
	}

	p.SetIndeterminate(true)
	if !p.IsIndeterminate() {
		t.Errorf("IsIndeterminate() = false after SetIndeterminate(true)")
	}
	if p.anim == nil || p.anim.State() != AnimRunning {
		t.Errorf("busy ProgressBar must have a running heartbeat; got %v", pbAnimState(p))
	}

	p.SetIndeterminate(false)
	if p.IsIndeterminate() {
		t.Errorf("IsIndeterminate() = true after SetIndeterminate(false)")
	}
	if p.anim != nil {
		t.Errorf("idle ProgressBar must clear its heartbeat, got %v", p.anim.State())
	}
}

// TestSetIndeterminateIdempotent: a redundant SetIndeterminate(true)
// must not replace the running heartbeat (no churn / re-alloc).
func TestSetIndeterminateIdempotent(t *testing.T) {
	p := NewProgressBar()
	defer p.SetIndeterminate(false)
	p.SetIndeterminate(true)
	first := p.anim
	p.SetIndeterminate(true) // already busy → no-op
	if p.anim != first {
		t.Errorf("redundant SetIndeterminate(true) replaced the heartbeat")
	}
}

// TestSetIndeterminateAffectsHasActiveAnimations exercises the actual
// requirement: HasActiveAnimations() goes true while busy and returns
// to false once busy is cleared and a tick drains the dead animation.
// Stop() only marks the heartbeat AnimDone; the manager evicts it on
// the next AnimationTick — so we tick before re-checking (this is also
// why an un-stopped looping anim would hang a drain loop; we never
// drain, we stop then tick exactly once).
func TestSetIndeterminateAffectsHasActiveAnimations(t *testing.T) {
	p := NewProgressBar()
	defer p.SetIndeterminate(false)

	p.SetIndeterminate(true)
	if !HasActiveAnimations() {
		t.Errorf("HasActiveAnimations() = false while ProgressBar is busy")
	}

	p.SetIndeterminate(false)
	AnimationTick() // evict the now-Done heartbeat from the manager
	if HasActiveAnimations() {
		t.Errorf("HasActiveAnimations() still true after busy off + tick")
	}
}

// TestDrawIndeterminateSkipsValueFill: in busy mode Draw must paint the
// background + exactly one sliding chunk and nothing else — no second
// value-fill rect, no percentage text. With showText defaulting to
// true, a determinate bar would also emit DrawText; busy mode must not.
func TestDrawIndeterminateSkipsValueFill(t *testing.T) {
	p := NewProgressBar()
	defer p.SetIndeterminate(false)
	p.SetSize(120, 20)
	p.SetValue(0.5)
	p.SetIndeterminate(true)

	rec := newPBRecorder()
	p.Draw(rec)

	// background FillPreserve + chunk Fill = 2 fill ops, 0 text.
	if rec.fillCount != 1 {
		t.Errorf("busy Draw issued %d Fill, want 1 (the chunk)", rec.fillCount)
	}
	if rec.fillPreserveCount != 1 {
		t.Errorf("busy Draw issued %d FillPreserve, want 1 (the track)", rec.fillPreserveCount)
	}
	if rec.textCount != 0 {
		t.Errorf("busy Draw drew %d text runs, want 0", rec.textCount)
	}
}

// TestDrawDeterminateUnchanged: with indeterminate off, Draw keeps the
// original behaviour — track + value fill + percentage text.
func TestDrawDeterminateUnchanged(t *testing.T) {
	p := NewProgressBar()
	p.SetSize(120, 20)
	p.SetValue(0.5)

	rec := newPBRecorder()
	p.Draw(rec)

	if rec.fillCount != 1 {
		t.Errorf("determinate Draw issued %d Fill, want 1 (the value fill)", rec.fillCount)
	}
	if rec.textCount != 1 {
		t.Errorf("determinate Draw drew %d text runs, want 1 (the percentage)", rec.textCount)
	}
}

func pbAnimState(p *ProgressBar) interface{} {
	if p.anim == nil {
		return "<nil>"
	}
	return p.anim.State()
}

// pbRecorder is a minimal paint.Painter that counts the draw ops
// ProgressBar.Draw uses so the indeterminate/determinate branches can
// be asserted without a real render target. Embeds nopPainter (declared
// in spinner_test.go) for every method we don't care about.
type pbRecorder struct {
	paint.Painter
	fillCount         int
	fillPreserveCount int
	textCount         int
}

func newPBRecorder() *pbRecorder {
	return &pbRecorder{Painter: nopPainter{}}
}

func (r *pbRecorder) Rectangle(x, y, w, h float64)     {}
func (r *pbRecorder) SetBrush1(c paint.Color)          {}
func (r *pbRecorder) SetPen1(c paint.Color, w float64) {}
func (r *pbRecorder) Stroke()                          {}
func (r *pbRecorder) Fill()                            { r.fillCount++ }
func (r *pbRecorder) FillPreserve()                    { r.fillPreserveCount++ }
func (r *pbRecorder) SetFont(f paint.Font)             {}

// Font returns the real theme font so the determinate path's
// g.Font().TextExtents(...) measures text headlessly (TextExtents works
// without a render target). The busy path never reaches here — it
// returns before any text work.
func (r *pbRecorder) Font() paint.Font         { return Theme().Font }
func (r *pbRecorder) Translate(tx, ty float64) {}
func (r *pbRecorder) DrawText(s string)        { r.textCount++ }
