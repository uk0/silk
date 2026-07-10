package stats

import (
	"testing"

	"github.com/uk0/silk/core"
)

const eps = 1e-9

func approx(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < eps
}

// TestCollectorRolling folds the sequence 10, 20, 5 and checks every KPI.
// The known initial value 10 is set BEFORE Track so the Subscribe prime folds
// it as the first (Count 1) sample; the two later SetValue calls bring Count
// to 3 without any dedupe (all three values differ).
func TestCollectorRolling(t *testing.T) {
	db := core.NewTagDB()
	tag := db.GetOrCreate("flow", core.Meta{})
	tag.SetValue(10.0) // primed as the first fold when Track subscribes

	c := NewCollector(db)
	c.Track("flow") // prime folds 10 -> Count 1
	tag.SetValue(20.0)
	tag.SetValue(5.0)

	st, ok := c.Get("flow")
	if !ok {
		t.Fatal("Get(flow): not tracked")
	}
	if st.Count != 3 {
		t.Fatalf("Count = %d, want 3 (prime 10 + 20 + 5)", st.Count)
	}
	if st.Min != 5 {
		t.Fatalf("Min = %v, want 5", st.Min)
	}
	if st.Max != 20 {
		t.Fatalf("Max = %v, want 20", st.Max)
	}
	if st.Sum != 35 {
		t.Fatalf("Sum = %v, want 35", st.Sum)
	}
	if st.Last != 5 {
		t.Fatalf("Last = %v, want 5", st.Last)
	}
	if !approx(st.Avg(), 35.0/3.0) {
		t.Fatalf("Avg = %v, want %v", st.Avg(), 35.0/3.0)
	}
}

// TestCollectorReset zeroes a stat and confirms the subscription stays live so
// new values fold from a clean slate.
func TestCollectorReset(t *testing.T) {
	db := core.NewTagDB()
	tag := db.GetOrCreate("t", core.Meta{})
	tag.SetValue(3.0)

	c := NewCollector(db)
	c.Track("t") // prime 3
	tag.SetValue(7.0)

	c.Reset("t")
	st, ok := c.Get("t")
	if !ok {
		t.Fatal("Get(t): not tracked after Reset")
	}
	if st != (Stat{}) {
		t.Fatalf("Reset did not zero: %+v", st)
	}
	if st.Avg() != 0 {
		t.Fatalf("Avg after Reset = %v, want 0", st.Avg())
	}

	tag.SetValue(9.0) // still subscribed -> folds afresh
	st, _ = c.Get("t")
	if st.Count != 1 || st.Min != 9 || st.Max != 9 || st.Last != 9 {
		t.Fatalf("post-Reset fold wrong: %+v", st)
	}
}

// TestCollectorIndependentTags proves two tracked tags accumulate separately.
func TestCollectorIndependentTags(t *testing.T) {
	db := core.NewTagDB()
	a := db.GetOrCreate("a", core.Meta{})
	b := db.GetOrCreate("b", core.Meta{})
	a.SetValue(1.0)
	b.SetValue(100.0)

	c := NewCollector(db)
	c.Track("a") // prime 1
	c.Track("b") // prime 100
	a.SetValue(2.0)
	a.SetValue(3.0)
	b.SetValue(200.0)

	sa, _ := c.Get("a")
	if sa.Count != 3 || sa.Min != 1 || sa.Max != 3 || sa.Sum != 6 || sa.Last != 3 {
		t.Fatalf("a = %+v, want Count3 Min1 Max3 Sum6 Last3", sa)
	}
	sb, _ := c.Get("b")
	if sb.Count != 2 || sb.Min != 100 || sb.Max != 200 || sb.Sum != 300 || sb.Last != 200 {
		t.Fatalf("b = %+v, want Count2 Min100 Max200 Sum300 Last200", sb)
	}
}

// TestCollectorGetUnknownAndAvgZero covers the untracked and empty cases.
func TestCollectorGetUnknownAndAvgZero(t *testing.T) {
	db := core.NewTagDB()
	c := NewCollector(db)
	if st, ok := c.Get("nope"); ok || st != (Stat{}) {
		t.Fatalf("Get(unknown) = %+v, %v; want zero,false", st, ok)
	}
	var zero Stat
	if zero.Avg() != 0 {
		t.Fatalf("zero.Avg() = %v, want 0", zero.Avg())
	}
}

// TestCollectorStopAll confirms values stop folding after unsubscribe while the
// last snapshot stays readable.
func TestCollectorStopAll(t *testing.T) {
	db := core.NewTagDB()
	tag := db.GetOrCreate("s", core.Meta{})
	tag.SetValue(1.0)

	c := NewCollector(db)
	c.Track("s") // prime 1
	tag.SetValue(2.0)
	if st, _ := c.Get("s"); st.Count != 2 {
		t.Fatalf("pre-stop Count = %d, want 2", st.Count)
	}

	c.StopAll()
	tag.SetValue(3.0) // no longer subscribed -> ignored
	st, _ := c.Get("s")
	if st.Count != 2 || st.Last != 2 {
		t.Fatalf("post-stop = %+v, want Count2 Last2", st)
	}
}

// TestCollectorConcurrent drives folds and reads from separate goroutines; run
// with -race to exercise the locking. Values 1..n are distinct (no Publish
// dedupe), so Count is deterministic: n plus the single primed sample.
func TestCollectorConcurrent(t *testing.T) {
	db := core.NewTagDB()
	tag := db.GetOrCreate("c", core.Meta{})
	tag.SetValue(0.5) // primed first fold

	c := NewCollector(db)
	c.Track("c") // prime 0.5 -> Count 1

	const n = 500
	done := make(chan struct{})
	go func() {
		for i := 1; i <= n; i++ {
			tag.SetValue(float64(i))
		}
		close(done)
	}()
	for r := 0; r < 4; r++ {
		go func() {
			for {
				select {
				case <-done:
					return
				default:
					c.Get("c")
				}
			}
		}()
	}
	<-done

	st, _ := c.Get("c")
	if st.Count != n+1 {
		t.Fatalf("Count = %d, want %d", st.Count, n+1)
	}
	if st.Min != 0.5 {
		t.Fatalf("Min = %v, want 0.5", st.Min)
	}
	if st.Max != float64(n) || st.Last != float64(n) {
		t.Fatalf("Max/Last = %v/%v, want %d", st.Max, st.Last, n)
	}
}
