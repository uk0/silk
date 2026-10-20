//go:build !windows

package processutil

import "os/exec"

func NoConsoleWindow(cmd *exec.Cmd) {}
