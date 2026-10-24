package gui

import "testing"

func TestSymbolObserversTrackRevisionAndDetach(t *testing.T) {
	e := NewCodeEditor()
	calls := 0
	cancel := e.SubscribeSymbolsChanged(func() { calls++; _ = e.TextRevision() })
	e.SetText("package p\n")
	if calls != 1 {
		t.Fatal(calls)
	}
	e.SetText(e.Text())
	e.MarkSaved()
	if calls != 1 {
		t.Fatal("no-op changed event", calls)
	}
	e.SetAdditionalSymbols(e.TextRevision(), []CodeSymbol{{Name: "Provider", Line: 0, Kind: SymMacro}})
	if calls != 2 {
		t.Fatal("provider event", calls)
	}
	e.filePath = "notes.txt"
	e.resetDocumentLanguage()
	if calls != 3 {
		t.Fatal("dialect event", calls)
	}
	cancel()
	cancel()
	e.SetText("other")
	if calls != 3 || len(e.symbolObservers) != 0 {
		t.Fatal("unsubscribe retained callback")
	}
	e.SubscribeSymbolsChanged(nil)()
}

func TestSymbolObserversCanUnsubscribeAndSubscribeDuringDelivery(t *testing.T) {
	e := NewCodeEditor()
	var order []int
	var stopSecond, stopThird func()
	stopFirst := e.SubscribeSymbolsChanged(func() {
		order = append(order, 1)
		stopSecond()
		if stopThird == nil {
			stopThird = e.SubscribeSymbolsChanged(func() { order = append(order, 3) })
		}
	})
	stopSecond = e.SubscribeSymbolsChanged(func() { order = append(order, 2) })
	e.SetText("first")
	if len(order) != 1 || order[0] != 1 {
		t.Fatal(order)
	}
	e.SetText("second")
	if len(order) != 3 || order[1] != 1 || order[2] != 3 {
		t.Fatal(order)
	}
	stopFirst()
	stopSecond()
	stopThird()
	if len(e.symbolObservers) != 0 {
		t.Fatal("retained consumer closures")
	}
}

func TestSymbolObserversNotifyAcceptedAsyncAndSyncTakeover(t *testing.T) {
	for _, syncTakeover := range []bool{false, true} {
		t.Run(map[bool]string{false: "async", true: "sync"}[syncTakeover], func(t *testing.T) {
			e, posts := asyncSymbolsEditor(t)
			calls := 0
			cancel := e.SubscribeSymbolsChanged(func() { calls++ })
			defer cancel()
			e.currentFunction()
			delivery := waitCompletionPost(t, posts)
			if calls != 0 {
				t.Fatal("notified before result")
			}
			if syncTakeover {
				e.ParseSymbols()
			}
			delivery()
			if calls != 1 {
				t.Fatal("publication was not exactly one notification", calls)
			}
			if got, ready := e.SymbolsSnapshot(); !ready || len(got) != 1 || got[0].Name != "TestAlpha" {
				t.Fatal(got, ready)
			}
		})
	}
}
