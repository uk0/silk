//go:build !windows

package gui

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
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

// Cursor wraps a GLFW cursor
type Cursor struct {
	glfwCursor *glfw.Cursor
}

func loadSystemCursor(shape glfw.StandardCursor) *Cursor {
	if !glfwReady {
		return &Cursor{}
	}
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
	if err := nativeDesktopError("NewCursorFromData"); err != nil {
		return nil, err
	}
	if err := validateCursorData(data); err != nil {
		return nil, err
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

// DefaultCursor returns the default arrow cursor
func DefaultCursor() *Cursor {
	return cursorArrow
}

// LoadCursor loads a named cursor, using cache
func LoadCursor(name string) *Cursor {
	name = strings.ToLower(name)
	cur, ok := cursorCache[name]
	if !ok {
		data, err := LoadCursorData(name)
		if err == nil {
			cur, _ = NewCursorFromData(data)
			paint.CloseSurface(data.Pixmap)
		}
		if cur == nil {
			cur = cursorArrow
		}
		cursorCache[name] = cur
	}
	return cur
}
