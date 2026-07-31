//go:build !windows

package gui

import (
	"github.com/uk0/silk/core"
	"sync"

	"github.com/go-gl/glfw/v3.3/glfw"
)

type clipBoard int

// Clipboard is the global clipboard object
var Clipboard clipBoard

// Process-local storage for non-text data
var clipboardMu sync.Mutex
var clipboardLocalData = make(map[string]interface{})

func getAnyGLFWWindow() *glfw.Window {
	for gw := range winMap {
		return gw
	}
	return nil
}

func (this *clipBoard) Formats() (formats []string, err error) {
	// Local formats need no window and Data serves them in that state, so
	// list them before the window check rather than hiding data we can
	// still hand out.
	clipboardMu.Lock()
	for k := range clipboardLocalData {
		formats = append(formats, k)
	}
	clipboardMu.Unlock()
	gw := getAnyGLFWWindow()
	if gw == nil {
		return formats, errNoClipboardWindow
	}
	text := gw.GetClipboardString()
	if clipboardPayloadPresent(text) {
		formats = append(formats, "text/plain")
	}
	return
}

func (this *clipBoard) Data(format string) (data interface{}, err error) {
	// Check local data first
	clipboardMu.Lock()
	d, ok := clipboardLocalData[format]
	clipboardMu.Unlock()
	if ok {
		return d, nil
	}

	if format == "text/plain" {
		gw := getAnyGLFWWindow()
		if gw == nil {
			return nil, errNoClipboardWindow
		}
		text := gw.GetClipboardString()
		if clipboardPayloadPresent(text) {
			return text, nil
		}
	}
	return nil, core.StrErr("format not available: " + format)
}

func (this *clipBoard) SetData(data interface{}) (format string, err error) {
	switch x := data.(type) {
	case core.PersistData:
		if x == nil {
			return "", core.StrErr("nil pointer")
		}
		// The serialised document deliberately stays out of the OS text
		// clipboard: Windows keeps it under the private CF_PERSIST format,
		// and mirroring it into text/plain here made Ctrl+V in any text
		// field paste a whole TDoc dump on non-Windows only.
		clipboardMu.Lock()
		clipboardLocalData["application/x-silk-persist"] = ((*core.TDoc)(x)).String()
		clipboardMu.Unlock()
		return "application/x-silk-persist", nil
	case string:
		// Vet the text before hunting for a window, the way win32 vets it
		// before it opens the clipboard: SetClipboardString hands GLFW a C
		// string, so an embedded NUL would put a silently truncated copy on
		// the clipboard and still report success.
		if err := clipboardTextErr(x); err != nil {
			return "", err
		}
		gw := getAnyGLFWWindow()
		if gw == nil {
			return "", errNoClipboardWindow
		}
		gw.SetClipboardString(x)
		return "text/plain", nil
	default:
		// The Windows backend rejects these outright; storing them in a
		// process-local slot nothing reads only made a copy look like it
		// succeeded on one platform.
		return "", core.StrErr("unsupported format")
	}
}

func (this *clipBoard) Clear() error {
	// Drop the local formats first so a windowless Clear still discards
	// what Data would otherwise keep serving.
	clipboardMu.Lock()
	clipboardLocalData = make(map[string]interface{})
	clipboardMu.Unlock()
	gw := getAnyGLFWWindow()
	if gw == nil {
		return errNoClipboardWindow
	}
	gw.SetClipboardString("")
	return nil
}
