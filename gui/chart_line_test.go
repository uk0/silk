package gui

import (
	"math"
	"testing"
	"time"

	"github.com/uk0/silk/paint"
)

// TestLineChartStaticSeriesUnchanged locks the original static AddSeries/plot
// path: AddSeries appends an index-X series (never rolling), auto-scale derives
// the same padded Y range as before, SetYRange disables auto-scale, and
// ClearSeries empties the chart.
func TestLineChartStaticSeriesUnchanged(t *testing.T) {
	c := NewLineChart()
	if !c.AutoScale() {
		t.Fatal("auto-scale should default to true")
	}
	c.AddSeries("v", paint.Color{R: 0x10, G: 0x20, B: 0x30, A: 255}, []float64{10, 20, 30})

	if len(c.series) != 1 {
		t.Fatalf("series count = %d, want 1", len(c.series))
	}
	s := c.series[0]
	if s.rolling {
		t.Error("AddSeries must NOT enable rolling mode")
	}
	if len(s.Points) != 3 || s.Points[0] != 10 || s.Points[2] != 30 {
		t.Errorf("static Points not preserved: %v", s.Points)
	}
	// Auto-scale: lo=10, hi=30, pad=(30-10)*0.05=1 -> [9, 31] (unchanged formula).
	if c.minY != 9 || c.maxY != 31 {
		t.Errorf("auto-scale range = [%v, %v], want [9, 31]", c.minY, c.maxY)
	}

	c.SetYRange(0, 50)
	if c.AutoScale() {
		t.Error("SetYRange must disable auto-scale")
	}
	if c.minY != 0 || c.maxY != 50 {
		t.Errorf("SetYRange = [%v, %v], want [0, 50]", c.minY, c.maxY)
	}

	c.ClearSeries()
	if len(c.series) != 0 {
		t.Errorf("ClearSeries left %d series", len(c.series))
	}
}

// TestLineChartRollingRingBuffer verifies EnableRolling + AddSample past
// capacity drops the oldest samples (ring behavior) and keeps them ordered
// oldest-first, and that AddSample auto-enables rolling when called first.
func TestLineChartRollingRingBuffer(t *testing.T) {
	c := NewLineChart()
	c.EnableRolling("temp", 3)
	base := time.Unix(1700000000, 0)
	for i := 0; i < 5; i++ {
		c.AddSample("temp", base.Add(time.Duration(i)*time.Second), float64(i))
	}

	s := &c.series[0]
	if !s.rolling {
		t.Fatal("EnableRolling did not set rolling")
	}
	if s.rcount != 3 {
		t.Fatalf("rcount = %d, want 3 (capacity cap)", s.rcount)
	}
	got := s.orderedSamples()
	if len(got) != 3 {
		t.Fatalf("orderedSamples len = %d, want 3", len(got))
	}
	// Oldest two (values 0,1) dropped; buffer holds 2,3,4 in chronological order.
	wantV := []float64{2, 3, 4}
	for i, smp := range got {
		if smp.V != wantV[i] {
			t.Errorf("sample[%d].V = %v, want %v (oldest not dropped)", i, smp.V, wantV[i])
		}
		wantT := base.Add(time.Duration(i+2) * time.Second)
		if !smp.T.Equal(wantT) {
			t.Errorf("sample[%d].T = %v, want %v", i, smp.T, wantT)
		}
	}

	// AddSample on an unknown series auto-enables rolling.
	c2 := NewLineChart()
	c2.AddSample("auto", base, 1)
	if len(c2.series) != 1 || !c2.series[0].rolling {
		t.Error("AddSample should auto-create and enable a rolling series")
	}
}

// TestLineChartRollingTimeWindow verifies SetTimeWindow filters the visible
// samples to the trailing window (newest at the right), with 0 meaning "all".
func TestLineChartRollingTimeWindow(t *testing.T) {
	c := NewLineChart()
	c.EnableRolling("flow", 10)
	base := time.Unix(1700000000, 0)
	for i := 0; i < 6; i++ { // t = 0..5s, values 0..5
		c.AddSample("flow", base.Add(time.Duration(i)*time.Second), float64(i))
	}

	// Default window (0) shows the whole buffer.
	if got := c.series[0].visibleSamples(c.timeWindow); len(got) != 6 {
		t.Fatalf("default window visible = %d, want 6 (full buffer)", len(got))
	}

	// 3s window ending at newest (t=5s) -> cutoff 2s -> keep t=2,3,4,5.
	c.SetTimeWindow(3 * time.Second)
	if c.TimeWindow() != 3*time.Second {
		t.Fatalf("TimeWindow = %v, want 3s", c.TimeWindow())
	}
	got := c.series[0].visibleSamples(c.timeWindow)
	if len(got) != 4 {
		t.Fatalf("windowed visible = %d, want 4", len(got))
	}
	if got[0].V != 2 || got[len(got)-1].V != 5 {
		t.Errorf("visible window = [%v..%v], want [2..5]", got[0].V, got[len(got)-1].V)
	}
}

// TestRollingXPixelMapping table-tests the pure time->pixel helper: newest at
// the right edge, one window older at the left edge, linear in between.
func TestRollingXPixelMapping(t *testing.T) {
	base := time.Unix(1700000000, 0)
	window := 10 * time.Second
	const left, width = 50.0, 200.0
	right := base.Add(window) // newest sample time

	cases := []struct {
		name   string
		sample time.Time
		window time.Duration
		want   float64
	}{
		{"newest at right edge", right, window, left + width},                          // 250
		{"one window old at left edge", right.Add(-window), window, left},              // 50
		{"half window at middle", right.Add(-window / 2), window, left + width/2},      // 150
		{"older than window off-screen", right.Add(-2 * window), window, left - width}, // -150
		{"zero window pins to right", right.Add(-5 * time.Second), 0, left + width},    // 250
	}
	for _, tc := range cases {
		got := rollingX(tc.sample, right, tc.window, left, width)
		if math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("%s: rollingX = %v, want %v", tc.name, got, tc.want)
		}
	}
}
