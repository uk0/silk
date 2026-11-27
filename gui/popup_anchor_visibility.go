package gui

// dismissAnchoredPopups retires open popup trees when a logical (non-native)
// container is hidden. Hiding a native window already follows its own
// platform dismissal path. Neither path crosses into an independent form.
func dismissAnchoredPopups(root IWidget) {
	if root == nil || root.Window() != nil {
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
		if root.IsVisible() {
			return // a hide callback made the original scope visible again
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
