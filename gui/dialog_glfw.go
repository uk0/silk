//go:build !windows

package gui

import (
	"bytes"
	"os/exec"
	"strings"

	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/uk0/silk/core"
)

// OpenFileDialog opens a native file dialog and returns the selected file path
func OpenFileDialog() string {
	return platformOpenFileDialog()
}

// SaveFileDialog opens a native save dialog and returns the selected file path
func SaveFileDialog() string {
	return platformSaveFileDialog()
}

// nativeDialogFrame pumps events and queued work while an external chooser
// owns input. Posting empty events without consuming them stalled the window.
var nativeDialogFrame = func() {
	glfw.WaitEventsTimeout(1.0 / 60.0)
	serviceUIFrame()
	for _, win := range winMap {
		if win != nil && win.dirty && win.IsVisible() {
			win.paint()
		}
	}
}

func runDialogCmd(name string, args ...string) string {
	cmd := exec.Command(name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		core.Warn("file dialog: ", err)
		return ""
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	// Modal semantics: keep repainting, but don't let a second shortcut open
	// another chooser or mutate the document being saved underneath this one.
	enable := disableForExternalFileDialog()
	defer enable()
	for {
		select {
		case err := <-done:
			if err != nil {
				// AppleScript uses -128 for an ordinary user cancellation.
				if message := strings.TrimSpace(stderr.String()); message != "" && !strings.Contains(message, "(-128)") {
					core.Warn("file dialog: ", message)
				}
				return ""
			}
			return strings.TrimSpace(stdout.String())
		default:
			if shouldQuit {
				_ = cmd.Process.Kill()
				<-done
				return ""
			}
			nativeDialogFrame()
		}
	}
}

// Re-enable only native windows from the original snapshot that are still
// registered; UI work during a dialog can close or replace a window.
func disableForExternalFileDialog() func() {
	enabled := make(map[*Window]bool)
	for _, win := range winMap {
		if win != nil {
			enabled[win] = win.IsEnabled()
			win.SetEnabled(false)
		}
	}
	return func() {
		for _, win := range winMap {
			if wasEnabled, ok := enabled[win]; ok {
				win.SetEnabled(wasEnabled)
			}
		}
	}
}

func linuxOpenFileDialog() string {
	return runDialogCmd("zenity", "--file-selection",
		"--title=Open SilkUI File",
		"--file-filter=SilkUI Files (*.silkui) | *.silkui",
		"--file-filter=Legacy Design Files (*.cml *.silk *.form) | *.cml *.silk *.form",
		"--file-filter=All Files | *")
}

func linuxSaveFileDialog() string {
	return runDialogCmd("zenity", "--file-selection", "--save",
		"--title=Save SilkUI File",
		"--filename=design.silkui",
		"--file-filter=SilkUI Files (*.silkui) | *.silkui",
		"--file-filter=All Files | *")
}
