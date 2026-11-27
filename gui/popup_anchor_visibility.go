package gui

// dismissAnchoredPopups retires open popup trees when a logical (non-native)
// container is hidden. Hiding a native window already follows its own
// platform dismissal path. Neither path crosses into an independent form.
func dismissAnchoredPopups(root IWidget) {
	dismissAnchoredPopupsWhile(root, func() bool { return !root.IsVisible() })
}

func dismissReparentedPopups(root IWidget, oldOwner *Window) {
	dismissAnchoredPopupsWhile(root, func() bool {
		return root.Parent() == nil || root.OwnerWindow() != oldOwner || !WidgetTreeVisible(root)
	})
}

// Hold no child iterators during Hide: handlers may reparent widgets or restore
// visibility. Revalidate both the branch and each popup after those callbacks.
func dismissAnchoredPopupsWhile(root IWidget, invalidScope func() bool) {
	if root == nil || root.Window() != nil || !invalidScope() {
		return
	}
	var small [4]IWidget
	targets := small[:0]
	var visit func(IWidget)
	visit = func(parent IWidget) {
		parent.NakedWidget().eachChild(func(_ int, child IWidget) bool {
			switch child.NakedWidget().wt {
			case WtForm:
				return true // the parent link is ownership, not inline visibility
			case WtPopup:
				if child.IsVisible() {
					targets = append(targets, child)
					return true // normal Hide dismisses this popup's descendants
				}
			}
			visit(child)
			return true
		})
	}
	visit(root)
	for _, popup := range targets {
		if !invalidScope() {
			return
		}
		if !popup.IsVisible() {
			continue
		}
		// A preceding Hide callback may have moved a popup to a surviving
		// branch. Only retire windows still anchored in the hidden scope.
		for anchor := popup.Parent(); anchor != nil; anchor = anchor.Parent() {
			if anchor == root {
				popup.Hide()
				break
			}
			if anchor.NakedWidget().wt == WtForm {
				break
			}
		}
	}
}
