package gui

import (
	//	"github.com/uk0/silk/diag"
	"errors"
	"github.com/uk0/silk/win32"
	//	"reflect"
	"github.com/uk0/silk/core"
	"syscall"
	"unsafe"
)

type clipBoard int // 给clipboard一个虚假的类型, 使其可拥有接口

// 全局唯一的剪贴板对象
var Clipboard clipBoard

type clipboardFormat struct {
	id     win32.CLIPFORMAT
	format string
}

const CF_PERSIST = win32.CF_PRIVATEFIRST + 74
const CF_UNUSED = win32.CF_PRIVATEFIRST + 7

var clipboardFormats = []clipboardFormat{
	clipboardFormat{win32.CF_UNICODETEXT, "text/plain"},
	clipboardFormat{win32.CF_TEXT, "text/plain"},
	//clipboardFormat{win32.CF_TEXT, "golang/*interface{}"},
	clipboardFormat{CF_PERSIST, "application/x-silk-persist"},
}

func (this *clipBoard) Formats() (formats []string, err error) {
	if !win32.OpenClipboard(win32.HWND(AnyWindowId())) {
		err = clipboardError("OpenClipboard failed")
		return
	}
	defer win32.CloseClipboard()

	// Windows enumerates ids silk has no mime for (CF_LOCALE, CF_OEMTEXT,
	// ...) and synthesises CF_TEXT alongside CF_UNICODETEXT, so the raw
	// list yields "" entries and text/plain twice — neither of which the
	// GLFW backend can ever produce.
	seen := make(map[string]bool)
	id := win32.EnumClipboardFormats(0)
	for id != 0 {
		f := clipboardIdToFormat(win32.CLIPFORMAT(id))
		if f != "" && !seen[f] {
			seen[f] = true
			formats = append(formats, f)
		}
		id = win32.EnumClipboardFormats(id)
	}
	return
}

// 读剪贴板数据
func (this *clipBoard) Data(format string) (data interface{}, err error) {
	if !win32.OpenClipboard(win32.HWND(AnyWindowId())) {
		err = clipboardError("OpenClipboard failed")
		return
	}
	defer win32.CloseClipboard()
	for _, fp := range clipboardFormats {
		if fp.format == format && win32.IsClipboardFormatAvailable(uint(fp.id)) {
			hBuf := win32.HGLOBAL(win32.GetClipboardData(uint(fp.id)))
			if hBuf != 0 {
				return decodeClipFormat(hBuf, fp.id)
			}
			err = clipboardError("GetClipboardData failed")
		}
	}
	if err == nil {
		// An absent format must not report success: Edit.paste type-asserts
		// the result whenever err is nil, and (nil, nil) panics it.
		err = core.StrErr("format not available: " + format)
	}
	return
}

// 往剪贴板里添加数据
func (this *clipBoard) SetData(data interface{}) (format string, err error) {
	hGlobal, format, id, err := encodeClipFormat(data)
	if err != nil {
		return format, err
	}
	err = setClipboardData(hGlobal, id)
	return format, err
}

// 清除剪贴板数据
func (this *clipBoard) Clear() error {
	if !win32.OpenClipboard(win32.HWND(AnyWindowId())) {
		return clipboardError("OpenClipboard failed")
	}
	defer win32.CloseClipboard()
	if !win32.EmptyClipboard() {
		return clipboardError("EmptyClipboard failed")
	}
	return nil
}

// 内部格式对应的mime
func clipboardIdToFormat(id win32.CLIPFORMAT) string {
	for _, f := range clipboardFormats {
		if f.id == id {
			return f.format
		}
	}
	return ""
}

// clipboardError reports why a win32 clipboard call failed. The thread
// error code is best-effort — syscall.GetLastError yields nil when the
// code is 0 and the runtime may clobber it before we read it — so a
// context string always backs it up. A failure path that returns a nil
// error is what hands Data() callers a (nil, nil) pair.
func clipboardError(what string) error {
	if err := syscall.GetLastError(); err != nil {
		return err
	}
	return core.StrErr(what)
}

func setClipboardData(hBuf win32.HGLOBAL, id win32.CLIPFORMAT) error {

	if !win32.OpenClipboard(win32.HWND(AnyWindowId())) {
		return clipboardError("OpenClipboard failed")
	}
	defer win32.CloseClipboard()

	// SetClipboardData only accepts data from the clipboard owner, and
	// ownership is what EmptyClipboard hands us. Without it a SetData that
	// is not preceded by Clear — TestResultsPanel.clipboardWrite does
	// exactly that — fails whenever another app owns the clipboard, while
	// the GLFW backend just replaces the content.
	if !win32.EmptyClipboard() {
		return clipboardError("EmptyClipboard failed")
	}

	if win32.SetClipboardData(uint(id), win32.HANDLE(hBuf)) == 0 {
		return clipboardError("SetClipboardData failed")
	}
	return nil
}

func bufToHGlobal(buf []byte) (win32.HGLOBAL, error) {
	length := len(buf)
	hBuf := win32.HGLOBAL(win32.GlobalAlloc(win32.GMEM_MOVEABLE, uint32(length)))
	if hBuf == 0 {
		return 0, clipboardError("GlobalAlloc failed")
	}
	p := win32.GlobalLock(hBuf)
	if p == nil {
		err := clipboardError("GlobalLock failed")
		win32.GlobalFree(hBuf)
		return 0, err
	}
	defer win32.GlobalUnlock(hBuf)
	buf1 := (*[1 << 30]byte)(p)[0:length]
	copy(buf1, buf)
	return hBuf, nil
}

func encodeClipFormat(data interface{}) (win32.HGLOBAL, string, win32.CLIPFORMAT, error) {
	switch x := data.(type) {
	case core.PersistData:
		if x == nil {
			return 0, "", 0, errors.New("nil pointer")
		}
		a, _, _, err := encodeClipFormat(((*core.TDoc)(x)).String())
		if err != nil {
			return 0, "", 0, err
		}
		return a, "application/x-silk-persist", CF_PERSIST, nil
	case string:
		id := win32.CF_UNICODETEXT
		utf16, err := syscall.UTF16FromString(x)
		if err != nil {
			return 0, "", 0, err
		}
		buf := make([]byte, len(utf16)*2, len(utf16)*2)
		for i, n := range utf16 {
			buf[i*2] = byte(n & 0xff)
			buf[i*2+1] = byte((n >> 8) & 0xff)
		}
		hGlobal, err := bufToHGlobal(buf)
		if err != nil {
			return 0, "", 0, err
		}
		return hGlobal, "text/plain", id, nil
	default:
	}
	return 0, "", 0, errors.New("unsupported format")
}

func getDataFromBuf(id win32.CLIPFORMAT, buf unsafe.Pointer, length int) interface{} {
	switch id {
	case win32.CF_TEXT:
		bytes := (*[1 << 30]byte)(buf)[0:length]
		// GlobalSize rounds the block up, so the tail holds the NUL
		// terminator plus slack; the GLFW backend never yields those, and
		// keeping them pastes NUL runs into the editor buffer.
		n := 0
		for n < length && bytes[n] != 0 {
			n++
		}
		return string(bytes[0:n])
	case CF_PERSIST:
		fallthrough
	case win32.CF_UNICODETEXT:
		utf16 := (*[1 << 29]uint16)(buf)[0 : length/2]
		return syscall.UTF16ToString(utf16)
	default:
		bytes := (*[1 << 30]byte)(buf)[0:length]
		return bytes
	}
}

func decodeClipFormat(hGlobal win32.HGLOBAL, id win32.CLIPFORMAT) (data interface{}, err error) {
	if hGlobal == 0 || id == 0 {
		err = errors.New("invalid args")
		return
	}
	buf := win32.GlobalLock(hGlobal)
	if buf == nil {
		err = clipboardError("GlobalLock failed")
		return
	}
	data = getDataFromBuf(id, buf, win32.GlobalSize(hGlobal))
	win32.GlobalUnlock(hGlobal)
	if data == nil {
		err = errors.New("failed to decode")
		return
	}
	err = nil
	return
}
