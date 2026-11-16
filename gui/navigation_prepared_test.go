package gui

import (
	"math/rand"
	"reflect"
	"testing"
)

func preparedNavigationFixture() (*NavigationStack, []NavPosition) {
	positions := []NavPosition{{FilePath: "a 空间.go", Line: 1, Column: 2, ScrollY: 3}, {FilePath: "b.go", Line: 4, Column: 5, ScrollY: 6}, {FilePath: "c.go", Line: 7, Column: 8, ScrollY: 9}, {FilePath: "d.go", Line: 10, Column: 11, ScrollY: 12}}
	ns := new(NavigationStack)
	for _, p := range positions {
		ns.Push(p)
	}
	return ns, positions
}

func TestNavigationCurrentPositionIsReadOnly(t *testing.T) {
	var absent *NavigationStack
	if p, ok := absent.CurrentPosition(); ok || p != (NavPosition{}) {
		t.Fatal("nil history has a current position")
	}
	ns, pos := preparedNavigationFixture()
	events := 0
	defer ns.SubscribeChanged(func() { events++ })()
	visits := ns.PrepareNavigation(pos[3], false)
	for i := 0; i < 10; i++ {
		p, ok := ns.CurrentPosition()
		if !ok || p != pos[3] {
			t.Fatal("wrong committed position")
		}
		p.Line = 999 // The returned value must not alias history.
	}
	if events != 0 || !ns.CanCommitNavigation(visits[0]) || !ns.CommitNavigation(visits[0]) {
		t.Fatal("read invalidated navigation or emitted events")
	}
	if p, ok := ns.CurrentPosition(); !ok || p != pos[2] {
		t.Fatal("commit not reflected")
	}
	ns.Clear()
	if _, ok := ns.CurrentPosition(); ok {
		t.Fatal("cleared history is not empty")
	}
}

func TestPreparedNavigationDefersMutationUntilSuccessfulOpen(t *testing.T) {
	ns, pos := preparedNavigationFixture()
	live := pos[3]
	live.Line = 22
	live.ScrollY = 99
	before := append([]NavPosition(nil), ns.stack...)
	events := 0
	stop := ns.SubscribeChanged(func() { events++ })
	defer stop()
	visits := ns.PrepareNavigation(live, false)
	if len(visits) != 3 || !reflect.DeepEqual(ns.stack, before) || ns.current != 3 || events != 0 {
		t.Fatal("prepare changed history", visits, ns.stack, events)
	}
	for i, v := range visits {
		if v.Destination() != pos[2-i] || !ns.CanCommitNavigation(v) {
			t.Fatal("candidate order/ownership", i, v.Destination())
		}
	}
	// The first two candidates failed to open. No provisional history mutation.
	if !ns.CommitNavigation(visits[2]) || ns.current != 0 || ns.stack[3] != live || events != 1 {
		t.Fatal("completed navigation was not atomic", ns.stack, ns.current, events)
	}
	for _, v := range visits {
		if ns.CanCommitNavigation(v) || ns.CommitNavigation(v) {
			t.Fatal("copied or sibling token committed twice")
		}
	}
	if events != 1 {
		t.Fatal("rejected commit notified")
	}
	backToLive := ns.PrepareNavigation(pos[0], true)
	if len(backToLive) != 3 || backToLive[2].Destination() != live || !ns.CommitNavigation(backToLive[2]) || ns.current != 3 || events != 2 {
		t.Fatal("forward lost captured live return position")
	}
}

func TestPreparedNavigationAbandonedOpenIsSilent(t *testing.T) {
	ns, _ := preparedNavigationFixture()
	before := append([]NavPosition(nil), ns.stack...)
	events := 0
	defer ns.SubscribeChanged(func() { events++ })()
	for i := 0; i < 1000; i++ {
		_ = ns.PrepareNavigation(NavPosition{FilePath: "live", Line: i}, false)
	}
	if !reflect.DeepEqual(ns.stack, before) || ns.current != 3 || events != 0 {
		t.Fatal("failed/canceled opens changed history")
	}
}

func TestPreparedNavigationHistoryMutationsInvalidate(t *testing.T) {
	mutations := map[string]func(*NavigationStack){
		"push":        func(n *NavigationStack) { n.Push(NavPosition{FilePath: "new"}) },
		"push-scroll": func(n *NavigationStack) { p := n.stack[n.current]; p.ScrollY++; n.Push(p) },
		"jump":        func(n *NavigationStack) { n.RecordJump(NavPosition{FilePath: "x"}, NavPosition{FilePath: "y"}) },
		"back":        func(n *NavigationStack) { n.GoBack() },
		"forward":     func(n *NavigationStack) { n.GoForward() },
		"synchronous-visit": func(n *NavigationStack) {
			n.Navigate(n.stack[n.current], false, func(NavPosition) bool { return true })
		},
		"rename": func(n *NavigationStack) { n.MapPaths(func(p string) string { return "renamed/" + p }) },
		"clear":  func(n *NavigationStack) { n.Clear() },
		"clear-rebuild-aba": func(n *NavigationStack) {
			old := append([]NavPosition(nil), n.stack...)
			n.Clear()
			for _, p := range old {
				n.Push(p)
			}
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			n, _ := preparedNavigationFixture()
			if name == "forward" {
				n.GoBack()
			}
			visit := n.PrepareNavigation(n.stack[n.current], false)[0]
			mutate(n)
			before := append([]NavPosition(nil), n.stack...)
			index := n.current
			events := 0
			stop := n.SubscribeChanged(func() { events++ })
			defer stop()
			if n.CanCommitNavigation(visit) || n.CommitNavigation(visit) || n.current != index || !reflect.DeepEqual(n.stack, before) || events != 0 {
				t.Fatal("stale token altered newer history")
			}
		})
	}
}

func TestPreparedNavigationNoopsAndSubscriptionsPreserveValidity(t *testing.T) {
	n, _ := preparedNavigationFixture()
	live := n.stack[n.current]
	v := n.PrepareNavigation(live, false)[0]
	n.Push(live)
	n.RecordJump(live, live)
	n.MapPaths(func(p string) string { return p })
	n.MapPaths(nil)
	n.GoForward()
	n.Navigate(live, false, nil)
	n.Navigate(live, false, func(NavPosition) bool { return false })
	cancel := n.SubscribeChanged(func() {})
	cancel()
	cancel()
	n.SubscribeChanged(nil)()
	if !n.CanCommitNavigation(v) || !n.CommitNavigation(v) {
		t.Fatal("exact no-op or subscription invalidated real open")
	}
}

func TestPreparedNavigationRejectsZeroForeignAndEmpty(t *testing.T) {
	a, _ := preparedNavigationFixture()
	b, _ := preparedNavigationFixture()
	v := a.PrepareNavigation(a.stack[a.current], false)[0]
	b.PrepareNavigation(b.stack[b.current], false)
	var empty NavigationStack
	var missing *NavigationStack
	if b.CanCommitNavigation(v) || b.CommitNavigation(v) || a.CommitNavigation(NavigationVisit{}) || empty.CommitNavigation(v) || missing.CommitNavigation(v) {
		t.Fatal("foreign/zero token accepted")
	}
	if len(empty.PrepareNavigation(NavPosition{}, false)) != 0 || len(missing.PrepareNavigation(NavPosition{}, true)) != 0 || (NavigationVisit{}).Destination() != (NavPosition{}) {
		t.Fatal("empty plan")
	}
}

func TestPreparedNavigationSkipsLiveCoordinatesAndIsBounded(t *testing.T) {
	n, pos := preparedNavigationFixture()
	visits := n.PrepareNavigation(pos[2], false)
	if len(visits) != 2 || visits[0].Destination() != pos[1] || visits[1].Destination() != pos[0] {
		t.Fatal("live position was not skipped")
	}
	for i := 0; i < 1024; i++ {
		n.Push(NavPosition{FilePath: "never-read.go", Line: i})
	}
	visits = n.PrepareNavigation(NavPosition{}, false)
	if n.Len() != 256 || len(visits) != 255 || visits[0].Destination().Line != 1022 || visits[254].Destination().Line != 768 {
		t.Fatal("unbounded or reversed snapshot")
	}
	snapshot := visits[0].Destination()
	n.MapPaths(func(string) string { return "other.go" })
	if visits[0].Destination() != snapshot || n.CanCommitNavigation(visits[0]) {
		t.Fatal("captured position aliased mutated history")
	}
}

func TestPreparedNavigationInvalidBeforeObserverRuns(t *testing.T) {
	n, _ := preparedNavigationFixture()
	v := n.PrepareNavigation(n.stack[n.current], false)[0]
	events := 0
	defer n.SubscribeChanged(func() {
		events++
		if n.CanCommitNavigation(v) || n.CommitNavigation(v) {
			t.Fatal("observer reused committed token")
		}
	})()
	if !n.CommitNavigation(v) || events != 1 {
		t.Fatal("commit event")
	}
}

func TestPreparedNavigationTokensCannotRetainStackOrCallbacks(t *testing.T) {
	seen := map[reflect.Type]bool{}
	var check func(reflect.Type)
	check = func(typ reflect.Type) {
		if seen[typ] {
			return
		}
		seen[typ] = true
		if typ == reflect.TypeOf(NavigationStack{}) {
			t.Fatal("token retains stack and its observers")
		}
		switch typ.Kind() {
		case reflect.Func, reflect.Interface, reflect.Map, reflect.Slice, reflect.Chan:
			t.Fatalf("token can retain an opaque owner through %v", typ)
		case reflect.Pointer, reflect.Array:
			check(typ.Elem())
		case reflect.Struct:
			for i := 0; i < typ.NumField(); i++ {
				check(typ.Field(i).Type)
			}
		}
	}
	check(reflect.TypeOf(NavigationVisit{}))
	if reflect.TypeOf(navigationGeneration{}).Size() == 0 {
		t.Fatal("zero-sized identity can share addresses")
	}
}

func TestPreparedNavigationUnusedAPIAddsNoMutationAllocation(t *testing.T) {
	n := new(NavigationStack)
	for i := 0; i < 256; i++ {
		n.Push(NavPosition{FilePath: "a", Line: i})
	}
	i := 256
	allocations := testing.AllocsPerRun(100, func() { i++; n.Push(NavPosition{FilePath: "a", Line: i}) })
	if allocations != 0 || n.prepared != nil {
		t.Fatal("legacy mutation allocates without using preparation", allocations)
	}
}

func TestPreparedNavigationSnapshotAllocatesOnce(t *testing.T) {
	n := new(NavigationStack)
	for i := 0; i < 256; i++ {
		n.Push(NavPosition{FilePath: "a", Line: i})
	}
	n.PrepareNavigation(NavPosition{}, false)
	allocations := testing.AllocsPerRun(100, func() {
		visits := n.PrepareNavigation(NavPosition{}, false)
		if len(visits) != 255 {
			panic("candidate count")
		}
	})
	if allocations != 1 {
		t.Fatal("bounded snapshot grew repeatedly", allocations)
	}
}

func TestPreparedNavigationMatchesSynchronousTraversal(t *testing.T) {
	rng := rand.New(rand.NewSource(20261115))
	for trial := 0; trial < 2000; trial++ {
		a, b := new(NavigationStack), new(NavigationStack)
		count := rng.Intn(300)
		for i := 0; i < count; i++ {
			p := NavPosition{FilePath: string(rune('a' + rng.Intn(4))), Line: rng.Intn(7), Column: rng.Intn(3), ScrollY: float64(rng.Intn(200))}
			a.Push(p)
			b.Push(p)
		}
		steps := rng.Intn(a.Len() + 1)
		for i := 0; i < steps; i++ {
			a.GoBack()
			b.GoBack()
		}
		live := NavPosition{FilePath: string(rune('a' + rng.Intn(5))), Line: rng.Intn(7), Column: rng.Intn(3), ScrollY: float64(rng.Intn(200))}
		forward := rng.Intn(2) != 0
		filter := rng.Intn(5)
		allowed := func(p NavPosition) bool { return (p.Line+p.Column+len(p.FilePath))%5 == filter }
		synchronous := a.Navigate(live, forward, allowed)
		prepared := false
		for _, visit := range b.PrepareNavigation(live, forward) {
			if allowed(visit.Destination()) {
				prepared = b.CommitNavigation(visit)
				break
			}
		}
		if synchronous != prepared || a.current != b.current || !reflect.DeepEqual(a.stack, b.stack) {
			t.Fatalf("trial %d: synchronous/prepared divergence: %v %v", trial, a, b)
		}
	}
}
