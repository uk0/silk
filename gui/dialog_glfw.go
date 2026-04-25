//go:build !windows

package gui

import (
	"bytes"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/go-gl/glfw/v3.3/glfw"
)

// OpenFileDialog opens a native file dialog and returns the selected file path
func OpenFileDialog() string {
	switch runtime.GOOS {
	case "darwin":
		return macOpenFileDialog()
	default:
		return linuxOpenFileDialog()
	}
}

// SaveFileDialog opens a native save dialog and returns the selected file path
func SaveFileDialog() string {
	switch runtime.GOOS {
	case "darwin":
		return macSaveFileDialog()
	default:
		return linuxSaveFileDialog()
	}
}

// runDialogCmd runs an external command (e.g. osascript, zenity) without
// blocking the GLFW event loop. It posts empty events at ~60fps so the
// window system stays responsive while the modal dialog is open.
func runDialogCmd(name string, args ...string) string {
	cmd := exec.Command(name, args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Start(); err != nil {
		return ""
	}

	done := make(chan struct{})
	go func() {
		cmd.Wait()
		close(done)
	}()

	for {
		select {
		case <-done:
			return strings.TrimSpace(stdout.String())
		default:
			glfw.PostEmptyEvent()
			time.Sleep(16 * time.Millisecond)
		}
	}
}

func macOpenFileDialog() string {
	// Restrict selection to SilkUI design files (extension-based),
	// while still accepting legacy .cml / .silk / .form files.
	script := `set theFile to POSIX path of (choose file with prompt "Open SilkUI File" of type {"silkui", "cml", "silk", "form"})
return theFile`
	return runDialogCmd("osascript", "-e", script)
}

func macSaveFileDialog() string {
	// osascript's "choose file name" does not take a file type filter,
	// but we can seed the default filename with a .silkui extension so
	// users get the right suffix by default.
	script := `set theFile to POSIX path of (choose file name with prompt "Save SilkUI File" default name "design.silkui")
return theFile`
	return runDialogCmd("osascript", "-e", script)
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
