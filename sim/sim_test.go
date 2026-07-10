package sim

import (
	"math"
	"testing"
	"time"

	"github.com/uk0/silk/driver"
)

// pinnedSim returns a Sim whose clock is driven by *elapsed: reads reflect the
// waveform at whatever duration elapsed points to when ReadPoint is called.
func pinnedSim(elapsed *time.Duration) *Sim {
	base := time.Unix(1_000_000, 0)
	s := NewSim()
	s.start = base
	s.Now = func() time.Time { return base.Add(*elapsed) }
	return s
}

func readFloat(t *testing.T, s *Sim, addr string, elapsed *time.Duration, at time.Duration) float64 {
	t.Helper()
	*elapsed = at
	v, err := s.ReadPoint(driver.TagPoint{Address: addr})
	if err != nil {
		t.Fatalf("ReadPoint(%q) at %v: %v", addr, at, err)
	}
	f, ok := v.(float64)
	if !ok {
		t.Fatalf("ReadPoint(%q) = %T, want float64", addr, v)
	}
	return f
}

func approx(t *testing.T, got, want float64, at time.Duration) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("at %v: got %v, want %v", at, got, want)
	}
}

// TestSine pins the phase: elapsed=0 sits at the rising zero-crossing (midpoint),
// a quarter period reaches max, three-quarters reaches min.
func TestSine(t *testing.T) {
	var el time.Duration
	s := pinnedSim(&el)
	const addr = "sine:min=0,max=100,period=10s"
	cases := []struct {
		at   time.Duration
		want float64
	}{
		{0, 50},                        // 0.5+0.5*sin(0)
		{2500 * time.Millisecond, 100}, // quarter period -> max
		{5 * time.Second, 50},          // half period -> midpoint
		{7500 * time.Millisecond, 0},   // three-quarter period -> min
		{10 * time.Second, 50},         // full period -> back to midpoint
	}
	for _, c := range cases {
		approx(t, readFloat(t, s, addr, &el, c.at), c.want, c.at)
	}
}

// TestRamp checks the sawtooth is linear across a period and wraps at the
// boundary.
func TestRamp(t *testing.T) {
	var el time.Duration
	s := pinnedSim(&el)
	const addr = "ramp:min=0,max=100,period=4s"
	cases := []struct {
		at   time.Duration
		want float64
	}{
		{0, 0},
		{1 * time.Second, 25},
		{2 * time.Second, 50},
		{3 * time.Second, 75},
		{4 * time.Second, 0}, // wraps
		{5 * time.Second, 25},
	}
	for _, c := range cases {
		approx(t, readFloat(t, s, addr, &el, c.at), c.want, c.at)
	}
}

// TestConst returns the fixed value regardless of elapsed time.
func TestConst(t *testing.T) {
	var el time.Duration
	s := pinnedSim(&el)
	for _, at := range []time.Duration{0, time.Second, time.Hour} {
		approx(t, readFloat(t, s, "const:42", &el, at), 42, at)
	}
}

// TestToggle stays false within the first period and flips true across the
// period boundary.
func TestToggle(t *testing.T) {
	var el time.Duration
	s := pinnedSim(&el)
	const addr = "toggle:period=2s"
	cases := []struct {
		at   time.Duration
		want bool
	}{
		{0, false},
		{1 * time.Second, false},
		{1999 * time.Millisecond, false},
		{2 * time.Second, true}, // crosses the period boundary
		{3 * time.Second, true},
		{4 * time.Second, false},
	}
	for _, c := range cases {
		el = c.at
		v, err := s.ReadPoint(driver.TagPoint{Address: addr})
		if err != nil {
			t.Fatalf("ReadPoint at %v: %v", c.at, err)
		}
		if b, ok := v.(bool); !ok {
			t.Fatalf("toggle = %T, want bool", v)
		} else if b != c.want {
			t.Errorf("at %v: toggle = %v, want %v", c.at, b, c.want)
		}
	}
}

// TestWriteOverride verifies a WritePoint value is read back verbatim and
// shadows the waveform the Address would otherwise produce.
func TestWriteOverride(t *testing.T) {
	var el time.Duration
	s := pinnedSim(&el)
	p := driver.TagPoint{Address: "sine:min=0,max=100,period=10s", Access: driver.ReadWrite}

	el = 2500 * time.Millisecond // waveform here would be 100
	if err := s.WritePoint(p, 77.0); err != nil {
		t.Fatalf("WritePoint: %v", err)
	}
	v, err := s.ReadPoint(p)
	if err != nil {
		t.Fatalf("ReadPoint: %v", err)
	}
	if v != 77.0 {
		t.Errorf("override read back = %v, want 77", v)
	}

	// A different, non-overridden address still computes from its waveform.
	other := readFloat(t, s, "const:5", &el, el)
	approx(t, other, 5, el)
}

// TestRandomRange checks the random waveform stays within [min,max] and is
// deterministic for a given instant (no global rand state).
func TestRandomRange(t *testing.T) {
	var el time.Duration
	s := pinnedSim(&el)
	const addr = "random:min=10,max=20"
	for _, at := range []time.Duration{0, 1, 250 * time.Millisecond, time.Second, time.Hour} {
		got := readFloat(t, s, addr, &el, at)
		if got < 10 || got >= 20 {
			t.Errorf("at %v: random = %v, want [10,20)", at, got)
		}
		again := readFloat(t, s, addr, &el, at)
		if got != again {
			t.Errorf("at %v: random not deterministic: %v != %v", at, got, again)
		}
	}
}

// TestBadAddress confirms malformed specs error rather than returning a value.
func TestBadAddress(t *testing.T) {
	s := NewSim()
	for _, addr := range []string{
		"",                 // empty
		"bogus:1",          // unknown wave
		"sine:min=oops",    // bad float
		"const:",           // missing const value
		"toggle:period=0s", // non-positive period
		"ramp:period=xyz",  // bad duration
		"sine:foo=1",       // unknown param
		"sine:min",         // param missing '='
	} {
		if _, err := s.ReadPoint(driver.TagPoint{Address: addr}); err == nil {
			t.Errorf("ReadPoint(%q) = nil error, want error", addr)
		}
	}
}

// TestConnectClose confirms the lifecycle calls are no-op successes.
func TestConnectClose(t *testing.T) {
	s := NewSim()
	if err := s.Connect(); err != nil {
		t.Errorf("Connect: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}
