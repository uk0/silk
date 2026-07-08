package gui

import (
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
)

// TestAlarmPanelSetAlarmsStoresAndCopies verifies SetAlarms keeps the rows in
// order and defensively copies on both the input and output boundaries: mutating
// the caller's slice after SetAlarms, or the slice returned by Alarms(), must not
// disturb the panel's stored state.
func TestAlarmPanelSetAlarmsStoresAndCopies(t *testing.T) {
	in := []core.AlarmState{
		{Tag: "TIC-101", Severity: core.HighHigh, Active: true},
		{Tag: "LIC-200", Severity: core.Low, Active: true, Acked: true},
	}
	p := NewAlarmPanel()
	p.SetAlarms(in)

	// Mutating the caller's slice must not reach the panel (input copied).
	in[0].Tag = "MUTATED"
	got := p.Alarms()
	if len(got) != 2 {
		t.Fatalf("Alarms() len = %d, want 2", len(got))
	}
	if got[0].Tag != "TIC-101" || got[1].Tag != "LIC-200" {
		t.Fatalf("order/copy wrong after input mutation: %+v", got)
	}
	if got[0].Severity != core.HighHigh || !got[1].Acked {
		t.Fatalf("fields not stored verbatim: %+v", got)
	}

	// Mutating the returned slice must not reach the panel (output copied).
	got[1].Tag = "MUTATED2"
	again := p.Alarms()
	if again[1].Tag != "LIC-200" {
		t.Fatalf("Alarms() did not return a copy: %+v", again)
	}
}

// TestAlarmSeverityColorMapping pins the severity->colour table: LoLo/HiHi red,
// Lo/Hi amber, None no accent.
func TestAlarmSeverityColorMapping(t *testing.T) {
	red := paint.Color{R: 230, G: 80, B: 80, A: 255}
	amber := paint.Color{R: 230, G: 180, B: 60, A: 255}

	cases := []struct {
		sev     core.AlarmSeverity
		want    paint.Color
		wantOn  bool
		nameFor string
	}{
		{core.None, paint.Color{}, false, "None"},
		{core.Low, amber, true, "Lo"},
		{core.High, amber, true, "Hi"},
		{core.LowLow, red, true, "LoLo"},
		{core.HighHigh, red, true, "HiHi"},
	}
	for _, c := range cases {
		got, on := alarmSeverityColor(c.sev)
		if on != c.wantOn {
			t.Errorf("%s: on = %v, want %v", c.nameFor, on, c.wantOn)
		}
		if got != c.want {
			t.Errorf("%s: colour = %v, want %v", c.nameFor, got, c.want)
		}
	}
}

// TestAlarmPanelRowAtY checks the pure header/scroll-aware hit-test: the header
// band maps to -1, the first pixel below it to row 0, and a scroll offset shifts
// the mapping by whole rows.
func TestAlarmPanelRowAtY(t *testing.T) {
	p := NewAlarmPanel()
	rh := p.rowHeight

	if got := p.rowAtY(alarmHeaderH - 1); got != -1 {
		t.Errorf("rowAtY(header) = %d, want -1", got)
	}
	if got := p.rowAtY(alarmHeaderH + 1); got != 0 {
		t.Errorf("rowAtY(first row) = %d, want 0", got)
	}
	if got := p.rowAtY(alarmHeaderH + rh + 1); got != 1 {
		t.Errorf("rowAtY(second row) = %d, want 1", got)
	}

	p.scrollY = 2 * rh
	if got := p.rowAtY(alarmHeaderH + 1); got != 2 {
		t.Errorf("rowAtY(first row, scrolled 2) = %d, want 2", got)
	}
}

// TestAlarmPanelAckClickFiresSig confirms a click in an unacked row's ACK column
// fires SigAckRequested with that row's tag, while a click elsewhere on the row
// does not.
func TestAlarmPanelAckClickFiresSig(t *testing.T) {
	p := NewAlarmPanel()
	p.SetAlarms([]core.AlarmState{
		{Tag: "TIC-101", Severity: core.HighHigh, Active: true, Acked: false},
	})
	p.SetBounds(0, 0, 300, 200) // give the panel a width so the ACK column exists

	var gotTag string
	fired := false
	p.SigAckRequested(func(tag string) { fired = true; gotTag = tag })

	// Click inside the right-anchored ACK column on row 0.
	p.OnLeftDown(300-8, alarmHeaderH+p.rowHeight*0.5)
	if !fired {
		t.Fatal("ACK-column click did not fire SigAckRequested")
	}
	if gotTag != "TIC-101" {
		t.Fatalf("SigAckRequested tag = %q, want TIC-101", gotTag)
	}

	// A click on the row body (outside the ACK column) must not fire.
	fired = false
	p.OnLeftDown(60, alarmHeaderH+p.rowHeight*0.5)
	if fired {
		t.Fatal("non-ACK row click fired SigAckRequested")
	}
}

// TestAlarmPanelBindAlarmDB wires the panel to a real AlarmDB, raises an alarm,
// and asserts the panel receives it only after the posted UI task is drained —
// mirroring how the tag-bridge test isolates gui.Post (uiWakeupFn nil'd so Post
// stays headless; drainUITasks runs the posted refresh on the "UI thread").
func TestAlarmPanelBindAlarmDB(t *testing.T) {
	resetUIQueue()
	t.Cleanup(resetUIQueue)

	db := core.NewAlarmDB()
	p := NewAlarmPanel()
	unsub := p.BindAlarmDB(db)
	defer unsub()

	if n := len(p.Alarms()); n != 0 {
		t.Fatalf("panel not empty at bind time: %d rows", n)
	}

	// Raise an alarm. The subscriber only Post-s the refresh; nothing has run on
	// the UI thread yet, so the panel must still be empty.
	db.Update("TIC-101", core.HighHigh, 95.0)
	if n := len(p.Alarms()); n != 0 {
		t.Fatalf("panel updated before UI drain: %d rows", n)
	}

	drainUITasks() // run the posted SetAlarms(db.Active())

	got := p.Alarms()
	if len(got) != 1 {
		t.Fatalf("after drain: %d rows, want 1", len(got))
	}
	if got[0].Tag != "TIC-101" || got[0].Severity != core.HighHigh || got[0].Acked {
		t.Fatalf("panel row = %+v, want unacked HiHi TIC-101", got[0])
	}

	// After unsubscribe, further transitions must not reach the panel.
	unsub()
	db.Update("TIC-101", core.None, 50.0) // clear
	drainUITasks()
	if n := len(p.Alarms()); n != 1 {
		t.Fatalf("panel changed after unsubscribe: %d rows, want frozen 1", n)
	}
}

// TestAlarmPanelFactoryRegistered checks the factory id resolves to a
// constructible *AlarmPanel so the designer can place it.
func TestAlarmPanelFactoryRegistered(t *testing.T) {
	obj := core.New("gui.AlarmPanel")
	if _, ok := obj.(*AlarmPanel); !ok {
		t.Fatalf("factory gui.AlarmPanel built %T, want *AlarmPanel", obj)
	}
}
