package gui

import (
	"reflect"
	"testing"
)

func watchTabChanges(t *testing.T, tabs *TabWidget) *[]int {
	t.Helper()
	var got []int
	tabs.SetCurrentChangedCallback(func(_ interface{}, index int) {
		got = append(got, index)
		if index != tabs.CurrentIndex() || index != tabs.Stack().CurrentIndex() || tabs.Count() != tabs.Stack().Count() || tabs.Count() != tabs.TabBar().Count() {
			t.Fatalf("incoherent callback: %d bar=%d stack=%d counts=%d/%d/%d", index, tabs.CurrentIndex(), tabs.Stack().CurrentIndex(), tabs.Count(), tabs.Stack().Count(), tabs.TabBar().Count())
		}
		if index >= 0 {
			ref, ok := tabs.TabBar().Data(index).(*tabPageRef)
			if !ok || ref.page != tabs.Stack().CurrentPage() || ref.title != tabs.TabTitle(index) {
				t.Fatal("callback sees stale page metadata")
			}
		}
	})
	return &got
}

func TestTabCurrentChangedExactlyOnceAndNoOp(t *testing.T) {
	tabs := newTabWidget3()
	got := watchTabChanges(t, tabs)
	tabs.SetCurrentIndex(1)
	tabs.SetCurrentIndex(1)
	tabs.SetCurrentIndex(-1)
	tabs.SetCurrentIndex(99)
	tabs.TabBar().SetActiveTab(2)
	if !reflect.DeepEqual(*got, []int{1, 2}) {
		t.Fatalf("changes %v, want [1 2]", *got)
	}
}
func TestTabCurrentChangedStructuralMutationsAreCoherent(t *testing.T) {
	tabs := NewTabWidget()
	got := watchTabChanges(t, tabs)
	tabs.AddTab(NewEdit(), "A", nil)
	tabs.AddTab(NewEdit(), "B", nil)
	tabs.AddTab(NewEdit(), "C", nil)
	tabs.SetCurrentIndex(1)
	tabs.RemoveTab(0)
	tabs.RemoveTab(0)
	tabs.RemoveTab(0)
	if len(*got) != 5 || (*got)[0] != 0 || (*got)[1] != 1 || (*got)[len(*got)-1] != -1 {
		t.Fatalf("first/add/remove/last notifications: %v", *got)
	}
}
func TestTabCurrentChangedReentrantSelectionWins(t *testing.T) {
	tabs := newTabWidget3()
	var got []int
	tabs.SetCurrentChangedCallback(func(_ interface{}, idx int) {
		got = append(got, idx)
		if idx == 1 {
			tabs.SetCurrentIndex(2)
		}
	})
	tabs.SetCurrentIndex(1)
	if tabs.CurrentIndex() != 2 || tabs.Stack().CurrentIndex() != 2 || !reflect.DeepEqual(got, []int{1, 2}) {
		t.Fatalf("reentrant selection overwritten: bar=%d stack=%d events=%v", tabs.CurrentIndex(), tabs.Stack().CurrentIndex(), got)
	}
}

func TestTabCurrentChangedReorderPublishesOnlyFinalIdentity(t *testing.T) {
	oldFocus := focusWidget
	t.Cleanup(func() { focusWidget = oldFocus })
	tabs := newTabWidgetABC()
	got := watchTabChanges(t, tabs)
	dragTab(t, tabs.TabBar(), 0, dropPointAt(t, tabs.TabBar(), 3))
	if !reflect.DeepEqual(*got, []int{2}) || currentPageText(tabs) != "pageA" {
		t.Fatalf("reorder emitted intermediate pages: %v", *got)
	}
	dragTab(t, tabs.TabBar(), 2, dropPointAt(t, tabs.TabBar(), 3))
	if len(*got) != 1 {
		t.Fatal("same-place drag notified")
	}
}
func TestTabCurrentChangedRemoveUnrelatedAndReentrantClose(t *testing.T) {
	tabs := newTabWidget3()
	got := watchTabChanges(t, tabs)
	tabs.RemoveTab(2)
	if len(*got) != 0 {
		t.Fatal("removing an inactive later tab changed selection")
	}
	tabs.SetCurrentChangedCallback(func(_ interface{}, idx int) {
		if idx == 1 {
			tabs.RemoveTab(idx)
		}
	})
	tabs.SetCurrentIndex(1)
	if tabs.Count() != 1 || tabs.CurrentIndex() != 0 || tabs.Stack().CurrentIndex() != 0 {
		t.Fatal("reentrant removal corrupted selection")
	}
}

func TestTabNavigateTransfersOnlyValidFocus(t *testing.T) {
	old := focusWidget
	t.Cleanup(func() { focusWidget = old })
	tabs := NewTabWidget()
	tabs.SetSize(600, 300)
	a := NewEdit()
	page := NewVBox()
	disabled := NewVBox()
	disabled.SetParent(page)
	disabled.SetEnabled(false)
	hiddenEdit := NewEdit()
	hiddenEdit.SetParent(disabled)
	b := NewEdit()
	b.SetParent(page)
	tabs.AddTab(a, "A", nil)
	tabs.AddTab(page, "B", nil)
	a.SetFocus()
	if !tabs.NavigateTab(false) || focusWidget != b {
		t.Fatalf("page child not focused: %T", focusWidget)
	}
	if !tabs.NavigateTab(false) || focusWidget != a {
		t.Fatal("forward wrap failed")
	}
	tabs.SetVisible(false)
	if tabs.NavigateTab(true) || tabs.CurrentIndex() != 0 {
		t.Fatal("hidden group navigated")
	}
	tabs.SetVisible(true)
	tabs.SetEnabled(false)
	if tabs.NavigateTab(true) {
		t.Fatal("disabled group navigated")
	}
	tabs.SetEnabled(true)
	elsewhere := NewEdit()
	tabs.SetCurrentChangedCallback(func(_ interface{}, idx int) { elsewhere.SetFocus() })
	tabs.NavigateTab(false)
	if focusWidget != elsewhere {
		t.Fatal("callback focus overwritten")
	}
	tabs.SetCurrentChangedCallback(nil)
	tabs.SetCurrentIndex(0)
	a.SetFocus()
	tabs.SetCurrentChangedCallback(func(_ interface{}, idx int) {
		if idx == 1 {
			tabs.RemoveTab(idx)
		}
	})
	tabs.NavigateTab(false)
	if focusWidget == b || focusWidget == page {
		t.Fatal("removed page focused")
	}
	if tabs.NavigateTab(false) {
		t.Fatal("single page navigated")
	}
}
func BenchmarkTabNavigateFocusedPage(b *testing.B) {
	old := focusWidget
	defer func() { focusWidget = old }()
	tabs := NewTabWidget()
	tabs.SetSize(600, 300)
	for _, name := range []string{"A", "B", "C"} {
		tabs.AddTab(NewEdit(), name, nil)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tabs.NavigateTab(false)
	}
}
