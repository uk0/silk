//go:build !windows

package gui

import (
	"silk/core"
	"silk/paint"
	"errors"
	"os"
	"strconv"
	"strings"

	"github.com/go-gl/glfw/v3.3/glfw"
)

var (
	cursorArrow    *Cursor
	cursorIBeam    *Cursor
	cursorCross    *Cursor
	cursorHand     *Cursor
	cursorSizeWE   *Cursor
	cursorSizeNS   *Cursor
	cursorWait     *Cursor
	cursorUpArrow  *Cursor
	cursorSizeNWSE *Cursor
	cursorSizeNESW *Cursor
	cursorSizeAll  *Cursor
	cursorNo       *Cursor
	cursorHelp     *Cursor

	overrideCursor *Cursor

	dndCursorData []CursorData

	cursorCache = make(map[string]*Cursor)
)

func initCursors() {
	cursorArrow = loadSystemCursor(glfw.ArrowCursor)
	cursorIBeam = loadSystemCursor(glfw.IBeamCursor)
	cursorCross = loadSystemCursor(glfw.CrosshairCursor)
	cursorHand = loadSystemCursor(glfw.HandCursor)
	cursorSizeWE = loadSystemCursor(glfw.HResizeCursor)
	cursorSizeNS = loadSystemCursor(glfw.VResizeCursor)
	cursorWait = loadSystemCursor(glfw.ArrowCursor)
	cursorUpArrow = loadSystemCursor(glfw.ArrowCursor)
	cursorSizeNWSE = loadSystemCursor(glfw.ArrowCursor)
	cursorSizeNESW = loadSystemCursor(glfw.ArrowCursor)
	cursorSizeAll = loadSystemCursor(glfw.ArrowCursor)
	cursorNo = loadSystemCursor(glfw.ArrowCursor)
	cursorHelp = loadSystemCursor(glfw.ArrowCursor)

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

// CursorData holds cursor image data
type CursorData struct {
	paint.Pixmap
	HotX, HotY int
}

// Cursor wraps a GLFW cursor
type Cursor struct {
	glfwCursor *glfw.Cursor
}

func loadSystemCursor(shape glfw.StandardCursor) *Cursor {
	c := glfw.CreateStandardCursor(shape)
	if c == nil {
		return &Cursor{}
	}
	return &Cursor{glfwCursor: c}
}

func (c *Cursor) apply() {
	if c == nil || c.glfwCursor == nil {
		return
	}
	for gw := range winMap {
		gw.SetCursor(c.glfwCursor)
	}
}

func (c *Cursor) destroy() {
	if c != nil && c.glfwCursor != nil {
		c.glfwCursor.Destroy()
		c.glfwCursor = nil
	}
}

// SetOverrideCursor sets a global override cursor
func SetOverrideCursor(c *Cursor) (old *Cursor) {
	old = overrideCursor
	if overrideCursor == c {
		return
	}
	overrideCursor = c
	if overrideCursor == nil {
		if mouseHoverWidget != nil && mouseHoverWidget.Cursor() != nil {
			mouseHoverWidget.Cursor().apply()
			return
		}
		cursorArrow.apply()
		return
	}
	overrideCursor.apply()
	return
}

// NewCursorFromIcon creates a cursor from an icon
func NewCursorFromIcon(icon paint.Icon, sz, hotX, hotY int) (*Cursor, error) {
	return NewCursorFromData(CursorData{icon.Pixmap(sz), hotX, hotY})
}

// NewCursorFromData creates a cursor from cursor data
func NewCursorFromData(data CursorData) (*Cursor, error) {
	if data.Pixmap == nil {
		return nil, core.StrErr("nil pixmap")
	}

	img, err := data.Pixmap.Image()
	if err != nil {
		return nil, err
	}

	c := glfw.CreateCursor(img, data.HotX, data.HotY)
	if c == nil {
		return nil, core.StrErr("failed to create cursor from image")
	}

	return &Cursor{glfwCursor: c}, nil
}

// LoadCursorData loads cursor data from a file
func LoadCursorData(name string) (data CursorData, err error) {
	dir, err := os.Open(core.ResourceDir() + "/cursor")
	if err != nil {
		return
	}
	defer dir.Close()

	infos, err := dir.Readdir(-1)
	if err != nil {
		return
	}
	for _, info := range infos {
		n := info.Name()
		if info.IsDir() {
			continue
		}
		ln := strings.ToLower(n)
		if !strings.HasSuffix(ln, ".png") {
			continue
		}
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

// DefaultCursor returns the default arrow cursor
func DefaultCursor() *Cursor {
	return cursorArrow
}

// LoadCursor loads a named cursor, using cache
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

// GenerateDropCursors generates a set of DnD cursors with the given content thumbnail
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
		if arrow.Pixmap == nil {
			curs = append(curs, cursorArrow)
			continue
		}
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

		cur, _ := NewCursorFromData(CursorData{pixmap, hotX, hotY})
		if cur == nil {
			cur = cursorArrow
		}
		curs = append(curs, cur)
	}
	return
}
