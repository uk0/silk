package gui

// setContentSlot changes a single managed child without modifying unregistered
// children. Attach first so SetParent can reject a cycle without discarding the
// previous content. Resetting the same widget does not alter its paint order.
func setContentSlot(owner IWidget, slot *IWidget, next IWidget) bool {
	if next != nil {
		next = next.Self()
	}
	if *slot == next {
		return false
	}
	if next != nil {
		next.SetParent(owner)
		if next.Parent() != owner {
			return false
		}
	}
	old := *slot
	*slot = next
	if old != nil && old != next && old.Parent() == owner {
		old.SetParent(nil)
	}
	return true
}
