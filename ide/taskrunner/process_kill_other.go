//go:build !windows

package taskrunner

import "os"

func killProcessFallback(p *os.Process) error { return p.Kill() }
