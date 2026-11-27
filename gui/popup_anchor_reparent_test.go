package gui

import "testing"

func TestInlineReparentRetiresOnlyPopupsThatLoseTheirOwner(t *testing.T) {
	for _, scenario := range []struct {
		name       string
		wantHidden bool
	}{
		{"no-op", false},
		{"same-visible-form", false},
		{"same-hidden-form", true},
		{"different-form", true},
		{"detach", true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			// Win/non-Win-independent ownership without creating native
			// windows from the Go test goroutine. Set these only after Init.
			first, second := NewForm(), NewForm()
			first.win = &Window{widget: first, wt: WtForm}
			second.win = &Window{widget: second, wt: WtForm}
			oldHost, visibleHost, hiddenHost := NewFrame(), NewFrame(), NewFrame()
			for _, host := range []*Frame{oldHost, visibleHost, hiddenHost} {
				host.SetParent(first)
			}
			hiddenHost.Hide()
			panel := NewFrame()
			panel.SetParent(oldHost)
			anchor := NewButton()
			anchor.SetParent(panel)
			popup := popupUnder(anchor)
			nested := popupUnder(popup)
			sibling := popupUnder(oldHost)
			independentForm := NewForm()
			independentForm.SetParent(panel)
			independentForm.wt = WtForm
			independentPopup := popupUnder(independentForm)

			destinations := map[string]IWidget{
				"no-op": oldHost, "same-visible-form": visibleHost, "same-hidden-form": hiddenHost,
				"different-form": second, "detach": nil,
			}
			panel.SetParent(destinations[scenario.name])
			count := 0
			if scenario.wantHidden {
				count = 1
			}
			if popup.IsVisible() == scenario.wantHidden || nested.IsVisible() == scenario.wantHidden ||
				popup.hides != count || nested.hides != count {
				t.Fatalf("anchor move: popup=%v/%d nested=%v/%d, expected hidden=%v",
					popup.IsVisible(), popup.hides, nested.IsVisible(), nested.hides, scenario.wantHidden)
			}
			if sibling.hides != 0 || independentPopup.hides != 0 || !independentForm.IsVisible() {
				t.Fatal("reparent crossed sibling or independent form boundary")
			}
			if popup.Parent() != anchor || nested.Parent() != popup || panel.Parent() != destinations[scenario.name] {
				t.Fatal("reparent discarded ownership or destroyed a reusable popup")
			}
			if scenario.wantHidden {
				panel.SetParent(visibleHost)
				if popup.IsVisible() || nested.IsVisible() {
					t.Fatal("reattach reopened an old popup automatically")
				}
			}
		})
	}
}

func TestReparentToHiddenHostPreservesPopupMovedByEarlierHideHandler(t *testing.T) {
	form := NewForm()
	form.win = &Window{widget: form, wt: WtForm}
	visible, hidden := NewFrame(), NewFrame()
	visible.SetParent(form)
	hidden.SetParent(form)
	hidden.Hide()
	panel := NewFrame()
	panel.SetParent(visible)
	first := popupUnder(panel)
	anchor := NewButton()
	anchor.SetParent(panel)
	survivor := popupUnder(anchor)
	first.hidden = func() { anchor.SetParent(visible) }

	panel.SetParent(hidden)

	if first.IsVisible() || first.hides != 1 {
		t.Fatal("first popup did not close when its panel moved into a hidden host")
	}
	if anchor.Parent() != visible || !survivor.IsVisible() || survivor.hides != 0 {
		t.Fatal("reparent dismissal used a stale snapshot for a now-visible branch")
	}
}
