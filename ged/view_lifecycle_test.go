package ged

import "testing"

func TestViewCloseSubscriptionsRetireBeforeLegacyAndRejectReentry(t *testing.T) {
	v := NewGedView()
	old := DocClosedCallback
	defer func() { DocClosedCallback = old }()
	legacy, calls := 0, 0
	DocClosedCallback = func(got *GedView) {
		legacy++
		if got != v || !v.IsClosed() || calls != 1 {
			t.Error("wrong close ordering")
		}
	}
	var sibling func()
	stop := v.SubscribeClosed(func() {
		calls++
		sibling()
		v.SubscribeClosed(func() { t.Error("late observer fired") })()
		v.Close()
	})
	sibling = v.SubscribeClosed(func() { t.Error("canceled sibling fired") })
	held := v.closeSubscriptions[0]
	v.Close()
	v.Close()
	stop()
	stop()
	sibling()
	if legacy != 1 || calls != 1 || held.view != nil || held.fn != nil || len(v.closeSubscriptions) != 0 {
		t.Fatal("close retained owner or repeated callbacks")
	}
}

func TestViewCanceledCloseHandleDoesNotRetainOwner(t *testing.T) {
	v := NewGedView()
	stop := v.SubscribeClosed(func() { t.Error("canceled observer fired") })
	held := v.closeSubscriptions[0]
	stop()
	stop()
	if held.view != nil || held.fn != nil || len(v.closeSubscriptions) != 0 {
		t.Fatal("held cancellation handle retained view/callback")
	}
	var absent *GedView
	if !absent.IsClosed() {
		t.Fatal("nil view considered open")
	}
	absent.SubscribeClosed(func() { t.Error("nil view callback") })()
	v.SubscribeClosed(nil)()
	v.Close()
}
