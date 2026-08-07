package graph

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
)

type SelectNode struct {
	next  *SelectNode
	prev  *SelectNode
	item  IItem
	decor IDecor
}

func (p *SelectNode) OnDraw(g paint.Painter) {

	tx, ty := p.item.CoordOffset()
	g.Translate(tx, ty)
	if p.decor != nil {
		p.decor.OnDraw(g)
	} else {
		p.item.DrawOutline(g)
	}
	g.Translate(-tx, -ty)

	//// 没有Decor, 默认画一个蓝色框
	//rect := p.item.Bounds1()
	//rect = p.item.MapRectToScene1(rect)

	//g.SetPen1(paint.Color{0, 127, 255, 255}, 0)
	//g.Rectangle1(rect)
	//g.Stroke()
}

func newSelection(view *GraphView) *Selection {
	s := new(Selection)
	s.view = view
	s.index = make(map[IItem]*SelectNode)
	return s
}

type Selection struct {
	view  *GraphView
	first *SelectNode
	last  *SelectNode
	index map[IItem]*SelectNode
	count int
}

func (s *Selection) findNode(a IItem) *SelectNode {
	p, _ := s.index[a]
	return p
}

//type _IEventSelectItem interface {
//	OnSelectItem(IItem)
//}

//type _IEventDeselectItem interface {
//	OnDeselectItem(IItem)
//}

//type _IEventSelect interface {
//	OnSelect()
//}

//type _IEventDeselect interface {
//	OnDeselect()
//}

func (s *Selection) AddMulti(v []IItem) {
	for _, a := range v {
		s.Add(a)
	}
}

func (s *Selection) Add(a IItem) {
	if a == nil {
		//core.Warn("add null item to selections.")
		return
	}
	if s.first == nil {
		p := new(SelectNode)
		p.item = a
		s.first = p
		s.last = p
		s.index[a] = p
	} else {
		p, ok := s.index[a]
		if ok {
			return
		}

		p = new(SelectNode)
		p.item = a
		if s.first == nil {
			s.first = p
			s.last = p

		} else {
			p.prev = s.last
			s.last.next = p
			s.last = p
		}

		s.index[a] = p
	}

	//	p.next = nil

	s.count++
	//	a.setSelected(true)

	//	a.(_IEventSelect).OnSelect()
	s.view.emitItemSelected(a)

}

func (s *Selection) RemoveMulti(v []IItem) {
	for _, a := range v {
		s.Remove(a)
	}
}

func (s *Selection) Remove(a IItem) {
	if a == nil {
		//core.Warn("remove null item from selections")
		return
	}
	p, ok := s.index[a]
	if !ok {
		return
	}
	if p.prev == nil {
		s.first = p.next
	} else {
		p.prev.next = p.next
	}

	if p.next == nil {
		s.last = p.prev
	} else {
		p.next.prev = p.prev
	}
	delete(s.index, a)
	s.count--

	//a.setSelected(false)

	//	a.(_IEventDeselect).OnDeselect()
	s.view.emitItemDeselected(a)

}

func (s *Selection) InvertMulti(v []IItem) {
	for _, a := range v {
		s.Invert(a)
	}
}

func (s *Selection) Invert(a IItem) {
	if s.Contains(a) {
		s.Remove(a)
	} else {
		s.Add(a)
	}
}

func (s *Selection) Contains(a IItem) bool {
	_, ok := s.index[a]
	return ok
}

func (s *Selection) ItemList() (ret []IItem) {
	for p := s.first; p != nil; p = p.next {
		ret = append(ret, p.item)
	}
	return
}

func (s *Selection) Count() int {
	return s.count
}

func (s *Selection) IsEmpty() bool {
	return s.first == nil
}

func (s *Selection) IsSingle() bool {
	return s.first != nil && s.first == s.last
}

func (s *Selection) Clear() {
	for _, a := range s.ItemList() {
		s.Remove(a)
	}
}

func (s *Selection) DebugDump() {
	if s.IsEmpty() {
		core.Debug("Selection: <nothing>")
		return
	}

	if s.IsSingle() {
		core.Debug("Selection: 1 item")
	} else {
		core.Debug("Selection: ", s.Count(), " items")

	}

	for i, a := range s.ItemList() {
		if i == 6 {
			core.Debug("    -> ...")
			break
		}
		core.Debug("    -> ", a.DebugPathString(nil))
	}
}

func (s *Selection) OnDraw(g paint.Painter) {
	for p := s.first; p != nil; p = p.next {
		p.OnDraw(g)
	}
}

func (s *Selection) isItemAncestorSelected(item IItem) bool {
	for p := item.Parent(); p != nil; p = p.Parent() {
		if s.Contains(p) {
			return true
		}
	}
	return false
}
func (s *Selection) GenerateMoveCommand(dx, dy float64) *MoveCommand {
	if dx == 0 && dy == 0 {
		return nil
	}

	if s.IsEmpty() {
		return nil
	}

	cmd := NewMoveCommand()
	for p := s.first; p != nil; p = p.next {
		item := p.item
		//if item.Parent() == nil{
		//	continue
		//}
		if item.IsLockPos() {
			continue
		}

		if s.isItemAncestorSelected(item) {
			continue
		}
		AddMoveSubtree(cmd, item, dx, dy)

	}
	if cmd.Count() == 0 {
		return nil
	}
	return cmd
}

// AddMoveSubtree records item at its shifted position, plus every descendant
// that keeps its own scene coordinates. A container does not carry its children
// on its own — SetPos moves the one item and OnMove only re-lays it out — so a
// tree whose coordinates are absolute (ged never turns on local coords) would
// leave the children sitting where they were.
//
// HasLocalCoord is the boundary: below such an item the children are stored
// relative to it and already ride along, so descending past it would move them
// a second time.
//
// A descendant rides along whatever its own IsLockPos says: the lock pins a
// widget inside its container, and moving the container is not a request to
// rearrange what is in it. Only the selected item's own lock, tested by the
// caller, keeps a move out.
//
// Exported for the callers that build their own MoveCommand because each item
// has its own target rather than one shared delta — ged's multi-item align.
func AddMoveSubtree(cmd *MoveCommand, item IItem, dx, dy float64) {
	moveSubtree(item, dx, dy, func(a IItem, x, y float64) {
		// item is the one with a target; everything under it was handed the
		// same delta, so it has to take item's grid snap as well or the two
		// round apart (moveRecord.carried).
		if a == item {
			cmd.AddItem(a, x, y)
			return
		}
		cmd.addCarried(a, x, y)
	})
}

// ShiftSubtree moves item by (dx, dy) right now, carrying exactly the
// descendants AddMoveSubtree would have recorded. It is for the corrections
// that are applied outside the undo stack — ged's post-drag grid snap tidies
// the resting place of a move that is already committed — which still have to
// take the subtree with them.
func ShiftSubtree(item IItem, dx, dy float64) {
	moveSubtree(item, dx, dy, func(a IItem, x, y float64) {
		a.SetPos(x, y)
	})
}

// moveSubtree walks the carried set and hands each item its shifted position,
// so the boundary rule documented on AddMoveSubtree has one implementation and
// the recorded and the applied form cannot drift apart.
func moveSubtree(item IItem, dx, dy float64, apply func(a IItem, x, y float64)) {
	x, y := item.Pos()
	apply(item, x+dx, y+dy)
	if item.HasLocalCoord() {
		return
	}
	for _, c := range item.Children() {
		moveSubtree(c, dx, dy, apply)
	}
}

// isSelfOrDescendant reports whether a is root itself or sits anywhere below
// it, by walking a's parent chain up to the tree root.
func isSelfOrDescendant(a, root IItem) bool {
	for p := a; p != nil; p = p.Parent() {
		if p == root {
			return true
		}
	}
	return false
}

// GenerateReparentCommand hands every selected item to newParent, keeping each
// item where it is on screen (TranslateBetweenParents), and returns one command
// whose Undo restores the original parent, sibling index and position. Returns
// nil when nothing would actually change, so a caller can forward that as a
// quiet no-op instead of pushing an empty command — same contract as
// GenerateMoveCommand.
//
// Skipped, and why:
//   - an item whose ancestor is also selected: the ancestor carries it along,
//     exactly as GenerateMoveCommand skips it;
//   - an IsLockPos item: a drag leaves it behind (GenerateMoveCommand skips it
//     too), so handing it a new owner would split one gesture in two;
//   - an item already parented to newParent: SetParentAt is a documented no-op
//     for it, and recording no-ops would make Count() claim work that is not
//     there;
//   - a newParent inside the item's own subtree: SetParentAt has no cycle
//     guard, and a cyclic child ring hangs every later traversal.
func (s *Selection) GenerateReparentCommand(newParent IItem) *ReparentCommand {
	if newParent == nil || s.IsEmpty() {
		return nil
	}
	// Items store the Self() form of their parent; normalise so the
	// "already there" test below compares like with like.
	newParent = newParent.Self()

	cmd := NewReparentCommand("")
	for p := s.first; p != nil; p = p.next {
		item := p.item
		if item.IsLockPos() {
			continue
		}

		if s.isItemAncestorSelected(item) {
			continue
		}
		oldParent := item.Parent()
		if oldParent == newParent || isSelfOrDescendant(newParent, item) {
			continue
		}
		x, y := item.Pos()
		x1, y1 := TranslateBetweenParents(oldParent, newParent, x, y)
		cmd.AddMoved(item, oldParent, newParent, x1, y1)

	}
	if cmd.Count() == 0 {
		return nil
	}
	return cmd
}

// GenerateResizeCommand grows every selected item by (dw, dh) millimetres,
// anchored at each item's top-left corner and floored at minSize by
// resizeRectBy. Returns nil when nothing would actually change, so a caller
// can forward that as a quiet no-op instead of pushing an empty command.
//
// Unlike the drag-handle path in ResizeDecor, an IsLockPos item is kept here:
// the top-left anchor means resizing never moves it, so a position lock has
// nothing to violate. Only IsLockSize opts an item out.
func (s *Selection) GenerateResizeCommand(dw, dh, minSize float64) *ResizeCommand {
	if dw == 0 && dh == 0 {
		return nil
	}

	if s.IsEmpty() {
		return nil
	}

	cmd := NewResizeCommand()
	for p := s.first; p != nil; p = p.next {
		item := p.item
		if item.IsLockSize() {
			continue
		}

		if s.isItemAncestorSelected(item) {
			continue
		}
		rect := item.Bounds1()
		newRect := resizeRectBy(rect, dw, dh, minSize)
		if newRect == rect {
			continue
		}
		cmd.AddItem(item, newRect)

	}
	if cmd.Count() == 0 {
		return nil
	}
	return cmd
}

// GenerateSameSizeCommand resizes every selected item onto the width and/or
// height of the LAST selected item, each item keeping its own X/Y. Add appends,
// so the last item is the one the designer picked most recently — the reference
// convention every form designer uses for "make same size".
//
// Same exclusions and same nil contract as GenerateResizeCommand: a size-locked
// or ancestor-selected item is left out, and nothing left to do returns nil so
// the caller forwards a quiet no-op instead of pushing an empty command. The
// reference needs no special case — resizing it onto its own size is not a
// change, so the unchanged-rect filter drops it.
func (s *Selection) GenerateSameSizeCommand(mode SameSizeMode, minSize float64) *ResizeCommand {
	if s.Count() < 2 {
		return nil
	}

	ref := s.last.item.Bounds1()
	cmd := NewResizeCommand()
	for p := s.first; p != nil; p = p.next {
		item := p.item
		if item.IsLockSize() {
			continue
		}

		if s.isItemAncestorSelected(item) {
			continue
		}
		rect := item.Bounds1()
		newRect := sameSizeRect(rect, ref, mode, minSize)
		if newRect == rect {
			continue
		}
		cmd.AddItem(item, newRect)

	}
	if cmd.Count() == 0 {
		return nil
	}
	return cmd
}

func (s *Selection) FindHandleAt(xMm, yMm float64) (decor IDecor, handle int) {
	// 和绘图方向相反, 从后往前找
	for p := s.last; p != nil; p = p.prev {
		if p.decor == nil {
			continue
		}
		dx, dy := p.item.CoordOffset()
		x, y := xMm-dx, yMm-dy
		h := p.decor.HandleAt(x, y)
		if h != 0 {
			decor = p.decor
			handle = h
			return
		}
	}
	return
}
