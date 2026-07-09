package eventlog

import (
	"path/filepath"
	"testing"
	"time"
)

func TestEventLogRecordAndQuery(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "events.sqlite")
	log, err := NewLog(dbPath)
	if err != nil {
		t.Fatalf("NewLog: %v", err)
	}
	defer log.Close()

	// Record several events across every kind. A tiny sleep between records
	// guarantees strictly increasing UnixNano timestamps so ordering is
	// deterministic to assert on.
	records := []struct {
		kind    Kind
		source  string
		message string
	}{
		{KindSystem, "core", "startup"},
		{KindLogin, "auth", "user admin logged in"},
		{KindAlarm, "sensor-1", "temperature high"},
		{KindWrite, "plc", "tag T1 set to 42"},
		{KindAlarm, "sensor-2", "pressure low"},
	}
	for _, r := range records {
		if err := log.Record(r.kind, r.source, r.message); err != nil {
			t.Fatalf("Record(%s): %v", r.kind, err)
		}
		time.Sleep(time.Millisecond)
	}

	from := time.Now().Add(-time.Hour)
	to := time.Now().Add(time.Hour)

	// A wide range returns every event, ts-ordered ascending.
	all, err := log.Query(from, to)
	if err != nil {
		t.Fatalf("Query all: %v", err)
	}
	if len(all) != len(records) {
		t.Fatalf("Query all returned %d events, want %d", len(all), len(records))
	}
	for i := 1; i < len(all); i++ {
		if all[i].TS.Before(all[i-1].TS) {
			t.Errorf("events not ts-ordered at %d: %v before %v", i, all[i].TS, all[i-1].TS)
		}
	}
	// Round-trip check on the first (oldest) event.
	if all[0].Kind != KindSystem || all[0].Source != "core" || all[0].Message != "startup" {
		t.Errorf("first event = %+v, want system/core/startup", all[0])
	}

	// Filtering by a single kind returns only events of that kind.
	alarms, err := log.Query(from, to, KindAlarm)
	if err != nil {
		t.Fatalf("Query alarms: %v", err)
	}
	if len(alarms) != 2 {
		t.Fatalf("Query alarms returned %d events, want 2", len(alarms))
	}
	for _, e := range alarms {
		if e.Kind != KindAlarm {
			t.Errorf("filtered query returned non-alarm event: %+v", e)
		}
	}

	// Filtering by multiple kinds unions them (exercises the IN clause).
	loginWrite, err := log.Query(from, to, KindLogin, KindWrite)
	if err != nil {
		t.Fatalf("Query login+write: %v", err)
	}
	if len(loginWrite) != 2 {
		t.Fatalf("Query login+write returned %d events, want 2", len(loginWrite))
	}
	for _, e := range loginWrite {
		if e.Kind != KindLogin && e.Kind != KindWrite {
			t.Errorf("multi-kind query returned out-of-set event: %+v", e)
		}
	}

	// A range with no events returns none.
	empty, err := log.Query(time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("Query empty: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("Query empty range returned %d events, want 0", len(empty))
	}
}
