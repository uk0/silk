//go:build !windows

package gui

import (
	"testing"

	"github.com/go-gl/glfw/v3.3/glfw"
)

func TestEnabledGLFWMouseDispatchGatesEveryButton(t *testing.T) {
	for _, button := range []glfw.MouseButton{glfw.MouseButtonLeft, glfw.MouseButtonRight, glfw.MouseButtonMiddle} {
		t.Run(buttonName(button), func(t *testing.T) {
			isolateEnabledTest(t)
			root := NewVBox()
			root.SetSize(300, 200)
			probe := new(enabledLegacyProbe)
			probe.Init(probe)
			probe.SetParent(root)
			probe.SetBounds(10, 10, 100, 50)
			win := &Window{widget: root, enabled: true, wt: WtForm}
			root.win, root.wt = win, WtForm
			root.SetEnabled(false)
			dispatchMouseButtonAt(win, button, glfw.Press, 30, 25)
			dispatchMouseButtonAt(win, button, glfw.Release, 30, 25)
			if probe.downs != 0 || probe.ups != 0 || win.toCapture {
				t.Fatal("disabled descendant accepted a native mouse event or armed capture")
			}
			root.SetEnabled(true)
			dispatchMouseButtonAt(win, button, glfw.Press, 30, 25)
			root.SetEnabled(false)
			dispatchMouseButtonAt(win, button, glfw.Release, 30, 25)
			if probe.downs != 1 || probe.ups != 0 {
				t.Fatal("disabling between press and release delivered a stale release")
			}
		})
	}
}

func buttonName(button glfw.MouseButton) string {
	switch button {
	case glfw.MouseButtonLeft:
		return "left"
	case glfw.MouseButtonRight:
		return "right"
	default:
		return "middle"
	}
}
