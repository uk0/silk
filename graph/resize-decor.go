package graph

import (
	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
	"math"
)

type resizeRectMode int

const (
	resizeRectMode_Free resizeRectMode = iota
	resizeRectMode_KeepAspect
	resizeRectMode_square
)

type direction int

const (
	dirNone direction = iota
	dirLeft
	dirRight
	dirTop
	dirBottom
	dirTopLeft
	dirTopRight
	dirBottomLeft
	dirBottomRight
)

func getRectSpecialPoint(rect geom.Rect, d9 direction) (x, y float64) {
	switch d9 {
	case dirTopLeft:
		return rect.Left(), rect.Top()
	case dirTop:
		return rect.Left() + rect.Width*0.5, rect.Top()
	case dirTopRight:
		return rect.Left() + rect.Width, rect.Top()
	case dirLeft:
		return rect.Left(), rect.Top() + rect.Height*0.5
	case dirRight:
		return rect.Left() + rect.Width, rect.Top() + rect.Height*0.5
	case dirBottomLeft:
		return rect.Left(), rect.Bottom()
	case dirBottom:
		return rect.Left() + rect.Width*0.5, rect.Bottom()
	case dirBottomRight:
		return rect.Left() + rect.Width, rect.Bottom()
	default:
		return rect.Left() + rect.Width*0.5, rect.Top() + rect.Height*0.5
	}

}

func resizeRect_Free(L, T, R, B float64, handle direction, x, y float64) geom.Rect {
	switch handle {
	case dirTopLeft:
		T = y
		L = x

	case dirTop:
		T = y

	case dirTopRight:
		T = y
		R = x

	case dirLeft:
		L = x

	case dirRight:
		R = x

	case dirBottomLeft:
		B = y
		L = x

	case dirBottom:
		B = y

	case dirBottomRight:
		B = y
		R = x

	default:

	}
	return geom.Rect{L, T, R - L, B - T}
}

func mirror(a, c float64) float64 {
	return c - (a - c)
}

func resizeRect_Free_mirror(L, T, R, B float64, handle direction, px, py float64) geom.Rect {

	xc := (L + R) * 0.5
	yc := (T + B) * 0.5
	switch handle {
	case dirTopLeft:
		T = py
		B = mirror(T, yc)
		L = px
		R = mirror(L, xc)

	case dirTop:
		T = py
		B = mirror(T, yc)

	case dirTopRight:
		T = py
		B = mirror(T, yc)
		R = px
		L = mirror(R, xc)

	case dirLeft:
		L = px
		R = mirror(L, xc)

	case dirRight:
		R = px
		L = mirror(R, xc)

	case dirBottomLeft:
		B = py
		T = mirror(B, yc)
		L = px
		R = mirror(L, xc)

	case dirBottom:
		B = py
		T = mirror(B, yc)

	case dirBottomRight:
		B = py
		T = mirror(B, yc)
		R = px
		L = mirror(R, xc)

	default:

	}

	return geom.Rect{L, T, R - L, B - T}
}

func KeepAspect(x0, y0, x1, y1 float64, x2, y2 *float64) {
	dx1 := x1 - x0
	dy1 := y1 - y0
	if dx1 == 0 && dy1 == 0 {
		// 原始矩形大小为0, 看作等比例
		dx1 = 1
		dy1 = 1
	} else if dx1 == 0 {
		*x2 = x0
		return
	} else if dy1 == 0 {
		*y2 = y0
		return
	}
	dx2 := *x2 - x0
	dy2 := *y2 - y0

	xs := dx2 / dx1
	ys := dy2 / dy1

	if math.Abs(xs) > math.Abs(ys) {
		*y2 = xs*dy1 + y0
	} else {
		*x2 = ys*dx1 + x0
	}
}

func resizeRect_KeepAspect(L, T, R, B float64, handle direction, px, py float64) geom.Rect {
	y := py
	x := px
	switch handle {
	case dirTopLeft:
		KeepAspect(R, B, L, T, &x, &y)
		L = x
		T = y

	case dirTop:
		T = py

	case dirTopRight:
		KeepAspect(L, B, R, T, &x, &y)
		T = y
		R = x

	case dirLeft:
		L = px

	case dirRight:
		R = px

	case dirBottomLeft:
		KeepAspect(R, T, L, B, &x, &y)
		B = y
		L = x

	case dirBottom:
		B = py

	case dirBottomRight:
		KeepAspect(L, T, R, B, &x, &y)
		B = y
		R = x

	default:

	}

	return geom.Rect{L, T, R - L, B - T}

}

func resizeRect_KeepAspect_mirror(L, T, R, B float64, handle direction, px, py float64) geom.Rect {
	xc := (L + R) * 0.5
	yc := (T + B) * 0.5
	y := py
	x := px
	switch handle {
	case dirTopLeft:
		KeepAspect(xc, yc, L, T, &x, &y)
		L = x
		R = mirror(L, xc)
		T = y
		B = mirror(T, yc)

	case dirTop:
		T = py
		B = mirror(T, yc)

	case dirTopRight:
		KeepAspect(xc, yc, R, T, &x, &y)
		T = y
		B = mirror(T, yc)
		R = x
		L = mirror(R, xc)

	case dirLeft:
		L = px
		R = mirror(L, xc)

	case dirRight:
		R = px
		L = mirror(R, xc)

	case dirBottomLeft:
		KeepAspect(xc, yc, L, B, &x, &y)
		B = y
		T = mirror(B, yc)
		L = x
		R = mirror(L, xc)

	case dirBottom:
		B = py
		T = mirror(B, yc)

	case dirBottomRight:
		KeepAspect(xc, yc, R, B, &x, &y)
		B = y
		T = mirror(B, yc)
		R = x
		L = mirror(R, xc)

	default:

	}

	return geom.Rect{L, T, R - L, B - T}
}

func square(x0, y0, x2, y2 float64) {
	dx := x2 - x0
	dy := y2 - y0

	if math.Abs(dx) > math.Abs(dy) {
		y2 = dx + y0
	} else {
		x2 = dy + x0
	}
}

func resizeRect_square(L, T, R, B float64, handle direction, px, py float64) geom.Rect {
	y := py
	x := px
	switch handle {
	case dirTopLeft:
		square(R, B, x, y)
		L = x
		T = y

	case dirTop:
		T = py

	case dirTopRight:
		square(L, B, x, y)
		T = y
		R = x

	case dirLeft:
		L = px

	case dirRight:
		R = px

	case dirBottomLeft:
		square(R, T, x, y)
		B = y
		L = x

	case dirBottom:
		B = py

	case dirBottomRight:
		square(L, T, x, y)
		B = y
		R = x

	default:

	}
	return geom.Rect{L, T, R - L, B - T}
}

func resizeRect_square_mirror(L, T, R, B float64, handle direction, px, py float64) geom.Rect {
	xc := (L + R) * 0.5
	yc := (T + B) * 0.5
	y := py
	x := px
	switch handle {
	case dirTopLeft:
		square(xc, yc, x, y)
		L = x
		R = mirror(L, xc)
		T = y
		B = mirror(T, yc)

	case dirTop:
		T = py
		B = mirror(T, yc)

	case dirTopRight:
		square(xc, yc, x, y)
		T = y
		B = mirror(T, yc)
		R = x
		L = mirror(L, xc)

	case dirLeft:
		L = px
		R = mirror(L, xc)

	case dirRight:
		R = px
		L = mirror(L, xc)

	case dirBottomLeft:
		square(xc, yc, x, y)
		B = y
		T = mirror(B, yc)
		L = x
		R = mirror(L, xc)

	case dirBottom:
		B = py

	case dirBottomRight:
		square(xc, yc, x, y)
		B = y
		T = mirror(B, yc)
		R = x
		L = mirror(L, xc)

	default:

	}
	return geom.Rect{L, T, R - L, B - T}
}

func resizeRect(org geom.Rect, handle direction, px, py float64, mode resizeRectMode, mirr bool) geom.Rect {
	L := org.Left()
	R := org.Right()
	T := org.Top()
	B := org.Bottom()

	if mirr {
		switch mode {
		case resizeRectMode_KeepAspect:
			return resizeRect_KeepAspect_mirror(L, T, R, B, handle, px, py)
		case resizeRectMode_square:
			return resizeRect_square_mirror(L, T, R, B, handle, px, py)
		case resizeRectMode_Free:
			fallthrough
		default:
			return resizeRect_Free_mirror(L, T, R, B, handle, px, py)
		}
	} else {
		switch mode {
		case resizeRectMode_KeepAspect:
			return resizeRect_KeepAspect(L, T, R, B, handle, px, py)
		case resizeRectMode_square:
			return resizeRect_square(L, T, R, B, handle, px, py)
		case resizeRectMode_Free:
			fallthrough
		default:
			return resizeRect_Free(L, T, R, B, handle, px, py)
		}
	}
}

// SelectionResizePolicy is optionally implemented by the visual GraphView
// host. It makes its built-in resize handles agree with domain constraints.
// CanResizeSelection is evaluated once per draw or hit-test of the selection;
// CanResizeItem can exclude a child that will follow an editable ancestor.
type SelectionResizePolicy interface {
	CanResizeSelection() bool
	CanResizeItem(IItem) bool
}

func selectionResizePolicy(view IView) SelectionResizePolicy {
	if view == nil {
		return nil
	}
	p, _ := view.Self().(SelectionResizePolicy)
	return p
}

func resizeGroupAllowed(policy SelectionResizePolicy) bool {
	return policy == nil || policy.CanResizeSelection()
}

func resizeItemAllowed(item IItem, policy SelectionResizePolicy) bool {
	return item != nil && !item.IsLockPos() && !item.IsLockSize() &&
		(policy == nil || policy.CanResizeItem(item))
}

type resizeTarget struct {
	item, parent IItem
	bounds       geom.Rect
}

type ResizeDecor struct {
	Decor
	handlePos [9]geom.Vec2
	curRect   geom.Rect
	orgRect   geom.Rect
	orgParent IItem
	version   uint64
	resizing  bool
	others    []resizeTarget
}

func NewResizeDecor() *ResizeDecor {
	p := new(ResizeDecor)
	p.Init(p)
	return p
}

func (this *ResizeDecor) Init(self IDecor) {
	this.Decor.Init(self)
}

func (this *ResizeDecor) calcHandlePos(rect geom.Rect) {
	for i := 0; i < 9; i++ {
		this.handlePos[i].X, this.handlePos[i].Y = getRectSpecialPoint(rect, direction(i))
	}
}

func (this *ResizeDecor) HandleAt(x, y float64) int {
	policy := selectionResizePolicy(this.View())
	return this.handleAt(x, y, policy, resizeGroupAllowed(policy))
}

func (this *ResizeDecor) handleAt(x, y float64, policy SelectionResizePolicy, groupAllowed bool) int {
	if !groupAllowed || !resizeItemAllowed(this.item, policy) {
		return 0
	}
	this.calcHandlePos(this.item.Bounds1())
	for i := 8; i >= 1; i-- {
		px := this.handlePos[i].X
		py := this.handlePos[i].Y
		if this.IsHitCircleHandle(px, py, x, y) {
			return i
		}
	}
	return 0
}

//func (this *ResizeDecor) OnPressHandle(handle int, x, y float64) {
//}

func (this *ResizeDecor) OnBeginMoveHandle(handle int, x, y float64) {
	this.resetResizeGesture()
	policy := selectionResizePolicy(this.View())
	if handle < 1 || handle > 8 || !resizeGroupAllowed(policy) || !resizeItemAllowed(this.item, policy) ||
		this.View() == nil || this.item.Scene() != this.View().Scene() || !this.View().Selection().Contains(this.item) {
		return
	}
	this.orgRect = this.item.Bounds1()
	this.orgParent = this.item.Parent()
	this.version = this.View().Selection().editVersion
	this.curRect = this.orgRect
	this.resizing = true
	for item := range this.View().Selection().Items {
		if item != this.item && resizeItemAllowed(item, policy) {
			this.others = append(this.others, resizeTarget{item, item.Parent(), item.Bounds1()})
		}
	}
}

func (this *ResizeDecor) resetResizeGesture() {
	this.resizing = false
	this.curRect = geom.Rect{}
	this.orgParent = nil
	clear(this.others)
	this.others = this.others[:0]
}

func (this *ResizeDecor) resizeGestureCurrent(policy SelectionResizePolicy, groupAllowed bool) bool {
	return this.resizing && groupAllowed && resizeItemAllowed(this.item, policy) &&
		this.item.Scene() != nil && this.item.Scene() == this.View().Scene() &&
		this.item.Parent() == this.orgParent && this.item.Bounds1() == this.orgRect &&
		this.View().Selection().editVersion == this.version
}

func (this *ResizeDecor) OnMoveHandle(handle int, x, y float64) {
	policy := selectionResizePolicy(this.View())
	if handle < 1 || handle > 8 || !this.resizeGestureCurrent(policy, resizeGroupAllowed(policy)) {
		this.resetResizeGesture()
		return
	}
	var resizeMode resizeRectMode
	if gui.IsKeyDown(gui.KeyShift) {
		resizeMode = resizeRectMode_KeepAspect
	} else {
		resizeMode = resizeRectMode_Free
	}

	mirr := gui.IsKeyDown(gui.KeyCtrl)

	this.curRect = resizeRect(this.orgRect, direction(handle), x, y, resizeMode, mirr)

}

func (this *ResizeDecor) OnEndMoveHandle(handle int, x, y float64) {

	defer this.resetResizeGesture()
	policy := selectionResizePolicy(this.View())

	if this.orgRect == this.curRect || handle < 1 || handle > 8 ||
		!this.resizeGestureCurrent(policy, resizeGroupAllowed(policy)) {
		return
	}

	//auto modifiers = qApp->keyboardModifiers();
	//bool mirror = modifiers == Qt::ControlModifier;
	mirr := gui.IsKeyDown(gui.KeyCtrl)

	for _, target := range this.others {
		w := target.item
		if w.Scene() != this.View().Scene() || w.Parent() != target.parent || w.Bounds1() != target.bounds ||
			!resizeItemAllowed(w, policy) {
			return // don't partially resize or retarget a changed group
		}
	}

	cmd := NewResizeCommand()

	cmd.AddItem(this.Item(), this.curRect.NormalizeCopy())

	var sx, sy float64
	if this.orgRect.Width == 0 {
		sx = 1
	} else {
		sx = this.curRect.Width / this.orgRect.Width
	}
	if this.orgRect.Height == 0 {
		sy = 1
	} else {
		sy = this.curRect.Height / this.orgRect.Height
	}

	for _, target := range this.others {
		w, rect0 := target.item, target.bounds

		w1 := rect0.Width * sx
		h1 := rect0.Height * sy
		var rect geom.Rect
		if mirr {
			xc, yc := rect0.Center()
			// rect.setX( xc - w1 * 0.5 );
			// rect.setY( yc - h1 * 0.5 );
			rect = geom.Rect{xc - w1*0.5, yc - h1*0.5, w1, h1}
		} else {
			x1 := rect0.X
			y1 := rect0.Y

			switch direction(handle) {
			case dirTopLeft:
				x1 = rect0.Right() - w1
				y1 = rect0.Bottom() - h1

			case dirTop:
				y1 = rect0.Bottom() - h1

			case dirTopRight:
				y1 = rect0.Bottom() - h1

			case dirLeft:
				x1 = rect0.Right() - w1

			case dirRight:

			case dirBottomLeft:
				x1 = rect0.Right() - w1

			case dirBottom:

			case dirBottomRight:

			default:

			}
			rect = geom.Rect{x1, y1, w1, h1}
		}

		//QUndoCommand * cmd = new GmResizeCommand(w, rect.normalized());
		// pScene->pushCommand(cmd);
		cmd.AddItem(w, rect.NormalizeCopy())

	}
	if cmd.Count() > 0 {
		appendResizeCarriedChildren(cmd)
		this.Scene().PushCommand(cmd)
	}

}

// appendResizeCarriedChildren preserves child-local offsets when a left/top
// resize moves an absolute-coordinate container's origin. Explicitly resized
// selections keep their own records; locally positioned or layout-managed
// descendants follow their parent automatically and must not be moved twice.
func appendResizeCarriedChildren(cmd *ResizeCommand) {
	direct := cmd.records
	explicit := make(map[IItem]bool, len(direct))
	for _, record := range direct {
		explicit[record.item] = true
	}
	for _, record := range direct {
		dx, dy := record.rect.X-record.item.X(), record.rect.Y-record.item.Y()
		if dx == 0 && dy == 0 {
			continue
		}
		var carry func(IItem)
		carry = func(parent IItem) {
			if parent.HasLocalCoord() {
				return
			}
			if manager, ok := parent.(interface{ ManagesChildGeometry() bool }); ok && manager.ManagesChildGeometry() {
				return
			}
			for _, child := range parent.Children() {
				if explicit[child] {
					continue
				}
				bounds := child.Bounds1()
				bounds.X += dx
				bounds.Y += dy
				cmd.AddItem(child, bounds)
				carry(child)
			}
		}
		carry(record.item)
	}
}

//func (this *ResizeDecor) OnReleaseHandle(handle int, x, y float64) {

//}

func (this *ResizeDecor) HandleCursor(handle int) *gui.Cursor {
	switch direction(handle) {
	case dirTopLeft, dirBottomRight:
		return gui.LoadCursor("size-nwse")
	case dirTopRight, dirBottomLeft:
		return gui.LoadCursor("size-nesw")
	case dirLeft, dirRight:
		return gui.LoadCursor("size-we")
	case dirTop, dirBottom:
		return gui.LoadCursor("size-ns")
	default:
		return gui.LoadCursor("size-all")
	}
}

func (this *ResizeDecor) OnDraw(g paint.Painter) {
	policy := selectionResizePolicy(this.View())
	this.draw(g, policy, resizeGroupAllowed(policy))
}

func (this *ResizeDecor) draw(g paint.Painter, policy SelectionResizePolicy, groupAllowed bool) {
	this.item.DrawOutline(g)
	if !groupAllowed || !resizeItemAllowed(this.item, policy) {
		return
	}
	this.calcHandlePos(this.item.Bounds1())
	activeHandle := this.ActiveHandle()

	for i := 1; i < 9; i++ {
		px := this.handlePos[i].X
		py := this.handlePos[i].Y
		this.DrawDefaultHandle(g, px, py, 0, activeHandle == i)
	}

	if activeHandle != 0 && this.resizeGestureCurrent(policy, groupAllowed) && !this.curRect.IsEmpty() {
		g.SetPen1(gui.Theme().HighLightColor, 0)
		g.Rectangle1(this.curRect)
		g.Stroke()
	}

}
