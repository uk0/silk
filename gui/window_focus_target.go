package gui

// Do not revive a global focus saved in an unrelated Form. In particular,
// opening a context menu in Preview must never restore the IDE's editor.
func focusTargetForOwner(owner *Window, candidate IWidget) IWidget {
	if owner == nil || owner.widget == nil {
		return nil
	}
	if candidate != nil && candidate.OwnerWindow() == owner {
		valid := true
		for p := candidate; p != nil; p = p.Parent() {
			if !p.IsVisible() || !p.IsEnabled() {
				valid = false
				break
			}
		}
		if valid {
			return candidate
		}
	}
	return owner.widget
}
