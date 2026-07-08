package core

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestTagBasics covers tag creation, the accessors, and payload coercion.
func TestTagBasics(t *testing.T) {
	db := NewTagDB()
	tag := db.GetOrCreate("reactor.temp", Meta{Unit: "℃", Min: 0, Max: 150, Desc: "reactor temperature"})

	if tag.Name() != "reactor.temp" {
		t.Fatalf("Name() = %q", tag.Name())
	}
	if m := tag.Meta(); m.Unit != "℃" || m.Max != 150 {
		t.Fatalf("Meta() = %+v", m)
	}

	before := time.Now()
	tag.SetValue(42.5)
	v := tag.Value()
	if v.Float() != 42.5 {
		t.Fatalf("Float() = %v, want 42.5", v.Float())
	}
	if v.Quality != QualityGood {
		t.Fatalf("Quality = %v, want QualityGood", v.Quality)
	}
	if v.Time.Before(before) {
		t.Fatalf("Time not stamped forward: %v", v.Time)
	}

	// coercion across payload kinds
	tag.SetValue(true)
	if !tag.Value().Bool() || tag.Value().Float() != 1 {
		t.Fatalf("bool coercion failed: %+v", tag.Value())
	}
	tag.SetValue(int64(7))
	if tag.Value().Int() != 7 || tag.Value().Float() != 7 {
		t.Fatalf("int64 coercion failed: %+v", tag.Value())
	}
	tag.SetValue("running")
	if tag.Value().String() != "running" {
		t.Fatalf("string payload = %q", tag.Value().String())
	}
}

// TestTagSetValueNotifiesOnChange verifies subscribers are primed with the
// current sample, notified on a changed value, and NOT notified when the same
// value is published again.
func TestTagSetValueNotifiesOnChange(t *testing.T) {
	db := NewTagDB()
	tag := db.GetOrCreate("level", Meta{})
	tag.SetValue(1.0) // no subscribers yet; cur = 1.0

	var got []float64
	cancel := tag.Subscribe(func(v Value) {
		got = append(got, v.Float())
	})
	defer cancel()
	// Subscribe primes synchronously with cur -> got == [1]

	tag.SetValue(1.0) // same value  -> no notification
	tag.SetValue(2.0) // change      -> notify
	tag.SetValue(2.0) // same value  -> no notification
	tag.SetValue(3.0) // change      -> notify

	want := []float64{1, 2, 3}
	if len(got) != len(want) {
		t.Fatalf("got %v (%d notifications), want %v — same-value dedup failed", got, len(got), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %v, want %v (full seq %v)", i, got[i], want[i], got)
		}
	}
}

// TestTagUnsubscribeStopsDelivery verifies the CancelFunc stops delivery and
// is idempotent (a second call is a safe no-op).
func TestTagUnsubscribeStopsDelivery(t *testing.T) {
	db := NewTagDB()
	tag := db.GetOrCreate("pump.run", Meta{})

	var n int
	cancel := tag.Subscribe(func(v Value) { n++ })
	// prime with the zero sample -> n = 1
	tag.SetValue(true) // change -> n = 2
	cancel()
	tag.SetValue(false) // no live subscriber -> n stays 2
	cancel()            // idempotent: must not panic or over-delete

	if n != 2 {
		t.Fatalf("subscriber saw %d notifications, want 2 (prime + one change before cancel)", n)
	}
}

// TestTagDBGetOrCreateIdempotent verifies GetOrCreate returns the same *Tag for
// a name and does not overwrite meta, plus Get / SetValue-by-name / All.
func TestTagDBGetOrCreateIdempotent(t *testing.T) {
	db := NewTagDB()
	a := db.GetOrCreate("t1", Meta{Unit: "℃", Max: 100})
	b := db.GetOrCreate("t1", Meta{Unit: "bar", Max: 10}) // meta must be ignored
	if a != b {
		t.Fatal("GetOrCreate returned a different *Tag for the same name")
	}
	if a.Meta().Unit != "℃" || a.Meta().Max != 100 {
		t.Fatalf("meta overwritten on second GetOrCreate: %+v", a.Meta())
	}

	got, ok := db.Get("t1")
	if !ok || got != a {
		t.Fatalf("Get(t1) = (%v, %v), want the created tag", got, ok)
	}
	if _, ok := db.Get("nope"); ok {
		t.Fatal("Get returned ok=true for a missing tag")
	}

	// SetValue-by-name creates the tag on first use.
	db.SetValue("t2", 3.14)
	t2, ok := db.Get("t2")
	if !ok || t2.Value().Float() != 3.14 {
		t.Fatalf("db.SetValue by name did not create+set t2: ok=%v tag=%v", ok, t2)
	}

	if all := db.All(); len(all) != 2 {
		t.Fatalf("All() len = %d, want 2", len(all))
	}
}

// TestTagConcurrentSetValue drives N writers (driver polls), UI-side readers,
// and subscribe/unsubscribe churn at once. Its job is to be -race clean and
// live: run with `go test -race ./core/ -run Tag`.
func TestTagConcurrentSetValue(t *testing.T) {
	db := NewTagDB()
	tag := db.GetOrCreate("flow", Meta{})

	var count int64
	cancel := tag.Subscribe(func(v Value) { atomic.AddInt64(&count, 1) })
	defer cancel()

	const (
		writers = 16
		perW    = 200
	)
	var wg sync.WaitGroup

	// N concurrent writers; disjoint value ranges so every write is a change.
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perW; i++ {
				tag.SetValue(float64(w*perW + i))
			}
		}(w)
	}
	// Concurrent UI-side readers.
	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perW; i++ {
				_ = tag.Value().Float()
			}
		}()
	}
	// Concurrent subscribe/unsubscribe churn (dynamic screen binds/unbinds).
	for s := 0; s < 4; s++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				tag.Subscribe(func(Value) {})()
			}
		}()
	}
	wg.Wait()

	if atomic.LoadInt64(&count) == 0 {
		t.Fatal("primary subscriber received no notifications")
	}
}
