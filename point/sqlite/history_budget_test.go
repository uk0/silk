package sqlite_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/point"
	pointsql "github.com/uk0/silk/point/sqlite"
)

func TestSQLiteRecentHistoryUsesEncodedPayloadBudget(t *testing.T) {
	if !pointsql.StoreAvailable() {
		t.Skip("SQLite storage requires cgo")
	}
	ctx := context.Background()
	s, err := pointsql.Open(filepath.Join(t.TempDir(), "history.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const name = "BI_LABEL"
	if err := s.Define(ctx, point.Definition{Name: name, Kind: point.String}); err != nil {
		t.Fatal(err)
	}
	// One raw MiB expands to six in JSON. An implementation that counts only
	// decoded strings, characters, or rows accepts all three observations.
	value := strings.Repeat("\x01", 1<<20)
	first := time.Unix(1_747_000_000, 0).UTC()
	for i := range 3 {
		err := s.Publish(ctx, name, core.Value{Raw: value, Quality: core.QualityGood, Time: first.Add(time.Duration(i) * time.Second)})
		if err != nil {
			t.Fatalf("store valid sample: %v", err)
		}
	}
	if rows, err := s.RecentHistory(ctx, name, 3); !errors.Is(err, point.ErrHistoryPayloadLimit) || len(rows) != 0 {
		t.Errorf("recent query accepted oversized encoded history: rows=%d err=%v", len(rows), err)
	}
	if rows, err := s.ReadHistory(ctx, name, first, first.Add(3*time.Second), 3); !errors.Is(err, point.ErrHistoryPayloadLimit) || len(rows) != 0 {
		t.Errorf("ascending history budget: rows=%d err=%v", len(rows), err)
	}
	rows, err := s.RecentHistory(ctx, name, 2)
	if err != nil || len(rows) != 2 || !rows[0].Value.Time.Equal(first.Add(2*time.Second)) || !rows[1].Value.Time.Equal(first.Add(time.Second)) || rows[0].Value.Raw != value {
		t.Fatalf("valid newest-first history changed: rows=%d err=%v", len(rows), err)
	}
	latest, err := s.Latest(ctx, name)
	if err != nil || !latest.Value.Time.Equal(first.Add(2*time.Second)) {
		t.Fatalf("bounded read changed stored point: err=%v", err)
	}
}
