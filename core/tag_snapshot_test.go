package core

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTagSnapshotRevisionsIncludeSilentObservations(t *testing.T) {
	tag := NewTagDB().GetOrCreate("BI11", Meta{})
	_, zero := tag.Snapshot()
	if zero != 0 {
		t.Fatal("new tag has a sample revision")
	}
	at := time.Now()
	sample := Value{Raw: int64(9007199254740993), Quality: QualityGood, Time: at}
	tag.Publish(sample)
	var calls atomic.Int32
	cancel := tag.Subscribe(func(Value) { calls.Add(1) })
	defer cancel()
	_, first := tag.Snapshot()
	next := sample
	next.Time = at.Add(time.Second)
	tag.Publish(next)
	v, revision := tag.Snapshot()
	if revision != first || !v.Time.Equal(at) || calls.Load() != 1 {
		t.Fatal("unchanged Publish is not a no-op")
	}
	tag.Observe(next)
	v, revision = tag.Snapshot()
	if revision <= first || !v.Time.Equal(next.Time) || v.Raw != sample.Raw || calls.Load() != 1 {
		t.Fatal("silent heartbeat lost atomic state")
	}
	tag.Observe(next)
	_, again := tag.Snapshot()
	if again <= revision || calls.Load() != 1 {
		t.Fatal("observation revision did not advance")
	}
	if !tag.MarkStale(next.Time.Add(time.Second)) {
		t.Fatal("stale transition missing")
	}
	v, stale := tag.Snapshot()
	if stale <= again || v.Quality != QualityUncertain || v.Raw != sample.Raw || calls.Load() != 2 {
		t.Fatal("stale revision/value mismatch")
	}
	if tag.MarkStale(next.Time.Add(time.Hour)) {
		t.Fatal("repeated stale transition")
	}
	v.Raw = "local"
	current, currentRevision := tag.Snapshot()
	if current.Raw != sample.Raw || currentRevision != stale {
		t.Fatal("snapshot is not owned")
	}
}

func TestTagConcurrentSilentObservationsAdvanceSnapshots(t *testing.T) {
	tag := NewTagDB().GetOrCreate("AI11", Meta{})
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); tag.Observe(Value{Raw: 12.5, Quality: QualityGood, Time: time.Now()}) }()
	}
	wg.Wait()
	v, revision := tag.Snapshot()
	if revision != 100 || v.Raw != 12.5 || v.Quality != QualityGood {
		t.Fatal("lost concurrent observation", v, revision)
	}
}
