package gui

import "testing"

func editListFixture(t *testing.T) (*Edit, *ListWidget, *VBox) {
	t.Helper()
	box := NewVBox()
	e := NewEdit()
	list := NewListWidget()
	box.AddWidget(e)
	box.AddWidget(list)
	e.SetNavigationList(list)
	list.SetSize(300, 180)
	for i := 0; i < 30; i++ {
		list.Append(ListItem{Text: "result", Data: i})
	}
	list.SetActiveIndex(0)
	old := focusWidget
	e.SetFocus()
	t.Cleanup(func() { e.SetNavigationList(nil); focusWidget = old })
	return e, list, box
}
func TestEditNavigationListKeepsQuerySelectionFocusAndSignals(t *testing.T) {
	e, list, _ := editListFixture(t)
	e.SetText("中🙂 query")
	e.SetSelection(7, 2)
	a, b := e.Selection()
	caret := e.CaretPos()
	changed, selected, submitted := 0, 0, 0
	e.SigTextChanged(func(interface{}, string) { changed++ })
	list.SigSelectionChanged(func(interface{}, []int) { selected++ })
	list.SigSubmit(func(interface{}) { submitted++ })
	e.OnKeyDown(KeyDown, false)
	e.OnKeyDown(KeyDown, true)
	if list.ActiveIndex() != 2 || selected != 2 || submitted != 0 {
		t.Fatal("navigation", list.ActiveIndex(), selected, submitted)
	}
	e.OnKeyDown(KeyPageDown, false)
	if list.ActiveIndex() != pageStepIndex(2, list.Count(), list.rowsPerPage(), 1) || list.ScrollY() <= 0 {
		t.Fatal("page reveal")
	}
	e.OnKeyDown(KeyPageUp, false)
	if list.ActiveIndex() != 2 {
		t.Fatal("page back")
	}
	c, d := e.Selection()
	if changed != 0 || c != a || d != b || e.CaretPos() != caret || !e.HasFocus() || list.HasFocus() {
		t.Fatal("navigation edited query/selection/focus")
	}
	e.SetReadOnly(true)
	e.OnKeyDown(KeyUp, false)
	if list.ActiveIndex() != 1 {
		t.Fatal("readonly navigation")
	}
	list.Clear()
	e.OnKeyDown(KeyDown, false)
	if list.ActiveIndex() != -1 || changed != 0 || submitted != 0 {
		t.Fatal("empty list")
	}
}
func TestEditNavigationListPreservesFieldKeysAndMultiline(t *testing.T) {
	e, list, _ := editListFixture(t)
	e.SetText("query")
	e.SetCaretPos(2)
	for _, key := range []int{KeyHome, KeyEnd, KeyLeft, KeyRight, KeyEnter, KeyTab, KeySpace, 'A', KeyBackSpace} {
		if e.navigateResultList(key, false, false) {
			t.Fatal("intercepted field key", key)
		}
	}
	for _, key := range []int{KeyUp, KeyDown, KeyPageUp, KeyPageDown} {
		if e.navigateResultList(key, true, false) {
			t.Fatal("intercepted modified key", key)
		}
	}
	e.OnKeyDown(KeyHome, false)
	if e.CaretPos() != 0 {
		t.Fatal("Home stopped editing query")
	}
	e.OnKeyDown(KeyEnd, false)
	if e.CaretPos() != 5 {
		t.Fatal("End stopped editing query")
	}
	e.SetMultiLine(true)
	e.SetText("one\ntwo")
	e.SetCaretPos(0)
	e.OnKeyDown(KeyDown, false)
	if e.CaretPos() == 0 || list.ActiveIndex() != 0 {
		t.Fatal("multiline down routed to results")
	}
	e.SetNavigationList(nil)
	if e.NavigationList() != nil || e.navigateResultList(KeyDown, false, false) {
		t.Fatal("binding retained")
	}
}
func TestEditNavigationListIgnoresUnavailableAndForeignOwners(t *testing.T) {
	for _, mode := range []string{"hidden-list", "disabled-list", "hidden-ancestor", "disabled-ancestor", "detached", "foreign-tree", "foreign-window", "disabled-input"} {
		t.Run(mode, func(t *testing.T) {
			e, list, box := editListFixture(t)
			switch mode {
			case "hidden-list":
				list.Hide()
			case "disabled-list":
				list.SetEnabled(false)
			case "hidden-ancestor":
				box.Hide()
			case "disabled-ancestor":
				box.SetEnabled(false)
			case "detached":
				list.SetParent(nil)
			case "foreign-tree":
				list.SetParent(NewVBox())
			case "foreign-window":
				list.win = &Window{widget: list}
				t.Cleanup(func() { list.win = nil })
			case "disabled-input":
				e.SetEnabled(false)
			}
			if e.navigateResultList(KeyDown, false, false) || list.ActiveIndex() != 0 {
				t.Fatal("navigated inaccessible results")
			}
		})
	}
}
func TestEditNavigationListNativeBoundaryAndReentrantSelection(t *testing.T) {
	e, list, box := editListFixture(t)
	hiddenOwner := NewVBox()
	hiddenOwner.Hide()
	box.SetParent(hiddenOwner)
	box.win = &Window{widget: box}
	t.Cleanup(func() { box.win = nil })
	if !e.navigateResultList(KeyDown, false, false) || list.ActiveIndex() != 1 {
		t.Fatal("visible native boundary inherited hidden owner")
	}
	list.SigSelectionChanged(func(interface{}, []int) { list.Clear(); e.SetNavigationList(nil) })
	e.OnKeyDown(KeyDown, false)
	if list.Count() != 0 || list.ActiveIndex() != -1 || e.NavigationList() != nil {
		t.Fatal("selection callback teardown")
	}
}
func BenchmarkEditResultNavigation(b *testing.B) {
	e := NewEdit()
	list := NewListWidget()
	box := NewVBox()
	box.AddWidget(e)
	box.AddWidget(list)
	e.SetNavigationList(list)
	list.SetSize(320, 200)
	for i := 0; i < 500; i++ {
		list.Append(ListItem{Text: "result"})
	}
	list.SetActiveIndex(0)
	e.OnKeyDown(KeyDown, false)
	e.OnKeyDown(KeyUp, false)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.OnKeyDown(KeyDown, false)
		e.OnKeyDown(KeyUp, false)
	}
}
