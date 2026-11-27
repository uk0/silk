package graph

// MoveSiblingToIndex moves an attached item to an absolute sibling index
// without detaching it or replacing its identity. Invalid targets and moves
// back to the same slot are no-ops. Callers record the operation in history
// when needed; direct z-order operations likewise do not push their own.
func MoveSiblingToIndex(item IItem, index int) bool {
	if item == nil || index < 0 {
		return false
	}
	self := item.NakedItem()
	if self.parent == nil || self.next == self {
		return false
	}
	p := self.parent.NakedItem()
	from := -1
	var at *Item
	cursor := p.child
	for i := 0; i == 0 || cursor != p.child; i++ {
		if cursor == self {
			from = i
		}
		if i == index {
			at = cursor
		}
		cursor = cursor.next
	}
	if from < 0 || at == nil || from == index {
		return false
	}
	if from < index {
		at = at.next
	}
	self.zorderUnlink()
	if at == self {
		at = p.child
	} // former head is moving after the old tail
	self.zorderInsertBefore(at)
	if index == 0 {
		p.child = self
	}
	return true
}
