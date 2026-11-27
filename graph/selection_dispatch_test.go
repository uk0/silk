package graph

import "testing"

func TestSelectionDispatchCoalescesToFinalSelectionWithoutIdle(t *testing.T) {
	v := NewView()
	view := &recordingPropView{}
	v.SetPropertyView(view)
	var got []IItem
	v.SigSelectionChanged(func(_ interface{}, selection *Selection) {
		got = selection.ItemList()
	})
	itemA, itemB := newTestItem(), newTestItem()
	var queue []func()
	post := func(run func()) { queue = append(queue, run) }
	current := func() bool { return true }
	v.Selection().Add(itemA)
	v.queueSelectionChanged(post, current)
	v.Selection().Remove(itemA)
	v.Selection().Add(itemB)
	v.queueSelectionChanged(post, current)
	if len(queue) != 1 || view.bindCalls != 0 {
		t.Fatalf("before dispatch: queue=%d binds=%d", len(queue), view.bindCalls)
	}
	queue[0]()
	if view.bindCalls != 1 || len(got) != 1 || got[0] != itemB {
		t.Fatalf("dispatch did not bind final selection once: binds=%d selected=%v", view.bindCalls, got)
	}
	v.OnIdle()
	if view.bindCalls != 1 {
		t.Fatal("idle redelivered the consumed change")
	}
}

func TestSelectionDispatchRejectsStaleOwnerAndDoesNotDoubleConsume(t *testing.T) {
	v := NewView()
	view := &recordingPropView{}
	v.SetPropertyView(view)
	var run func()
	post := func(fn func()) { run = fn }
	v.Selection().Add(newTestItem())
	v.queueSelectionChanged(post, func() bool { return false })
	run()
	if view.bindCalls != 0 || v.selectionChangedQueued {
		t.Fatalf("stale owner was notified or retained: binds=%d", view.bindCalls)
	}

	// A detached view may continue to run with a manually pumped idle loop.
	v.OnIdle()
	if view.bindCalls != 1 {
		t.Fatalf("manual idle fallback lost the change: binds=%d", view.bindCalls)
	}
	v.Selection().Clear()
	v.queueSelectionChanged(post, func() bool { return true })
	v.OnIdle()
	run()
	if view.bindCalls != 2 {
		t.Fatalf("idle then queued turn should consume only once: binds=%d", view.bindCalls)
	}
}
