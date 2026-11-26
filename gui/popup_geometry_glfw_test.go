//go:build !windows

package gui

import (
	"fmt"
	"math"
	"os"
	"testing"
	"time"

	"github.com/go-gl/glfw/v3.3/glfw"
)

var nativePopupGeometryErr error

func TestGLFWNativePopupGeometry(t *testing.T) {
	if os.Getenv("SILK_TEST_NATIVE_POPUP_GEOMETRY") != "1" {
		t.Skip("opt-in native geometry readback: SILK_TEST_NATIVE_POPUP_GEOMETRY=1")
	}
	if nativePopupGeometryErr != nil {
		t.Fatal(nativePopupGeometryErr)
	}
}

// Runs on the OS main thread. Do not pre-size the popup: its actual size must
// be computed after attaching the hidden native window, as in the workbench.
func runNativePopupGeometryProbe() error {
	if !glfwReady {
		return fmt.Errorf("native popup geometry probe needs a desktop")
	}
	defer glfw.DefaultWindowHints()
	form := NewForm()
	form.SetVisible(false)
	form.SetSize(560, 320)
	if err := form.TryAttachWindow(WtForm); err != nil {
		return err
	}
	defer form.DetachWindow()
	anchor := NewButton1("Export", nil)
	anchor.SetParent(form)
	anchor.SetBounds(60, 40, 80, 30)
	menu := NewPopupMenu()
	menu.AddButton1("Export as SVG...", nil)
	menu.AddButton1("Export as PDF...", nil)
	anchor.SetSubPopup(menu)
	form.Show()
	defer form.Hide()
	defer menu.DetachWindow()
	defer menu.Hide()
	glfw.PollEvents()
	for pass := 0; pass < 2; pass++ {
		hints := menu.SizeHints()
		wantW, wantH := int(math.Ceil(hints.Width)), int(math.Ceil(hints.Height))
		anchor.ShowSubPopup()
		if menu.Window() == nil {
			return fmt.Errorf("pass %d: no native menu window", pass)
		}
		if w, h := menu.Size(); int(w) != wantW || int(h) != wantH {
			return fmt.Errorf("pass %d: widget size before next event is %gx%g, want %dx%d", pass, w, h, wantW, wantH)
		}
		win := menu.Window().NakedWindow()
		deadline := time.Now().Add(time.Second)
		for {
			glfw.PollEvents()
			w, h := win.glfwWin.GetSize()
			if win.IsVisible() && w == wantW && h == wantH {
				break
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("pass %d: native size %dx%d visible=%t, want %dx%d visible", pass, w, h, win.IsVisible(), wantW, wantH)
			}
			time.Sleep(5 * time.Millisecond)
		}
		anchor.HideSubPopup()
		glfw.PollEvents()
		if win.IsVisible() {
			return fmt.Errorf("pass %d: menu still visible after hide", pass)
		}
		menu.AddButton1("A longer new export destination...", nil)
	}
	return nil
}
