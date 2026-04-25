package graph

import (
	"silk/paint"
)

// 图元树的遍历条件.
// 在绘图系统中, 查找/绘制/单选/框选等操作都涉及到图元的遍历,
// 相关的方法都接受一个TraversalCond参数, 用来筛选出满足条件的图元.
// TraversalCond需要提供两个返回值:
//    self : 当前结点是否满足要求
//    descendants : 是否需要遍历当前结点的子孙结点
type TraversalCond func(IItem) (self, descendants bool)

func TraversalCond_Selectable(p IItem) (self, descendants bool) {
	self = p.IsVisible() && p.IsSelectable()
	descendants = p.IsVisible()
	return
}

func TraversalCond_SelectableAndMoveable(p IItem) (self, descendants bool) {
	self = p.IsVisible() && p.IsSelectable() && !p.IsLockPos()
	descendants = p.IsVisible()
	return
}

func LoadIcon(s string) paint.Icon {
	return paint.LoadIcon(s)
}
