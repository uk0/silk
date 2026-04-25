package gui

import (
	"silk/core"
	"silk/paint"
	//	"silk/core"
	"silk/win32"
	"errors"
	"image"
	"os"
	"runtime"
	"strconv"
	"strings"
	"unsafe"
)

var (
	cursorArrow    = loadSystemCursor(win32.IDC_ARROW)
	cursorIBeam    = loadSystemCursor(win32.IDC_IBEAM)
	cursorWait     = loadSystemCursor(win32.IDC_WAIT)
	cursorCross    = loadSystemCursor(win32.IDC_CROSS)
	cursorUpArrow  = loadSystemCursor(win32.IDC_UPARROW)
	cursorSizeNWSE = loadSystemCursor(win32.IDC_SIZENWSE)
	cursorSizeNESW = loadSystemCursor(win32.IDC_SIZENESW)
	cursorSizeWE   = loadSystemCursor(win32.IDC_SIZEWE)
	cursorSizeNS   = loadSystemCursor(win32.IDC_SIZENS)
	cursorSizeAll  = loadSystemCursor(win32.IDC_SIZEALL)
	cursorNo       = loadSystemCursor(win32.IDC_NO)
	cursorHand     = loadSystemCursor(win32.IDC_HAND)
	//	cursorAppStart = loadSystemCursor(win32.IDC_APPSTARTING)
	cursorHelp = loadSystemCursor(win32.IDC_HELP)
	//	cursorIcon     = loadSystemCursor(win32.IDC_ICON)
	//	cursorSize     = loadSystemCursor(win32.IDC_SIZE)

	overrideCursor *Cursor

	dndCursorData []CursorData

	cursorCache = make(map[string]*Cursor)
)

func init() {
	cursorCache["arrow"] = cursorArrow
	cursorCache["ibeam"] = cursorIBeam
	cursorCache["wait"] = cursorWait
	cursorCache["cross"] = cursorCross
	cursorCache["up-arrow"] = cursorUpArrow
	cursorCache["size-nwse"] = cursorSizeNWSE
	cursorCache["size-nesw"] = cursorSizeNESW
	cursorCache["size-we"] = cursorSizeWE
	cursorCache["size-ns"] = cursorSizeNS
	cursorCache["size-all"] = cursorSizeAll
	cursorCache["no"] = cursorNo
	cursorCache["hand"] = cursorHand
	cursorCache["help"] = cursorHelp
}

// 光标数据
type CursorData struct {
	// 图标
	paint.Pixmap
	// 热点
	HotX, HotY int
}

// 光标的引用
type Cursor win32.HCURSOR

func loadSystemCursor(id int) *Cursor {
	hc := win32.LoadCursor(0, (*uint16)(unsafe.Pointer(uintptr(id))))
	if hc == 0 {
		return nil
	}
	ret := new(Cursor)
	*ret = Cursor(hc)
	return ret
}

// 获取操作系统的原生的光标对象
func (c *Cursor) Native() win32.HCURSOR {
	return win32.HCURSOR(*c)
}

// 和光标对象分离,
// 返回值是分离后的原生对象
// Detach()以后不再指向有效对象, 但分离出来的原生光标对象仍可用
func (c *Cursor) Detach() win32.HCURSOR {
	ret := win32.HCURSOR(*c)
	*c = 0
	return ret
}

func (c *Cursor) destroy() {
	if *c != 0 {
		win32.DestroyIcon(win32.HICON(*c))
		*c = 0
	}
}

// 设置全局覆盖光标, 此光标会覆盖控件/窗口的光标
func SetOverrideCursor(c *Cursor) (old *Cursor) {
	old = overrideCursor
	if overrideCursor == c {
		return
	}
	overrideCursor = c
	if overrideCursor == nil {
		if mouseHoverWidget != nil && mouseHoverWidget.Cursor() != nil {
			win32.SetCursor(mouseHoverWidget.Cursor().Native())
			return
		}
		win32.SetCursor(cursorArrow.Native())
		return
	}
	win32.SetCursor(overrideCursor.Native())
	return
}

// 用指定图标生成光标
func NewCursorFromIcon(icon paint.Icon, sz, hotX, hotY int) (*Cursor, error) {
	return NewCursorFromData(CursorData{icon.Pixmap(sz), hotX, hotY})
}

// 从光标数据生成光标
func NewCursorFromData(data CursorData) (*Cursor, error) {
	if data.Pixmap == nil {
		return nil, core.StrErr("nil pixmap")
	}

	if data.Pixmap.Format() != paint.FormatARGB32 {
		return nil, core.StrErr("unsupported format")
	}

	width := data.Pixmap.Width()
	height := data.Pixmap.Height()

	img, err := data.Pixmap.Image()
	if err != nil {
		return nil, err
	}

	var bi win32.BITMAPV5HEADER
	bi.BV5Size = uint32(unsafe.Sizeof(bi))
	bi.BV5Width = int32(width)
	bi.BV5Height = int32(height)
	bi.BV5Planes = 1
	bi.BV5BitCount = 32
	bi.BV5Compression = win32.BI_BITFIELDS
	bi.BV5RedMask = 0x00FF0000
	bi.BV5GreenMask = 0x0000FF00
	bi.BV5BlueMask = 0x000000FF
	bi.BV5AlphaMask = 0xFF000000

	var lpBits unsafe.Pointer

	hdc := win32.GetDC(0)
	hBitmap := win32.CreateDIBSection(hdc, (*win32.BITMAPINFO)(unsafe.Pointer(&bi)),
		win32.DIB_RGB_COLORS, &lpBits, 0, 0)
	//hMemDC := win32.CreateCompatibleDC(hdc)
	win32.ReleaseDC(0, hdc)

	//win32.DeleteDC(hMemDC)
	srcImg := img.(*image.RGBA)
	length := width * height * 4
	//pData.Pix
	src := srcImg.Pix
	dst := (*(*[1 << 30]byte)(lpBits))[:length]
	//copy(dst, src)
	// swap Red and Blue channel, flip up down
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			i := (x + y*width) * 4
			j := (x * 4) + (height-y-1)*srcImg.Stride
			dst[i] = src[j+2]
			dst[i+1] = src[j+1]
			dst[i+2] = src[j]
			dst[i+3] = src[j+3]
		}
	}

	hMonoBitmap := win32.CreateBitmap(int32(width), int32(height), 1, 1, nil)

	var ii win32.ICONINFO
	ii.Icon = 0
	ii.XHotspot = uint32(data.HotX)
	ii.YHotspot = uint32(data.HotY)
	ii.HbmMask = hMonoBitmap
	ii.HbmColor = hBitmap

	hCursor := win32.CreateIconIndirect(&ii)

	win32.DeleteObject(win32.HGDIOBJ(hBitmap))
	win32.DeleteObject(win32.HGDIOBJ(hMonoBitmap))

	//return hCursor, nil
	ret := new(Cursor)
	*ret = Cursor(hCursor)
	runtime.SetFinalizer(ret, (*Cursor).destroy)
	return ret, nil

}

// 从文件加载光标数据
func LoadCursorData(name string) (data CursorData, err error) {
	dir, err := os.Open(core.ResourceDir() + "/cursor")
	if err != nil {
		return
	}
	defer dir.Close()

	infos, err := dir.Readdir(-1)
	for _, info := range infos {
		n := info.Name()
		if info.IsDir() {
			continue
		}
		ln := strings.ToLower(n)
		if !strings.HasSuffix(ln, ".png") {
			continue
		}
		//strings.Split(ln, "-")
		ln = ln[:len(ln)-4]
		parts := strings.Split(ln, "_")
		if strings.ToLower(name) != parts[0] {
			continue
		}
		data.Pixmap, err = paint.LoadPngFile(core.ResourceDir() + "/cursor/" + n)
		if err != nil {
			return
		}

		if len(parts) == 3 {
			data.HotX, _ = strconv.Atoi(parts[1])
			data.HotY, _ = strconv.Atoi(parts[2])
		}
		return
	}

	err = errors.New(`cursor not found: "` + name + `"`)
	return
}

// 默认的箭头光标
func DefaultCursor() *Cursor {
	return cursorArrow
}

// 加载光标
func LoadCursor(name string) *Cursor {
	cur, ok := cursorCache[name]
	if !ok {
		data, err := LoadCursorData(name)
		if err == nil {
			cur, _ = NewCursorFromData(data)
		}
		if cur == nil {
			cur = cursorArrow
		}
		cursorCache[name] = cur
	}
	return cur
}

// 生成一组表示拖放的光标, content是拖放内容的缩略图
func GenerateDropCursors(content paint.Pixmap) (curs []*Cursor) {
	if dndCursorData == nil {
		dndCursorData = make([]CursorData, 4)
		var err error
		dndCursorData[0], err = LoadCursorData("arrow-no")
		if err != nil {
			core.Warn(err)
		}
		dndCursorData[1], err = LoadCursorData("arrow")
		if err != nil {
			core.Warn(err)
		}
		dndCursorData[2], err = LoadCursorData("arrow-copy")
		if err != nil {
			core.Warn(err)
		}
		dndCursorData[3], err = LoadCursorData("arrow-link")
		if err != nil {
			core.Warn(err)
		}
	}

	for _, arrow := range dndCursorData {
		w := content.Width()
		h := content.Height()
		hotX := w / 2
		hotY := h / 2
		w1 := hotX + arrow.Pixmap.Width() - arrow.HotX
		h1 := hotY + arrow.Pixmap.Height() - arrow.HotY
		if w1 > w {
			w = w1
		}
		if h1 > h {
			h = h1
		}

		pixmap := paint.NewPixmap(w, h)
		g := pixmap.NewPainter()
		g.DrawPixmap2(0, 0, arrow.Pixmap,
			float64(hotX-arrow.HotX), float64(hotY-arrow.HotY))
		/*
			g.SetSourceSurface(content, 0, 0)
			g.PaintWithAlpha(160)
			g.SetSourceSurface(arrow.Pixmap,
				float64(hotX-arrow.HotX), float64(hotY-arrow.HotY))
			g.Paint()
		*/
		cur, _ := NewCursorFromData(CursorData{pixmap, hotX, hotY})
		curs = append(curs, cur)
	}
	return
}
