package gateway

import (
	"sync"
	"testing"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/driver"
)

// fakeSource is a canned-value source Driver: ReadPoint returns whatever value
// is staged for a point's address. It only feeds the poller.
type fakeSource struct {
	mu   sync.Mutex
	vals map[string]interface{}
}

func newSource() *fakeSource { return &fakeSource{vals: map[string]interface{}{}} }

func (s *fakeSource) Connect() error { return nil }
func (s *fakeSource) Close() error   { return nil }
func (s *fakeSource) ReadPoint(p driver.TagPoint) (interface{}, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.vals[p.Address], nil
}
func (s *fakeSource) WritePoint(driver.TagPoint, interface{}) error { return nil }

// fakeSink records every WritePoint keyed by the point's address. All fields are
// mutex-guarded so the poll goroutine and the test observer never race.
type fakeSink struct {
	mu        sync.Mutex
	writes    map[string]interface{}
	nWrite    int
	connected bool
	closed    bool
}

func newSink() *fakeSink { return &fakeSink{writes: map[string]interface{}{}} }

func (s *fakeSink) Connect() error {
	s.mu.Lock()
	s.connected = true
	s.mu.Unlock()
	return nil
}
func (s *fakeSink) Close() error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	return nil
}
func (s *fakeSink) ReadPoint(driver.TagPoint) (interface{}, error) { return nil, nil }
func (s *fakeSink) WritePoint(p driver.TagPoint, v interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writes[p.Address] = v
	s.nWrite++
	return nil
}

func (s *fakeSink) get(addr string) (interface{}, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.writes[addr]
	return v, ok
}
func (s *fakeSink) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.nWrite
}
func (s *fakeSink) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// asF coerces a forwarded raw value to float64. Numeric tag values arrive as
// float64 (driver normalizes them), but int16 is handled too so the assertion
// does not silently depend on the exact concrete type.
func asF(v interface{}) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int16:
		return float64(x)
	}
	return 0
}

func roPoint(tag, addr string) driver.TagPoint {
	return driver.TagPoint{Tag: tag, Address: addr, Type: driver.TypeInt16, Order: driver.BigEndian, Access: driver.ReadOnly}
}
func rwPoint(tag, addr string) driver.TagPoint {
	return driver.TagPoint{Tag: tag, Address: addr, Type: driver.TypeInt16, Order: driver.BigEndian, Access: driver.ReadWrite}
}

// TestGatewayForwardsMatchedTags drives the source once (ForwardOnce / PollOnce)
// and asserts each matched tag lands at its SINK address (name is the join,
// address is remapped), while unmatched tags — a source-only tag and a
// sink-only tag — are never written to the sink.
func TestGatewayForwardsMatchedTags(t *testing.T) {
	src := newSource()
	src.vals["S1"] = int16(11) // -> tag "level"
	src.vals["S2"] = int16(22) // -> tag "temp"
	src.vals["S9"] = int16(99) // -> tag "orphan" (no sink point: source-only)
	sink := newSink()
	db := core.NewTagDB()

	srcPts := []driver.TagPoint{
		roPoint("level", "S1"),
		roPoint("temp", "S2"),
		roPoint("orphan", "S9"),
	}
	sinkPts := []driver.TagPoint{
		rwPoint("level", "D1"), // sink address differs from source S1
		rwPoint("temp", "D2"),
		rwPoint("ghost", "D9"), // sink-only: source never produces "ghost"
	}

	g := NewGateway(src, sink, srcPts, sinkPts, db, time.Hour)
	defer g.Stop()
	g.ForwardOnce() // deterministic: source read -> tags -> forward to sink

	if v, ok := sink.get("D1"); !ok || asF(v) != 11 {
		t.Errorf("sink[D1] (tag level) = %v ok=%v, want 11", v, ok)
	}
	if v, ok := sink.get("D2"); !ok || asF(v) != 22 {
		t.Errorf("sink[D2] (tag temp) = %v ok=%v, want 22", v, ok)
	}
	// Unmatched source tag: read into the TagDB but never forwarded (no sink point).
	if _, ok := sink.get("S9"); ok {
		t.Errorf("source-only tag orphan was forwarded, want not forwarded")
	}
	// Unmatched sink tag: source never produces it, so the priming nil is skipped
	// and nothing is written.
	if _, ok := sink.get("D9"); ok {
		t.Errorf("sink-only tag ghost was written, want not forwarded")
	}
	if n := sink.count(); n != 2 {
		t.Errorf("sink writes = %d, want 2 (one per matched tag)", n)
	}
	// The forwarded value must reach the SINK address, not the source address.
	if _, ok := sink.get("S1"); ok {
		t.Errorf("value forwarded to source address S1; tag name must remap to sink address")
	}
}

// TestGatewayStartForwardsAndCloses exercises the live path: Start launches the
// poll loop, a bounded wait observes the forwarded values, and Stop closes the
// sink.
func TestGatewayStartForwardsAndCloses(t *testing.T) {
	src := newSource()
	src.vals["S1"] = int16(7)
	sink := newSink()
	db := core.NewTagDB()

	g := NewGateway(src, sink,
		[]driver.TagPoint{roPoint("flow", "S1")},
		[]driver.TagPoint{rwPoint("flow", "D1")},
		db, time.Millisecond)
	if err := g.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Bounded wait for the async poll loop to forward the value (no fixed sleep).
	deadline := time.Now().Add(2 * time.Second)
	for sink.count() == 0 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	if v, ok := sink.get("D1"); !ok || asF(v) != 7 {
		t.Errorf("sink[D1] (tag flow) = %v ok=%v, want 7", v, ok)
	}

	g.Stop()
	if !sink.isClosed() {
		t.Errorf("sink not closed after Stop")
	}
}
