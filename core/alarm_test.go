package core

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestEvaluateAlarmBands(t *testing.T) {
	// All limits non-zero (and a real range) so every band is armed.
	m := Meta{Unit: "℃", Min: -50, Max: 200, LoLo: 10, Lo: 20, Hi: 80, HiHi: 90}
	cases := []struct {
		v    float64
		want AlarmSeverity
	}{
		{-100, LowLow},   // far below
		{5, LowLow},      // below LoLo
		{10, LowLow},     // == LoLo (inclusive)
		{15, Low},        // between LoLo and Lo
		{20, Low},        // == Lo (inclusive)
		{21, None},       // just inside
		{50, None},       // mid band
		{79, None},       // just inside high
		{80, High},       // == Hi (inclusive)
		{85, High},       // between Hi and HiHi
		{90, HighHigh},   // == HiHi (inclusive)
		{1000, HighHigh}, // far above
	}
	for _, c := range cases {
		if got := EvaluateAlarm(c.v, m); got != c.want {
			t.Errorf("EvaluateAlarm(%g) = %v, want %v", c.v, got, c.want)
		}
	}
}

func TestEvaluateAlarmUnset(t *testing.T) {
	// Zero Meta arms nothing: no value produces an alarm.
	for _, v := range []float64{-1e9, -1, 0, 1, 1e9} {
		if got := EvaluateAlarm(v, Meta{}); got != None {
			t.Errorf("EvaluateAlarm(%g, Meta{}) = %v, want None", v, got)
		}
	}

	// High-only config with the range left at zero: low side stays disabled,
	// high side arms.
	m := Meta{Hi: 80, HiHi: 90}
	if got := EvaluateAlarm(-1e9, m); got != None {
		t.Errorf("low side should be unset: got %v, want None", got)
	}
	if got := EvaluateAlarm(50, m); got != None {
		t.Errorf("mid: got %v, want None", got)
	}
	if got := EvaluateAlarm(85, m); got != High {
		t.Errorf("high: got %v, want High", got)
	}
	if got := EvaluateAlarm(95, m); got != HighHigh {
		t.Errorf("hihi: got %v, want HighHigh", got)
	}

	// Lo=0 alone (no range) stays unset.
	if got := EvaluateAlarm(-1, Meta{Lo: 0}); got != None {
		t.Errorf("Lo=0 without a range should be unset: got %v, want None", got)
	}
	// Once a range is configured, a 0 limit is honored as a real threshold.
	// Other limits take non-colliding values to isolate Lo=0.
	armed := Meta{Min: -50, Max: 50, LoLo: -40, Lo: 0, Hi: 40, HiHi: 45}
	if got := EvaluateAlarm(-1, armed); got != Low {
		t.Errorf("armed Lo=0: EvaluateAlarm(-1) = %v, want Low", got)
	}
	if got := EvaluateAlarm(1, armed); got != None {
		t.Errorf("armed Lo=0: EvaluateAlarm(1) = %v, want None", got)
	}
}

func TestAlarmDBRaiseClearAck(t *testing.T) {
	db := NewAlarmDB()

	// Raise.
	db.Update("TT-101", Low, 15)
	act := db.Active()
	if len(act) != 1 {
		t.Fatalf("after raise: %d active, want 1", len(act))
	}
	if a := act[0]; a.Tag != "TT-101" || a.Severity != Low || !a.Active || a.Acked {
		t.Fatalf("after raise: %+v", a)
	}

	// Same severity refresh -> no new transition.
	db.Update("TT-101", Low, 14)
	if h := db.History(); len(h) != 1 {
		t.Fatalf("same-severity refresh recorded %d history entries, want 1", len(h))
	}

	// Ack.
	db.Ack("TT-101")
	act = db.Active()
	if len(act) != 1 || !act[0].Acked {
		t.Fatalf("after ack: %+v", act)
	}
	// Re-ack is a no-op.
	db.Ack("TT-101")

	// Escalate severity -> re-armed (unacked again).
	db.Update("TT-101", LowLow, 5)
	act = db.Active()
	if len(act) != 1 || act[0].Severity != LowLow || act[0].Acked {
		t.Fatalf("after escalate: %+v", act)
	}

	// Clear.
	db.Update("TT-101", None, 50)
	if act = db.Active(); len(act) != 0 {
		t.Fatalf("after clear: %d active, want 0", len(act))
	}

	// History: raise(Low), ack, re-raise(LowLow), clear(None). Re-ack and
	// same-severity refresh are not transitions.
	h := db.History()
	if len(h) != 4 {
		t.Fatalf("history len = %d, want 4: %+v", len(h), h)
	}
	if h[0].Severity != Low || !h[0].Active || h[0].Acked {
		t.Errorf("h[0] raise: %+v", h[0])
	}
	if !h[1].Acked || !h[1].Active {
		t.Errorf("h[1] ack: %+v", h[1])
	}
	if h[2].Severity != LowLow || !h[2].Active || h[2].Acked {
		t.Errorf("h[2] re-raise: %+v", h[2])
	}
	if h[3].Severity != None || h[3].Active {
		t.Errorf("h[3] clear: %+v", h[3])
	}
}

func TestAlarmActiveOrdering(t *testing.T) {
	db := NewAlarmDB()

	// unacked before acked; then higher severity first.
	db.Update("warn", Low, 15)      // unacked Low
	db.Update("crit", HighHigh, 95) // unacked HighHigh
	db.Update("mid", High, 85)      // unacked High
	db.Ack("crit")                  // crit becomes acked -> sinks below unacked

	got := db.Active()
	want := []string{"mid", "warn", "crit"} // High, Low (unacked) then acked HighHigh
	if len(got) != len(want) {
		t.Fatalf("active len = %d, want %d: %+v", len(got), len(want), got)
	}
	for i, name := range want {
		if got[i].Tag != name {
			t.Errorf("Active()[%d].Tag = %q, want %q (full: %+v)", i, got[i].Tag, name, got)
		}
	}

	// Same severity+ack: older first, driven by Since (injected clock).
	db2 := NewAlarmDB()
	t1 := time.Unix(1_700_000_000, 0)
	db2.now = func() time.Time { return t1 }
	db2.Update("old", High, 85)
	db2.now = func() time.Time { return t1.Add(time.Minute) }
	db2.Update("new", High, 85)
	ord := db2.Active()
	if len(ord) != 2 || ord[0].Tag != "old" || ord[1].Tag != "new" {
		t.Fatalf("Since ordering: %+v", ord)
	}
}

func TestAlarmDBSubscribeNotifies(t *testing.T) {
	db := NewAlarmDB()
	var events []AlarmState
	cancel := db.Subscribe(func(s AlarmState) { events = append(events, s) })

	db.Update("X", High, 85)     // raise      -> event
	db.Update("X", High, 86)     // same sev   -> no event
	db.Update("X", HighHigh, 95) // escalate   -> event
	db.Ack("X")                  // ack        -> event
	db.Update("X", None, 50)     // clear      -> event

	if len(events) != 4 {
		t.Fatalf("got %d events, want 4: %+v", len(events), events)
	}
	if events[0].Severity != High || !events[0].Active {
		t.Errorf("event[0] raise: %+v", events[0])
	}
	if events[1].Severity != HighHigh || events[1].Acked {
		t.Errorf("event[1] escalate: %+v", events[1])
	}
	if !events[2].Acked {
		t.Errorf("event[2] ack: %+v", events[2])
	}
	if events[3].Active || events[3].Severity != None {
		t.Errorf("event[3] clear: %+v", events[3])
	}

	// After cancel, no further events.
	cancel()
	db.Update("X", High, 88)
	if len(events) != 4 {
		t.Errorf("event fired after cancel: %d", len(events))
	}
}

func TestAlarmHistoryRingBounded(t *testing.T) {
	db := NewAlarmDBWithHistory(3)
	// 5 raise+clear cycles = 10 transitions; ring keeps only the last 3.
	for i := 0; i < 5; i++ {
		name := fmt.Sprintf("T%d", i)
		db.Update(name, High, 85)
		db.Update(name, None, 50)
	}
	h := db.History()
	if len(h) != 3 {
		t.Fatalf("history len = %d, want 3 (bounded)", len(h))
	}
	// Newest entry is the clear of the last tag.
	last := h[len(h)-1]
	if last.Tag != "T4" || last.Active || last.Severity != None {
		t.Errorf("newest history entry = %+v, want cleared T4", last)
	}
}

func TestAlarmDBWatch(t *testing.T) {
	tags := NewTagDB()
	tag := tags.GetOrCreate("TT-201", Meta{Unit: "℃", Min: -50, Max: 200, LoLo: 10, Lo: 20, Hi: 80, HiHi: 90})

	db := NewAlarmDB()
	cancel := db.Watch(tag)
	defer cancel()

	// Priming sample is bad-quality (zero Value) -> ignored, no alarm.
	if act := db.Active(); len(act) != 0 {
		t.Fatalf("watch primed a spurious alarm: %+v", act)
	}

	// Cross the HiHi limit (SetValue stamps QualityGood).
	tag.SetValue(95.0)
	act := db.Active()
	if len(act) != 1 || act[0].Severity != HighHigh || act[0].Tag != "TT-201" {
		t.Fatalf("after HiHi crossing: %+v", act)
	}

	// De-escalate into the Hi warning band.
	tag.SetValue(85.0)
	if act = db.Active(); len(act) != 1 || act[0].Severity != High {
		t.Fatalf("after Hi crossing: %+v", act)
	}

	// Return to normal -> cleared.
	tag.SetValue(50.0)
	if act = db.Active(); len(act) != 0 {
		t.Fatalf("after return-to-normal: %+v", act)
	}

	// Bad-quality sample must not clear or alter an active alarm.
	tag.SetValue(5.0) // good -> LowLow
	tag.Publish(Value{Raw: 0.0, Quality: QualityBad, Time: time.Now()})
	if act = db.Active(); len(act) != 1 || act[0].Severity != LowLow {
		t.Fatalf("bad-quality sample disturbed alarm state: %+v", act)
	}
}

func TestAlarmDBConcurrentUpdate(t *testing.T) {
	db := NewAlarmDB()
	m := Meta{Min: -50, Max: 200, LoLo: 10, Lo: 20, Hi: 80, HiHi: 90}
	names := []string{"A", "B", "C", "D"}

	var wg sync.WaitGroup

	// Writers hammering Update across shared tags.
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			for i := 0; i < 2000; i++ {
				name := names[(i+seed)%len(names)]
				v := float64((i * 7) % 100)
				db.Update(name, EvaluateAlarm(v, m), v)
			}
		}(g)
	}

	// Readers hammering Active/History.
	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 2000; i++ {
				_ = db.Active()
				_ = db.History()
			}
		}()
	}

	// Ack + subscribe/unsubscribe churn.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 2000; i++ {
			cancel := db.Subscribe(func(AlarmState) {})
			db.Ack(names[i%len(names)])
			cancel()
		}
	}()

	wg.Wait()

	// Sanity: state is still coherent (no panic, active count bounded by tags).
	if n := len(db.Active()); n > len(names) {
		t.Fatalf("active count %d exceeds tag count %d", n, len(names))
	}
}
