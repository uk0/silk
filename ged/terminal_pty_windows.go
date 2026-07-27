//go:build windows

package ged

// Windows placeholder for the pseudo-terminal session implemented in
// terminal_pty_unix.go, so package ged builds on Windows with the same
// internal API.
//
// A real implementation needs ConPTY (Windows 10 1809+): CreatePseudoConsole
// over a pair of anonymous pipes, process creation through
// STARTUPINFOEX + PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, ResizePseudoConsole
// for SIGWINCH-equivalent geometry changes, and ClosePseudoConsole on
// teardown. None of those are wrapped by the win32 bindings in this repo yet.
//
// TODO(silk): add the ConPTY bindings to win32/ and implement the session
// here; the ANSI state machine and TerminalPanel need no changes because
// ConPTY already speaks VT sequences. Until then TerminalPanel keeps working
// on Windows through its built-in command runner.

import "errors"

// errPTYUnsupported is what every session operation returns on Windows.
var errPTYUnsupported = errors.New("terminal: pty sessions are not supported on Windows yet (ConPTY not implemented)")

// terminalSession mirrors the unix session type but never runs anything.
type terminalSession struct {
	onData func([]byte)
	onExit func(error)
}

func newTerminalSession(onData func([]byte), onExit func(error)) *terminalSession {
	return &terminalSession{onData: onData, onExit: onExit}
}

// Start always fails: see the ConPTY note above.
func (s *terminalSession) Start(shell, dir string, env []string) error {
	return errPTYUnsupported
}

// Write always fails because no session can be running.
func (s *terminalSession) Write(p []byte) (int, error) { return 0, errPTYUnsupported }

// Resize always fails because no session can be running.
func (s *terminalSession) Resize(rows, cols int) error { return errPTYUnsupported }

// Close is a no-op: there is nothing to tear down.
func (s *terminalSession) Close() error { return nil }

// Running always reports false.
func (s *terminalSession) Running() bool { return false }
