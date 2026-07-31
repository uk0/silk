//go:build !windows

package gui

import (
	"testing"

	"github.com/go-gl/glfw/v3.3/glfw"
)

// TestVkToGLFWKeysCoversBothModifierSides: KeyState probes exactly the keys
// this returns, so a modifier that omits either side reads as up whenever the
// user happens to be holding the other one. The reverse lookup walks a Go map,
// whose iteration order is randomised per call — returning one arbitrary match
// dropped Ctrl on roughly a quarter of the calls and let Ctrl+arrow fall
// through to the plain-arrow nudge. Repeated so a lucky ordering cannot pass.
func TestVkToGLFWKeysCoversBothModifierSides(t *testing.T) {
	cases := []struct {
		name  string
		vk    int
		left  glfw.Key
		right glfw.Key
	}{
		{"ctrl", KeyCtrl, glfw.KeyLeftControl, glfw.KeyRightControl},
		{"shift", KeyShift, glfw.KeyLeftShift, glfw.KeyRightShift},
		{"alt", KeyMenu, glfw.KeyLeftAlt, glfw.KeyRightAlt},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for i := 0; i < 200; i++ {
				keys := vkToGLFWKeys(tc.vk)
				var gotLeft, gotRight bool
				for _, k := range keys {
					gotLeft = gotLeft || k == tc.left
					gotRight = gotRight || k == tc.right
				}
				if !gotLeft || !gotRight {
					t.Fatalf("call %d: vkToGLFWKeys(%#x) = %v, missing left=%v right=%v",
						i, tc.vk, keys, !gotLeft, !gotRight)
				}
			}
		})
	}
}

// TestVkToGLFWKeysUnmappedIsEmpty: KeyState ranges over the result, so an
// unmapped VK must yield nothing to probe rather than a sentinel key.
func TestVkToGLFWKeysUnmappedIsEmpty(t *testing.T) {
	if keys := vkToGLFWKeys(0); len(keys) != 0 {
		t.Errorf("vkToGLFWKeys(0) = %v, want no keys", keys)
	}
}
