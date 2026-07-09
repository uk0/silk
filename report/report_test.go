package report

import (
	"bytes"
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/historian"
)

// The pure reducer over a fixed slice: every aggregate on a known bucket, plus
// the empty-bucket contract (Sum/Count = 0, everything else NaN). No historian.
func TestAggregateReducer(t *testing.T) {
	// Ascending-ts order, as historian.Query returns: First=2, Last=4.
	s := []historian.Sample{{Value: 2}, {Value: 8}, {Value: 4}}
	cases := []struct {
		a    Aggregate
		want float64
	}{
		{Avg, 14.0 / 3.0},
		{Min, 2},
		{Max, 8},
		{Sum, 14},
		{Count, 3},
		{First, 2},
		{Last, 4},
	}
	for _, c := range cases {
		if got := aggregate(s, c.a); got != c.want {
			t.Errorf("aggregate(%s) = %v, want %v", c.a, got, c.want)
		}
	}

	if got := aggregate(nil, Sum); got != 0 {
		t.Errorf("empty Sum = %v, want 0", got)
	}
	if got := aggregate(nil, Count); got != 0 {
		t.Errorf("empty Count = %v, want 0", got)
	}
	if got := aggregate(nil, Avg); !math.IsNaN(got) {
		t.Errorf("empty Avg = %v, want NaN", got)
	}
	if got := aggregate(nil, Max); !math.IsNaN(got) {
		t.Errorf("empty Max = %v, want NaN", got)
	}

	if Avg.String() != "Avg" || Max.String() != "Max" || Count.String() != "Count" {
		t.Errorf("Aggregate.String mismatch: %s/%s/%s", Avg, Max, Count)
	}
}

// buildFixture records two tags, drives a few values through them (all inside
// the first bucket), and returns the historian plus the [start,to) window.
func buildFixture(t *testing.T) (h *historian.Historian, start, to time.Time, interval time.Duration) {
	t.Helper()
	h, err := historian.NewHistorian(filepath.Join(t.TempDir(), "hist.db"))
	if err != nil {
		t.Fatalf("NewHistorian: %v", err)
	}
	t.Cleanup(func() { h.Close() })

	tags := core.NewTagDB()
	start = time.Now()
	stop := h.Record(tags, []string{"temp", "flow"})
	// Record primes each fresh tag with its current value (0). The SetValues run
	// in a tight loop with no sleeps, so every sample lands within a few ms of
	// start — comfortably inside bucket 0 = [start, start+100ms).
	//   temp samples: [0, 10, 20, 30]
	//   flow samples: [0, 6, 12]
	for _, v := range []float64{10, 20, 30} {
		tags.SetValue("temp", v)
	}
	for _, v := range []float64{6, 12} {
		tags.SetValue("flow", v)
	}
	stop()

	// 100ms interval over a 1s window -> 10 buckets; only bucket 0 holds samples.
	return h, start, start.Add(time.Second), 100 * time.Millisecond
}

// Build an Avg and a Max report and assert the first bucket's numbers, plus that
// a later (sample-free) bucket omits every tag.
func TestBuildAvgAndMax(t *testing.T) {
	h, start, to, interval := buildFixture(t)
	tags := []string{"temp", "flow"}

	rAvg, err := Build(h, tags, start, to, interval, Avg)
	if err != nil {
		t.Fatalf("Build Avg: %v", err)
	}
	if len(rAvg.Buckets) != 10 {
		t.Fatalf("Avg buckets = %d, want 10", len(rAvg.Buckets))
	}
	// temp: mean(0,10,20,30)=15 ; flow: mean(0,6,12)=6
	if got := rAvg.Buckets[0].Values["temp"]; got != 15 {
		t.Errorf("Avg temp bucket0 = %v, want 15", got)
	}
	if got := rAvg.Buckets[0].Values["flow"]; got != 6 {
		t.Errorf("Avg flow bucket0 = %v, want 6", got)
	}
	// A sample-free bucket omits the tag entirely (empty cell, not NaN).
	if _, ok := rAvg.Buckets[9].Values["temp"]; ok {
		t.Errorf("expected bucket 9 to omit temp (no samples)")
	}
	if len(rAvg.Buckets[9].Values) != 0 {
		t.Errorf("bucket 9 Values = %v, want empty", rAvg.Buckets[9].Values)
	}
	// Buckets are the requested half-open slices, in order.
	if !rAvg.Buckets[0].From.Equal(start) || !rAvg.Buckets[0].To.Equal(start.Add(interval)) {
		t.Errorf("bucket0 span = [%v,%v), want [%v,%v)", rAvg.Buckets[0].From, rAvg.Buckets[0].To, start, start.Add(interval))
	}

	rMax, err := Build(h, tags, start, to, interval, Max)
	if err != nil {
		t.Fatalf("Build Max: %v", err)
	}
	if got := rMax.Buckets[0].Values["temp"]; got != 30 {
		t.Errorf("Max temp bucket0 = %v, want 30", got)
	}
	if got := rMax.Buckets[0].Values["flow"]; got != 12 {
		t.Errorf("Max flow bucket0 = %v, want 12", got)
	}
}

// A non-positive interval is a caller error, not an infinite loop.
func TestBuildRejectsNonPositiveInterval(t *testing.T) {
	h, start, to, _ := buildFixture(t)
	if _, err := Build(h, []string{"temp"}, start, to, 0, Avg); err == nil {
		t.Fatalf("Build with interval 0: expected error, got nil")
	}
}

// WriteCSV emits the "time + tags" header and one row per bucket; the first row
// carries bucket 0's aggregated values.
func TestWriteCSV(t *testing.T) {
	h, start, to, interval := buildFixture(t)
	r, err := Build(h, []string{"temp", "flow"}, start, to, interval, Avg)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	var buf bytes.Buffer
	if err := r.WriteCSV(&buf); err != nil {
		t.Fatalf("WriteCSV: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "time,temp,flow") {
		t.Errorf("CSV missing header, got:\n%s", out)
	}
	if !strings.Contains(out, ",15,6") { // bucket 0 data row: <time>,15,6
		t.Errorf("CSV missing bucket-0 data row, got:\n%s", out)
	}
	// header + 10 bucket rows = 11 non-empty lines.
	lines := strings.Split(strings.TrimRight(out, "\r\n"), "\n")
	if len(lines) != 11 {
		t.Errorf("CSV line count = %d, want 11", len(lines))
	}
}

// WriteHTML emits a <table> with a header row and bucket 0's aggregated cell.
func TestWriteHTML(t *testing.T) {
	h, start, to, interval := buildFixture(t)
	r, err := Build(h, []string{"temp", "flow"}, start, to, interval, Avg)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	var buf bytes.Buffer
	if err := r.WriteHTML(&buf); err != nil {
		t.Fatalf("WriteHTML: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"<table>", "<th>temp</th>", "<th>flow</th>", "<td>15</td>", "<td>6</td>"} {
		if !strings.Contains(out, want) {
			t.Errorf("HTML missing %q, got:\n%s", want, out)
		}
	}
}
