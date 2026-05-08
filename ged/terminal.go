package ged

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"silk/core"
	"silk/gui"
	"silk/paint"
)

func init() {
	core.RegisterFactory("ged.TerminalPanel", gui.TypeOf(TerminalPanel{}))
	gui.RegisterToolView(gui.ToolViewDef{
		Id:   "ged.TerminalPanel",
		Name: "终端",
		Icon: "edit",
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

// TerminalPanel is a lightweight integrated terminal panel. It executes
// one shell command at a time in the project directory, streaming stdout
// and stderr back into the scrollback. Command execution happens in a
// goroutine; a gui.Timer polls for new output on the UI thread so the
// scrollback updates without blocking.
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

	// History -- last 50 typed commands, latest at the end.
	history    []string
	historyPos int // -1 = not browsing; 0..len-1 = browsing

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

// Clear wipes the scrollback.
func (this *TerminalPanel) Clear() {
	this.mu.Lock()
	this.lines = nil
	this.pending = nil
	this.mu.Unlock()
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
// streams in via pollPending. No-op if a command is already running
// (the panel handles one command at a time).
func (this *TerminalPanel) Run(cmd string) {
	if cmd == "" || this.running {
		return
	}
	this.inputText = cmd
	this.submitCommand()
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
// output into the visible scrollback and refreshes the view.
func (this *TerminalPanel) pollPending() {
	this.mu.Lock()
	if len(this.pending) == 0 {
		this.mu.Unlock()
		return
	}
	drained := this.pending
	this.pending = nil
	this.mu.Unlock()

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
		if len(this.history) > 50 {
			this.history = this.history[len(this.history)-50:]
		}
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
	this.mu.Unlock()

	go this.runWorker(cmd, this.cwd, cancel)
}

// runWorker executes a single command in the background, streaming its
// output into the shared buffer. It MUST NOT touch any UI state directly.
func (this *TerminalPanel) runWorker(cmdLine, cwd string, cancel chan struct{}) {
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
	c.Env = os.Environ()

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

// OnKeyDown handles Enter, arrows, Backspace, and Ctrl shortcuts.
func (this *TerminalPanel) OnKeyDown(key int, repeat bool) {
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

// OnTextInput appends typed characters to the pending input line. Enter
// does NOT arrive here — it flows through OnKeyDown.
func (this *TerminalPanel) OnTextInput(s string) {
	if s == "\r" || s == "\n" {
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
func (this *TerminalPanel) OnMouseWheel(x, y, z float64) {
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

// SizeHints returns the panel's preferred size.
func (this *TerminalPanel) SizeHints() gui.SizeHints {
	return gui.SizeHints{MinWidth: 320, MinHeight: 150, Height: 200}
}
