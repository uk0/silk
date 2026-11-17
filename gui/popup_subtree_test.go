package gui

import "testing"

type inlinePopupProbe struct {
	Widget
	calls int
	hide  func()
}

func newInlinePopupProbe(parent IWidget) *inlinePopupProbe {
	p := new(inlinePopupProbe)
	p.Init(p)
	p.SetParent(parent)
	return p
}
func (p *inlinePopupProbe) HideSubPopup() {
	p.calls++
	if p.hide != nil {
		p.hide()
	}
}

func TestDismissInlinePopupsNativeOwnershipAndSpill(t *testing.T) {
	root := newInlinePopupProbe(nil)
	var inline []*inlinePopupProbe
	for i := 0; i < 40; i++ {
		inline = append(inline, newInlinePopupProbe(root))
	}
	form, popup, child := newInlinePopupProbe(root), newInlinePopupProbe(root), newInlinePopupProbe(root)
	insideForm, insidePopup, insideChild := newInlinePopupProbe(form), newInlinePopupProbe(popup), newInlinePopupProbe(child)
	// Wire first, then describe ownership without creating any native windows.
	form.wt, popup.wt, child.wt = WtForm, WtPopup, WtChild
	t.Cleanup(func() { form.wt, popup.wt, child.wt = WtInherit, WtInherit, WtInherit })
	DismissInlinePopups(root)
	for _, p := range append(inline, root, child, insideChild) {
		if p.calls != 1 {
			t.Fatal("owned inline popup not retired", p.calls)
		}
	}
	for _, p := range []*inlinePopupProbe{form, popup, insideForm, insidePopup} {
		if p.calls != 0 {
			t.Fatal("independent native ownership was crossed")
		}
	}
	DismissInlinePopups(nil)
}

func TestDismissInlinePopupSnapshotRevalidatesCallbacks(t *testing.T) {
	for _, mutation := range []string{"detach", "reparent-branch", "native-boundary"} {
		t.Run(mutation, func(t *testing.T) {
			root, other := newInlinePopupProbe(nil), newInlinePopupProbe(nil)
			branch := newInlinePopupProbe(root)
			leaf := newInlinePopupProbe(branch)
			var newOwner *inlinePopupProbe
			root.hide = func() {
				newOwner = newInlinePopupProbe(root)
				switch mutation {
				case "detach":
					branch.SetParent(nil)
				case "reparent-branch":
					branch.SetParent(other)
				case "native-boundary":
					branch.wt = WtPopup
				}
			}
			t.Cleanup(func() { branch.wt = WtInherit })
			DismissInlinePopups(root)
			if root.calls != 1 || branch.calls != 0 || leaf.calls != 0 || newOwner.calls != 0 || other.calls != 0 {
				t.Fatal("stale snapshot invoked foreign or new popup owners")
			}
		})
	}
}

func TestDismissInlinePopupSmallSnapshotDoesNotAllocate(t *testing.T) {
	root := newInlinePopupProbe(nil)
	for i := 0; i < 12; i++ {
		newInlinePopupProbe(root)
	}
	if got := testing.AllocsPerRun(100, func() { DismissInlinePopups(root) }); got != 0 {
		t.Fatalf("small retirement snapshot allocated %v", got)
	}
}

func TestDismissInlinePopupsReentryIsOnePass(t *testing.T) {
	root := newInlinePopupProbe(nil)
	child := newInlinePopupProbe(root)
	root.hide = func() { DismissInlinePopups(root) }
	DismissInlinePopups(root)
	if root.calls != 1 || child.calls != 1 || inlinePopupPassDepth != 0 {
		t.Fatal("reentry repeated an owner", root.calls, child.calls)
	}
	DismissInlinePopups(root)
	if root.calls != 2 || child.calls != 2 {
		t.Fatal("later retirement never admitted old owners")
	}
}

func TestDismissInlinePopupPanicDoesNotPoisonNextPass(t *testing.T) {
	root := newInlinePopupProbe(nil)
	root.hide = func() { panic("host failure") }
	func() {
		defer func() {
			if recover() == nil {
				t.Error("host panic swallowed")
			}
		}()
		DismissInlinePopups(root)
	}()
	if inlinePopupPassDepth != 0 {
		t.Fatal("retirement pass stayed active after panic")
	}
	root.hide = nil
	DismissInlinePopups(root)
	if root.calls != 2 {
		t.Fatal("panic poisoned later retirement")
	}
}
