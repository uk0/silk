package historian

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/uk0/silk/core"
)

// Record a live tag, drive it through several values, then read the range back
// and assert every point landed in timestamp order.
func TestHistorianRecordAndQuery(t *testing.T) {
	h, err := NewHistorian(filepath.Join(t.TempDir(), "hist.db"))
	if err != nil {
		t.Fatalf("NewHistorian: %v", err)
	}
	defer h.Close()

	tags := core.NewTagDB()
	start := time.Now()
	stop := h.Record(tags, []string{"temp"})

	// Subscribe primes with the fresh tag's current value (0), so the recorded
	// history is that priming sample followed by one row per SetValue. The sleep
	// keeps timestamps strictly increasing so the ordering assertion is exact.
	vals := []float64{10, 20, 30, 25}
	for _, v := range vals {
		time.Sleep(2 * time.Millisecond)
		tags.SetValue("temp", v)
	}
	stop()

	// A value pushed after stop() must be ignored: the subscription is cancelled.
	tags.SetValue("temp", 999)

	got, err := h.Query("temp", start.Add(-time.Hour), start.Add(time.Hour))
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	want := []float64{0, 10, 20, 30, 25} // priming sample, then each SetValue
	if len(got) != len(want) {
		t.Fatalf("got %d samples, want %d: %+v", len(got), len(want), got)
	}
	for i, s := range got {
		if s.Value != want[i] {
			t.Errorf("sample %d value = %v, want %v", i, s.Value, want[i])
		}
		if i > 0 && !got[i-1].TS.Before(s.TS) {
			t.Errorf("samples out of ascending ts order at %d: %v then %v", i, got[i-1].TS, s.TS)
		}
	}
}

// Querying a window with no samples in it returns an empty result, not an error.
func TestHistorianQueryEmptyRange(t *testing.T) {
	h, err := NewHistorian(filepath.Join(t.TempDir(), "hist.db"))
	if err != nil {
		t.Fatalf("NewHistorian: %v", err)
	}
	defer h.Close()

	tags := core.NewTagDB()
	stop := h.Record(tags, []string{"temp"})
	tags.SetValue("temp", 42)
	stop()

	// Window entirely before any recorded sample.
	past := time.Now().Add(-time.Hour)
	got, err := h.Query("temp", past.Add(-time.Hour), past)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no samples in empty range, got %d: %+v", len(got), got)
	}
}
