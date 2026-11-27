package hmi_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/hmi"
	"github.com/uk0/silk/point"
)

func TestRuntimePublishRejectsNonPersistentObservationsBeforeStoreOrLiveUpdate(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	for _, tc := range []struct {
		name   string
		sample core.Value
	}{
		{"timestamp outside database range", core.Value{Raw: "state", Quality: core.QualityGood, Time: time.Date(2450, 1, 1, 0, 0, 0, 0, time.UTC)}},
		{"oversized text", core.Value{Raw: strings.Repeat("x", (1<<20)+1), Quality: core.QualityGood, Time: now}},
		{"invalid UTF-8", core.Value{Raw: "valid\xff", Quality: core.QualityGood, Time: now}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := new(livePointStore)
			r, err := hmi.NewRuntime(ctx, livePointProject(), store, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer r.Stop()
			before, _ := r.PointState("TEXT")
			err = r.Publish(ctx, "TEXT", tc.sample)
			after, _ := r.PointState("TEXT")
			if err == nil || after.Revision != before.Revision || len(store.values) != 0 {
				t.Errorf("invalid sample reached an observer/store: rejected=%v revisionChanged=%v writes=%d", err != nil, after.Revision != before.Revision, len(store.values))
			}
		})
	}
}

func TestRuntimeHistoryRejectsIllFormedTextRatherThanPersistingAnotherValue(t *testing.T) {
	ctx := context.Background()
	rec := point.SampleRecord{Name: "TEXT", Value: core.Value{Raw: "raw\xff", Quality: core.QualityGood, Time: time.Now()}}
	store := &budgetHistoryStore{rows: []point.SampleRecord{rec}}
	r, err := hmi.NewRuntime(ctx, livePointProject(), store, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Stop()
	if items, err := r.RecentHistory(ctx, "TEXT", 1); err == nil || len(items) != 0 {
		t.Fatalf("invalid text escaped custom store validation: rows=%d err=%v", len(items), err)
	}
}
