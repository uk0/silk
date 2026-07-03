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
		x, y := item.Pos()
		cmd.AddItem(item, x+dx, y+dy)

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
