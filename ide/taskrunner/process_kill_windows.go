//go:build windows

package taskrunner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/uk0/silk/internal/processutil"
)

// Windows has no Unix process groups. Stop this task's PID and descendants,
// never every executable with the same name. Use the system utility directly
// (no shell/PATH lookup when SystemRoot is available) and bound its runtime.
func killProcessFallback(p *os.Process) error {
	tool := "taskkill.exe"
	if root := os.Getenv("SystemRoot"); root != "" {
		tool = filepath.Join(root, "System32", tool)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, tool, "/PID", strconv.Itoa(p.Pid), "/T", "/F")
	processutil.NoConsoleWindow(cmd)
	if err := cmd.Run(); err != nil {
		if fallback := p.Kill(); fallback != nil {
			return fmt.Errorf("taskkill: %v; direct child: %w", err, fallback)
		}
		return fmt.Errorf("taskkill failed (%v); direct child stopped, descendant cleanup not confirmed", err)
	}
	return nil
}
