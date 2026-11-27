package hmi

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/point"
)

type parkedPruner struct {
	entered chan time.Time
	stopped chan struct{}
}

func (*parkedPruner) Define(context.Context, point.Definition) error    { return nil }
func (*parkedPruner) Publish(context.Context, string, core.Value) error { return nil }

func (p *parkedPruner) Prune(ctx context.Context, before time.Time, limit int) (int64, error) {
	p.entered <- before
	<-ctx.Done()
	close(p.stopped)
	return 0, ctx.Err()
}

func TestRuntimeSlowPruneDoesNotHoldFreshnessOrAlarmDwell(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		p := alarmProject()
		p.RetentionDays = 2
		p.Points[1].StaleMS = 150
		p.Alarms[0].RaiseMS = 175
		store := &parkedPruner{make(chan time.Time, 1), make(chan struct{})}
		now := time.Now()
		r, err := NewRuntime(ctx, p, store, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer r.Stop()
		if err := r.Publish(ctx, "BI11", core.Value{Raw: alarmBoundary, Quality: core.QualityGood, Time: now}); err != nil {
			t.Fatal(err)
		}
		if err := r.Start(ctx); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		time.Sleep(120 * time.Millisecond)
		synctest.Wait()
		select {
		case before := <-store.entered:
			if want := now.Add(100*time.Millisecond - 48*time.Hour); !before.Equal(want) {
				t.Fatalf("retention cutoff %v, want %v", before, want)
			}
		default:
			t.Fatal("retention never entered")
		}
		fresh, err := r.PointState("DI11")
		if err != nil || fresh.Value.Quality != core.QualityGood || r.AlarmStates()[0].Active {
			t.Fatal("freshness or dwell completed before deadline")
		}
		time.Sleep(100 * time.Millisecond)
		synctest.Wait()
		stale, err := r.PointState("DI11")
		if err != nil || stale.Value.Quality != core.QualityUncertain || !stale.Value.Time.Equal(now) {
			t.Fatalf("blocked retention delayed point freshness: sample=%+v err=%v", stale.Value, err)
		}
		if a := r.AlarmStates()[0]; !a.Active || !a.ConditionActive || a.Value != alarmBoundary {
			t.Fatalf("blocked retention delayed alarm crossing: %+v", a)
		}
		r.Stop()
		select {
		case <-store.stopped:
		default:
			t.Fatal("Stop returned before retention worker exited")
		}
		if r.Errors()["storage"] != "" {
			t.Fatal("Stop cancellation reported as a storage fault")
		}
	})
}

type scheduledPrune struct {
	before time.Time
	limit  int
}
type retentionRecorder struct {
	mu    sync.Mutex
	calls []scheduledPrune
}

func (*retentionRecorder) Define(context.Context, point.Definition) error    { return nil }
func (*retentionRecorder) Publish(context.Context, string, core.Value) error { return nil }
func (p *retentionRecorder) Prune(_ context.Context, before time.Time, limit int) (int64, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, scheduledPrune{before, limit})
	if len(p.calls) == 1 {
		return 0, context.DeadlineExceeded
	}
	return 1, nil
}

func (p *retentionRecorder) snapshot() []scheduledPrune {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]scheduledPrune(nil), p.calls...)
}

func TestRuntimeRetentionDoesNotArmIdleSupervision(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		p := alarmProject()
		p.Alarms = nil
		p.RetentionDays = 1
		store := new(retentionRecorder)
		now := time.Now()
		r, err := NewRuntime(ctx, p, store, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer r.Stop()
		if ticker := r.newServiceTicker(); ticker != nil {
			ticker.Stop()
			t.Fatal("retention-only runtime armed a 10 Hz supervision ticker")
		}
		if err := r.Start(ctx); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		time.Sleep(150 * time.Millisecond)
		synctest.Wait()
		if calls := store.snapshot(); len(calls) != 1 || calls[0].limit != 10000 ||
			!calls[0].before.Equal(now.Add(100*time.Millisecond-24*time.Hour)) ||
			r.Errors()["storage"] != context.DeadlineExceeded.Error() {
			t.Fatalf("first retention: calls=%v errors=%v", calls, r.Errors())
		}
		time.Sleep(59 * time.Second)
		synctest.Wait()
		if calls := store.snapshot(); len(calls) != 1 {
			t.Fatalf("retention caught up prematurely: %v", calls)
		}
		time.Sleep(time.Second)
		synctest.Wait()
		if calls := store.snapshot(); len(calls) != 2 || calls[1].limit != 10000 ||
			calls[1].before.Sub(calls[0].before) != time.Minute ||
			r.Errors()["storage"] != "" {
			t.Fatalf("repeat retention: calls=%v errors=%v", calls, r.Errors())
		}
	})
}
