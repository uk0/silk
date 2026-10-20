package processutil

import (
	"os/exec"
	"syscall"
)

// NoConsoleWindow is for piped/background commands, not interactive external
// terminals. CREATE_NO_WINDOW affects console programs only, leaving a child
// GUI application's windows alone. Explicit console/detached flags take priority.
// Existing process-group, environment and cancellation settings are preserved.
func NoConsoleWindow(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	const createNewConsole, detachedProcess, createNoWindow = 0x10, 0x8, 0x08000000
	if cmd.SysProcAttr.CreationFlags&(createNewConsole|detachedProcess) == 0 {
		cmd.SysProcAttr.CreationFlags |= createNoWindow
	}
}
