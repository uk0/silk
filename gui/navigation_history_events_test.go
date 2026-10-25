package gui

import (
	"reflect"
	"testing"
)

func TestNavigationChangedAtomicMutationsAndNoops(t *testing.T) {
	var ns NavigationStack
	type state struct {
		count, current int
		back, forward  bool
	}
	var events []state
	stop := ns.SubscribeChanged(func() { events = append(events, state{ns.Len(), ns.current, ns.CanGoBack(), ns.CanGoForward()}) })
	defer stop()
	a, b := NavPosition{FilePath: "a", Line: 1}, NavPosition{FilePath: "b", Line: 2}
	ns.Clear()
	ns.GoBack()
	ns.GoForward()
	ns.MapPaths(nil)
	ns.RecordJump(a, a)
	if len(events) != 0 {
		t.Fatal("empty/noop event", events)
	}
	ns.RecordJump(a, b)
	if !reflect.DeepEqual(events, []state{{2, 1, true, false}}) {
		t.Fatal("partial jump event", events)
	}
	ns.Push(b)
	ns.RecordJump(b, b)
	ns.GoForward()
	ns.MapPaths(func(p string) string { return p })
	if ns.Navigate(b, false, func(NavPosition) bool { return false }) || ns.Navigate(b, false, nil) || len(events) != 1 {
		t.Fatal("failed/noop changed history", events)
	}
	b.ScrollY = 18
	ns.Push(b)
	if len(events) != 2 {
		t.Fatal("scroll update not observed")
	}
	if got, ok := ns.GoBack(); !ok || got != a {
		t.Fatal("back", got, ok)
	}
	ns.GoBack()
	if len(events) != 3 || !events[2].forward || events[2].back {
		t.Fatal("back boundary", events)
	}
	if got, ok := ns.GoForward(); !ok || got != b {
		t.Fatal("forward", got, ok)
	}
	live := b
	live.Column = 9
	if !ns.Navigate(live, false, func(p NavPosition) bool { return p == a }) || ns.stack[1] != live || len(events) != 5 {
		t.Fatal("live caret return position")
	}
	ns.MapPaths(func(p string) string { return "new/" + p })
	if len(events) != 6 || ns.stack[0].FilePath != "new/a" || ns.stack[1].FilePath != "new/b" {
		t.Fatal("atomic rename")
	}
	ns.Clear()
	ns.Clear()
	if ns.Len() != 0 || ns.CanGoBack() || ns.CanGoForward() || ns.stack != nil || len(events) != 7 {
		t.Fatal("clear", events)
	}
	ns.RecordJump(a, b)
	if len(events) != 8 {
		t.Fatal("clear detached subscriptions")
	}
	stop()
	stop()
	ns.Clear()
	if len(events) != 8 || len(ns.observers) != 0 {
		t.Fatal("unsubscribe retained listener")
	}
	ns.SubscribeChanged(nil)()
}

func TestNavigationChangedObserverMembershipDuringDelivery(t *testing.T) {
	var ns NavigationStack
	var calls []string
	var first, second, third func()
	first = ns.SubscribeChanged(func() {
		calls = append(calls, "first")
		first()
		second()
		third = ns.SubscribeChanged(func() { calls = append(calls, "third") })
	})
	second = ns.SubscribeChanged(func() { calls = append(calls, "removed") })
	last := ns.SubscribeChanged(func() { calls = append(calls, "last") })
	ns.Push(NavPosition{FilePath: "a"})
	if !reflect.DeepEqual(calls, []string{"first", "last"}) {
		t.Fatal("changed in-flight membership", calls)
	}
	calls = nil
	ns.Push(NavPosition{FilePath: "b"})
	if !reflect.DeepEqual(calls, []string{"last", "third"}) {
		t.Fatal("next event membership", calls)
	}
	last()
	third()
	if ns.observers != nil {
		t.Fatal("listeners retained")
	}
}

func TestNavigationHistoryBoundAndDiscardedPaths(t *testing.T) {
	var ns NavigationStack
	events := 0
	defer ns.SubscribeChanged(func() { events++ })()
	for i := 0; i < 1024; i++ {
		ns.Push(NavPosition{FilePath: "f", Line: i})
	}
	if ns.Len() != 256 || ns.stack[0].Line != 768 || ns.stack[255].Line != 1023 || events != 1024 {
		t.Fatal("unbounded/corrupt history")
	}
	ns.GoBack()
	ns.GoBack()
	ns.GoBack()
	backing := ns.stack[:cap(ns.stack)]
	ns.Push(NavPosition{FilePath: "branch", Line: 1})
	if ns.CanGoForward() || ns.Len() != 254 || backing[254] != (NavPosition{}) || backing[255] != (NavPosition{}) {
		t.Fatal("retained forward paths")
	}
	ns.Clear()
	if ns.stack != nil {
		t.Fatal("clear retains positions")
	}
}

func TestNavigationHistoryNotificationsDoNotAllocate(t *testing.T) {
	var ns NavigationStack
	for i := 0; i < 256; i++ {
		ns.Push(NavPosition{FilePath: "f", Line: i})
	}
	calls := 0
	defer ns.SubscribeChanged(func() { calls++ })()
	line := 256
	if allocs := testing.AllocsPerRun(1000, func() { line++; ns.Push(NavPosition{FilePath: "f", Line: line}) }); allocs != 0 {
		t.Fatalf("history update allocates: %v", allocs)
	}
	if calls == 0 {
		t.Fatal("observer did not run")
	}
}

func BenchmarkNavigationHistoryObservedPush(b *testing.B) {
	var ns NavigationStack
	for i := 0; i < 256; i++ {
		ns.Push(NavPosition{FilePath: "f", Line: i})
	}
	defer ns.SubscribeChanged(func() { _ = ns.CanGoBack() })()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ns.Push(NavPosition{FilePath: "f", Line: i + 256})
	}
}
