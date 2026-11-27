package gui

import (
	"context"
	"testing"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/point"
)

type retryHistoryStore struct {
	pointMonitorStore
	queries chan int
	release chan struct{}
}

func (s *retryHistoryStore) RecentHistory(ctx context.Context, name string, limit int) ([]point.SampleRecord, error) {
	s.queries <- limit
	if limit != 2 {
		return nil, point.ErrHistoryPayloadLimit
	}
	select {
	case <-s.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return []point.SampleRecord{{Name: name, Value: core.Value{Raw: int64(42), Quality: core.QualityGood, Time: time.Now()}}}, nil
}

func TestProcessHistoryBudgetAllowsExplicitSmallerReadWithoutPartialRows(t *testing.T) {
	oldShow := showProcessPointDialog
	t.Cleanup(func() { showProcessPointDialog = oldShow })
	store := &retryHistoryStore{queries: make(chan int, 4), release: make(chan struct{})}
	v, _, posts := newMonitorTest(t, store)
	drainPointPosts(posts)
	v.SelectPoint("BI11")
	awaitLimit := func(want int) {
		t.Helper()
		select {
		case got := <-store.queries:
			if got != want {
				t.Fatalf("history row limit %d, want %d", got, want)
			}
		case <-time.After(4 * time.Second):
			t.Fatal("history query did not run")
		}
	}
	showProcessPointDialog = func(d *Dialog) DialogResult {
		body := d.Content().(*processPointDialogBody)
		pumpPointPosts(t, posts, func() bool {
			return body.status.Text() == i18n.T("History exceeds 16 MiB. Lower the row limit and read again.")
		})
		awaitLimit(200)
		if body.table.Model().RowCount() != 0 || !body.limit.IsEnabled() || !body.read.IsEnabled() || body.limit.Value() != 200 {
			t.Fatal("oversized read exposed partial data or hid smaller-read controls")
		}
		body.limit.SetValue(2)
		if len(store.queries) != 0 {
			t.Fatal("changing limit secretly queried the historian")
		}
		body.read.Action().Trigger(nil)
		awaitLimit(2)
		body.read.Action().Trigger(nil) // no duplicate in-flight query
		if body.read.IsEnabled() || body.limit.IsEnabled() {
			t.Fatal("parallel history commands remained enabled")
		}
		close(store.release)
		pumpPointPosts(t, posts, func() bool { return body.table.Model().RowCount() == 1 })
		if got := body.table.Model().CellText(0, 1); got != "42" || !body.read.IsEnabled() || len(store.queries) != 0 {
			t.Fatalf("bounded retry or command state is wrong: %q", got)
		}
		body.SetSize(360, 240)
		if body.limit.X()+body.limit.Width() > body.read.X() ||
			body.read.Y()+body.read.Height() > body.table.Y() ||
			body.table.Y()+body.table.Height() > body.status.Y() {
			t.Fatal("history row control covers table or status")
		}
		return DialogCancel
	}
	v.ShowHistory()
}
