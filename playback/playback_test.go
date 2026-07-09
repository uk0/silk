package playback

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/historian"
)

// mkSamples builds n samples one second apart with value == index, so
// decimation results are easy to check by value and timestamp.
func mkSamples(n int) []historian.Sample {
	base := time.Unix(0, 0)
	out := make([]historian.Sample, n)
	for i := 0; i < n; i++ {
		out[i] = historian.Sample{TS: base.Add(time.Duration(i) * time.Second), Value: float64(i)}
	}
	return out
}

// Empty and single-element inputs pass through untouched.
func TestDownsampleEmptyAndSingle(t *testing.T) {
	if got := Downsample(nil, 10); len(got) != 0 {
		t.Fatalf("nil: got %d points, want 0", len(got))
	}
	if got := Downsample([]historian.Sample{}, 10); len(got) != 0 {
		t.Fatalf("empty slice: got %d points, want 0", len(got))
	}
	one := mkSamples(1)
	got := Downsample(one, 10)
	if len(got) != 1 || got[0].Value != 0 {
		t.Fatalf("single element must be unchanged, got %+v", got)
	}
}

// When there are no more than maxPoints samples the input is returned unchanged
// (both the strictly-fewer and the exactly-equal cases).
func TestDownsampleUnderOrEqualMaxUnchanged(t *testing.T) {
	in := mkSamples(3)
	if got := Downsample(in, 5); len(got) != 3 {
		t.Fatalf("len<max: got %d points, want 3", len(got))
	}
	if got := Downsample(in, 3); len(got) != 3 {
		t.Fatalf("len==max: got %d points, want 3", len(got))
	}
	// maxPoints < 1 also returns unchanged.
	if got := Downsample(in, 0); len(got) != 3 {
		t.Fatalf("maxPoints=0: got %d points, want 3", len(got))
	}
}

// A larger-than-budget input is bounded to maxPoints, keeps the first and last
// points exactly, and preserves ascending timestamp order.
func TestDownsampleOverMaxBounded(t *testing.T) {
	in := mkSamples(100)
	const max = 10
	got := Downsample(in, max)

	if len(got) > max {
		t.Fatalf("got %d points, exceeds max %d", len(got), max)
	}
	if len(got) < 2 {
		t.Fatalf("got %d points, want at least the two endpoints", len(got))
	}
	first, last := in[0], in[len(in)-1]
	if got[0].Value != first.Value || !got[0].TS.Equal(first.TS) {
		t.Errorf("first point not preserved: got %+v, want %+v", got[0], first)
	}
	end := got[len(got)-1]
	if end.Value != last.Value || !end.TS.Equal(last.TS) {
		t.Errorf("last point not preserved: got %+v, want %+v", end, last)
	}
	for i := 1; i < len(got); i++ {
		if !got[i-1].TS.Before(got[i].TS) {
			t.Errorf("timestamps not strictly ascending at index %d: %v then %v",
				i, got[i-1].TS, got[i].TS)
		}
	}
}

// recordTemp opens a temp-file historian, records the "temp" tag while it is
// driven through vals, and returns the historian plus the recording start time.
// The priming sample (0) plus one row per value means the stored count is
// len(vals)+1.
func recordTemp(t *testing.T, vals []float64) (*historian.Historian, time.Time) {
	t.Helper()
	h, err := historian.NewHistorian(filepath.Join(t.TempDir(), "hist.db"))
	if err != nil {
		t.Fatalf("NewHistorian: %v", err)
	}
	tags := core.NewTagDB()
	start := time.Now()
	stop := h.Record(tags, []string{"temp"})
	for _, v := range vals {
		time.Sleep(2 * time.Millisecond) // keep timestamps strictly increasing
		tags.SetValue("temp", v)
	}
	stop()
	return h, start
}

// LoadRange reads back exactly what a temp historian recorded.
func TestLoadRange(t *testing.T) {
	h, start := recordTemp(t, []float64{10, 20, 30})
	defer h.Close()

	got, err := LoadRange(h, "temp", start.Add(-time.Hour), start.Add(time.Hour))
	if err != nil {
		t.Fatalf("LoadRange: %v", err)
	}
	if len(got) != 4 { // priming 0 + three SetValue rows
		t.Fatalf("got %d samples, want 4: %+v", len(got), got)
	}
	if got[0].Value != 0 || got[3].Value != 30 {
		t.Fatalf("unexpected endpoints: %+v", got)
	}
}

// PlayInto loads, decimates, and feeds a temp historian's range into a GL-free
// LineChart without error or panic. The chart is never Drawn.
func TestPlayInto(t *testing.T) {
	h, start := recordTemp(t, []float64{10, 20, 30, 40, 50})
	defer h.Close()

	chart := gui.NewLineChart() // GL-free construction; do NOT Draw
	err := PlayInto(chart, h, "trend", "temp", start.Add(-time.Hour), start.Add(time.Hour), 3)
	if err != nil {
		t.Fatalf("PlayInto: %v", err)
	}
}
