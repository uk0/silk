package graph

import (
	"silk/core"
	//	"silk/factory"
	"silk/geom"
	"silk/gui"
	"silk/paint"
	"silk/prop"
	"math"
)

// defaultWheelScrollLines is the number of lines to scroll per wheel notch.
// Matches the Windows default of 3 lines per WHEEL_DELTA.
const defaultWheelScrollLines = 3

func init() {
	core.RegisterFactory("graph.GraphView", gui.TypeOf(GraphView{}))
}

type PageLayout int

const (
	PL_FIT_WIDTH PageLayout = iota
	PL_FIT_HEIGHT
	PL_FIT_VIEW
	PL_FREE_ZOOM
)

func (v PageLayout) String() string {
	switch v {
	default:
		fallthrough
	case PL_FIT_WIDTH:
		return "FitWidth"
	case PL_FIT_HEIGHT:
		return "FitHeight"
	case PL_FIT_VIEW:
		return "FitView"
	case PL_FREE_ZOOM:
		return "FreeZoom"
	}
}

func ParsePageLayout(s string) PageLayout {
	switch s {
	default:
		fallthrough
	case "FitWidth":
		return PL_FIT_WIDTH
	case "FitHeight":
		return PL_FIT_HEIGHT
	case "FitView":
		return PL_FIT_VIEW
	case "FreeZoom":
		return PL_FREE_ZOOM
	}
}

type IView interface {
	gui.IWidget
	Scene() IScene
	Selection() *Selection

	AddTool(tool ...ITool)
	Tools() []ITool
	SetActiveTool(tool ITool)
	SetDefaultTool(tool ITool)
	DefaultTool() ITool
	ActiveTool() ITool

	SetScene(IScene)

	// 判断图元是否在当前视图中被选中
	IsItemSelected(IItem) bool

	// 从视图坐标转到场景坐标.
	// 注: 视图坐标等同于Widget坐标, 不随滚动条变化
	MapToScene(x, y float64) (x1, y1 float64)

	// 从场景坐标转到视图坐标
	// 注: 视图坐标等同于Widget坐标, 不随滚动条变化
	MapFromScene(x, y float64) (x1, y1 float64)

	// 从视图坐标转到场景坐标.
	// 注: 视图坐标等同于Widget坐标, 不随滚动条变化
	MapRectToScene(x, y, w, h float64) (x1, y1, w1, h1 float64)

	// 从场景坐标转到视图坐标
	// 注: 视图坐标等同于Widget坐标, 不随滚动条变化
	MapRectFromScene(x, y, w, h float64) (x1, y1, w1, h1 float64)

	MapRectToScene1(rect geom.Rect) geom.Rect
	MapRectFromScene1(rect geom.Rect) geom.Rect

	ZoomFactor() float64

	emitItemSelected(item IItem)
	emitItemDeselected(item IItem)
	emitItemAttached(item, parent IItem)
	emitItemDetached(item, parent IItem)

	SigItemSelected(func(s interface{}, item IItem))
	SigItemDeselected(func(s interface{}, item IItem))
	SigItemAttached(func(s interface{}, item, parent IItem))
	SigItemDetached(func(s interface{}, item, parent IItem))
	SigGenDecors(func(s interface{}, item IItem) []IDecor)
	SigSelectionChanged(func(s interface{}, selection *Selection))

	FindHandleAt(xMm, yMm float64) (decor IDecor, handle int)

	//SetPropertyView(prop.IPropertyView)

	SetPropertyConfigName(cfgName string)
	PropertyConfigName() (cfgName string)

	// 布局, 一般由内部调用
	Layout()
}

type GraphView struct {
	gui.ScrollArea
	scene  IScene
	ownDoc bool

	// 页面和视图边界之间的最小填充距离, 像素
	padLeftPx   float64
	padRightPx  float64
	padTopPx    float64
	padBottomPx float64

	// 页边距, 毫米
	pageMarginLeft   float64
	pageMarginRight  float64
	pageMarginTop    float64
	pageMarginBottom float64
	showPageMargin   bool

	// 页面居中
	vertAlign gui.VertAlign
	horzAlign gui.HorzAlign

	// 纸张实际位置
	pageLeftPx float64
	pageTopPx  float64

	// 场景的实际位置
	sceneLeftPx float64
	sceneTopPx  float64

	pageLayout PageLayout
	zoom       float64

	tools       []ITool
	actions     []gui.IAction
	defaultTool ITool
	activeTool  ITool

	mouseDownBtn   int     // 0 = nul, 1 = left, 2 = right, 3 = middle
	mouseDownX     float64 // pixels
	mouseDownY     float64 // pixels
	mouseDragStart bool

	undoAction *gui.Action
	redoAction *gui.Action

	selection *Selection

	cbItemSelected     func(s interface{}, item IItem)
	cbItemDeselected   func(s interface{}, item IItem)
	cbItemAttached     func(s interface{}, item, parent IItem)
	cbItemDetached     func(s interface{}, item, parent IItem)
	cbGenDecors        func(s interface{}, item IItem) []IDecor
	cbSelectionChanged func(s interface{}, selection *Selection)
	cbZoomChanged      func(s interface{}, zoom float64)

	needEmitSelectionChanged bool

	//	propertyView prop.IPropertyView

	propertyCfgName string
}

func NewView() *GraphView {
	p := new(GraphView)
	p.Init(p)
	return p
}

func (this *GraphView) Init(iw gui.IWidget) {
	this.ScrollArea.Init(iw)
	this.padLeftPx, this.padRightPx = 10, 10
	this.padTopPx, this.padBottomPx = 10, 10

	this.pageMarginLeft = 5
	this.pageMarginRight = 5
	this.pageMarginTop = 5
	this.pageMarginBottom = 5
	this.showPageMargin = true

	this.zoom = 1

	this.pageLayout = PL_FIT_WIDTH

	this.vertAlign = gui.VA_CENTER
	this.horzAlign = gui.HA_CENTER

	this.HorzScrollBar().SetAutoHide(true)
	this.VertScrollBar().SetAutoHide(true)

	this.undoAction = gui.NewAction()
	this.undoAction.SetObjName("edit-undo")
	this.undoAction.SetIcon(gui.LoadIcon("edit-undo"))
	this.AddAction(this.undoAction)

	this.redoAction = gui.NewAction()
	this.redoAction.SetObjName("edit-redo")
	this.redoAction.SetIcon(gui.LoadIcon("edit-redo"))
	this.AddAction(this.redoAction)
}

func (this *GraphView) SceneSizeMm() (width, height float64) {
	if this.scene != nil {
		width, height = this.scene.Size()
	}
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	return
}

func (this *GraphView) PageSizeMm(includeMargin bool) (width, height float64) {
	width, height = this.SceneSizeMm()

	// 页边距
	if includeMargin {
		width += this.pageMarginLeft + this.pageMarginRight
		height += this.pageMarginTop + this.pageMarginBottom
	}
	return
}

func (this *GraphView) PageSizePx(includeMargin bool, zoom float64) (width, height float64) {
	width, height = this.PageSizeMm(includeMargin)

	// 缩放
	width, height = width*zoom, height*zoom

	// 转为像素
	width, height = gui.MmToPixel(width), gui.MmToPixel(height)

	// 像素取整
	width, height = math.Ceil(width), math.Ceil(height)

	return
}

func (this *GraphView) ScrollSizePx() (width, height float64) {
	return this.requireScrollSizePx(this.ZoomFactor())
}

func (this *GraphView) requireScrollSizePx(zoom float64) (width, height float64) {
	width, height = this.PageSizePx(this.showPageMargin, zoom)

	// 填充
	width += this.padLeftPx + this.padRightPx
	height += this.padTopPx + this.padBottomPx
	return
}

func (this *GraphView) SetScene(scene IScene) {
	this.scene = scene
	scene.(interface {
		setView(IView)
	}).setView(this.Self().(IView))
	this.undoAction.BindAction(scene.UndoStack().UndoAction())
	this.redoAction.BindAction(scene.UndoStack().RedoAction())
	this.Layout()
}

func (this *GraphView) Scene() IScene {
	return this.scene
}

/*
func (this *GraphView) calcViewportSizePx(zoom float64) (width, height float64) {
	horzVisible, vertVisible := this.callScrollVisibility(zoom)
	width, height = this.Size()
	if vertVisible {
		width -= gui.Theme().ScrollWidth
	}
	if horzVisible {
		height -= gui.Theme().ScrollWidth
	}
	return
}

func (this *GraphView) callScrollVisibility(zoom float64) (horz, vert bool) {
	hr, vr := this.calcScrollRange(zoom)
	horz = hr > 0
	vert = vr > 0
	return
}

func (this *GraphView) calcScrollRange(zoom float64) (horz, vert float64) {
	viewWidthPx, viewHeightPx := this.calcViewportSizePx(zoom)
	//pageWidthPx, pageHeightPx := this.PageSizePx(true, zoom)
	scrollWidthPx, scrollHeightPx := this.ScrollSizePx()

	horzRangePx := scrollWidthPx - viewWidthPx
	if horzRangePx < 2 {
		horzRangePx = 0
	}

	vertRangePx := scrollHeightPx - viewHeightPx
	if vertRangePx < 2 {
		vertRangePx = 0
	}
	return horzRangePx, vertRangePx
}

func (this *GraphView) calcZoomFitWidth() (zoom float64) {
	pageWidth, _ := this.PageSize(this.showPageMargin)

	vw := this.Width() - this.padLeftPx - this.padRightPx

	if vw < 100 {
		vw = 100
	}

	vw = gui.PixelToMm(vw)
	zoom = limitZoomFactor(vw / pageWidth)

	// 看是否需要纵向滚动条, 如果需要则重新计算

	_, visible := this.callScrollVisibility(zoom)
	if visible {
		vw := this.Width() - this.padLeftPx - this.padRightPx - gui.Theme().ScrollWidth

		if vw < 100 {
			vw = 100
		}

		vw = gui.PixelToMm(vw)
		zoom = limitZoomFactor(vw / pageWidth)
	}

	return
}

func (this *GraphView) calcZoomFitHeight() {

}

func (this *GraphView) calcZoomFitView() {

}
*/

func (this *GraphView) viewportSizePx(horzVisible, vertVisible bool) (width, height float64) {
	width, height = this.Size()
	//width -= this.padLeftPx + this.padRightPx
	//height -= this.padTopPx + this.padBottomPx
	if vertVisible {
		width -= gui.Theme().ScrollWidth
	}
	if horzVisible {
		height -= gui.Theme().ScrollWidth
	}
	if width < this.padLeftPx+this.padRightPx+10 {
		width = this.padLeftPx + this.padRightPx + 10
	}
	if height < this.padTopPx+this.padBottomPx+10 {
		height = this.padTopPx + this.padBottomPx + 10
	}
	return
}

func (this *GraphView) Layout() {

	// 把场景调整到合适的尺寸和位置
	if this.scene != nil {
		hints := this.scene.SizeHints()
		this.scene.SetBounds(0, 0, hints.Width, hints.Height)
	}

	// 初始以滚动条均隐藏来迭代
	horzVisible, vertVisible := false, false
	var viewWidthPx, viewHeightPx float64
	var scrollWidthPx, scrollHeightPx float64
	var pageWidthPx, pageHeightPx float64
	var vertRangePx, horzRangePx float64

	zoom := 1.0
	pageWidth, pageHeight := this.PageSizeMm(this.showPageMargin)
_AGAIN:
	viewWidthPx, viewHeightPx = this.viewportSizePx(horzVisible, vertVisible)
	vw := gui.PixelToMm(viewWidthPx - this.padLeftPx - this.padRightPx)
	vh := gui.PixelToMm(viewHeightPx - this.padTopPx - this.padBottomPx)
	if this.scene != nil {
		switch this.pageLayout {
		default:
		case PL_FIT_WIDTH:
			zoom = limitZoomFactor(vw / pageWidth)
		case PL_FIT_HEIGHT:
			zoom = limitZoomFactor(vh / pageHeight)
		case PL_FIT_VIEW:
			zoom1 := limitZoomFactor(vw / pageWidth)
			zoom2 := limitZoomFactor(vh / pageHeight)
			zoom = math.Min(zoom1, zoom2)
		case PL_FREE_ZOOM:
			zoom = this.ZoomFactor()
		}
	}

	scrollWidthPx, scrollHeightPx = this.requireScrollSizePx(zoom)
	pageWidthPx, pageHeightPx = this.PageSizePx(true, zoom)
	//	core.Debug("scrollWidthPx, scrollHeightPx = ", scrollWidthPx, scrollHeightPx)
	//	core.Debug("viewWidthPx, viewHeightPx = ", viewWidthPx, viewHeightPx)
	vertRangePx = scrollHeightPx - viewHeightPx
	if vertRangePx < 2 {
		vertRangePx = 0
	}
	horzRangePx = scrollWidthPx - viewWidthPx
	if horzRangePx < 2 {
		horzRangePx = 0
	}

	if horzRangePx > 0 && !horzVisible {
		horzVisible = true
		if vertRangePx > 0 {
			vertVisible = true
		}
		//		core.Debug("horzRangePx > 0 &&  !horzVisible")
		goto _AGAIN
	}

	if vertRangePx > 0 && !vertVisible {
		vertVisible = true
		//		core.Debug("vertRangePx > 0 && !vertVisible")
		goto _AGAIN
	}

	this.zoom = zoom

	// 水平对齐
	if scrollWidthPx < viewWidthPx {
		switch this.horzAlign {
		default:
			fallthrough
		case gui.HA_LEFT:
			this.pageLeftPx = this.padLeftPx
			break
		case gui.HA_CENTER:
			this.pageLeftPx = (viewWidthPx - pageWidthPx) * 0.5
			break
		case gui.HA_RIGHT:
			this.pageLeftPx = viewWidthPx - pageWidthPx
			break
		}
	} else {
		this.pageLeftPx = this.padLeftPx
	}

	// 垂直对齐
	if scrollHeightPx < viewHeightPx {
		switch this.vertAlign {
		default:
			fallthrough
		case gui.VA_TOP:
			this.pageTopPx = this.padLeftPx
			break
		case gui.VA_CENTER:
			this.pageTopPx = (viewHeightPx - pageHeightPx) * 0.5
			break
		case gui.VA_BOTTOM:
			this.pageTopPx = viewHeightPx - pageHeightPx
			break
		}
	} else {
		this.pageTopPx = this.padLeftPx
	}

	// 计算场景原点位置
	this.sceneLeftPx, this.sceneTopPx = this.pageLeftPx, this.pageTopPx
	if this.showPageMargin {
		this.sceneLeftPx += gui.MmToPixel(this.pageMarginLeft * this.zoom)
		this.sceneTopPx += gui.MmToPixel(this.pageMarginTop * this.zoom)
	}

	// 更新滚动条属性

	vertBar := this.VertScrollBar()
	//	core.Debug("vertVisible=", vertVisible)
	vertBar.SetRange(0, vertRangePx)
	vertBar.SetDelta(100, viewHeightPx)
	vertBar.SetVisible(vertVisible)

	horzBar := this.HorzScrollBar()
	//	core.Debug("horzVisible=", horzVisible)
	horzBar.SetRange(0, horzRangePx)
	horzBar.SetDelta(100, viewWidthPx)
	horzBar.SetVisible(horzVisible)

	// 定位滚动条
	this.ScrollArea.Layout()
}

func (this *GraphView) Draw(g paint.Painter) {
	g.Save()
	defer g.Restore()

	/*
		if this.scene == nil {
			g.Rectangle(0, 0, this.Width(), this.Height())
			g.SetSourceColor(gui.Theme().ViewBGColor)
			g.Fill()
			if core.CfgDrawDebug() {
				g.SetFont(gui.Theme().Font)

				g.DrawText("No Scene")
			}
			return
		}
	*/
	// 底色
	g.Rectangle(0, 0, this.Width(), this.Height())
	g.SetBrush1(paint.Color{96, 96, 96, 255})
	g.Fill()

	if this.Scene() == nil {
		if core.IsDebugOn() {
			g.Translate(10, 20)
			g.SetFont(gui.Theme().Font)
			g.SetBrush(paint.Color{255, 168, 0, 255})
			g.DrawText("No Scene")
		}
		return
	}

	g.Translate(this.pageLeftPx-this.ScrollX(), this.pageTopPx-this.ScrollY())

	// 画页面
	pw, ph := this.PageSizePx(true, this.ZoomFactor())
	g.Rectangle(0, 0, pw, ph)
	g.SetBrush1(paint.Color{255, 255, 255, 255})
	g.FillPreserve()
	//g.SetSourceRGB(0, 0, 0)
	//g.SetLineWidth(0.3)
	g.SetPen1(paint.Color{0, 127, 255, 255}, 0)
	g.Stroke()

	pageScale := gui.ScreenDpmm() * this.ZoomFactor()
	g.Scale(pageScale, pageScale)

	if this.showPageMargin {
		g.Translate(this.pageMarginLeft, this.pageMarginTop)
	}

	if core.IsDebugOn() {
		sceneWidth, sceneHeight := this.SceneSizeMm()
		g.Rectangle(0, 0, sceneWidth, sceneHeight)
		//g.SetSourceRGBA(160, 96, 0, 127)
		//g.SetLineWidth(1)
		g.SetPen1(paint.Color{160, 96, 0, 127}, 0)
		g.Stroke()

	}

	g.Save()
	this.scene.DrawAll(g)
	g.Restore()

	g.Save()
	this.Selection().OnDraw(g)
	g.Restore()

	if activeTool := this.ActiveTool(); activeTool != nil {
		g.Save()
		activeTool.OnDraw(g)
		g.Restore()
	}

}

func (this *GraphView) OnHorzScroll(sender gui.IWidget) {
	this.Self().(gui.ILayout).Layout()
}

func (this *GraphView) OnVertScroll(sender gui.IWidget) {
	this.Self().(gui.ILayout).Layout()
}

func (this *GraphView) OnMouseWheel(x, y, z float64) {
	if gui.IsKeyDown(gui.KeyCtrl) {
		// Zoom centered on cursor position
		sx, sy := this.MapToScene(x, y)

		oldZoom := this.ZoomFactor()
		var newZoom float64
		if z > 0 {
			newZoom = oldZoom * 1.15
		} else {
			newZoom = oldZoom / 1.15
		}
		// Clamp to 0.1x .. 5.0x
		if newZoom < 0.1 {
			newZoom = 0.1
		}
		if newZoom > 5.0 {
			newZoom = 5.0
		}
		if newZoom == oldZoom {
			return
		}

		this.SetZoomFactor(newZoom)

		// Adjust scroll so the scene point under the cursor stays in place
		nx, ny := this.MapFromScene(sx, sy)
		this.SetScrollX(this.ScrollX() + (nx - x))
		this.SetScrollY(this.ScrollY() + (ny - y))
		this.Self().Update()
		return
	}

	if gui.IsKeyDown(gui.KeyShift) {
		// Horizontal scroll
		this.SetScrollX(this.ScrollX() - z*defaultWheelScrollLines)
		return
	}

	// Default: vertical scroll
	this.SetScrollY(this.ScrollY() - z*defaultWheelScrollLines)
}

// 从视图坐标转到场景坐标.
// 注: 视图坐标等同于Widget坐标, 不随滚动条变化
func (this *GraphView) MapToScene(x, y float64) (x1, y1 float64) {
	x1 = x - this.sceneLeftPx + this.ScrollX()
	y1 = y - this.sceneTopPx + this.ScrollY()
	x1 = gui.PixelToMm(x1) / this.ZoomFactor()
	y1 = gui.PixelToMm(y1) / this.ZoomFactor()
	return
}

// 从场景坐标转到视图坐标
// 注: 视图坐标等同于Widget坐标, 不随滚动条变化
func (this *GraphView) MapFromScene(x, y float64) (x1, y1 float64) {
	x1 = gui.MmToPixel(x) * this.ZoomFactor()
	y1 = gui.MmToPixel(y) * this.ZoomFactor()
	x1 = x1 + this.sceneLeftPx - this.ScrollX()
	y1 = y1 + this.sceneTopPx - this.ScrollY()
	return
}

// 注: 视图坐标等同于Widget坐标, 不随滚动条变化
func (this *GraphView) MapRectToScene(x, y, w, h float64) (x1, y1, w1, h1 float64) {
	x1, y1 = this.MapToScene(x, y)
	x2, y2 := this.MapToScene(x+w, y+h)
	w1 = x2 - x1
	h1 = y2 - y1
	return
}

// 从场景坐标转到视图坐标
// 注: 视图坐标等同于Widget坐标, 不随滚动条变化
func (this *GraphView) MapRectFromScene(x, y, w, h float64) (x1, y1, w1, h1 float64) {
	x1, y1 = this.MapFromScene(x, y)
	x2, y2 := this.MapFromScene(x+w, y+h)
	w1 = x2 - x1
	h1 = y2 - y1
	return
}

func (this *GraphView) MapRectToScene1(rect geom.Rect) geom.Rect {
	x, y, w, h := this.MapRectToScene(rect.X, rect.Y, rect.Width, rect.Height)
	return geom.Rect{x, y, w, h}

}
func (this *GraphView) MapRectFromScene1(rect geom.Rect) geom.Rect {
	x, y, w, h := this.MapRectFromScene(rect.X, rect.Y, rect.Width, rect.Height)
	return geom.Rect{x, y, w, h}

}

func (this *GraphView) OnLeftDown(x, y float64) {
	if this.mouseDownBtn > 0 {
		return
	}
	this.mouseDownBtn = 1

	this.mouseDownX = x
	this.mouseDownY = y
	this.mouseDragStart = false

	if this.Scene() == nil {
		return
	}

	tool := this.ActiveTool()
	if tool == nil {
		return
	}

	x1, y1 := this.MapToScene(x, y)
	tool.OnLeftDown(x1, y1)
	//core.Debug("Item under mouse: ", this.Scene().FindItemAt(x1, y1, nil))
}

func (this *GraphView) OnLeftUp(x, y float64) {
	if this.mouseDownBtn != 1 {
		return
	}
	this.mouseDownBtn = 0

	if this.Scene() == nil {
		return
	}

	tool := this.ActiveTool()
	if tool == nil {
		return
	}

	x1, y1 := this.MapToScene(x, y)

	if !this.mouseDragStart {
		tool.OnLeftClick(x1, y1)
	}
	tool.OnLeftUp(x1, y1)

	this.mouseDownX = 0
	this.mouseDownY = 0
	this.mouseDragStart = false

}

func (this *GraphView) OnMouseMove(x, y float64) {
	drawStart := false
	if this.mouseDownBtn != 0 &&
		!this.mouseDragStart &&
		(math.Abs(x-this.mouseDownX) > 1 ||
			math.Abs(y-this.mouseDownY) > 1) {
		drawStart = true
		this.mouseDragStart = true
	}

	if this.Scene() == nil {
		return
	}

	tool := this.ActiveTool()
	if tool == nil {
		return
	}
	x1, y1 := this.MapToScene(x, y)
	//core.Debug("x1, y1 = ", x1, y1)
	if drawStart {
		switch this.mouseDownBtn {
		case 1:
			tool.OnLeftDragStart(x1, y1)
		case 2:
		case 3:
		}

	}
	tool.OnMouseMove(x1, y1)

	//core.Debug("Item under mouse: ", this.Scene().FindItemAt(x1, y1, nil))
}

func (this *GraphView) OnIdle() {
	if this.needEmitSelectionChanged {
		this.emitSelectionChanged()
	}
}

func (this *GraphView) SetPaddingMm(left, right, top, bottom float64) {
	this.SetPaddingPx(gui.MmToPixel(left),
		gui.MmToPixel(right),
		gui.MmToPixel(top),
		gui.MmToPixel(bottom))
}

// 设置页面和窗口边界之间的最小填充距离, 像素坐标, 不足1像素按1像素计算.
func (this *GraphView) SetPaddingPx(left, right, top, bottom float64) {
	this.padLeftPx = math.Ceil(left)
	this.padRightPx = math.Ceil(right)
	this.padTopPx = math.Ceil(top)
	this.padBottomPx = math.Ceil(bottom)
	this.Layout()
}

// 获取页面和窗口边界之间的最小填充距离, 像素.
// 注: 这个函数获取的只是最小填充距离, 由于有布局的影响, 实际填充距离可能和这个值不符.
func (this *GraphView) PaddingPx() (left, right, top, bottom float64) {
	left, right, top, bottom = this.padLeftPx, this.padRightPx, this.padTopPx, this.padBottomPx
	return
}

// 获取页面左上角相对于像素原点的偏移量
// 注: 页面通常不紧贴着视图边界, 这个值可能随着视图的缩放而变化
func (this *GraphView) PageOriginPx() (x0, y0 float64) {
	x0, y0 = this.pageLeftPx, this.pageTopPx
	return
}

// 获取场景左上角相对于像素原点的偏移量
func (this *GraphView) SceneOriginPx() (x0, y0 float64) {
	x0, y0 = this.sceneLeftPx, this.sceneTopPx
	return
}

func limitZoomFactor(z float64) float64 {
	if z < 0.01 {
		return 0.01
	}
	if z > 100 {
		return 100
	}
	return z
}

func (this *GraphView) PageLayout() PageLayout {
	return this.pageLayout
}

func (this *GraphView) SetPageLayout(mode PageLayout) {
	if this.pageLayout == mode {
		return
	}
	this.pageLayout = mode
	this.Layout()
}

func (this *GraphView) ZoomFactor() float64 {
	return limitZoomFactor(this.zoom)
}

func (this *GraphView) SetZoomFactor(z float64) {
	changed := false
	zoomChanged := false
	if this.PageLayout() != PL_FREE_ZOOM {
		this.pageLayout = PL_FREE_ZOOM
		changed = true
	}
	z = limitZoomFactor(z)
	if this.zoom != z {
		this.zoom = z
		changed = true
		zoomChanged = true
	}
	if changed {
		this.Layout()
	}
	if zoomChanged && this.cbZoomChanged != nil {
		this.cbZoomChanged(this.Self(), z)
	}
}

// SigZoomChanged registers a callback fired whenever the view's zoom
// factor moves to a new value. Fires after Layout, so observers see
// the new zoom AND a re-laid-out viewport. Used by silkide to keep
// the status-bar zoom % cell synced with Ctrl+wheel zoom — without
// the signal, only the keyboard shortcuts would update the cell and
// wheel-driven zoom would silently desync the indicator.
func (this *GraphView) SigZoomChanged(fn func(s interface{}, zoom float64)) {
	this.cbZoomChanged = fn
}

func (this *GraphView) Icon() paint.Icon {
	if p, ok := this.Scene().(interface {
		Icon() paint.Icon
	}); ok {
		return p.Icon()
	}

	return paint.LoadIcon("diagram")
}

func (this *GraphView) Title() string {
	if this.Scene() == nil {
		return "空视图"
	}

	if p, ok := this.Scene().(interface {
		Title() string
	}); ok {
		return p.Title()
	}

	return "常规图件"
}

func (this *GraphView) AddAction(a gui.IAction) {
	this.actions = append(this.actions, a)
}

func (this *GraphView) Actions() (ret []gui.IAction) {
	return this.actions
}

func (this *GraphView) AddTool(tool ...ITool) {
	for _, t := range tool {
		this.addTool(t)
	}
}

func (this *GraphView) addTool(tool ITool) {
	tool = tool.Self()
	im := tool.(interface {
		setOwnerView(*GraphView)
	})

	im.setOwnerView(this)

	this.tools = append(this.tools, tool)
	this.AddAction(tool.Action())
	if this.defaultTool == nil {
		this.defaultTool = tool
		this.SetActiveTool(tool)
	}
}

func (this *GraphView) Tools() []ITool {
	return this.tools
}

func (this *GraphView) SetActiveTool(tool ITool) {
	oldTool := this.activeTool
	newTool := tool.Self()
	if oldTool == newTool {
		return
	}
	if oldTool != nil {
		oldTool.updateMTime()
	}
	this.activeTool = newTool
	if newTool != nil {
		newTool.updateMTime()
	}
}

func (this *GraphView) SetDefaultTool(tool ITool) {
	this.defaultTool = tool.Self()
}

func (this *GraphView) DefaultTool() ITool {
	return this.defaultTool
}

func (this *GraphView) ActiveTool() ITool {
	return this.activeTool
}

func (this *GraphView) AddStandardTools() {
	this.AddTool(NewArrowTool())
	this.AddTool(NewRectTool())
}

func (this *GraphView) Selection() *Selection {
	if this.selection == nil {
		this.selection = newSelection(this)
	}
	return this.selection
}

func (this *GraphView) emitItemSelected(item IItem) {
	this.needEmitSelectionChanged = true
	var ds []IDecor
	if this.cbGenDecors != nil {
		ds = this.cbGenDecors(this.Self(), item)
	}
	if len(ds) == 0 {
		ds = item.GenDecors()
	}

	if len(ds) != 0 {
		node := this.Selection().findNode(item)
		node.decor = ds[0]
		node.decor.setItem(item)
		node.decor.setView(this)
	}
	if this.cbItemSelected != nil {
		this.cbItemSelected(this.Self(), item)
	}
}

func (this *GraphView) emitItemDeselected(item IItem) {
	this.needEmitSelectionChanged = true
	if this.cbItemDeselected != nil {
		this.cbItemDeselected(this.Self(), item)
	}
}

//func (this *GraphView) emitGenDecors(item IItem) {
//	this.cbItemDeselected.Call(item, this.Self())
//}

func (this *GraphView) IsItemSelected(a IItem) bool {
	return this.selection.Contains(a)
}

func (this *GraphView) emitItemAttached(item, parent IItem) {
	this.Update()
	if this.cbItemAttached != nil {
		this.cbItemAttached(this.Self(), item, parent)
	}
}

func (this *GraphView) emitItemDetached(item, parent IItem) {
	if this.selection != nil {
		this.selection.Remove(item)
	}
	this.Update()
	if this.cbItemAttached != nil {
		this.cbItemAttached(this.Self(), item, parent)
	}
}

func (this *GraphView) emitSelectionChanged() {
	if core.IsDebugOn() {
		this.Selection().DebugDump()
	}
	this.needEmitSelectionChanged = false

	propertyView := this.GetPropertyView()
	if propertyView != nil {
		var objs []interface{}
		for _, v := range this.Selection().ItemList() {
			objs = append(objs, v)
		}
		propertyView.Bind(objs, this.PropertyConfigName(), this.Scene())
	}
	if this.cbSelectionChanged != nil {
		this.cbSelectionChanged(this.Self(), this.Selection())
	}
}

func (this *GraphView) SigItemSelected(fn func(s interface{}, item IItem)) {
	this.cbItemSelected = fn
}

func (this *GraphView) SigItemDeselected(fn func(s interface{}, item IItem)) {
	this.cbItemDeselected = fn
}

func (this *GraphView) SigItemAttached(fn func(s interface{}, item, parent IItem)) {
	this.cbItemAttached = fn
}

func (this *GraphView) SigItemDetached(fn func(s interface{}, item, parent IItem)) {
	this.cbItemDetached = fn
}

func (this *GraphView) SigGenDecors(fn func(s interface{}, item IItem) []IDecor) {
	this.cbGenDecors = fn
}

//func (this *GraphView) SigBindPropSheet() core.ICallback {
//	return &this.cbBindPropSheet
//}

func (this *GraphView) SigSelectionChanged(fn func(s interface{}, selection *Selection)) {
	this.cbSelectionChanged = fn
}

func (this *GraphView) FindHandleAt(xMm, yMm float64) (decor IDecor, handle int) {
	return this.Selection().FindHandleAt(xMm, yMm)
}

//func (this *GraphView) SetPropertyView(propView prop.IPropertyView) {
//	if this.propertyView == propView {
//		return
//	}
//	if this.propertyView != nil {
//		this.propertyView.Clear(this.Self())
//	}
//	this.propertyView = propView
//}

func (this *GraphView) SetPropertyConfigName(cfgName string) {
	this.propertyCfgName = cfgName
}

func (this *GraphView) PropertyConfigName() (cfgName string) {
	if this.propertyCfgName != "" {
		return this.propertyCfgName
	}
	if this.Scene() != nil {
		return this.Scene().PropertyConfigName()
	}
	return "default"
}

func (this *GraphView) DirtyList() []string {
	if this.scene == nil {
		return nil
	}
	return this.scene.DirtyList()
}

func (this *GraphView) Save() bool {
	if this.scene == nil {
		core.Debug("try to save a GraphView which has no Scene attached.")
		return true
	}
	return this.scene.Self().(interface {
		Save() bool
	}).Save()
}

func (this *GraphView) SetPageMarginVisible(b bool) {
	this.showPageMargin = b
	this.Layout()
}

func (this *GraphView) GetPropertyView() prop.IPropertyView {
	frame := gui.FindOwnerFrame(this)
	p, _ := frame.ToolViewById("prop.PropertySheet").(prop.IPropertyView)
	return p
}
