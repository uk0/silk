package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/point"
	pointsql "github.com/uk0/silk/point/sqlite"
)

func TestSQLiteRejectsLosingTextBeforeTransactionCommit(t *testing.T) {
	if !pointsql.StoreAvailable() {
		t.Skip("requires cgo")
	}
	ctx := context.Background()
	s, err := pointsql.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.Define(ctx, point.Definition{Name: "STATE", Kind: point.String}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err = s.Publish(ctx, "STATE", core.Value{Raw: "ready", Quality: core.QualityGood, Time: now}); err != nil {
		t.Fatal(err)
	}
	err = s.Publish(ctx, "STATE", core.Value{Raw: "state\xff", Quality: core.QualityGood, Time: now.Add(time.Second)})
	current, readErr := s.Latest(ctx, "STATE")
	recent, historyErr := s.RecentHistory(ctx, "STATE", 10)
	if err == nil || readErr != nil || historyErr != nil || current.Value.Raw != "ready" || len(recent) != 1 {
		t.Fatalf("lossy input was persisted: rejected=%v latest=%v count=%d errors=%v,%v", err != nil, current.Value.Raw, len(recent), readErr, historyErr)
	}
}
