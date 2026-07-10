// Package sim provides a synthetic driver.Driver that generates values from
// simple time-based waveforms, so silk HMIs can run, be demoed and be tested
// without any real PLC or field device attached (the "仿真" / simulation mode
// familiar from FameView and other SCADA tools).
//
// A point's Address encodes the waveform and its parameters as
// "<wave>:<params>":
//
//	sine:min=0,max=100,period=10s   smooth 0.5+0.5*sin, one full cycle per period
//	ramp:min=0,max=100,period=5s    linear sawtooth min->max, repeating each period
//	toggle:period=2s                bool, flips every period
//	const:42                        a fixed number
//	random:min=0,max=100            a value in [min,max], deterministic per instant
//
// Params are comma-separated key=value pairs; min and max default to 0 and 100,
// period defaults to 1s when omitted. const takes a single bare number instead
// of key=value pairs. period is any Go duration (time.ParseDuration).
//
// Waveform values track elapsed = Now()-start, where start is captured in
// NewSim and Now is an injectable clock (time.Now in production, a pinned
// function in tests). WritePoint records an override keyed by Address; a
// subsequent ReadPoint of that Address returns the written value instead of the
// waveform, so a read-write point behaves like a holding register once set.
package sim

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/uk0/silk/driver"
)

// Sim is a hardware-free driver.Driver. Connect/Close are no-ops. ReadPoint
// derives a value from the point's Address waveform (or a prior WritePoint
// override); WritePoint stores an override under the Address.
type Sim struct {
	// Now is the clock used for elapsed time. Defaults to time.Now in NewSim;
	// tests replace it with a pinned function to control the waveform phase.
	Now func() time.Time

	start     time.Time              // captured in NewSim; elapsed = Now()-start
	mu        sync.Mutex             // guards overrides
	overrides map[string]interface{} // Address -> value written via WritePoint
}

var _ driver.Driver = (*Sim)(nil)

// NewSim builds a simulator with the real clock and the current time as its
// waveform origin.
func NewSim() *Sim {
	s := &Sim{Now: time.Now, overrides: map[string]interface{}{}}
	s.start = s.Now()
	return s
}

// Connect is a no-op: the simulator has no device to reach.
func (s *Sim) Connect() error { return nil }

// Close is a no-op.
func (s *Sim) Close() error { return nil }

// ReadPoint returns the override for p.Address if one was written, otherwise the
// point's waveform value at the current elapsed time. It errors if the Address
// is not a valid "<wave>:<params>" spec.
func (s *Sim) ReadPoint(p driver.TagPoint) (interface{}, error) {
	s.mu.Lock()
	v, ok := s.overrides[p.Address]
	s.mu.Unlock()
	if ok {
		return v, nil
	}
	spec, err := parseWave(p.Address)
	if err != nil {
		return nil, err
	}
	return spec.value(s.Now().Sub(s.start)), nil
}

// WritePoint records v as an override for p.Address; later reads of that Address
// return v until overwritten.
func (s *Sim) WritePoint(p driver.TagPoint, v interface{}) error {
	s.mu.Lock()
	s.overrides[p.Address] = v
	s.mu.Unlock()
	return nil
}

// waveSpec is a parsed Address: the waveform kind plus its parameters.
type waveSpec struct {
	kind   string
	min    float64
	max    float64
	period time.Duration
	konst  float64 // value for the "const" waveform
}

// parseWave parses an Address of the form "<wave>:<params>". For sine/ramp/
// toggle/random the params are comma-separated key=value pairs (min, max,
// period); for const the params are a single bare number.
func parseWave(addr string) (waveSpec, error) {
	kind, rest, hasParams := strings.Cut(addr, ":")
	kind = strings.TrimSpace(kind)
	spec := waveSpec{kind: kind, min: 0, max: 100, period: time.Second}

	switch kind {
	case "sine", "ramp", "toggle", "random":
		if err := parseParams(rest, &spec, addr); err != nil {
			return spec, err
		}
		if kind != "random" && spec.period <= 0 {
			return spec, fmt.Errorf("sim: %q: period must be > 0", addr)
		}
	case "const":
		if !hasParams {
			return spec, fmt.Errorf("sim: %q: const needs a value", addr)
		}
		f, err := strconv.ParseFloat(strings.TrimSpace(rest), 64)
		if err != nil {
			return spec, fmt.Errorf("sim: %q: bad const value: %w", addr, err)
		}
		spec.konst = f
	default:
		return spec, fmt.Errorf("sim: %q: unknown wave %q", addr, kind)
	}
	return spec, nil
}

// parseParams fills spec from a "min=..,max=..,period=.." parameter string.
func parseParams(rest string, spec *waveSpec, addr string) error {
	for _, kv := range strings.Split(rest, ",") {
		kv = strings.TrimSpace(kv)
		if kv == "" {
			continue
		}
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return fmt.Errorf("sim: %q: bad param %q (want key=value)", addr, kv)
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		switch k {
		case "min":
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return fmt.Errorf("sim: %q: bad min %q: %w", addr, v, err)
			}
			spec.min = f
		case "max":
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return fmt.Errorf("sim: %q: bad max %q: %w", addr, v, err)
			}
			spec.max = f
		case "period":
			d, err := time.ParseDuration(v)
			if err != nil {
				return fmt.Errorf("sim: %q: bad period %q: %w", addr, v, err)
			}
			spec.period = d
		default:
			return fmt.Errorf("sim: %q: unknown param %q", addr, k)
		}
	}
	return nil
}

// value evaluates the waveform at elapsed. sine/ramp/const/random return
// float64; toggle returns bool.
func (s waveSpec) value(elapsed time.Duration) interface{} {
	span := s.max - s.min
	switch s.kind {
	case "sine":
		phase := 2 * math.Pi * float64(elapsed) / float64(s.period)
		return s.min + span*(0.5+0.5*math.Sin(phase))
	case "ramp":
		x := float64(elapsed) / float64(s.period)
		return s.min + span*(x-math.Floor(x))
	case "toggle":
		return int64(elapsed/s.period)%2 == 1
	case "const":
		return s.konst
	case "random":
		return s.min + span*hashUnit(uint64(elapsed.Nanoseconds()))
	}
	return 0.0 // unreachable: parseWave rejects unknown kinds
}

// hashUnit maps n to a deterministic float64 in [0,1) via splitmix64, so the
// "random" waveform is reproducible for a given instant and needs no global
// math/rand state (keeping tests stable).
func hashUnit(n uint64) float64 {
	n += 0x9E3779B97F4A7C15
	n = (n ^ (n >> 30)) * 0xBF58476D1CE4E5B9
	n = (n ^ (n >> 27)) * 0x94D049BB133111EB
	n ^= n >> 31
	return float64(n>>11) / float64(1<<53)
}
