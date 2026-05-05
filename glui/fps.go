package glui

import (
	"fmt"
	"time"
)

// Frame-rate measurement helper.
//
// FPSCounter stores a sliding-window history of frame timestamps and
// reports the rolling 1-second average. The window self-trims on every
// Tick(): timestamps older than 1 second are dropped from the front of
// the slice. Callers wire it into the render loop by calling Tick() once
// per frame (typically right after Begin()) and reading FPS() /
// AvgFrameMs() from the HUD overlay.

// FPSCounter tracks frame timing for performance measurement.
//
// Usage: call Tick() once per frame. Read FPS() and AvgFrameMs() to get
// rolling 1-second averages. The counter does not allocate per-frame
// once the sliding window settles into a steady state — Tick reuses the
// underlying slice's capacity for the lifetime of the counter.
type FPSCounter struct {
	frameTimes []time.Time // recent frame timestamps (sliding window)
	capacity   int
}

// NewFPSCounter constructs an FPSCounter with a 120-frame initial
// capacity — comfortable for a 120 Hz display, more than enough for the
// 60 Hz baseline. Tick() grows the slice as needed and compacts when it
// drifts far above target.
func NewFPSCounter() *FPSCounter {
	return &FPSCounter{capacity: 120}
}

// Tick records the current frame. Call once per Begin() or after End().
// Stale frames (older than 1 second) are dropped from the front of the
// window in the same call so reads of FPS() / AvgFrameMs() are always
// up-to-date without a separate trim step.
func (f *FPSCounter) Tick() {
	now := time.Now()
	cutoff := now.Add(-time.Second)

	// Drop frames older than 1 second from the front. If every recorded
	// frame predates the cutoff (e.g. after a long stall) the loop
	// completes without finding a "recent" entry and we drop the whole
	// slice — initialising keep to len(frameTimes) is the cheap way to
	// express that, since a real trim point will overwrite it via break.
	keep := len(f.frameTimes)
	for i, t := range f.frameTimes {
		if t.After(cutoff) {
			keep = i
			break
		}
	}
	if keep > 0 {
		f.frameTimes = f.frameTimes[keep:]
	}

	// Compact occasionally — if the underlying array has grown well
	// past the steady-state capacity (e.g. after a burst), reallocate
	// to free the unused tail.
	if cap(f.frameTimes) > f.capacity*2 {
		nt := make([]time.Time, len(f.frameTimes), f.capacity)
		copy(nt, f.frameTimes)
		f.frameTimes = nt
	}
	f.frameTimes = append(f.frameTimes, now)
}

// FPS returns frames recorded in the trailing 1-second window. The
// numerator is (count - 1) because we measure intervals between frames,
// not frame counts themselves; with N timestamps there are N-1 intervals
// spanning the window.
func (f *FPSCounter) FPS() float64 {
	if len(f.frameTimes) < 2 {
		return 0
	}
	span := f.frameTimes[len(f.frameTimes)-1].Sub(f.frameTimes[0]).Seconds()
	if span <= 0 {
		return 0
	}
	return float64(len(f.frameTimes)-1) / span
}

// AvgFrameMs returns the average frame time over the trailing window in
// milliseconds. Handy for HUD readouts where frame budget is more
// intuitive than rate (e.g. "16.7 ms" vs. "60 fps").
func (f *FPSCounter) AvgFrameMs() float64 {
	fps := f.FPS()
	if fps <= 0 {
		return 0
	}
	return 1000.0 / fps
}

// Format returns a one-line string for HUD overlay. Format is fixed at
// "<fps> fps · <ms> ms" with one and two decimal places respectively —
// enough precision for visual diagnosis without truncation jitter.
func (f *FPSCounter) Format() string {
	return fmt.Sprintf("%.1f fps · %.2f ms", f.FPS(), f.AvgFrameMs())
}
