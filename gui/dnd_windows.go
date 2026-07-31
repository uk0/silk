package gui

import (
	"github.com/uk0/silk/core"
	//"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
	"github.com/uk0/silk/win32"
	"runtime"
	"unsafe"
)

var privateDndData []interface{} // "[]interface{}"

func init() {
	win32.OleInitialize()
	//core.AtExit(win32.OleUninitialize)
}

type dndContext struct {
	do     *win32.IDataObject
	pa     DndAction
	action DndAction
	from   interface{}
}

func (this *dndContext) destroy() {
	this.do.Release()
}

func newDndContext(do *win32.IDataObject, posibleActions DndAction) *dndContext {
	p := new(dndContext)
	p.do = do
	p.pa = posibleActions
	do.AddRef()

	//	p.posibleActions = DndAction(posibleActions)
	i := do.QueryInterface(win32.IID_OurDataObject)
	if i != nil {
		p.from = (*win32.DataObject)(unsafe.Pointer(i)).From()
		i.Release()
	}
	runtime.SetFinalizer(p, (*dndContext).destroy)
	return p
}

func (this *dndContext) Formats() (formats []string) {
	cfs := this.do.Formats()
	for _, cf := range cfs {
		formats = append(formats, clipboardIdToFormat(cf))
	}
	if privateDndData != nil {
		formats = append(formats, "[]interface{}")
	}
	return
}

func (this *dndContext) HasFormat(format string) bool {
	if format == "[]interface{}" {
		return privateDndData != nil
	}
	for _, fp := range clipboardFormats {
		if fp.format == format && this.do.HasFormat(fp.id) {
			return true
		}
	}
	return false
}

func (this *dndContext) Data(format string) (data interface{}) {
	if format == "[]interface{}" {
		return privateDndData
	}
	for _, fp := range clipboardFormats {
		if fp.format == format {
			medium := this.do.Data(fp.id)
			if medium != nil && medium.TyMed == win32.TYMED_HGLOBAL {
				data, _ = decodeClipFormat(medium.HGlobal(), fp.id)
				return
			}
		}
	}
	return
}

func (this *dndContext) PosibleActions() DndAction {
	return this.pa
}

func (this *dndContext) Action() DndAction {
	return this.action
}

func (this *dndContext) SetAction(act DndAction) {
	if this.pa&act == 0 {
		this.action = DndIgnore
		return
	}
	this.action = act
}

func (this *dndContext) From() interface{} {
	return this.from
}

func (this *Window) DragEnter(pDataObj *win32.IDataObject,
	grfKeyState uint32, x0, y0 int32, pdwEffect *uint32) win32.HRESULT {
	if pdwEffect == nil {
		return win32.E_INVALIDARG
	}
	this.lastDndWidget = nil
	this.dndContext = newDndContext(pDataObj, DndAction(*pdwEffect))
	// Report the effect the widget under the cursor would apply, not the one the
	// source proposed: OLE shows that effect until the first DragOver, and it is
	// also what decides whether a release without any motion reaches Drop at all.
	return this.DragOver(grfKeyState, x0, y0, pdwEffect)
}

// E_UNEXPECTED, E_INVALIDARG, E_OUTOFMEMORY
func (this *Window) DragOver(grfKeyState uint32,
	x0, y0 int32, pdwEffect *uint32) win32.HRESULT {
	//core.Debug("DragOver")
	if pdwEffect == nil {
		return win32.E_INVALIDARG
	}
	if this.dndContext == nil {
		return win32.E_UNEXPECTED
	}

	this.dndContext.pa = DndAction(*pdwEffect)

	x1, y1 := this.mapFromGlobal(float64(x0), float64(y0))
	widget := this.widget.FindWidgetAt(x1, y1)
	x, y := 0.0, 0.0
	if widget != nil {
		x, y = widget.MapFromWindow(x1, y1)
	}
	this.lastDndWidget = dndDragOver(widget, this.lastDndWidget, x, y, this.dndContext)

	*pdwEffect = uint32(this.dndContext.Action())
	return win32.S_OK
}

// E_OUTOFMEMORY
func (this *Window) DragLeave() win32.HRESULT {
	this.dndContext = nil
	dndDragLeave(this.lastDndWidget)
	this.lastDndWidget = nil
	return win32.S_OK
}

// E_UNEXPECTED, E_INVALIDARG, E_OUTOFMEMORY
func (this *Window) Drop(pDataObj *win32.IDataObject,
	grfKeyState uint32, x0, y0 int32, pdwEffect *uint32) win32.HRESULT {
	if pdwEffect == nil {
		return win32.E_INVALIDARG
	}

	// The nil test has to come first: OLE calls Drop with no live context after
	// a DragLeave, and reading dndContext.do to compare the data object faults
	// the process.
	if this.dndContext == nil || pDataObj != this.dndContext.do {
		return win32.E_UNEXPECTED
	}

	x1, y1 := this.mapFromGlobal(float64(x0), float64(y0))
	widget := this.widget.FindWidgetAt(x1, y1)
	x, y := 0.0, 0.0
	if widget != nil {
		x, y = widget.MapFromWindow(x1, y1)
	}
	dndDrop(widget, x, y, this.dndContext)

	*pdwEffect = uint32(this.dndContext.Action())
	// OLE sends no DragLeave after a drop, and the target that highlighted
	// itself during the drag only clears that highlight in OnDragLeave.
	dndDragLeave(this.lastDndWidget)
	this.dndContext = nil
	this.lastDndWidget = nil
	return win32.S_OK
}

func (this *Window) HWND() win32.HWND {
	return this.hWnd
}

func (this *Window) DoDragDrop(from interface{},
	content paint.Pixmap,
	availableActions DndAction,
	data ...interface{}) DndAction {

	privateDndData = nil

	if len(data) == 0 {
		core.Warn("drag nothing")
		return DndIgnore
	}

	// convert data to hGlobal an CF_FORMAT
	var v []win32.HGLOBAL
	var f []win32.CLIPFORMAT
	for _, d := range data {
		hGlobal, _, id, err := encodeClipFormat(d)
		if err != nil {
			privateDndData = append(privateDndData, d)
			continue
		}
		v = append(v, hGlobal)
		f = append(f, id)
	}

	// 如果没有普通数据, 则给一个虚假的数据, 以免因为没有数据而被特殊处理
	// 没有普通数据时, 拖动的是特殊数据, 存放在privateDndData里
	if len(v) == 0 {
		v = append(v, 0)
		f = append(f, CF_UNUSED)
	}

	// 如果没有提供缩略图, 则用一个灰色矩形代替
	if content == nil {
		content = paint.NewPixmap(32, 16)
		g := content.NewPainter()
		g.SetBrush1(paint.Color{0, 0, 0, 127})
		g.Rectangle(0, 0, 32, 16)
		g.Fill()
	}

	// 把缩略图附加到光标上
	// TODO: Windows XP 不支持大图片
	cursors := GenerateDropCursors(content)

	do := win32.NewDataObject(f, v, from)
	ds := win32.NewDropSource(
		func(ef uint32) {
			switch DndAction(ef) {
			default:
				fallthrough
			case DndIgnore:
				SetCursor(cursors[0])
			case DndMove:
				SetCursor(cursors[1])
			case DndCopy:
				SetCursor(cursors[2])
			case DndLink:
				SetCursor(cursors[3])
			}
		})
	// DndLink belongs in the mask: DROPEFFECT_* and DndAction share their values,
	// the feedback closure above already draws a link cursor, and dropping the
	// bit here left a link-only drag with no allowed effect at all.
	acts := uint32(availableActions & (DndCopy | DndMove | DndLink))
	// No re-entrancy guard here, unlike dndActive on the GLFW side: OLE runs its
	// own modal loop with the mouse captured and swallows every mouse and key
	// message, so no widget handler runs until DoDragDrop returns. The GLFW
	// backend pumps events itself inside its drag loop and does need the guard.
	effect, err := win32.DoDragDrop(do, ds, acts)
	privateDndData = nil
	if err != nil {
		core.Warn(err)
		return 0
	}
	return DndAction(effect)
}
