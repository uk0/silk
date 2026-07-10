package alarmbridge

import (
	"testing"

	"github.com/uk0/silk/core"
)

// event / note record the arguments each injected sink was called with.
type event struct{ source, message string }
type note struct{ title, body string }

// recorders builds fake onEvent/notify closures that append into the returned
// slices. Callbacks fire synchronously on the goroutine that drives the tag /
// db, which in these tests is the test goroutine, so no locking is needed.
func recorders() (events *[]event, notes *[]note, onEvent func(string, string), notify func(string, string)) {
	events, notes = &[]event{}, &[]note{}
	onEvent = func(source, message string) { *events = append(*events, event{source, message}) }
	notify = func(title, body string) { *notes = append(*notes, note{title, body}) }
	return events, notes, onEvent, notify
}

func TestFormatAlarm(t *testing.T) {
	title, message := formatAlarm(core.AlarmState{Tag: "TIC-101", Severity: core.HighHigh, Value: 160})
	if want := "[HiHi] TIC-101"; title != want {
		t.Errorf("title = %q, want %q", title, want)
	}
	if want := "TIC-101 HiHi = 160"; message != want {
		t.Errorf("message = %q, want %q", message, want)
	}

	// A non-integral value must render cleanly (no trailing zeros).
	_, message = formatAlarm(core.AlarmState{Tag: "PT-7", Severity: core.Low, Value: 3.5})
	if want := "PT-7 Lo = 3.5"; message != want {
		t.Errorf("message = %q, want %q", message, want)
	}
}

// TestWatchRaiseFiresEventAndNotify drives the full, realistic path: a tag with
// alarm limits, watched by an AlarmDB, whose value crosses HiHi via SetValue.
func TestWatchRaiseFiresEventAndNotify(t *testing.T) {
	events, notes, onEvent, notify := recorders()

	db := core.NewAlarmDB()
	stop := Watch(db, onEvent, notify)
	defer stop()

	tag := core.NewTagDB().GetOrCreate("TIC-101", core.Meta{
		Unit: "C", Min: 0, Max: 200, LoLo: 5, Lo: 10, Hi: 100, HiHi: 150,
	})
	cancel := db.Watch(tag)
	defer cancel()

	tag.SetValue(160.0) // 160 >= HiHi(150) -> raise HighHigh

	if len(*events) != 1 {
		t.Fatalf("onEvent calls = %d, want 1: %+v", len(*events), *events)
	}
	if got := (*events)[0]; got.source != "TIC-101" || got.message != "TIC-101 HiHi = 160" {
		t.Errorf("onEvent arg = %+v, want {TIC-101 TIC-101 HiHi = 160}", got)
	}
	if len(*notes) != 1 {
		t.Fatalf("notify calls = %d, want 1: %+v", len(*notes), *notes)
	}
	if got := (*notes)[0]; got.title != "[HiHi] TIC-101" || got.body != "TIC-101 HiHi = 160" {
		t.Errorf("notify arg = %+v, want {[HiHi] TIC-101 TIC-101 HiHi = 160}", got)
	}
}

// TestWatchSkipsCleared documents the choice to stay silent on return-to-normal.
func TestWatchSkipsCleared(t *testing.T) {
	events, notes, onEvent, notify := recorders()

	db := core.NewAlarmDB()
	stop := Watch(db, onEvent, notify)
	defer stop()

	db.Update("X", core.HighHigh, 99) // raise  -> fires
	db.Update("X", core.None, 20)     // clear  -> must NOT fire

	if len(*events) != 1 || len(*notes) != 1 {
		t.Fatalf("after raise+clear: events=%d notes=%d, want 1/1", len(*events), len(*notes))
	}
}

// TestWatchSkipsAck documents the choice to not re-notify on an operator ack.
func TestWatchSkipsAck(t *testing.T) {
	events, notes, onEvent, notify := recorders()

	db := core.NewAlarmDB()
	stop := Watch(db, onEvent, notify)
	defer stop()

	db.Update("X", core.High, 105) // raise -> fires
	db.Ack("X")                    // ack   -> must NOT fire

	if len(*events) != 1 || len(*notes) != 1 {
		t.Fatalf("after raise+ack: events=%d notes=%d, want 1/1", len(*events), len(*notes))
	}
}

// TestWatchNilSinks proves both sinks are nil-guarded.
func TestWatchNilSinks(t *testing.T) {
	db := core.NewAlarmDB()
	stop := Watch(db, nil, nil)
	defer stop()
	db.Update("X", core.HighHigh, 99) // must not panic
}

// TestWatchUnsubscribeStops proves the returned func halts further callbacks.
func TestWatchUnsubscribeStops(t *testing.T) {
	var calls int
	inc := func(string, string) { calls++ }

	db := core.NewAlarmDB()
	stop := Watch(db, inc, inc)

	db.Update("X", core.HighHigh, 99) // fires: +2 (onEvent + notify)
	if calls != 2 {
		t.Fatalf("before unsubscribe: calls = %d, want 2", calls)
	}

	stop()
	stop()                            // idempotent
	db.Update("Y", core.HighHigh, 99) // must NOT fire
	if calls != 2 {
		t.Fatalf("after unsubscribe: calls = %d, want 2", calls)
	}
}
