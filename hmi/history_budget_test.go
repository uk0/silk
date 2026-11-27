package hmi_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/hmi"
	"github.com/uk0/silk/point"
)

type budgetHistoryStore struct {
	rows []point.SampleRecord
}

func (*budgetHistoryStore) Define(context.Context, point.Definition) error    { return nil }
func (*budgetHistoryStore) Publish(context.Context, string, core.Value) error { return nil }
func (s *budgetHistoryStore) ReadHistory(context.Context, string, time.Time, time.Time, int) ([]point.SampleRecord, error) {
	return s.rows, nil
}
func (s *budgetHistoryStore) RecentHistory(context.Context, string, int) ([]point.SampleRecord, error) {
	return s.rows, nil
}

func TestRuntimeRejectsUnboundedThirdPartyRecentHistory(t *testing.T) {
	ctx := context.Background()
	store := new(budgetHistoryStore)
	text := strings.Repeat("a", 1<<20)
	first := time.Unix(1_747_000_000, 0).UTC()
	// Equal timestamps are legal; the same immutable raw bytes are referenced
	// so the negative test does not spend 17 MiB populating the fake store.
	row := point.SampleRecord{Name: "TEXT", Value: core.Value{Raw: text, Quality: core.QualityGood, Time: first}}
	for range 17 {
		store.rows = append(store.rows, row)
	}
	r, err := hmi.NewRuntime(ctx, livePointProject(), store, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Stop()
	if rows, err := r.RecentHistory(ctx, "TEXT", 17); !errors.Is(err, point.ErrHistoryPayloadLimit) || len(rows) != 0 {
		t.Errorf("unbounded history reached process monitor: rows=%d err=%v", len(rows), err)
	}
	if rows, err := r.History(ctx, "TEXT", first, first.Add(time.Second), 17); !errors.Is(err, point.ErrHistoryPayloadLimit) || len(rows) != 0 {
		t.Errorf("unbounded range reached process monitor: rows=%d err=%v", len(rows), err)
	}
	store.rows = store.rows[:16]
	if rows, err := r.RecentHistory(ctx, "TEXT", 16); err != nil || len(rows) != 16 || rows[0].Value.Raw != text {
		t.Fatalf("exact payload budget should be accepted: rows=%d err=%v", len(rows), err)
	}
}
