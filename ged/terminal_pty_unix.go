//go:build !windows

package ged

// Persistent shell sessions on a real pseudo terminal.
//
// A pty gives the child process an actual controlling terminal, so the shell
// runs interactively: line editing, job control, SIGINT from ^C, SIGWINCH on
// resize and full ANSI output all work, none of which a pipe can provide.
// Output is read on a goroutine and handed to the onData callback; the raw
// bytes go straight into AnsiTerm (see terminal_ansi.go).

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"unsafe"
)

// ptyWinsize mirrors struct winsize from <sys/ioctl.h>.
type ptyWinsize struct {
	rows   uint16
	cols   uint16
	xpixel uint16
	ypixel uint16
}

// ptyReqs holds the ioctl request numbers needed to allocate a pty. The
// symbolic constants live in GOOS-specific halves of package syscall
// (TIOCPTYGNAME exists only on darwin, TIOCGPTN only on linux), so this
// file — compiled for every non-Windows target — carries the numbers and
// picks them at run time instead. The values are the ones shared by the
// amd64 and arm64 layouts of each platform; a platform without an entry
// reports "unsupported" rather than issuing a wrong ioctl.
type ptyReqs struct {
	grant    uintptr // TIOCPTYGRANT (darwin); 0 when not needed
	unlock   uintptr // TIOCPTYUNLK (darwin) / TIOCSPTLCK (linux)
	name     uintptr // TIOCPTYGNAME (darwin) / TIOCGPTN (linux)
	winsize  uintptr // TIOCSWINSZ
	numbered bool    // name yields a /dev/pts index instead of a device path
}

func ptyRequests() (ptyReqs, bool) {
	switch runtime.GOOS {
	case "darwin":
		return ptyReqs{
			grant:   0x20007454,
			unlock:  0x20007452,
			name:    0x40807453,
			winsize: 0x80087467,
		}, true
	case "linux":
		return ptyReqs{
			unlock:   0x40045431,
			name:     0x80045430,
			winsize:  0x5414,
			numbered: true,
		}, true
	}
	return ptyReqs{}, false
}

// terminalSession is a persistent shell attached to a pseudo terminal.
//
// onData is called on the reader goroutine with the bytes just read; the
// slice is reused between reads, so a consumer that needs to keep the data
// must copy it. onExit is called once, after the shell has been reaped.
// Both callbacks run off the UI thread.
type terminalSession struct {
	onData func([]byte)
	onExit func(error)

	mu       sync.Mutex
	ptmx     *os.File
	cmd      *exec.Cmd
	closed   bool
	stopping bool // Close() was called: a non-zero exit is expected
	rows     int
	cols     int
}

func newTerminalSession(onData func([]byte), onExit func(error)) *terminalSession {
	return &terminalSession{onData: onData, onExit: onExit, rows: 24, cols: 80}
}

// Start allocates a pty and launches shell inside dir with env (nil inherits
// this process's environment). The shell is given no arguments: with a tty on
// its standard descriptors every common shell starts interactive on its own.
func (s *terminalSession) Start(shell, dir string, env []string) error {
	req, ok := ptyRequests()
	if !ok {
		return fmt.Errorf("terminal: pty unsupported on %s", runtime.GOOS)
	}
	s.mu.Lock()
	running := s.ptmx != nil && !s.closed
	rows, cols := s.rows, s.cols
	s.mu.Unlock()
	if running {
		return fmt.Errorf("terminal: session already running")
	}

	// syscall.Open + os.NewFile (rather than os.OpenFile + Fd) keeps the
	// master registered with the runtime poller, so Close unblocks the
	// pending Read in readLoop instead of leaving the goroutine wedged.
	fd, err := syscall.Open("/dev/ptmx", syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOCTTY, 0)
	if err != nil {
		return fmt.Errorf("terminal: open /dev/ptmx: %w", err)
	}
	name, err := ptySlaveName(uintptr(fd), req)
	if err != nil {
		syscall.Close(fd)
		return err
	}
	master := os.NewFile(uintptr(fd), "/dev/ptmx")

	slave, err := os.OpenFile(name, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		master.Close()
		return fmt.Errorf("terminal: open %s: %w", name, err)
	}
	// The parent must not keep the slave open: the master only reports EOF
	// once every slave descriptor outside the child is gone.
	defer slave.Close()

	// Size the pty before the child starts so its first prompt already knows
	// the geometry (no initial SIGWINCH needed).
	ws := ptyWinsize{rows: uint16(rows), cols: uint16(cols)}
	// Non-fatal: on failure the shell falls back to its own 80x24 default.
	_, _, _ = syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), req.winsize, uintptr(unsafe.Pointer(&ws)))

	cmd := exec.Command(shell)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	// Setsid detaches the child from our session and Setctty makes the pty
	// its controlling terminal; Ctty defaults to child descriptor 0, which is
	// the slave assigned above. That combination is what enables job control.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	if err := cmd.Start(); err != nil {
		master.Close()
		return fmt.Errorf("terminal: start %s: %w", shell, err)
	}

	s.mu.Lock()
	s.ptmx = master
	s.cmd = cmd
	s.closed = false
	s.stopping = false
	s.mu.Unlock()

	go s.readLoop(master, cmd)
	return nil
}

// ptySlaveName grants and unlocks the pty behind the master fd and returns
// the slave device path.
func ptySlaveName(fd uintptr, req ptyReqs) (string, error) {
	if req.grant != 0 {
		if err := ptyIoctl(fd, req.grant, 0); err != nil {
			return "", fmt.Errorf("terminal: grantpt: %w", err)
		}
	}
	if req.numbered {
		// linux: unlockpt takes a pointer to an int holding 0.
		var zero int32
		if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, req.unlock, uintptr(unsafe.Pointer(&zero))); e != 0 {
			return "", fmt.Errorf("terminal: unlockpt: %w", e)
		}
		var n int32
		if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, req.name, uintptr(unsafe.Pointer(&n))); e != 0 {
			return "", fmt.Errorf("terminal: ptsname: %w", e)
		}
		return "/dev/pts/" + strconv.Itoa(int(n)), nil
	}
	if err := ptyIoctl(fd, req.unlock, 0); err != nil {
		return "", fmt.Errorf("terminal: unlockpt: %w", err)
	}
	// darwin: TIOCPTYGNAME fills a fixed 128-byte device path.
	buf := make([]byte, 128)
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, req.name, uintptr(unsafe.Pointer(&buf[0]))); e != 0 {
		return "", fmt.Errorf("terminal: ptsname: %w", e)
	}
	end := 0
	for end < len(buf) && buf[end] != 0 {
		end++
	}
	if end == 0 {
		return "", fmt.Errorf("terminal: ptsname returned an empty path")
	}
	return string(buf[:end]), nil
}

// ptyIoctl issues an ioctl that takes no pointer argument.
func ptyIoctl(fd, req, arg uintptr) error {
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, req, arg); e != 0 {
		return e
	}
	return nil
}

// readLoop drains the master until it fails (EOF on darwin, EIO on linux once
// the last slave closes, ErrClosed after Close), reaps the shell and reports
// the exit exactly once.
func (s *terminalSession) readLoop(master *os.File, cmd *exec.Cmd) {
	buf := make([]byte, 8192)
	for {
		n, err := master.Read(buf)
		if n > 0 && s.onData != nil {
			s.onData(buf[:n])
		}
		if err != nil {
			break
		}
	}
	werr := cmd.Wait()

	s.mu.Lock()
	stopping := s.stopping
	s.closed = true
	s.mu.Unlock()
	_ = master.Close()

	if stopping {
		// We hung the shell up ourselves; "signal: hangup" is not news.
		werr = nil
	}
	if s.onExit != nil {
		s.onExit(werr)
	}
}

// Write sends p to the shell's standard input.
func (s *terminalSession) Write(p []byte) (int, error) {
	s.mu.Lock()
	f, closed := s.ptmx, s.closed
	s.mu.Unlock()
	if f == nil || closed {
		return 0, errTerminalSessionClosed
	}
	return f.Write(p)
}

// Resize sets the pty window size, which makes the kernel deliver SIGWINCH to
// the foreground process group. The size is remembered even with no session
// running so the next Start picks it up.
func (s *terminalSession) Resize(rows, cols int) error {
	if rows <= 0 || cols <= 0 {
		return nil
	}
	req, ok := ptyRequests()
	if !ok {
		return fmt.Errorf("terminal: pty unsupported on %s", runtime.GOOS)
	}
	s.mu.Lock()
	s.rows, s.cols = rows, cols
	f, closed := s.ptmx, s.closed
	s.mu.Unlock()
	if f == nil || closed {
		return nil
	}
	conn, err := f.SyscallConn()
	if err != nil {
		return err
	}
	ws := ptyWinsize{rows: uint16(rows), cols: uint16(cols)}
	var ioErr error
	if err := conn.Control(func(fd uintptr) {
		if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, req.winsize, uintptr(unsafe.Pointer(&ws))); e != 0 {
			ioErr = e
		}
	}); err != nil {
		return err
	}
	return ioErr
}

// Close hangs up the shell and releases the pty. It is safe to call twice.
func (s *terminalSession) Close() error {
	s.mu.Lock()
	f, alive := s.ptmx, !s.closed
	pid := 0
	// Only signal a shell we know is still unreaped: after Wait the pid can
	// be recycled by the operating system.
	if alive && s.cmd != nil && s.cmd.Process != nil {
		pid = s.cmd.Process.Pid
	}
	s.stopping = true
	s.closed = true
	s.mu.Unlock()

	if f == nil || !alive {
		// Never started, or readLoop already reaped it and closed the master.
		return nil
	}
	if pid > 0 {
		// Start used Setsid, so the shell's pid doubles as its process-group
		// id: signalling the group takes its children down with it.
		_ = syscall.Kill(-pid, syscall.SIGHUP)
	}
	return f.Close()
}

// Running reports whether a shell is attached and still alive.
func (s *terminalSession) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ptmx != nil && !s.closed
}
