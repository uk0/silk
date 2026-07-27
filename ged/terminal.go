package ged

import (
	"bufio"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

func init() {
	core.RegisterFactory("ged.TerminalPanel", gui.TypeOf(TerminalPanel{}))
	gui.RegisterToolView(gui.ToolViewDef{
		Id:   "ged.TerminalPanel",
		Name: "终端",
		Icon: "terminal",
		Desc: "集成终端 — 在项目目录中执行 shell 命令",
	})
}

// terminalLine is one rendered line in the terminal scrollback.
type terminalLine struct {
	Text    string
	IsInput bool // command line typed by the user (echoed with prompt)
	IsError bool // stderr output (rendered in red)
	IsHint  bool // system/help messages (rendered in dim blue)
}

// terminalHistoryMax bounds both the in-memory command history and the
// history file written by SetHistoryFile.
const terminalHistoryMax = 500

// errTerminalSessionClosed is returned by session writes once the shell is
// gone. Declared here because both PTY back ends (unix, windows) need it.
var errTerminalSessionClosed = errors.New("terminal: session is not running")

// TerminalPanel is the integrated terminal panel. It has two modes:
//
//   - session mode (StartSession): a persistent shell on a real pseudo
//     terminal. Keys are forwarded as bytes, output is fed to an AnsiTerm
//     state machine and the panel draws that cell grid, so prompts, cursor
//     motion, colors, line editing and job control all behave.
//   - runner mode (the default): one shell command at a time in the project
//     directory, streamed line by line into a scrollback.
//
// Both modes read process output on a worker goroutine; a gui.Timer drains it
// on the UI thread so nothing blocks the main loop.
type TerminalPanel struct {
	gui.Widget

	// Output lines. Protected by mu when the worker goroutine is running.
	mu    sync.Mutex
	lines []terminalLine

	// Buffer written to by the worker goroutine; drained by pollPending.
	pending []terminalLine

	// Input state
	inputText string
	promptX   float64 // x-offset where the user's input starts (after prompt)

	// Current working directory.
	cwd string

	// Scroll state
	scrollY    float64
	rowHeight  float64
	autoScroll bool

	// Command execution state.
	running bool
	cancel  chan struct{}

	// Extra env applied to every spawned process (KEY=VALUE entries). Merged
	// on top of os.Environ() with override semantics (Qt Creator / VS Code).
	extraEnv []string

	// Per-invocation env passed to the next worker spawn. Set by submitCommand
	// just before starting runWorker; only read by that worker.
	nextEnv []string

	// History -- last terminalHistoryMax typed commands, latest at the end.
	history    []string
	historyPos int // -1 = not browsing; 0..len-1 = browsing

	// File the history is persisted to, "" = memory only.
	historyFile string

	// PTY session state. session != nil means a shell is attached; term
	// non-nil means the panel draws the ANSI screen instead of the
	// line-oriented scrollback (it outlives the session so the final screen
	// stays readable until the shell's output is flushed to the scrollback).
	session *terminalSession
	term    *AnsiTerm

	// Raw PTY output buffer written by the reader goroutine, drained into
	// term by pollPending. Protected by mu together with the flags below.
	pendingIO      []byte
	sessionEnded   bool
	sessionExitMsg string

	// Glyph advance measured in Draw; used to derive the PTY grid size.
	cellW float64

	// Timer that pulls pending lines back into the UI thread.
	pollTimer gui.Timer

	// Optional observer for command submission (used by tests).
	cbSubmit func(cmd string)
}

// NewTerminalPanel creates a new integrated terminal.
func NewTerminalPanel() *TerminalPanel {
	p := new(TerminalPanel)
	p.Init(p)
	return p
}

func (this *TerminalPanel) Init(self gui.IWidget) {
	this.Widget.Init(self)
	this.rowHeight = 16
	this.autoScroll = true
	this.historyPos = -1
	this.cellW = 7
	if cwd, err := os.Getwd(); err == nil {
		this.cwd = cwd
	} else {
		this.cwd = "."
	}
	this.appendLine(terminalLine{
		Text:   "Silk 终端就绪 — 输入 shell 命令 (help 查看内建命令)",
		IsHint: true,
	})
	// Poll pending worker output 10x/sec so streamed command output appears
	// live on the UI thread (timer callbacks fire in the main loop).
	this.pollTimer.Start(100, this.pollPending)
}

// SetCwd overrides the working directory used for command execution.
func (this *TerminalPanel) SetCwd(cwd string) {
	if abs, err := filepath.Abs(cwd); err == nil {
		cwd = abs
	}
	this.cwd = cwd
}

// Cwd returns the current working directory for command execution.
func (this *TerminalPanel) Cwd() string {
	return this.cwd
}

// Clear wipes the scrollback, and the ANSI screen when one is attached.
func (this *TerminalPanel) Clear() {
	this.mu.Lock()
	this.lines = nil
	this.pending = nil
	this.mu.Unlock()
	if this.term != nil {
		this.term.Reset()
	}
	this.scrollY = 0
	this.autoScroll = true
	this.Self().Update()
}

// SigSubmit registers an optional observer that fires right after the user
// presses Enter on a non-empty command. Intended for tests and logging.
func (this *TerminalPanel) SigSubmit(fn func(cmd string)) {
	this.cbSubmit = fn
}

// Run dispatches `cmd` as if the user typed it and pressed Enter.
// Used by the IDE to wire toolbar "Run" / "Build" actions through
// the same terminal scrollback the user types into. Returns
// immediately — execution happens on a worker goroutine and output
// streams in via pollPending. In session mode the line is fed to the
// live shell, which queues it like any typed command; in runner mode
// it is a no-op while another command is running. Routes through
// RunWithEnv so any SetExtraEnv state applies here too.
func (this *TerminalPanel) Run(cmd string) {
	this.RunWithEnv(cmd, this.extraEnv)
}

// RunWithEnv is like Run but augments the spawned process env with
// the supplied KEY=VALUE entries. extraEnv entries OVERRIDE matching
// keys in os.Environ() (Qt Creator / VS Code semantics: explicit env
// wins). Entries with no '=' are preserved as-is. extraEnv does not
// apply in session mode: the shell already owns its environment, which
// was fixed when the session started.
func (this *TerminalPanel) RunWithEnv(cmd string, extraEnv []string) {
	if cmd == "" {
		return
	}
	if this.session != nil {
		this.sessionWrite([]byte(cmd + "\r"))
		return
	}
	if this.running {
		return
	}
	// Snapshot extraEnv so later caller mutation can't race the worker.
	if len(extraEnv) > 0 {
		this.nextEnv = append([]string(nil), extraEnv...)
	} else {
		this.nextEnv = nil
	}
	this.inputText = cmd
	this.submitCommand()
}

// SetExtraEnv installs a persistent set of KEY=VALUE entries applied
// to every subsequent Run / RunWithEnv invocation on this panel. The
// slice is copied so later caller mutation does not affect us.
func (this *TerminalPanel) SetExtraEnv(env []string) {
	if len(env) == 0 {
		this.extraEnv = nil
		return
	}
	this.extraEnv = append([]string(nil), env...)
}

// ExtraEnv returns a copy of the persistent extra-env slice. Mutating
// the returned slice does not affect the panel.
func (this *TerminalPanel) ExtraEnv() []string {
	if len(this.extraEnv) == 0 {
		return nil
	}
	return append([]string(nil), this.extraEnv...)
}

// Hint pushes one system message line into the scrollback. Renders
// in the dim-blue hint style (same as the welcome banner). Use
// instead of Run("echo …") when the IDE wants to surface a
// platform-neutral message — POSIX single-quote escaping doesn't
// translate to cmd.exe and a real subprocess adds latency for
// what's just text.
func (this *TerminalPanel) Hint(msg string) {
	this.appendLine(terminalLine{Text: msg, IsHint: true})
}

// ---------------------------------------------------------------------------
// Command history
// ---------------------------------------------------------------------------

// SetHistoryFile makes the typed-command history survive restarts. Entries
// already in the file are loaded immediately (oldest first, newest last) and
// every later command rewrites the file, capped at terminalHistoryMax lines.
// An empty path detaches the file and keeps history in memory only.
func (this *TerminalPanel) SetHistoryFile(path string) {
	this.historyFile = path
	if path == "" {
		return
	}
	if loaded := loadTerminalHistory(path); len(loaded) > 0 {
		this.history = loaded
		this.historyPos = -1
	}
}

// History returns a copy of the command history, oldest first.
func (this *TerminalPanel) History() []string {
	if len(this.history) == 0 {
		return nil
	}
	return append([]string(nil), this.history...)
}

// loadTerminalHistory reads a history file: one command per line, blanks
// dropped, capped to the newest terminalHistoryMax entries. A missing or
// unreadable file yields nil — history is a convenience, never an error.
func loadTerminalHistory(path string) []string {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		out = append(out, line)
	}
	if len(out) > terminalHistoryMax {
		out = out[len(out)-terminalHistoryMax:]
	}
	return out
}

// saveTerminalHistory rewrites path with the newest terminalHistoryMax
// entries, one per line. Entries containing a newline are skipped because the
// format cannot represent them. A "" path is a no-op.
func saveTerminalHistory(path string, hist []string) error {
	if path == "" {
		return nil
	}
	if len(hist) > terminalHistoryMax {
		hist = hist[len(hist)-terminalHistoryMax:]
	}
	var b strings.Builder
	for _, h := range hist {
		if strings.ContainsAny(h, "\r\n") {
			continue
		}
		b.WriteString(h)
		b.WriteByte('\n')
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, []byte(b.String()), 0o600)
}

// ---------------------------------------------------------------------------
// PTY session
// ---------------------------------------------------------------------------

// StartSession attaches a persistent shell running on a real pseudo terminal,
// switching the panel from the built-in command runner to a full terminal.
// An empty shell means "$SHELL, else the platform default". The session
// inherits the panel's cwd, os.Environ() and SetExtraEnv entries, plus
// TERM=xterm-256color unless the caller overrides it.
//
// Returns an error if the session cannot start (notably on Windows, where the
// ConPTY back end is not implemented yet); the panel then stays in runner
// mode and the reason is also pushed into the scrollback.
func (this *TerminalPanel) StartSession(shell string) error {
	if this.session != nil {
		return nil
	}
	if shell == "" {
		shell = defaultShell()
	}
	rows, cols := this.gridSize()
	term := NewAnsiTerm(rows, cols)
	s := newTerminalSession(this.onSessionData, this.onSessionExit)
	// TERM comes first so an explicit extraEnv entry still wins (mergeEnv is
	// last-wins within its overlay).
	env := mergeEnv(os.Environ(), append([]string{"TERM=xterm-256color"}, this.extraEnv...))
	if err := s.Start(shell, this.cwd, env); err != nil {
		this.Hint("终端会话启动失败: " + err.Error())
		return err
	}
	_ = s.Resize(rows, cols)
	this.session = s
	this.term = term
	this.Hint("终端会话已启动: " + shell)
	this.Self().Update()
	return nil
}

// StopSession hangs up the shell and returns the panel to runner mode. Any
// output still on screen is flushed into the scrollback by pollPending.
func (this *TerminalPanel) StopSession() {
	if this.session == nil {
		return
	}
	_ = this.session.Close()
	this.Self().Update()
}

// SessionActive reports whether a live shell session is attached.
func (this *TerminalPanel) SessionActive() bool {
	return this.session != nil && this.session.Running()
}

// Term returns the ANSI screen model driven by the session, or nil in runner
// mode. Read-only: the panel owns it and feeds it on the UI thread.
func (this *TerminalPanel) Term() *AnsiTerm {
	return this.term
}

// defaultShell picks the shell used when StartSession gets an empty name.
func defaultShell() string {
	if runtime.GOOS == "windows" {
		if sh := os.Getenv("COMSPEC"); sh != "" {
			return sh
		}
		return "cmd.exe"
	}
	if sh := os.Getenv("SHELL"); sh != "" {
		return sh
	}
	return "/bin/sh"
}

// gridSize derives the PTY grid from the panel geometry, falling back to the
// classic 80x24 before the first layout pass.
func (this *TerminalPanel) gridSize() (rows, cols int) {
	w, h := this.Size()
	rows, cols = 24, 80
	if this.rowHeight > 0 && h >= this.rowHeight {
		rows = int(h / this.rowHeight)
	}
	if this.cellW > 0 && w-8 >= this.cellW {
		cols = int((w - 8) / this.cellW)
	}
	if rows < 1 {
		rows = 1
	}
	if cols < 1 {
		cols = 1
	}
	return rows, cols
}

// sessionWrite forwards raw bytes to the shell's stdin.
func (this *TerminalPanel) sessionWrite(p []byte) {
	if this.session == nil || len(p) == 0 {
		return
	}
	if _, err := this.session.Write(p); err != nil {
		this.Hint("终端写入失败: " + err.Error())
	}
}

// sessionKey translates one key press into the bytes a terminal application
// expects and forwards them. It reports false for keys the session does not
// consume, letting the caller fall back to panel handling.
func (this *TerminalPanel) sessionKey(key int) bool {
	// Ctrl+A..Ctrl+Z map onto control codes 0x01..0x1a, which is how the shell
	// sees ^C, ^D, ^L, ^Z and friends. Letters arrive as the uppercase virtual
	// key code; the lowercase range is deliberately not accepted because
	// 0x61..0x7a is where the numpad and function keys live.
	if gui.IsKeyDown(gui.KeyCtrl) && key >= 'A' && key <= 'Z' {
		this.sessionWrite([]byte{byte(key - 'A' + 1)})
		return true
	}
	switch key {
	case gui.KeyEnter:
		this.sessionWrite([]byte{'\r'})
	case gui.KeyBackSpace:
		this.sessionWrite([]byte{0x7f})
	case gui.KeyTab:
		this.sessionWrite([]byte{'\t'})
	case gui.KeyEsc:
		this.sessionWrite([]byte{0x1b})
	case gui.KeyUp:
		this.sessionWrite([]byte("\x1b[A"))
	case gui.KeyDown:
		this.sessionWrite([]byte("\x1b[B"))
	case gui.KeyRight:
		this.sessionWrite([]byte("\x1b[C"))
	case gui.KeyLeft:
		this.sessionWrite([]byte("\x1b[D"))
	case gui.KeyHome:
		this.sessionWrite([]byte("\x1b[H"))
	case gui.KeyEnd:
		this.sessionWrite([]byte("\x1b[F"))
	case gui.KeyDelete:
		this.sessionWrite([]byte("\x1b[3~"))
	case gui.KeyPageUp:
		this.sessionWrite([]byte("\x1b[5~"))
	case gui.KeyPageDown:
		this.sessionWrite([]byte("\x1b[6~"))
	default:
		return false
	}
	return true
}

// onSessionData runs on the session reader goroutine. The slice is reused by
// the reader, so the bytes are copied into the pending buffer for pollPending
// to hand to the ANSI machine on the UI thread.
func (this *TerminalPanel) onSessionData(p []byte) {
	const maxPendingIO = 1 << 20
	this.mu.Lock()
	this.pendingIO = append(this.pendingIO, p...)
	if len(this.pendingIO) > maxPendingIO {
		// Runaway output (cat of a huge file): drop the oldest bytes. This can
		// split an escape sequence, which the state machine recovers from on
		// the next one.
		this.pendingIO = this.pendingIO[len(this.pendingIO)-maxPendingIO:]
	}
	this.mu.Unlock()
}

// onSessionExit runs on the session reader goroutine once the shell is reaped.
func (this *TerminalPanel) onSessionExit(err error) {
	msg := "shell 会话已结束"
	if err != nil {
		msg += ": " + err.Error()
	}
	this.mu.Lock()
	this.sessionEnded = true
	this.sessionExitMsg = msg
	this.mu.Unlock()
}

// endSession runs on the UI thread: it flushes the last screen into the
// scrollback and drops back to runner mode.
func (this *TerminalPanel) endSession(msg string) {
	if this.session != nil {
		_ = this.session.Close()
		this.session = nil
	}
	if this.term != nil {
		for r := 0; r < this.term.Rows(); r++ {
			if text := this.term.LineString(r); text != "" {
				this.lines = append(this.lines, terminalLine{Text: text})
			}
		}
		this.term = nil
	}
	this.appendLine(terminalLine{Text: msg, IsHint: true})
}

// ---------------------------------------------------------------------------
// Scrollback management
// ---------------------------------------------------------------------------

// appendLine appends a line to the scrollback on the UI thread. Callers
// outside the goroutine worker should use this path.
func (this *TerminalPanel) appendLine(ln terminalLine) {
	this.lines = append(this.lines, ln)
	if len(this.lines) > 5000 {
		this.lines = this.lines[len(this.lines)-5000:]
	}
	if this.autoScroll {
		this.scrollToBottom()
	}
	this.Self().Update()
}

// pushWorkerLine is called from the command worker goroutine; it pushes
// into a mutex-protected buffer that the UI timer drains.
func (this *TerminalPanel) pushWorkerLine(text string, isError bool) {
	this.mu.Lock()
	this.pending = append(this.pending, terminalLine{Text: text, IsError: isError})
	this.mu.Unlock()
}

// pollPending runs on the UI thread (timer callback). It moves any worker
// output into the visible scrollback, feeds raw PTY bytes to the ANSI machine
// and refreshes the view.
func (this *TerminalPanel) pollPending() {
	this.mu.Lock()
	drained := this.pending
	this.pending = nil
	io := this.pendingIO
	this.pendingIO = nil
	ended, endMsg := this.sessionEnded, this.sessionExitMsg
	this.sessionEnded = false
	this.mu.Unlock()

	if len(io) > 0 && this.term != nil {
		this.term.Write(io)
		this.Self().Update()
	}
	if ended {
		this.endSession(endMsg)
	}
	if len(drained) == 0 {
		return
	}

	for _, ln := range drained {
		this.lines = append(this.lines, ln)
	}
	if len(this.lines) > 5000 {
		this.lines = this.lines[len(this.lines)-5000:]
	}
	if this.autoScroll {
		this.scrollToBottom()
	}
	this.Self().Update()
}

func (this *TerminalPanel) scrollToBottom() {
	_, h := this.Size()
	totalH := float64(len(this.lines)+1) * this.rowHeight // +1 for prompt row
	if totalH > h {
		this.scrollY = totalH - h
	} else {
		this.scrollY = 0
	}
}

// ---------------------------------------------------------------------------
// Command execution
// ---------------------------------------------------------------------------

// submitCommand dispatches the current input line. Built-in commands are
// handled directly; everything else spawns a shell.
func (this *TerminalPanel) submitCommand() {
	cmd := strings.TrimSpace(this.inputText)

	// Always echo the prompt+command as an input line.
	this.appendLine(terminalLine{
		Text:    this.promptString() + this.inputText,
		IsInput: true,
	})

	// Reset input regardless of whether we run anything.
	this.inputText = ""
	this.historyPos = -1

	if cmd == "" {
		return
	}

	// Persist into history (dedupe consecutive duplicates).
	if len(this.history) == 0 || this.history[len(this.history)-1] != cmd {
		this.history = append(this.history, cmd)
		if len(this.history) > terminalHistoryMax {
			this.history = this.history[len(this.history)-terminalHistoryMax:]
		}
		// Best effort: a terminal must not fail because $HOME is read-only.
		_ = saveTerminalHistory(this.historyFile, this.history)
	}

	if this.cbSubmit != nil {
		this.cbSubmit(cmd)
	}

	// Built-ins handled inline so they work even if running == true.
	switch {
	case cmd == "clear" || cmd == "cls":
		this.Clear()
		return
	case cmd == "help":
		this.appendLine(terminalLine{Text: "内建命令: help, clear, cd <dir>, pwd, exit-clear", IsHint: true})
		return
	case cmd == "pwd":
		this.appendLine(terminalLine{Text: this.cwd})
		return
	case strings.HasPrefix(cmd, "cd "), cmd == "cd":
		target := strings.TrimSpace(strings.TrimPrefix(cmd, "cd"))
		if target == "" {
			if home, err := os.UserHomeDir(); err == nil {
				target = home
			} else {
				this.appendLine(terminalLine{Text: "cd: 无法解析主目录", IsError: true})
				return
			}
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(this.cwd, target)
		}
		target = filepath.Clean(target)
		info, err := os.Stat(target)
		if err != nil || !info.IsDir() {
			this.appendLine(terminalLine{Text: "cd: 目录不存在: " + target, IsError: true})
			return
		}
		this.cwd = target
		return
	}

	this.mu.Lock()
	if this.running {
		this.mu.Unlock()
		this.appendLine(terminalLine{Text: "(有命令正在执行中，请稍候或按 Ctrl+C 取消)", IsHint: true})
		return
	}
	this.running = true
	this.cancel = make(chan struct{})
	cancel := this.cancel
	// Consume per-invocation env so the worker owns it for this run only.
	env := this.nextEnv
	this.nextEnv = nil
	this.mu.Unlock()

	go this.runWorker(cmd, this.cwd, env, cancel)
}

// runWorker executes a single command in the background, streaming its
// output into the shared buffer. It MUST NOT touch any UI state directly.
// extraEnv (if non-empty) is merged on top of os.Environ() with override
// semantics; nil means use the inherited environment verbatim.
func (this *TerminalPanel) runWorker(cmdLine, cwd string, extraEnv []string, cancel chan struct{}) {
	// done is closed unconditionally when the worker exits, so the cancel
	// watcher goroutine below always returns and never leaks.
	done := make(chan struct{})
	defer close(done)

	defer func() {
		if r := recover(); r != nil {
			this.pushWorkerLine("[terminal] internal panic in worker", true)
		}
		this.mu.Lock()
		this.running = false
		this.mu.Unlock()
	}()

	// Build the command with a platform-appropriate shell.
	var c *exec.Cmd
	if runtime.GOOS == "windows" {
		c = exec.Command("cmd", "/C", cmdLine)
	} else {
		// /bin/sh is universally available on macOS and Linux.
		c = exec.Command("/bin/sh", "-c", cmdLine)
	}
	c.Dir = cwd
	if len(extraEnv) > 0 {
		c.Env = mergeEnv(os.Environ(), extraEnv)
	} else {
		c.Env = os.Environ()
	}

	stdout, err := c.StdoutPipe()
	if err != nil {
		this.pushWorkerLine("[terminal] cannot open stdout pipe: "+err.Error(), true)
		return
	}
	stderr, err := c.StderrPipe()
	if err != nil {
		this.pushWorkerLine("[terminal] cannot open stderr pipe: "+err.Error(), true)
		return
	}

	if err := c.Start(); err != nil {
		this.pushWorkerLine("[terminal] 启动失败: "+err.Error(), true)
		return
	}

	// Ship cancel signal to the process -- best-effort only. The `done`
	// channel ensures this goroutine exits even for processes that finish
	// without ever being cancelled.
	go func() {
		select {
		case <-cancel:
			if c.Process != nil {
				_ = c.Process.Kill()
			}
		case <-done:
			return
		}
	}()

	var wg sync.WaitGroup
	wg.Add(2)
	go this.pipeLines(&wg, stdout, false)
	go this.pipeLines(&wg, stderr, true)
	wg.Wait()

	if err := c.Wait(); err != nil {
		// Exit codes surface here as *exec.ExitError. Report non-zero exits
		// as hints so they stay visually distinct from stderr output.
		if _, ok := err.(*exec.ExitError); ok {
			this.pushWorkerLine("[terminal] 命令退出: "+err.Error(), true)
		}
	}
}

// mergeEnv returns base with extra overlaid: every "KEY=..." entry in
// extra overrides the matching KEY= entry in base (preserving its
// relative position); entries whose KEY is not present in base are
// appended at the end. Within extra, last-wins for duplicate keys.
// Malformed entries (no '=') are preserved verbatim — both lists pass
// them through untouched. Neither input slice is mutated.
func mergeEnv(base []string, extra []string) []string {
	if len(extra) == 0 {
		out := make([]string, len(base))
		copy(out, base)
		return out
	}
	// Last-wins collapse within extra; track ordered keys for appends.
	override := make(map[string]string, len(extra))
	var extraOrder []string
	var malformed []string
	for _, e := range extra {
		i := strings.IndexByte(e, '=')
		if i < 0 {
			malformed = append(malformed, e)
			continue
		}
		k := e[:i]
		if _, seen := override[k]; !seen {
			extraOrder = append(extraOrder, k)
		}
		override[k] = e
	}
	out := make([]string, 0, len(base)+len(extraOrder)+len(malformed))
	usedFromExtra := make(map[string]bool, len(override))
	for _, e := range base {
		i := strings.IndexByte(e, '=')
		if i < 0 {
			out = append(out, e)
			continue
		}
		k := e[:i]
		if v, ok := override[k]; ok {
			out = append(out, v)
			usedFromExtra[k] = true
		} else {
			out = append(out, e)
		}
	}
	for _, k := range extraOrder {
		if !usedFromExtra[k] {
			out = append(out, override[k])
		}
	}
	out = append(out, malformed...)
	return out
}

// pipeLines reads an io.Reader line-by-line and pushes each line into the
// pending buffer. On EOF or error it decrements the wait group.
func (this *TerminalPanel) pipeLines(wg *sync.WaitGroup, r io.ReadCloser, isError bool) {
	defer wg.Done()
	defer r.Close()
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		this.pushWorkerLine(scanner.Text(), isError)
	}
}

// ---------------------------------------------------------------------------
// Input handling
// ---------------------------------------------------------------------------

func (this *TerminalPanel) promptString() string {
	// Show a shortened directory name: parent + base to avoid noise.
	base := filepath.Base(this.cwd)
	if parent := filepath.Base(filepath.Dir(this.cwd)); parent != "." && parent != "/" {
		return parent + "/" + base + " $ "
	}
	return base + " $ "
}

// OnKeyDown handles Enter, arrows, Backspace, and Ctrl shortcuts. With a
// session attached the key is forwarded to the shell instead: the shell owns
// the line editing, history and ^C semantics there.
func (this *TerminalPanel) OnKeyDown(key int, repeat bool) {
	if this.session != nil && this.sessionKey(key) {
		return
	}
	ctrl := gui.IsKeyDown(gui.KeyCtrl)
	switch {
	case key == gui.KeyEnter:
		this.submitCommand()
	case key == gui.KeyBackSpace:
		if len(this.inputText) > 0 {
			// Trim one rune (UTF-8 safe) rather than one byte.
			r := []rune(this.inputText)
			this.inputText = string(r[:len(r)-1])
			this.Self().Update()
		}
	case key == gui.KeyUp:
		if len(this.history) == 0 {
			return
		}
		if this.historyPos == -1 {
			this.historyPos = len(this.history) - 1
		} else if this.historyPos > 0 {
			this.historyPos--
		}
		this.inputText = this.history[this.historyPos]
		this.autoScroll = true
		this.scrollToBottom()
		this.Self().Update()
	case key == gui.KeyDown:
		if this.historyPos == -1 || len(this.history) == 0 {
			return
		}
		this.historyPos++
		if this.historyPos >= len(this.history) {
			this.historyPos = -1
			this.inputText = ""
		} else {
			this.inputText = this.history[this.historyPos]
		}
		this.Self().Update()
	case ctrl && (key == 'L' || key == 'l'):
		this.Clear()
	case ctrl && (key == 'C' || key == 'c'):
		// Cancel running command; if nothing is running, clear input.
		this.mu.Lock()
		running := this.running
		cancel := this.cancel
		this.mu.Unlock()
		if running && cancel != nil {
			select {
			case <-cancel:
			default:
				close(cancel)
			}
			this.appendLine(terminalLine{Text: "^C", IsHint: true})
		} else {
			this.inputText = ""
			this.Self().Update()
		}
	}
}

// OnTextInput appends typed characters to the pending input line, or sends
// them straight to the shell in session mode. Enter does NOT arrive here — it
// flows through OnKeyDown.
func (this *TerminalPanel) OnTextInput(s string) {
	if s == "\r" || s == "\n" {
		return
	}
	if this.session != nil {
		this.sessionWrite([]byte(s))
		return
	}
	this.inputText += s
	this.autoScroll = true
	this.scrollToBottom()
	this.Self().Update()
}

// OnLeftDown focuses the terminal.
func (this *TerminalPanel) OnLeftDown(x, y float64) {
	this.SetFocus()
}

// OnMouseWheel scrolls the scrollback. Scrolling up disables auto-scroll
// so that streaming output doesn't yank the viewport away from the user.
// Screen mode has no scrollback of its own, so the wheel is inert there.
func (this *TerminalPanel) OnMouseWheel(x, y, z float64) {
	if this.term != nil {
		return
	}
	this.scrollY -= z * 3 * this.rowHeight
	if this.scrollY < 0 {
		this.scrollY = 0
	}
	_, h := this.Size()
	totalH := float64(len(this.lines)+1) * this.rowHeight
	maxScroll := totalH - h
	if maxScroll < 0 {
		maxScroll = 0
	}
	if this.scrollY > maxScroll {
		this.scrollY = maxScroll
	}
	this.autoScroll = this.scrollY >= maxScroll-this.rowHeight
	this.Self().Update()
}

// ---------------------------------------------------------------------------
// Drawing
// ---------------------------------------------------------------------------

func (this *TerminalPanel) Draw(g paint.Painter) {
	w, h := this.Size()

	// Dark terminal background.
	g.SetBrush1(paint.Color{R: 22, G: 22, B: 28, A: 255})
	g.Rectangle(0, 0, w, h)
	g.Fill()

	if this.term != nil {
		this.drawScreen(g, w, h)
		return
	}

	font := paint.NewFont("Menlo", 12, false, false)
	g.SetFont(font)
	fe := font.FontExtents()
	rh := this.rowHeight

	// Render visible scrollback.
	startIdx := int(this.scrollY / rh)
	if startIdx < 0 {
		startIdx = 0
	}
	visibleCount := int(h/rh) + 2

	for i := startIdx; i < startIdx+visibleCount && i < len(this.lines); i++ {
		y := float64(i)*rh - this.scrollY
		line := this.lines[i]
		textY := y + fe.Ascent + (rh-fe.Height)/2

		switch {
		case line.IsError:
			g.SetBrush1(paint.Color{R: 230, G: 85, B: 85, A: 255})
		case line.IsInput:
			g.SetBrush1(paint.Color{R: 185, G: 225, B: 125, A: 255})
		case line.IsHint:
			g.SetBrush1(paint.Color{R: 115, G: 170, B: 235, A: 255})
		default:
			g.SetBrush1(paint.Color{R: 215, G: 215, B: 220, A: 255})
		}
		g.DrawText1(8, textY, line.Text)
	}

	// Draw the active prompt row at the bottom of the scrollback.
	promptRow := len(this.lines)
	py := float64(promptRow)*rh - this.scrollY
	if py+rh > 0 && py < h {
		textY := py + fe.Ascent + (rh-fe.Height)/2
		prompt := this.promptString()

		g.SetBrush1(paint.Color{R: 115, G: 210, B: 140, A: 255})
		g.DrawText1(8, textY, prompt)

		promptW := font.TextExtents(prompt).XAdvance
		g.SetBrush1(paint.Color{R: 230, G: 230, B: 235, A: 255})
		g.DrawText1(8+promptW, textY, this.inputText)

		// Solid caret block at the end of the input text.
		inputW := font.TextExtents(this.inputText).XAdvance
		caretX := 8 + promptW + inputW
		g.SetBrush1(paint.Color{R: 230, G: 230, B: 235, A: 180})
		g.Rectangle(caretX, py+2, 7, rh-4)
		g.Fill()
	}

	// Running indicator in the top-right corner.
	this.mu.Lock()
	running := this.running
	this.mu.Unlock()
	if running {
		g.SetBrush1(paint.Color{R: 249, G: 168, B: 37, A: 255})
		g.Rectangle(w-80, 4, 72, 16)
		g.Fill()
		g.SetFont(paint.NewFont("Menlo", 10, true, false))
		g.SetBrush1(paint.Color{R: 20, G: 20, B: 28, A: 255})
		g.DrawText1(w-74, 16, "running…")
	}
}

// terminalScreenFg is the color of a cell with the default foreground.
var terminalScreenFg = paint.Color{R: 215, G: 215, B: 220, A: 255}

// drawScreen renders the ANSI screen model. Cells are batched into runs of
// equal attributes so one row costs a handful of draw calls instead of one per
// character. This is also where the PTY grid is matched to the panel size,
// because the glyph advance is only measurable with a live font.
func (this *TerminalPanel) drawScreen(g paint.Painter, w, h float64) {
	// [bold][italic] variants of the fixed-width face.
	var fonts [2][2]paint.Font
	for b := 0; b < 2; b++ {
		for i := 0; i < 2; i++ {
			fonts[b][i] = paint.NewFont("Menlo", 12, b == 1, i == 1)
		}
	}
	base := fonts[0][0]
	g.SetFont(base)
	fe := base.FontExtents()
	rh := this.rowHeight
	if cw := base.TextExtents("M").XAdvance; cw > 0 {
		this.cellW = cw
	}

	// Keep the pty geometry in sync with the visible grid; the shell gets a
	// SIGWINCH out of the session resize.
	rows, cols := this.gridSize()
	if rows != this.term.Rows() || cols != this.term.Cols() {
		this.term.Resize(rows, cols)
		if this.session != nil {
			_ = this.session.Resize(rows, cols)
		}
	}

	screen := this.term.Screen()
	for r, row := range screen {
		y := float64(r) * rh
		if y >= h {
			break
		}
		textY := y + fe.Ascent + (rh-fe.Height)/2

		// Backgrounds first so text is never clipped by a later fill. Fg takes
		// part in the run key because reverse video paints it as background.
		for c := 0; c < len(row); {
			cell := row[c]
			end := c + 1
			for end < len(row) && row[end].Bg == cell.Bg &&
				row[end].Fg == cell.Fg && row[end].Reverse == cell.Reverse {
				end++
			}
			if col, ok := terminalCellBg(cell); ok {
				g.SetBrush1(col)
				g.Rectangle(8+float64(c)*this.cellW, y, float64(end-c)*this.cellW, rh)
				g.Fill()
			}
			c = end
		}

		// Then one text draw per attribute run.
		for c := 0; c < len(row); {
			cell := row[c]
			end := c + 1
			for end < len(row) && terminalSameStyle(row[end], cell) {
				end++
			}
			var text strings.Builder
			for _, cc := range row[c:end] {
				if cc.Rune == 0 {
					text.WriteByte(' ')
				} else {
					text.WriteRune(cc.Rune)
				}
			}
			s := text.String()
			if strings.TrimSpace(s) != "" {
				x := 8 + float64(c)*this.cellW
				bold, italic := 0, 0
				if cell.Bold {
					bold = 1
				}
				if cell.Italic {
					italic = 1
				}
				g.SetFont(fonts[bold][italic])
				g.SetBrush1(terminalCellFg(cell))
				g.DrawText1(x, textY, s)
				if cell.Underline {
					g.Rectangle(x, textY+2, float64(end-c)*this.cellW, 1)
					g.Fill()
				}
			}
			c = end
		}
	}

	// Cursor block.
	cy, cx := this.term.CursorPos()
	if cyf := float64(cy) * rh; cyf < h {
		g.SetBrush1(paint.Color{R: 230, G: 230, B: 235, A: 180})
		g.Rectangle(8+float64(cx)*this.cellW, cyf+2, this.cellW, rh-4)
		g.Fill()
	}
}

// terminalSameStyle reports whether two cells can share one text draw.
func terminalSameStyle(a, b Cell) bool {
	return a.Fg == b.Fg && a.Bg == b.Bg && a.Bold == b.Bold &&
		a.Italic == b.Italic && a.Underline == b.Underline && a.Reverse == b.Reverse
}

// terminalCellFg resolves a cell's text color, honouring reverse video.
func terminalCellFg(c Cell) paint.Color {
	src := c.Fg
	if c.Reverse {
		src = c.Bg
		if src == AnsiDefaultColor {
			// Reverse on the default background paints text in the panel's
			// own background color.
			return paint.Color{R: 22, G: 22, B: 28, A: 255}
		}
	}
	if r, g, b, ok := AnsiColorRGB(src); ok {
		return paint.Color{R: r, G: g, B: b, A: 255}
	}
	return terminalScreenFg
}

// terminalCellBg resolves a cell's background color. ok is false when the
// panel background already shows through.
func terminalCellBg(c Cell) (paint.Color, bool) {
	src := c.Bg
	if c.Reverse {
		src = c.Fg
		if src == AnsiDefaultColor {
			return terminalScreenFg, true
		}
	}
	if r, g, b, ok := AnsiColorRGB(src); ok {
		return paint.Color{R: r, G: g, B: b, A: 255}, true
	}
	return paint.Color{}, false
}

// SizeHints returns the panel's preferred size.
func (this *TerminalPanel) SizeHints() gui.SizeHints {
	return gui.SizeHints{MinWidth: 320, MinHeight: 150, Height: 200}
}
