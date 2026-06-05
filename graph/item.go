package graph

import (
	"silk/core"
	"silk/geom"
	"silk/gui"
	"silk/paint"
	"silk/prop"
	"fmt"
	"math"
	"reflect"
)

type IItem interface {
	Self() IItem
	NakedItem() *Item

	Parent() IItem
	SetParent(a IItem)
	Root() IItem
	Scene() IScene

	DrawAll(g paint.Painter)
	DrawSelf(g paint.Painter)
	DrawOutline(g paint.Painter)
	DrawChildren(g paint.Painter)

	X() float64
	Y() float64
	Width() float64
	Height() float64

	SetX(x float64)
	SetY(y float64)
	SetWidth(w float64)
	SetHeight(h float64)

	Pos() (x, y float64)
	SetPos(x, y float64)
	Size() (width, height float64)
	SetSize(width, height float64)
	Bounds() (x, y, width, height float64)
	Bounds1() geom.Rect
	SetBounds1(geom.Rect)
	SetBounds(x, y, width, height float64)

	EaveBounds() (x, y, width, height float64)
	EaveBounds1() geom.Rect

	IsVisible() bool
	SetVisible(bool)

	IsSelectable() bool
	SetSelectable(b bool)

	IsLockPos() bool
	SetLockPos(b bool)

	IsLockSize() bool
	SetLockSize(b bool)

	//IsSelected() bool
	//Select()
	//Deselect()

	Show()
	Hide()

	HasLocalCoord() bool
	SetLocalCoord(bool)

	HasChildren() bool
	Children() []IItem

	// slow
	IndexInParent() int

	IsHitBody(x, y float64) bool
	MaybeHitBody(x, y float64) bool

	// 从子树中查找指定坐标处第一个满足指定条件的图元.
	// 如果未指定条件, 则默认使用 TraversalCond_Selectable.
	// 如果有多个图元满足要求, 则上层图元优先.
	// 此方法通常用来实现点选/拖放等鼠标操作
	FindItemAt(x, y float64, cond TraversalCond) IItem

	// 从子树中查找指定坐标处所有满足指定条件的图元.
	// 如果未指定条件, 则默认使用 TraversalCond_Selectable.
	// 返回结果中上层图元排在前面, 和绘图顺序相反
	FindItemsAt(x, y float64, cond TraversalCond) []IItem

	// 从子树中查找指定矩形内所有满足指定条件的图元.
	// 如果未指定条件, 则默认使用 TraversalCond_Selectable.
	// 如果allIntersected为true, 则返回结果中也包含部分相交的图元
	// 返回结果中上层图元排在前面, 和绘图顺序相反
	// 相交/包含检测仅考虑图元的边框, 不考虑内部形状.
	FindItemsInRectF(x, y, width, height float64, cond TraversalCond) []IItem
	FindItemsInRect(bounds geom.Rect, cond TraversalCond) []IItem

	//setSelected(bool)

	// 获取从ancestor到当前结点的路径中的全部结点,
	// 如果路径存在, 则返回值不为空, 且ancestor在最前, 当前节点在最后.
	// 如果路径不存在, 则返回值为nil
	Path(ancestor IItem) []IItem

	Update()

	// 获取以当前节点为根的整棵子树的全部结点.
	// 返回的结点的排列顺序和绘图顺序相同(Pre-Order遍历)
	TreeItems() []IItem

	MapToChild(x, y float64) (x1, y1 float64)
	MapFromChild(x, y float64) (x1, y1 float64)
	MapToScene(x, y float64) (x1, y1 float64)
	MapFromScene(x, y float64) (x1, y1 float64)
	MapToItem(a IItem, x, y float64) (x1, y1 float64)
	MapFromItem(a IItem, x, y float64) (x1, y1 float64)

	MapRectToChild(x, y, width, height float64) (x1, y1, width1, height1 float64)
	MapRectFromChild(x, y, width, height float64) (x1, y1, width1, height1 float64)
	MapRectToScene(x, y, width, height float64) (x1, y1, width1, height1 float64)
	MapRectFromScene(x, y, width, height float64) (x1, y1, width1, height1 float64)
	MapRectToItem(a IItem, x, y, width, height float64) (x1, y1, width1, height1 float64)
	MapRectFromItem(a IItem, x, y, width, height float64) (x1, y1, width1, height1 float64)

	MapRectToChild1(rect geom.Rect) geom.Rect
	MapRectFromChild1(rect geom.Rect) geom.Rect
	MapRectToScene1(rect geom.Rect) geom.Rect
	MapRectFromScene1(rect geom.Rect) geom.Rect
	MapRectToItem1(a IItem, rect geom.Rect) geom.Rect
	MapRectFromItem1(a IItem, rect geom.Rect) geom.Rect

	CoordOffset() (dx, dy float64)

	GenDecors() []IDecor

	Icon() paint.Icon

	String() string

	Title() string

	// 获取字符串形式表示的从ancestor到当前结点的路径,
	// 此函数返回的值只用作调试信息
	DebugPathString(ancestor IItem) string
	DebugLabel() string
	DebugFlagsString() (ret string)

	//OnResize()
	Layout()
	SizeHints() gui.SizeHints

	Detach()

	// 调整此图元在兄弟图元中的堆叠顺序. 队首图元最先绘制(在最底层),
	// 队尾图元最后绘制(在最上层).
	Raise()
	Lower()
	BringToFront()
	SendToBack()
}

type Item struct {
	bounds geom.Rect
	self   IItem

	_root   IItem
	_offset *geom.Vec2

	hidden     bool
	localCoord bool
	selectable bool
	lockPos    bool
	lockSize   bool

	parent IItem
	next   *Item
	prev   *Item
	child  *Item

	eave float64

	dt *string
}

func (this *Item) HasChildren() bool {
	return this.child != nil
}

func (this *Item) Self() IItem {
	return this.self
}

func (this *Item) Init(ig IItem) {
	this.self = ig
	this.eave = 1
	this.selectable = true
	if core.IsDebugOn() {
		this.dt = core.LiveCycleTrace(ig)
	}
}

func (this *Item) X() float64 {
	return this.bounds.X
}

func (this *Item) Y() float64 {
	return this.bounds.Y
}

func (this *Item) Width() float64 {
	return this.bounds.Width
}

func (this *Item) Height() float64 {
	return this.bounds.Height
}

func (this *Item) SetX(x float64) {
	this.SetPos(x, this.Y())
}

func (this *Item) SetY(y float64) {
	this.SetPos(this.X(), y)
}

func (this *Item) SetWidth(w float64) {
	this.SetSize(w, this.Height())
}

func (this *Item) SetHeight(h float64) {
	this.SetSize(this.Width(), h)
}

func (this *Item) Pos() (x, y float64) {
	return this.bounds.X, this.bounds.Y
}

type _IMoveEvent interface {
	OnMove()
}

type _IResizeEvent interface {
	OnResize()
}

type _IEventHide interface {
	OnHide()
}

type _IEventShow interface {
	OnShow()
}

func (this *Item) SetPos(x, y float64) {
	if this.bounds.X != x || this.bounds.Y != y {
		this.Update()
		this.invalidateCoordOffset()
		this.bounds.X, this.bounds.Y = x, y
		if im, ok := this.Self().(_IMoveEvent); ok {
			im.OnMove()
		}
		this.Update()
	}
}

func (this *Item) Size() (width, height float64) {
	return this.bounds.Width, this.bounds.Height
}

func (this *Item) SetSize(width, height float64) {
	if this.bounds.Width != width || this.bounds.Height != height {
		this.Update()
		this.bounds.Width, this.bounds.Height = width, height
		if ir, ok := this.Self().(_IResizeEvent); ok {
			ir.OnResize()

		}
		this.Update()
	}
}

func (this *Item) OnMove() {
	if !this.localCoord {
		this.Self().Layout()
	}
}

func (this *Item) OnResize() {
	this.Self().Layout()
}

func (this *Item) Layout() {
}

func (this *Item) Bounds1() geom.Rect {
	return this.bounds
}

func (this *Item) Bounds() (x, y, width, height float64) {
	x, y, width, height = this.bounds.X, this.bounds.Y, this.bounds.Width, this.bounds.Height
	return
}

func (this *Item) SetBounds(x, y, width, height float64) {
	this.SetPos(x, y)
	this.SetSize(width, height)
}

func (this *Item) SetBounds1(bounds geom.Rect) {
	this.SetBounds(bounds.X, bounds.Y, bounds.Width, bounds.Height)
}

func (this *Item) Children() []IItem {
	if this == nil {
		return nil
	}
	head := this.child
	if head == nil {
		return nil
	}
	ret := make([]IItem, 0, 10)

	end := head.prev
	for p := head; ; p = p.next {
		ret = append(ret, p.Self())
		if p == end {
			break
		}
	}
	return ret
}

func (this *Item) NakedItem() *Item {
	return this
}

func (this *Item) Parent() IItem {
	if this.parent != nil && this.parent != this.parent.Self() {
		core.Warn("this.parent != this.parent.Self()")
	}
	return this.parent
}

func (this *Item) Detach() {
	this.SetParent(nil)
	//	return this.Self()
}

func (this *Item) SetParent(newParent IItem) {
	if newParent != nil {
		newParent = newParent.Self()
	}

	oldParent := this.parent

	if oldParent == newParent {
		return
	}

	this.invalidateCachedRoot()

	if oldParent != nil {
		p := oldParent.NakedItem()
		if this.next == this {
			p.child = nil
		} else {
			if p.child == this {
				p.child = this.next
			}
			this.next.prev = this.prev
			this.prev.next = this.next
		}
		//oldParent.Scene()
		scene := oldParent.Scene()
		if scene != nil {
			scene.NakedScene().emitItemDetached(oldParent, this.Self())
		}
	}

	if newParent == nil {
		this.next = nil
		this.prev = nil
		this.parent = nil
	} else {
		p := newParent.NakedItem()
		if p.child == nil {
			p.child = this
			this.next = this
			this.prev = this
		} else {
			this.next = p.child
			this.prev = p.child.prev
			this.prev.next = this
			this.next.prev = this
		}
		this.parent = newParent
		scene := newParent.Scene()
		if scene != nil {
			scene.NakedScene().emitItemAttached(newParent, this.Self())
		}
	}
	//	this.syncWindow(this.IsVisible())
}

// Stacking order is the child iteration order: the parent's child pointer
// is the head (drawn first, at the back) and head.prev is the tail (drawn
// last, in front). The four methods below reorder a child among its
// siblings by relinking the circular list; the actual pointer work lives
// in the unexported zorder* helpers so it can be unit-tested directly.

// Raise moves this item one step toward the front (drawn later).
func (this *Item) Raise() {
	this.zorderRaise()
	this.Update()
}

// Lower moves this item one step toward the back (drawn earlier).
func (this *Item) Lower() {
	this.zorderLower()
	this.Update()
}

// BringToFront moves this item in front of all its siblings.
func (this *Item) BringToFront() {
	this.zorderToFront()
	this.Update()
}

// SendToBack moves this item behind all its siblings.
func (this *Item) SendToBack() {
	this.zorderToBack()
	this.Update()
}

// zorderUnlink detaches this from the sibling ring without touching the
// parent pointer. Returns the parent's naked item, or nil when there is
// nothing to reorder (no parent, or a lone child).
func (this *Item) zorderUnlink() *Item {
	if this.parent == nil {
		return nil
	}
	p := this.parent.NakedItem()
	if this.next == this { // single child — nothing to reorder
		return nil
	}
	if p.child == this {
		p.child = this.next
	}
	this.prev.next = this.next
	this.next.prev = this.prev
	return p
}

// zorderInsertBefore relinks this into the ring immediately before at.
func (this *Item) zorderInsertBefore(at *Item) {
	this.next = at
	this.prev = at.prev
	this.prev.next = this
	this.next.prev = this
}

// zorderSwapAdjacent exchanges the ring positions of a and b, which MUST be
// neighbours with b == a.next. Re-links by detaching a and re-inserting it
// after b, and fixes the parent's head pointer when either node was the head.
func zorderSwapAdjacent(p *Item, a, b *Item) {
	before := a.prev // node before a (== b for a 2-element ring)
	// Detach a from between `before` and b.
	before.next = b
	b.prev = before
	// Insert a immediately after b.
	a.prev = b
	a.next = b.next
	b.next.prev = a
	b.next = a
	// a moved one slot toward the tail; b moved one slot toward the head.
	if p.child == a {
		p.child = b
	}
}

// zorderRaise swaps this with the sibling drawn just after it. No-op when
// this is already the tail (frontmost) or has no movable siblings.
func (this *Item) zorderRaise() {
	if this.parent == nil {
		return
	}
	p := this.parent.NakedItem()
	if this.next == this || this.next == p.child { // single child or already tail
		return
	}
	zorderSwapAdjacent(p, this, this.next)
}

// zorderLower swaps this with the sibling drawn just before it. No-op when
// this is already the head (backmost) or has no movable siblings.
func (this *Item) zorderLower() {
	if this.parent == nil {
		return
	}
	p := this.parent.NakedItem()
	if this.prev == this || this == p.child { // single child or already head
		return
	}
	zorderSwapAdjacent(p, this.prev, this)
}

// zorderToFront moves this to the tail (drawn last). No-op when already there.
func (this *Item) zorderToFront() {
	p := this.parent
	if p == nil {
		return
	}
	pn := p.NakedItem()
	if this.next == pn.child { // already the tail / frontmost
		return
	}
	if this.zorderUnlink() == nil {
		return
	}
	// tail == just before the (possibly new) head in the ring
	this.zorderInsertBefore(pn.child)
}

// zorderToBack moves this to the head (drawn first). No-op when already there.
func (this *Item) zorderToBack() {
	p := this.parent
	if p == nil {
		return
	}
	pn := p.NakedItem()
	if this == pn.child { // already the head / backmost
		return
	}
	if this.zorderUnlink() == nil {
		return
	}
	this.zorderInsertBefore(pn.child)
	pn.child = this // the new head
}

func (this *Item) HasLocalCoord() bool {
	return this.localCoord
}

func (this *Item) SetLocalCoord(b bool) {
	if this.localCoord == b {
		return
	}
	this.localCoord = b
	this.invalidateCoordOffset()
}

func (this *Item) IsVisible() bool {
	return !this.hidden
}

func (this *Item) SetVisible(b bool) {
	if this.hidden == !b {
		return
	}
	this.hidden = !b

	if this.hidden {
		if i, ok := this.Self().(_IEventHide); ok {
			i.OnHide()
		}
	} else {
		if i, ok := this.Self().(_IEventShow); ok {
			i.OnShow()
		}
	}

	this.Update()
}

func (this *Item) Show() {
	this.SetVisible(true)
}

func (this *Item) Hide() {
	this.SetVisible(false)
}

func intersectRect(x0, y0, w0, h0, x1, y1, w1, h1 float64) (x, y, w, h float64) {
	r0 := x0 + w0
	b0 := y0 + h0
	r1 := x1 + w1
	b1 := y1 + h1
	x = math.Max(x0, x1)
	y = math.Max(y0, y1)
	w = math.Min(r0, r1) - x
	h = math.Min(b0, b1) - y
	return
}

type _IDrawOverlay interface {
	DrawOverlay(cc paint.Painter)
}

func drawErrCross(g paint.Painter, x, y, w, h float64) {
	g.Save()
	g.Rectangle(x, y, w, h)
	g.SetBrush1(paint.Color{248, 248, 250, 240})
	g.FillPreserve()
	g.SetPen1(paint.Color{180, 180, 200, 200}, 0.15)
	g.Stroke()
	g.Restore()
}

func drawItemSelf(iw IItem, g paint.Painter) (ok bool) {
	if core.IsDebugOn() {
		defer func() {
			if e := recover(); e != nil {
				core.Warn(fmt.Sprintf("Recover drawItemSelf(...): %s", e))
			}
		}()
	}

	iw.DrawSelf(g)
	return true
}

func drawItemOverlay(iw IItem, g paint.Painter) (ok bool) {
	ida, ok := iw.(_IDrawOverlay)
	if !ok {
		return
	}
	if core.IsDebugOn() {
		s0 := g.Save()
		defer func() {
			if e := recover(); e != nil {
				core.Warn(fmt.Sprintf("Recover drawItemOverlay(...): \"%s\": %s", reflect.TypeOf(iw).Elem().Name(), e))
			}
			s1 := g.Restore()
			if s0 != s1 {
				core.Warn(`unbalance painter save()/restore() in "`, reflect.TypeOf(iw).Elem().Name(), `"`)
				g.RestoreTo(s0)
			}

		}()
	}
	ida.DrawOverlay(g)

	return true
}

func (this *Item) DrawChildren(g paint.Painter) {
	iw := this.Self()
	//	cx, cy, cw, ch := g.ClipBounds()

	localCoord := iw.HasLocalCoord()
	x0, y0 := iw.Pos()
	if localCoord {
		//cx -= x0
		//cy -= y0
		g.Translate(x0, y0)
	}

	head := iw.NakedItem().child
	if head == nil {
		return
	}
	end := head.prev
	for c := head; ; c = c.next {
		ic := c.Self()
		ic.DrawAll(g)
		if c == end {
			break
		}
	}

	if localCoord {
		g.Translate(-x0, -y0)
	}

}

func (this *Item) DrawSelf(g paint.Painter) {
	//g.Save()
	//defer g.Restore()
	drawErrCross(g, this.X(), this.Y(), this.Width(), this.Height())
}

func (this *Item) DrawOutline(g paint.Painter) {
	g.SetPen1(gui.Theme().HighLightColor, 0)
	g.Rectangle1(this.Bounds1())
	g.Stroke()
}

func (this *Item) DrawAll(g paint.Painter) {
	ic := this.Self()

	if !ic.IsVisible() {
		return
	}
	clip := g.ClipBounds1()
	bounds := ic.Bounds1()
	intersect := clip.IntersectCopy(bounds)
	if intersect.IsEmpty() {
		return
	}

	g.Save()
	g.Rectangle1(intersect)
	g.Clip()

	drawItemSelf(ic, g)
	ic.DrawChildren(g)
	drawItemOverlay(ic, g)

	g.Restore()

}

func (this *Item) Update() {
	scene := this.Scene()
	if scene == nil {
		//	panic("root is not scene")
		return
	}
	scene.Update()
}

//func (this *Item) setSelected(b bool) {
//	if this.selected != b {
//		this.selected = b
//		this.Update()
//	}
//}

//func (this *Item) IsSelected() bool {
//	return this.selected
//}

func (this *Item) IsSelectable() bool {
	return this.selectable
}

func (this *Item) SetSelectable(b bool) {
	if b == this.selectable {
		return
	}
	this.selectable = b
}

func (this *Item) IsHitBody(x, y float64) bool {
	return this.bounds.Contains(x, y)
}

func (this *Item) MaybeHitBody(x, y float64) bool {
	return this.EaveBounds1().Contains(x, y)
}

func (this *Item) EaveBounds() (x, y, width, height float64) {
	rc := this.bounds.ExpandCopy(this.eave)
	return rc.X, rc.Y, rc.Width, rc.Height
}

func (this *Item) EaveBounds1() geom.Rect {
	return this.bounds.ExpandCopy(this.eave)
}

func findItemAt(a IItem, x, y float64, cond TraversalCond) IItem {
	if cond == nil {
		cond = TraversalCond_Selectable
	}
	if !a.MaybeHitBody(x, y) {
		return nil
	}
	self, enter := cond(a)
	if enter && a.HasChildren() {
		x1, y1 := x, y
		if a.HasLocalCoord() {
			x1 -= a.X()
			y1 -= a.Y()
		}
		head := a.NakedItem().child
		for c := head.prev; ; c = c.prev {
			ic := c.Self()
			ret := findItemAt(ic, x1, y1, cond)
			if ret != nil {
				return ret
			}
			if c == head {
				break
			}
		}
	}
	if self && a.IsHitBody(x, y) {
		return a
	}
	return nil
}

func (this *Item) FindItemAt(x, y float64, cond TraversalCond) IItem {
	return findItemAt(this.Self(), x, y, cond)
}

func findItemsAt(a IItem, x, y float64, cond TraversalCond, buf []IItem) []IItem {
	if cond == nil {
		cond = TraversalCond_Selectable
	}
	if !a.MaybeHitBody(x, y) {
		return buf
	}
	self, enter := cond(a)
	if enter && a.HasChildren() {
		x1, y1 := x, y
		if a.HasLocalCoord() {
			x1 -= a.X()
			y1 -= a.Y()
		}
		head := a.NakedItem().child
		for c := head.prev; ; c = c.prev {
			ic := c.Self()
			buf = findItemsAt(ic, x1, y1, cond, buf)
			if c == head {
				break
			}
		}
	}
	if self && a.IsHitBody(x, y) {
		buf = append(buf, a)
	}
	return buf
}

func (this *Item) FindItemsAt(x, y float64, cond TraversalCond) []IItem {
	return findItemsAt(this.Self(), x, y, cond, nil)
}

func findItemsInRectF(a IItem, cx, cy, cw, ch float64, cond TraversalCond, buf []IItem) []IItem {

	// 为了提高效率, 框选操作也考虑剪裁
	ex, ey, ew, eh := a.EaveBounds()
	cx1, cy1, cw1, ch1 := intersectRect(ex, ey, ew, eh, cx, cy, cw, ch)
	if cw1 <= 0 || ch1 <= 0 {
		return buf
	}

	if cond == nil {
		cond = TraversalCond_Selectable
	}

	self, enter := cond(a)
	if enter && a.HasChildren() {
		cx2, cy2, cw2, ch2 := cx1, cy1, cw1, ch1
		if a.HasLocalCoord() {
			cx2 -= a.X()
			cy2 -= a.Y()
		}
		head := a.NakedItem().child
		for c := head.prev; ; c = c.prev {
			buf = findItemsInRectF(c.Self(), cx2, cy2, cw2, ch2, cond, buf)
			if c == head {
				break
			}
		}
	}

	if self {
		//	if allIntersected {
		// buf = append(buf, a)
		////	} else {
		x, y, w, h := a.Bounds()
		cr := geom.Rect{cx, cy, cw, ch}
		if cr.Contains(x, y) && cr.Contains(x+w, y+h) {
			buf = append(buf, a)
		}
		//	}

	}
	return buf
}

func (this *Item) FindItemsInRectF(cx, cy, cw, ch float64, cond TraversalCond) []IItem {
	return findItemsInRectF(this.Self(),
		cx, cy, cw, ch,
		cond, nil)
}

func (this *Item) FindItemsInRect(bounds geom.Rect, cond TraversalCond) []IItem {
	return findItemsInRectF(this.Self(),
		bounds.X, bounds.Y, bounds.Width, bounds.Height,
		cond, nil)
}

func (this *Item) invalidateCoordOffset() {
	this._offset = nil
	head := this.child
	if head == nil {
		return
	}
	end := head.prev
	for c := head; ; c = c.next {
		c.invalidateCoordOffset()
		if c == end {
			break
		}
	}
}

func (this *Item) invalidateCachedRoot() {
	this._offset = nil
	this._root = nil
	head := this.child
	if head == nil {
		return
	}
	end := head.prev
	for c := head; ; c = c.next {
		c.invalidateCachedRoot()
		if c == end {
			break
		}
	}
}

func (this *Item) CoordOffset() (dx, dy float64) {
	if this._offset == nil {
		p := this.Parent()
		if p == nil {
			this._offset = &geom.Vec2{0, 0}
		} else if p.HasLocalCoord() {
			dx, dy := p.CoordOffset()
			this._offset = &geom.Vec2{dx + p.X(), dy + p.Y()}
		} else {
			dx, dy := p.CoordOffset()
			this._offset = &geom.Vec2{dx, dy}
		}
	}
	return this._offset.X, this._offset.Y
}

func (this *Item) Root() IItem {
	if this._root == nil {
		p := this.Parent()
		if p == nil {
			this._root = this.Self()
		} else {
			this._root = p.Root()
		}
	}
	return this._root
}

func (this *Item) Scene() IScene {
	root := this.Root()
	is, ok := root.(IScene)
	if ok {
		return is
	}
	return nil
}

//func (this *Item) OnSelect() {
//}

//func (this *Item) OnDeselect() {
//}

//func (this *Item) Select() {
//	self := this.Self()
//	view := this.Scene()
//	//this.selected = true
//	if scene != nil {
//		scene.Selection().Add(self)
//	}
//}

//func (this *Item) Deselect() {
//	self := this.Self()
//	scene := this.Scene()
//	//this.selected = false
//	if scene != nil {
//		scene.Selection().Remove(self)
//	}
//}

//func (this *Item) InvertSelect() {
//	self := this.Self()
//	scene := this.Scene()
//	if scene != nil {
//		scene.Selection().Invert(self)
//	}
//}

func (this *Item) Path(ancestor IItem) []IItem {
	root := ancestor
	if root == nil {
		root = this.Root()
	}
	var path []IItem
	a := this.Self()
	ok := false
	for a != nil {
		path = append(path, a)
		if a == root {
			ok = true
			break
		}
		a = a.Parent()
	}
	if !ok {
		return nil
	}
	var ret []IItem
	for i := len(path) - 1; i >= 0; i-- {
		ret = append(ret, path[i])
	}
	return ret
}

func (this *Item) DebugPathString(ancestor IItem) string {
	var ret string
	for _, a := range this.Path(ancestor) {
		ret += "/" + reflect.TypeOf(a).String()[1:]
	}
	return ret

}

func (this *Item) String() string {
	return reflect.TypeOf(this.Self()).String()[1:]
}

func (this *Item) DebugLabel() string {
	return reflect.TypeOf(this.Self()).String()[1:]
}

func (this *Item) DebugFlagsString() (ret string) {
	//if this.IsVisible()
	p := this.Self()
	if p.IsVisible() {
		ret += "V "
	}
	if p.IsSelectable() {
		ret += "S "
	}

	if p.IsLockSize() {
		ret += "Ls "
	}
	if p.IsLockPos() {
		ret += "Lp "
	}

	return ret
}

//func (this *Item) SetDecor(decor IDecor) {
//	this.decor = decor
//	decor.setItem(this.Self())
//}

//func (this *Item) Decor() IDecor {
//	return this.decor
//}

func (this *Item) IsLockPos() bool {
	return this.lockPos
}

func (this *Item) SetLockPos(b bool) {
	this.lockPos = b
}

func (this *Item) IsLockSize() bool {
	return this.lockSize
}

func (this *Item) SetLockSize(b bool) {
	this.lockSize = b
}

func appendTreeItems(buf []IItem, item *Item) []IItem {
	buf = append(buf, item.Self())
	head := item.child
	if head != nil {
		end := head.prev
		for c := head; ; c = c.next {
			buf = appendTreeItems(buf, c)
			if c == end {
				break
			}
		}
	}
	return buf
}

func (this *Item) TreeItems() []IItem {
	return appendTreeItems(nil, this)
}

func (this *Item) MapToChild(x, y float64) (x1, y1 float64) {
	if this.localCoord {
		x1, y1 = x-this.bounds.X, y-this.bounds.Y
	} else {
		x1, y1 = x, y
	}
	return
}

func (this *Item) MapFromChild(x, y float64) (x1, y1 float64) {
	if this.localCoord {
		x1, y1 = x+this.bounds.X, y+this.bounds.Y
	} else {
		x1, y1 = x, y
	}
	return
}

func (this *Item) MapRectToChild(x, y, width, height float64) (x1, y1, width1, height1 float64) {
	if this.localCoord {
		x1, y1 = x-this.bounds.X, y-this.bounds.Y
	} else {
		x1, y1 = x, y
	}
	width1, height1 = width, height
	return

}

func (this *Item) MapRectFromChild(x, y, width, height float64) (x1, y1, width1, height1 float64) {
	if this.localCoord {
		x1, y1 = x+this.bounds.X, y+this.bounds.Y
	} else {
		x1, y1 = x, y
	}
	width1, height1 = width, height
	return
}

func (this *Item) MapToScene(x, y float64) (x1, y1 float64) {
	dx, dy := this.CoordOffset()
	x1, y1 = x+dx, y+dy
	return
}

func (this *Item) MapFromScene(x, y float64) (x1, y1 float64) {
	dx, dy := this.CoordOffset()
	x1, y1 = x-dx, y-dy
	return
}

func (this *Item) MapRectToScene(x, y, width, height float64) (x1, y1, width1, height1 float64) {
	x1, y1 = this.MapToScene(x, y)
	width1, height1 = width, height
	return
}

func (this *Item) MapRectFromScene(x, y, width, height float64) (x1, y1, width1, height1 float64) {
	x1, y1 = this.MapFromScene(x, y)
	width1, height1 = width, height
	return
}

func (this *Item) MapToItem(a IItem, x, y float64) (x1, y1 float64) {
	x1, y1 = this.MapToScene(x, y)
	x1, y1 = a.MapFromScene(x1, y1)
	return
}

func (this *Item) MapFromItem(a IItem, x, y float64) (x1, y1 float64) {
	x1, y1 = a.MapToScene(x, y)
	x1, y1 = this.MapFromScene(x1, y1)
	return
}

func (this *Item) MapRectToItem(a IItem, x, y, width, height float64) (x1, y1, width1, height1 float64) {
	x1, y1, width1, height1 = this.MapRectToScene(x, y, width, height)
	x1, y1, width1, height1 = a.MapRectFromScene(x1, y1, width1, height1)
	return
}

func (this *Item) MapRectFromItem(a IItem, x, y, width, height float64) (x1, y1, width1, height1 float64) {
	x1, y1, width1, height1 = a.MapRectToScene(x, y, width, height)
	x1, y1, width1, height1 = this.MapRectFromScene(x1, y1, width1, height1)
	return
}

func (this *Item) MapRectToChild1(rect geom.Rect) geom.Rect {
	x, y, w, h := this.MapRectToChild(rect.X, rect.Y, rect.Width, rect.Height)
	return geom.Rect{x, y, w, h}
}

func (this *Item) MapRectFromChild1(rect geom.Rect) geom.Rect {
	x, y, w, h := this.MapRectFromChild(rect.X, rect.Y, rect.Width, rect.Height)
	return geom.Rect{x, y, w, h}

}

func (this *Item) MapRectToScene1(rect geom.Rect) geom.Rect {
	x, y, w, h := this.MapRectToScene(rect.X, rect.Y, rect.Width, rect.Height)
	return geom.Rect{x, y, w, h}

}

func (this *Item) MapRectFromScene1(rect geom.Rect) geom.Rect {
	x, y, w, h := this.MapRectFromScene(rect.X, rect.Y, rect.Width, rect.Height)
	return geom.Rect{x, y, w, h}

}

func (this *Item) MapRectToItem1(a IItem, rect geom.Rect) geom.Rect {
	x, y, w, h := this.MapRectToItem(a, rect.X, rect.Y, rect.Width, rect.Height)
	return geom.Rect{x, y, w, h}

}

func (this *Item) MapRectFromItem1(a IItem, rect geom.Rect) geom.Rect {
	x, y, w, h := this.MapRectFromItem(a, rect.X, rect.Y, rect.Width, rect.Height)
	return geom.Rect{x, y, w, h}
}

func (this *Item) GenDecors() []IDecor {
	if this.self.IsLockSize() || this.self.IsLockPos() {
		return nil
	}

	return []IDecor{NewResizeDecor()}
}

func (this *Item) EnumProperties(list prop.IPropertyList) {
	list.AddProperty("graph_item_visible", this.IsVisible, this.SetVisible)
	//list.AddProperty("graph_item_bounds", this.Bounds1, this.SetBounds1)
	//list.AddProperty("graph_item_has_child", this.HasChildren, nil)
}

func (this *Item) IndexInParent() int {
	parent := this.parent
	if parent == nil {
		return -1
	}

	head := parent.NakedItem().child
	if head == nil {
		return -1
	}
	// Walk the sibling ring head-first (same order as Children()) and count
	// the siblings that precede this item. The body runs before the wrap-around
	// terminator check so the tail (head.prev) is counted too: the last child
	// returns len(children)-1.
	idx := 0
	end := head.prev
	for p := head; ; p = p.next {
		if p == this {
			return idx
		}
		if p == end {
			break
		}
		idx++
	}
	return -1
}

func (this *Item) Icon() paint.Icon {
	return paint.LoadIcon("document")
}

func (this *Item) Title() string {
	return this.String()
}

func (this *Item) SizeHints() gui.SizeHints {
	w, h := this.Size()
	return gui.SizeHints{Width: w, Height: h}
}
