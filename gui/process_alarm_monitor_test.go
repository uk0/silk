package gui

import (
	"context"
	"testing"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/hmi"
	"github.com/uk0/silk/paint"
	"github.com/uk0/silk/point"
)

func TestProcessAlarmMonitorAcknowledgesOnlyDisplayedOccurrence(t *testing.T) {
	project := hmi.Project{Version: hmi.Version, Width: 500, Height: 360,
		Points: []hmi.Point{{Definition: point.Definition{Name: "DI1", Kind: point.Bool}, Initial: false}},
		Alarms: []hmi.AlarmRule{{ID: "door", Point: "DI1", Condition: hmi.AlarmMatch, Priority: hmi.AlarmWarning, Threshold: true, Latch: true}},
	}
	ctx := context.Background()
	makeRuntime := func() *hmi.Runtime {
		t.Helper()
		rt, err := hmi.NewRuntime(ctx, project, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(rt.Stop)
		return rt
	}
	publish := func(rt *hmi.Runtime, on bool) {
		t.Helper()
		if err := rt.Publish(ctx, "DI1", core.Value{Raw: on, Quality: core.QualityGood, Time: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	var queued []func()
	v := newProcessAlarmMonitor(func(fn func()) { queued = append(queued, fn) })
	defer v.Close()
	drain := func() {
		for len(queued) > 0 {
			f := queued[0]
			queued = queued[1:]
			f()
		}
	}
	r := makeRuntime()
	v.SetRuntime(r)
	publish(r, true)
	drain()
	v.table.SetSelectedRow(0)
	if !v.ack.IsEnabled() {
		t.Fatal("active selection not actionable")
	}
	publish(r, false)
	publish(r, true) // new occurrence; the view still shows the previous token
	if len(queued) != 1 {
		t.Fatal("alarm burst was not coalesced")
	}
	v.ack.Action().Trigger(nil)
	if r.AlarmStates()[0].Acknowledged || v.ack.IsEnabled() {
		t.Fatal("stale row acknowledged a different occurrence")
	}
	drain()
	v.table.SetSelectedRow(0)
	publish(r, false)
	drain()
	if !v.ack.IsEnabled() || v.states["door"].ConditionActive {
		t.Fatal("returned latch not shown for confirmation")
	}
	v.ack.Action().Trigger(nil)
	if r.AlarmStates()[0].Active || v.ack.IsEnabled() {
		t.Fatal("confirmed latch did not clear")
	}
	v.toggle.Action().Trigger(nil)
	if !v.history || v.model.RowCount() != len(r.AlarmEvents(0)) || v.ack.IsEnabled() {
		t.Fatal("history table or ACK gate incorrect")
	}
	v.table.SetSelectedRow(0)
	if v.SelectedAlarm() != "door" || v.SelectAlarm("door") || v.ack.IsEnabled() {
		t.Fatal("history selection should navigate but not acknowledge")
	}
	publish(r, true) // already queued for the old owner
	replacement := makeRuntime()
	v.SetRuntime(replacement)
	drain()
	if v.history || v.model.RowCount() != 1 || v.states["door"].Active || v.ack.IsEnabled() || v.SelectedAlarm() != "" {
		t.Fatal("queued old source overwrote replacement")
	}
	v.Close()
	publish(replacement, true)
	if len(queued) != 0 || v.model.RowCount() != 0 || v.SelectedAlarm() != "" {
		t.Fatal("closed alarm panel retained its subscription")
	}
}

func TestProcessAlarmMonitorSortRevokesPreviousSelection(t *testing.T) {
	p := hmi.Project{Version: hmi.Version, Width: 500, Height: 360,
		Points: []hmi.Point{{Definition: point.Definition{Name: "DI1", Kind: point.Bool}, Initial: true}},
		Alarms: []hmi.AlarmRule{
			{ID: "z", Point: "DI1", Condition: hmi.AlarmMatch, Priority: hmi.AlarmWarning, Threshold: true},
			{ID: "a", Point: "DI1", Condition: hmi.AlarmMatch, Priority: hmi.AlarmWarning, Threshold: true},
		},
	}
	rt, err := hmi.NewRuntime(context.Background(), p, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Stop()
	v := newProcessAlarmMonitor(func(func()) {})
	defer v.Close()
	v.SetRuntime(rt)
	v.table.SetSelectedRow(0)
	if v.selectedID() != "z" || !v.ack.IsEnabled() {
		t.Fatal("initial selection missing")
	}
	v.table.sortByColumn(0)
	if v.selectedID() != "" || v.ack.IsEnabled() || v.model.RowData(0)[0] != "a" {
		t.Fatal("sort retained an ACK action for a different row")
	}
	v.ack.Action().Trigger(nil)
	for _, s := range rt.AlarmStates() {
		if s.Acknowledged {
			t.Fatal("sort acknowledged", s.ID)
		}
	}
}

func TestProcessAlarmHistoryKeepsSelectedEventUntilEvicted(t *testing.T) {
	ctx := context.Background()
	p := hmi.Project{Version: hmi.Version, Width: 500, Height: 360,
		Points: []hmi.Point{{Definition: point.Definition{Name: "DI1", Kind: point.Bool}, Initial: true}},
		Alarms: []hmi.AlarmRule{{ID: "door", Point: "DI1", Condition: hmi.AlarmMatch, Priority: hmi.AlarmWarning, Threshold: true}},
	}
	rt, err := hmi.NewRuntime(ctx, p, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Stop()
	var queued func()
	v := newProcessAlarmMonitor(func(fn func()) { queued = fn })
	defer v.Close()
	v.SetRuntime(rt)
	v.toggle.Action().Trigger(nil)
	v.table.SetSelectedRow(0)
	if v.SelectedAlarm() != "door" || v.model.RowCount() != 1 || v.ack.IsEnabled() {
		t.Fatal("initial historical event not selected or unexpectedly actionable")
	}
	publish := func(on bool) {
		t.Helper()
		if err := rt.Publish(ctx, "DI1", core.Value{Raw: on, Quality: core.QualityGood, Time: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	drain := func() {
		t.Helper()
		if queued == nil {
			t.Fatal("alarm updates did not queue a UI refresh")
		}
		fn := queued
		queued = nil
		fn()
	}
	publish(false)
	publish(true) // another raised event with the same rule ID
	drain()
	if got, want := v.table.SelectedRow(), v.model.RowCount()-1; got != want || v.SelectedAlarm() != "door" || v.ack.IsEnabled() {
		t.Fatalf("refresh lost the original historical event or enabled ACK: row=%d want=%d target=%q", got, want, v.SelectedAlarm())
	}
	key := historyAlarmKey(v.model, v.table.SelectedRow())
	if key == "" || v.model.ColumnCount() != 7 {
		t.Fatal("history event key is missing or was exposed as a table column")
	}
	v.table.sortByColumn(2)
	for i := 0; i < v.model.RowCount(); i++ {
		if historyAlarmKey(v.model, i) == key {
			v.table.SetSelectedRow(i)
			break
		}
	}
	publish(false)
	publish(true)
	drain()
	if historyAlarmKey(v.model, v.table.SelectedRow()) != key || v.SelectedAlarm() != "door" {
		t.Fatal("selection did not follow the exact event in the sorted history")
	}
	v.toggle.Action().Trigger(nil)
	if v.history || v.SelectedAlarm() != "" || v.ack.IsEnabled() {
		t.Fatal("history selection became an actionable live selection")
	}
	v.toggle.Action().Trigger(nil)
	v.table.SetSelectedRow(v.model.RowCount() - 1)
	for i := range hmi.AlarmHistoryLimit + 2 {
		publish(i%2 == 1)
	}
	drain()
	if v.model.RowCount() != hmi.AlarmHistoryLimit || v.table.SelectedRow() != -1 || v.SelectedAlarm() != "" || v.ack.IsEnabled() {
		t.Fatal("evicted event was carried over to a different event for the same rule")
	}
}

func TestTableClippedCellInkStaysOutOfNeighbor(t *testing.T) {
	table := NewTable()
	table.SetSize(250, 100)
	draw := func(text string, clip bool) []uint32 {
		t.Helper()
		model := NewSimpleTableModel([]string{"A", "B"})
		model.SetColumnWidth(0, 40)
		model.SetColumnWidth(1, 200)
		model.AddRow(text, "")
		table.SetModel(model)
		table.SetClipCellText(clip)
		pm := paint.NewPixmap(250, 100)
		defer paint.CloseSurface(pm)
		painter := pm.NewPainter()
		table.Draw(painter)
		paint.ClosePainter(painter)
		img, err := pm.Image()
		if err != nil {
			t.Fatal(err)
		}
		var px []uint32
		for y := 0; y < 100; y++ {
			for x := 40; x < 232; x++ {
				r, g, b, a := img.At(x, y).RGBA()
				px = append(px, r>>8<<24|g>>8<<16|b>>8<<8|a>>8)
			}
		}
		return px
	}
	base := draw("", true)
	const caption = "Returned / awaiting acknowledgment"
	overflow := draw(caption, false)
	bounded := draw(caption, true)
	foundControl := false
	for i, p := range base {
		foundControl = foundControl || overflow[i] != p
		if bounded[i] != p {
			t.Fatal("cell text drew in its neighbor", i)
		}
	}
	if !foundControl {
		t.Fatal("baseline overflow not reproduced by the raster test")
	}
}
