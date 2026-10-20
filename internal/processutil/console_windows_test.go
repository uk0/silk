package processutil

import (
	"os/exec"
	"syscall"
	"testing"
)

func TestNoConsoleWindowPreservesProcessSettings(t *testing.T) {
	NoConsoleWindow(nil)
	cmd := exec.Command("go", "version")
	NoConsoleWindow(cmd)
	if cmd.SysProcAttr.CreationFlags != 0x08000000 || cmd.SysProcAttr.HideWindow {
		t.Fatal("background console policy is not console-only")
	}
	NoConsoleWindow(cmd)
	cmd.SysProcAttr.CreationFlags = 0x200 // Preserve process-group cancellation.
	attr := cmd.SysProcAttr
	NoConsoleWindow(cmd)
	if cmd.SysProcAttr != attr || attr.CreationFlags != 0x08000200 {
		t.Fatal("process settings were replaced")
	}
	for _, flags := range []uint32{0x10, 0x8} {
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: flags}
		NoConsoleWindow(cmd)
		if cmd.SysProcAttr.CreationFlags != flags {
			t.Fatal("explicit external console request was overridden")
		}
	}
}
