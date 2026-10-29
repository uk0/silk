//go:build !windows

package gui

import (
	"github.com/go-gl/glfw/v3.3/glfw"
	"testing"
)

func TestGLFWPointerTargetUsesNativeOcclusionNotMapOrder(t *testing.T) {
	owner := &Window{enabled: true}
	dialog := &Window{enabled: true, inModal: true}
	popup := &Window{enabled: true, wt: WtPopup}
	other := &Window{enabled: true}
	windows := map[*glfw.Window]*Window{new(glfw.Window): owner, new(glfw.Window): dialog, new(glfw.Window): popup, new(glfw.Window): other}
	for _, tc := range []struct {
		name                                  string
		hover, focus, outside, disabled, want *Window
	}{
		{name: "modal-over-owner", hover: dialog, focus: dialog, disabled: owner, want: dialog},
		{name: "release-without-hover", focus: dialog, disabled: owner, want: dialog},
		{name: "nonmodal-over-focused-owner", hover: other, focus: owner, want: other},
		{name: "popup-over-dialog", hover: popup, focus: dialog, want: popup},
		{name: "disabled-occlusion", hover: dialog, focus: owner, disabled: dialog},
		{name: "hidden-or-outside", hover: popup, focus: dialog, outside: popup, want: dialog},
		{name: "all-background"},
		{name: "outside-focused", focus: dialog, outside: dialog},
		{name: "disabled-focused-owner", focus: owner, disabled: owner},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, w := range windows {
				w.enabled = w != tc.disabled
			}
			for range 256 {
				got := findGLFWPointerWindow(windows, func(w *Window) bool { return w != tc.outside }, func(w *Window) bool { return w == tc.hover }, func(w *Window) bool { return w == tc.focus })
				if got != tc.want {
					t.Fatalf("target=%p want=%p", got, tc.want)
				}
			}
		})
	}
}
