package core

import (
	"testing"
	"time"
)

func TestTagObserveRefreshesWithoutNotify(t *testing.T) {
	tag := NewTagDB().GetOrCreate("AI11", Meta{})
	calls := 0
	stop := tag.Subscribe(func(Value) { calls++ })
	defer stop()
	a := Value{Raw: 1., Quality: QualityGood, Time: time.Unix(1, 0)}
	tag.Publish(a)
	n := calls
	b := a
	b.Time = time.Unix(2, 0)
	tag.Observe(b)
	if calls != n || tag.Value().Time != b.Time {
		t.Fatal("heartbeat woke UI or lost freshness")
	}
	b.Quality = QualityBad
	tag.Observe(b)
	if calls != n+1 {
		t.Fatal("quality change suppressed")
	}
}
