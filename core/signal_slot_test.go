package core

import "testing"

// The signal-slot layer in this package is a 1:1 wiring adaptor: a "signal" is a
// method SigXxx(fn) that stores a single concrete slot func and fires it directly.
// These holders replicate that widget pattern so the tests exercise Connect end to
// end (wire via Connect, then emit) without pulling in the gui package.

type sigHolder0 struct{ fn func(interface{}) }

func (h *sigHolder0) SigTest(fn func(interface{})) { h.fn = fn }
func (h *sigHolder0) emit(sender interface{}) {
	if h.fn != nil {
		h.fn(sender)
	}
}

type sigHolder1 struct{ fn func(interface{}, string) }

func (h *sigHolder1) SigTest(fn func(interface{}, string)) { h.fn = fn }
func (h *sigHolder1) emit(sender interface{}, s string) {
	if h.fn != nil {
		h.fn(sender, s)
	}
}

type sigHolder2 struct{ fn func(interface{}, int, int) }

func (h *sigHolder2) SigTest(fn func(interface{}, int, int)) { h.fn = fn }
func (h *sigHolder2) emit(sender interface{}, a, b int) {
	if h.fn != nil {
		h.fn(sender, a, b)
	}
}

type sigHolderRet struct{ fn func(interface{}) bool }

func (h *sigHolderRet) SigTest(fn func(interface{}) bool) { h.fn = fn }
func (h *sigHolderRet) emit(sender interface{}) bool      { return h.fn(sender) }

// --- correctness ----------------------------------------------------------

// Slot drops the sender arg -> adaptor. func(interface{}) + func() fast path.
func TestSignalNoArgSlotFires(t *testing.T) {
	h := &sigHolder0{}
	fired := 0
	Connect(h.SigTest, func() { fired++ })
	h.emit(h)
	h.emit(h)
	if fired != 2 {
		t.Fatalf("no-arg slot: want 2 fires, got %d", fired)
	}
}

// Slot drops sender and receives the payload. func(interface{},string) + func(string) fast path.
func TestSignalOneArgSlotReceivesArg(t *testing.T) {
	h := &sigHolder1{}
	var got string
	Connect(h.SigTest, func(s string) { got = s })
	h.emit(h, "hello")
	if got != "hello" {
		t.Fatalf("one-arg slot: want %q, got %q", "hello", got)
	}
}

// Arity matches the signal -> direct path (no adaptor); sender must reach the slot.
func TestSignalDirectMatchPassesSender(t *testing.T) {
	h := &sigHolder0{}
	var got interface{}
	Connect(h.SigTest, func(o interface{}) { got = o })
	h.emit(h)
	if got != interface{}(h) {
		t.Fatalf("direct match: sender not passed, got %v", got)
	}
}

// Multi-arg with no fast path -> reflect.MakeFunc fallback must still deliver args.
func TestSignalReflectFallbackMultiArg(t *testing.T) {
	h := &sigHolder2{}
	sum := 0
	Connect(h.SigTest, func(a, b int) { sum = a + b })
	h.emit(h, 3, 4)
	if sum != 7 {
		t.Fatalf("reflect fallback: want 7, got %d", sum)
	}
}

// Signal carries a return value -> stays on the reflect fallback; return must propagate.
func TestSignalReflectFallbackReturnValue(t *testing.T) {
	h := &sigHolderRet{}
	Connect(h.SigTest, func() bool { return true })
	if !h.emit(h) {
		t.Fatal("reflect fallback: return value not propagated")
	}
}

// Signals store a single slot; a second Connect replaces the first (last wins).
func TestSignalReconnectReplacesSlot(t *testing.T) {
	h := &sigHolder0{}
	a, b := 0, 0
	Connect(h.SigTest, func() { a++ })
	Connect(h.SigTest, func() { b++ })
	h.emit(h)
	if a != 0 || b != 1 {
		t.Fatalf("last-wins violated: a=%d b=%d", a, b)
	}
}

// Arity mismatch must not connect: the signal's slot stays nil.
func TestSignalMismatchDoesNotConnect(t *testing.T) {
	h := &sigHolder0{}
	Connect(h.SigTest, func(x, y, z int) {})
	if h.fn != nil {
		t.Fatal("mismatched slot should not connect")
	}
	h.emit(h) // must be a no-op, not a panic
}

// --- benchmarks -----------------------------------------------------------

// Adaptor fast path: signal func(interface{}), slot func().
func BenchmarkSignalEmitNoArg(b *testing.B) {
	h := &sigHolder0{}
	n := 0
	Connect(h.SigTest, func() { n++ })
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.emit(h)
	}
	_ = n
}

// Adaptor fast path: signal func(interface{}, string), slot func(string).
func BenchmarkSignalEmitOneArg(b *testing.B) {
	h := &sigHolder1{}
	var last string
	Connect(h.SigTest, func(s string) { last = s })
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.emit(h, "x")
	}
	_ = last
}

// Reflection fallback: signal func(interface{}, int, int), slot func(int, int).
func BenchmarkSignalEmitReflect(b *testing.B) {
	h := &sigHolder2{}
	sum := 0
	Connect(h.SigTest, func(a, c int) { sum += a + c })
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.emit(h, 1, 2)
	}
	_ = sum
}
