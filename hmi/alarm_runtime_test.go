package hmi

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/point"
)

const alarmBoundary int64 = 9007199254740993

func alarmProject() Project {
	return Project{
		Version: Version, Title: "Typed alarm QA", Width: 600, Height: 360,
		Points: []Point{
			{Definition: point.Definition{Name: "BI11", Kind: point.Int}, Initial: alarmBoundary - 1},
			{Definition: point.Definition{Name: "DI11", Kind: point.Bool}, Initial: false},
		},
		Alarms: []AlarmRule{
			{ID: "limit", Point: "BI11", Condition: AlarmHigh, Priority: AlarmCritical, Threshold: alarmBoundary, Reset: alarmBoundary - 2, RaiseMS: 100, ClearMS: 200, Latch: true},
			{ID: "trip", Point: "DI11", Condition: AlarmMatch, Priority: AlarmWarning, Threshold: true},
		},
	}
}

func TestAlarmProjectRoundTripAndValidation(t *testing.T) {
	p := alarmProject()
	raw, err := p.JSON()
	if err != nil {
		t.Fatal(err)
	}
	q, err := Parse(raw)
	if err != nil || q.Alarms[0].Threshold != json.Number("9007199254740993") {
		t.Fatalf("exact threshold lost: %v %v", q.Alarms, err)
	}
	cloned, err := CloneProject(p)
	if err != nil {
		t.Fatal(err)
	}
	cloned.Alarms[0].Reset = int64(0)
	if p.Alarms[0].Reset != alarmBoundary-2 {
		t.Fatal("authoring clone shares alarm rules")
	}
	for _, tc := range []struct {
		name string
		edit func(*Project)
	}{
		{"missing point", func(p *Project) { p.Alarms[0].Point = "missing" }},
		{"duplicate ID", func(p *Project) { p.Alarms[1].ID = p.Alarms[0].ID }},
		{"overlapping hysteresis", func(p *Project) { p.Alarms[0].Reset = alarmBoundary }},
		{"missing reset", func(p *Project) { p.Alarms[0].Reset = nil }},
		{"wrong condition", func(p *Project) { p.Alarms[0].Condition = AlarmMatch }},
		{"bad priority", func(p *Project) { p.Alarms[0].Priority = "undefined" }},
		{"fractional int", func(p *Project) { p.Alarms[0].Threshold = json.Number("9007199254740993.1") }},
		{"past dwell budget", func(p *Project) { p.Alarms[0].RaiseMS = 86400001 }},
		{"boolean coercion", func(p *Project) { p.Alarms[1].Threshold = 1 }},
		{"match reset", func(p *Project) { p.Alarms[1].Reset = false }},
		{"alarm budget", func(p *Project) { p.Alarms = make([]AlarmRule, MaxAlarmRules+1) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := alarmProject()
			tc.edit(&p)
			if _, err := p.JSON(); err == nil {
				t.Fatal("accepted invalid rule")
			}
		})
	}
	p.Alarms = nil
	raw, err = p.JSON()
	if err != nil || strings.Contains(string(raw), `"alarms"`) {
		t.Fatal("legacy project unexpectedly changed", err)
	}
}

func TestAlarmDwellQualityLatchAndExactHysteresis(t *testing.T) {
	p := alarmProject()
	start := time.Unix(75, 0)
	now := start
	clock := func() time.Time { return now }
	r := &Runtime{Tags: core.NewTagDB()}
	tag := r.Tags.GetOrCreate("BI11", core.Meta{})
	r.Tags.GetOrCreate("DI11", core.Meta{}).Observe(core.Value{Raw: false, Quality: core.QualityGood, Time: now})
	tag.Observe(core.Value{Raw: alarmBoundary - 1, Quality: core.QualityGood, Time: now})
	var err error
	r.alarms, err = newAlarmMonitor(p, r.Tags, clock)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Stop()
	write := func(ms int, val int64, q core.Quality) {
		now = start.Add(time.Duration(ms) * time.Millisecond)
		tag.Observe(core.Value{Raw: val, Quality: q, Time: now})
	}
	tick := func(ms int) {
		now = start.Add(time.Duration(ms) * time.Millisecond)
		r.Tick(now)
	}
	state := func() AlarmState { return r.AlarmStates()[0] }
	if state().Active || len(r.AlarmEvents(0)) != 0 {
		t.Fatal("int rounded up into the high alarm")
	}
	write(10, alarmBoundary, core.QualityGood)
	write(90, alarmBoundary, core.QualityBad)
	write(99, alarmBoundary, core.QualityGood)
	tick(198)
	if state().Active {
		t.Fatal("bad quality failed to reset pending raise")
	}
	tick(199)
	active := state()
	if !active.Active || !active.ConditionActive || active.Since != now || active.Value != alarmBoundary {
		t.Fatal("exact dwell did not raise", active)
	}
	write(201, alarmBoundary-1, core.QualityGood)
	tick(499)
	if !state().ConditionActive {
		t.Fatal("hysteresis band cleared active condition")
	}
	write(500, alarmBoundary-2, core.QualityGood)
	tick(699)
	if !state().ConditionActive {
		t.Fatal("cleared before reset dwell")
	}
	tick(700)
	if s := state(); s.ConditionActive || !s.Active || s.Acknowledged || s.Token != active.Token {
		t.Fatal("latch lost its returned occurrence", s)
	}
	write(720, alarmBoundary-2, core.QualityUncertain)
	if err = r.AcknowledgeAlarm("limit", active.Token); err != nil {
		t.Fatal(err)
	}
	if !state().Active || !state().Acknowledged {
		t.Fatal("ack cleared on unknown quality")
	}
	write(800, alarmBoundary-2, core.QualityGood)
	tick(999)
	if !state().Active {
		t.Fatal("recovery skipped clear dwell")
	}
	tick(1000)
	if state().Active || !errors.Is(r.AcknowledgeAlarm("limit", active.Token), ErrAlarmConflict) {
		t.Fatal("ended activation still acknowledges")
	}
	want := []AlarmEventKind{AlarmRaised, AlarmReturned, AlarmAcknowledged, AlarmCleared}
	var got []AlarmEventKind
	for i, e := range r.AlarmEvents(0) {
		got = append(got, e.Kind)
		if e.Sequence != uint64(i+1) {
			t.Fatal("unordered event", e)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("lifecycle %v want %v", got, want)
	}
}

type alarmTestStore struct{ writes int }

func (s *alarmTestStore) Define(context.Context, point.Definition) error { return nil }
func (s *alarmTestStore) Publish(context.Context, string, core.Value) error {
	s.writes++
	return nil
}

func TestAlarmRuntimePublishesStatesNotPointWrites(t *testing.T) {
	p := alarmProject()
	store := new(alarmTestStore)
	ctx := context.Background()
	r, err := NewRuntime(ctx, p, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Stop()
	publish := func(on bool) {
		t.Helper()
		if err := r.Publish(ctx, "DI11", core.Value{Raw: on, Quality: core.QualityGood, Time: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	publish(true)
	s := r.AlarmStates()[1]
	if !s.Active || s.Acknowledged || s.Value != true {
		t.Fatal("owned point did not raise alarm", s)
	}
	tag, _ := r.Tags.Get("DI11")
	_, revision := tag.Snapshot()
	if err := r.AcknowledgeAlarm("trip", s.Token); err != nil {
		t.Fatal(err)
	}
	if err := r.AcknowledgeAlarm("trip", s.Token); err != nil {
		t.Fatal("same ack is not idempotent", err)
	}
	if _, after := tag.Snapshot(); after != revision || store.writes != 1 || len(r.AlarmEvents(0)) != 2 {
		t.Fatal("ack created a point change, persistence write, or duplicate event")
	}
	publish(false)
	publish(true)
	if err := r.AcknowledgeAlarm("trip", s.Token); !errors.Is(err, ErrAlarmConflict) {
		t.Fatal("stale ack admitted in new occurrence", err)
	}
	restarted, err := NewRuntime(ctx, p, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Stop()
	if err := restarted.Publish(ctx, "DI11", tag.Value()); err != nil {
		t.Fatal(err)
	}
	if err := restarted.AcknowledgeAlarm("trip", s.Token); !errors.Is(err, ErrAlarmConflict) {
		t.Fatal("foreign session token admitted", err)
	}
	before := r.AlarmEvents(0)
	current := r.AlarmStates()[1].Token
	r.Stop()
	tag.SetValue(false)
	if !reflect.DeepEqual(r.AlarmEvents(0), before) || !errors.Is(r.AcknowledgeAlarm("trip", current), ErrAlarmsClosed) {
		t.Fatal("runtime admitted changes after shutdown")
	}
}

func TestAlarmHistoryIsBoundedAndReadableDuringPublication(t *testing.T) {
	r, err := NewRuntime(context.Background(), alarmProject(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Stop()
	var wg sync.WaitGroup
	wg.Go(func() {
		for i := 0; i < AlarmHistoryLimit+30; i++ {
			if err := r.Publish(context.Background(), "DI11", core.Value{Raw: i%2 == 0, Quality: core.QualityGood, Time: time.Now()}); err != nil {
				t.Error(err)
				return
			}
		}
	})
	for i := 0; i < 300; i++ {
		_ = r.AlarmStates()
		_ = r.AlarmEvents(uint64(i))
	}
	wg.Wait()
	events := r.AlarmEvents(0)
	if len(events) != AlarmHistoryLimit {
		t.Fatalf("ring length %d", len(events))
	}
	if events[0].Sequence != 31 || events[len(events)-1].Sequence != AlarmHistoryLimit+30 {
		t.Fatalf("ring range %d to %d", events[0].Sequence, events[len(events)-1].Sequence)
	}
	if tail := r.AlarmEvents(AlarmHistoryLimit + 29); len(tail) != 1 || tail[0].Sequence != AlarmHistoryLimit+30 {
		t.Fatal("cursor skipped/duplicated event", tail)
	}
	events[0].State.ID = "mutated result"
	if r.AlarmEvents(0)[0].State.ID != "trip" {
		t.Fatal("history leaked ownership")
	}
}

func TestAlarmObservationChangesAreDistinctFromAnalogSamples(t *testing.T) {
	p := alarmProject()
	p.Alarms[0].RaiseMS = 0
	ctx := context.Background()
	r, err := NewRuntime(ctx, p, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Stop()
	changes := 0
	stop := r.SubscribeAlarmChanges(func() {
		changes++
		_ = r.AlarmStates() // reentrant snapshot must not deadlock
	})
	defer stop()
	write := func(value int64, quality core.Quality) {
		t.Helper()
		if err := r.Publish(ctx, "BI11", core.Value{Raw: value, Quality: quality, Time: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	write(alarmBoundary, core.QualityGood)
	write(alarmBoundary+1, core.QualityGood)
	if changes != 1 || r.AlarmStates()[0].Value != alarmBoundary+1 {
		t.Fatal("analog refresh acted like an alarm event")
	}
	write(alarmBoundary+1, core.QualityBad)
	if changes != 2 || !r.AlarmStates()[0].Active {
		t.Fatal("bad quality did not invalidate the view or cleared alarm")
	}
	stop()
	write(alarmBoundary+1, core.QualityGood)
	if changes != 2 {
		t.Fatal("canceled subscriber received a later notification")
	}
}
