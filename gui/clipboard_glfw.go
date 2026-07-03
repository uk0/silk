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
	gw := getAnyGLFWWindow()
	if gw == nil {
		return nil, core.StrErr("no window available")
	}
	text := gw.GetClipboardString()
	if text != "" {
		formats = append(formats, "text/plain")
	}
	clipboardMu.Lock()
	for k := range clipboardLocalData {
		formats = append(formats, k)
	}
	clipboardMu.Unlock()
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
			return nil, core.StrErr("no window available")
		}
		text := gw.GetClipboardString()
		if text != "" {
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
		s := ((*core.TDoc)(x)).String()
		gw := getAnyGLFWWindow()
		if gw != nil {
			gw.SetClipboardString(s)
		}
		clipboardMu.Lock()
		clipboardLocalData["application/x-silk-persist"] = s
		clipboardMu.Unlock()
		return "application/x-silk-persist", nil
	case string:
		gw := getAnyGLFWWindow()
		if gw != nil {
			gw.SetClipboardString(x)
		}
		return "text/plain", nil
	default:
		// Store locally for non-standard types
		clipboardMu.Lock()
		clipboardLocalData["application/octet-stream"] = data
		clipboardMu.Unlock()
		return "application/octet-stream", nil
	}
}

func (this *clipBoard) Clear() error {
	gw := getAnyGLFWWindow()
	if gw != nil {
		gw.SetClipboardString("")
	}
	clipboardMu.Lock()
	clipboardLocalData = make(map[string]interface{})
	clipboardMu.Unlock()
	return nil
}
