package scada

import (
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

// findStat returns the row for tag, or nil.
func findStat(rows []gui.StatRow, tag string) *gui.StatRow {
	for i := range rows {
		if rows[i].Tag == tag {
			return &rows[i]
		}
	}
	return nil
}

// TestBindScreenFeedsStatsPanel builds a tiny tree (a tag-bound Tank plus a
// StatsPanel), wires it, and asserts the panel received the tracked stat through
// its getter — no Draw, no drivers, no sockets. The synchronous seed is what a
// headless test can observe (the Post-based live refresh needs the GL event
// loop); the shared collector then folds a later sample to prove the live path.
func TestBindScreenFeedsStatsPanel(t *testing.T) {
	s := newTestServices(t)

	// Drive the stat tag before wiring so Track primes to it and the seed carries
	// it into the panel.
	s.Tags.GetOrCreate("flow", core.Meta{}).SetValue(42.0)

	root := gui.NewVBox()
	tank := gui.NewTank()
	tank.SetTagName("level")
	root.AddWidget(tank)
	sp := gui.NewStatsPanel()
	root.AddWidget(sp)

	stop, err := BindScreen(s, root, ScreenOptions{StatsTags: []string{"flow"}})
	if err != nil {
		t.Fatalf("BindScreen: %v", err)
	}

	// The Tank's TagName seam resolved and created its tag through the tree walk.
	if _, ok := s.Tags.Get("level"); !ok {
		t.Fatal("Tank tag 'level' was not bound during the tree walk")
	}

	// The StatsPanel received the seeded snapshot via its getter.
	got := findStat(sp.Stats(), "flow")
	if got == nil {
		t.Fatalf("stats panel has no flow row; got %+v", sp.Stats())
	}
	if got.Last != 42 {
		t.Fatalf("flow Last = %v, want 42", got.Last)
	}
	if got.Count < 1 {
		t.Fatalf("flow Count = %d, want >= 1", got.Count)
	}

	// A later sample folds into the shared collector (the backend live path).
	s.Tags.GetOrCreate("flow", core.Meta{}).SetValue(7.0)
	if st, ok := s.Stats.Get("flow"); !ok || st.Count != 2 || st.Last != 7 {
		t.Fatalf("collector after 2nd sample = %+v ok=%v, want Count=2 Last=7", st, ok)
	}

	// stop is idempotent.
	stop()
	stop()
}

// TestBindScreenWiresAlarmPanel binds an AlarmPanel live to the shared AlarmDB
// and asserts an ACK click is routed back to AlarmDB.Ack. The DB seed and the
// ack are synchronous, so both are observable without draining gui.Post.
func TestBindScreenWiresAlarmPanel(t *testing.T) {
	s := newTestServices(t)

	// Raise an alarm before wiring so BindAlarmDB seeds the panel with it.
	s.Alarms.Update("P-1", core.High, 5)

	root := gui.NewVBox()
	ap := gui.NewAlarmPanel()
	ap.SetSize(300, 200)
	root.AddWidget(ap)

	stop, err := BindScreen(s, root, ScreenOptions{})
	if err != nil {
		t.Fatalf("BindScreen: %v", err)
	}
	defer stop()

	rows := ap.Alarms()
	if len(rows) != 1 || rows[0].Tag != "P-1" {
		t.Fatalf("alarm panel seed = %+v, want one P-1 row", rows)
	}
	if rows[0].Acked {
		t.Fatal("P-1 should seed unacked")
	}

	// Click the row's right-anchored ACK cell (header 22px, row 0 at y~30; ACK
	// column starts at width-46). SigAckRequested must route to AlarmDB.Ack.
	ap.OnLeftDown(270, 30)

	active := s.Alarms.Active()
	if len(active) != 1 || active[0].Tag != "P-1" || !active[0].Acked {
		t.Fatalf("AlarmDB after ACK click = %+v, want P-1 acked", active)
	}
}
