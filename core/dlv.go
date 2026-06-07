package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strconv"
	"sync"
	"time"
)

// Delve (`dlv`) headless 调试器的进程控制器
// 这一版只覆盖 IDE Debug 工具栏需要的最小生命周期 + 几个命令,
// 不打算把 Delve 的整张 RPC 表都包过来. Wired 的方法:
//   - RPCServer.CreateBreakpoint
//   - RPCServer.ListBreakpoints
//   - RPCServer.Command         (continue / next)
//   - RPCServer.Detach          (Close 时尽力调用一次)
// 其它(Eval, Stacktrace, ListGoroutines, ...) 留给后续 commit, 等 UI 真要
// 用到的时候再加, 避免现在就把表面铺得太大. 第三方库一律不引入, stdlib only.
//
// 协议补丁: Delve 的 API v2 走的是 net/rpc 的 JSON-RPC 1.0 frame, 即
//   {"method":"RPCServer.X","params":[...],"id":N}\n
// 应答:
//   {"id":N,"result":..., "error":null}\n
// 不是 LSP 那种 Content-Length 分帧, 也不严格遵守 JSON-RPC 2.0
// (没有 "jsonrpc":"2.0" 字段, error 是字符串或 null). 我们这边的 envelope
// 把 "jsonrpc":"2.0" 也写上 -- Delve 会忽略未知字段, 不影响往返;
// 收到 result 时按 raw json 解到调用方提供的 out 指针.

// DebugSession 是一个正在跑的 dlv headless 进程 + JSON-RPC 长连接
// 同一个 session 上的 rpcCall 串行化 (rpc 1.0 over TCP 无法多路复用)
type DebugSession struct {
	cmd  *exec.Cmd
	port int

	mu        sync.Mutex
	conn      net.Conn
	enc       *json.Encoder
	dec       *json.Decoder
	nextRPCID int
	closed    bool
}

// Breakpoint 是用户/我们在源码某一行下的断点
// Delve 的字段比这多得多 (Cond, HitCount, Tracepoint, ...), 当前 IDE 还用不上,
// 先只暴露 ID/File/Line/Function 四个核心字段
type Breakpoint struct {
	ID       int
	File     string
	Line     int
	Function string // 可选
}

// StopState 表示 program 在某次 Continue/Step 之后停下来的位置和原因
// 注意 dlv 的 Reason 可能是 "breakpoint"/"next"/"step"/"exited" 等;
// "exited" 时 File/Line/Function 为空.
type StopState struct {
	Reason   string
	File     string
	Line     int
	Function string
}

// LaunchDebug 在 packageDir 目录下启动 dlv headless server 并连上去
// args 是要传给被调试程序的命令行参数, 会跟在 "--" 之后. 若 dlv 不在 PATH
// 上则立即返回错误; 若 dlv 启动后 ~3 秒内仍连不上 也返回错误并尝试杀掉进程.
// 端口选择: 先拿一个空闲 TCP 端口的号, 再让 dlv 占用同一个端口.
// 这有一个无害的竞态窗口 -- 极少数情况下端口在我们 Listen.Close() 与 dlv Listen
// 之间被别的进程抢走, 但本机交互式调试场景几乎不会撞到, 不值得为它加复杂的重试.
func LaunchDebug(packageDir string, args []string) (*DebugSession, error) {
	if _, err := exec.LookPath("dlv"); err != nil {
		return nil, fmt.Errorf("dlv not on PATH: %w", err)
	}
	port, err := pickFreePort()
	if err != nil {
		return nil, fmt.Errorf("pick free port: %w", err)
	}

	cmdArgs := []string{
		"debug",
		"--headless",
		"--listen=127.0.0.1:" + strconv.Itoa(port),
		"--api-version=2",
		"--accept-multiclient",
	}
	if len(args) > 0 {
		cmdArgs = append(cmdArgs, "--")
		cmdArgs = append(cmdArgs, args...)
	}
	cmd := exec.Command("dlv", cmdArgs...)
	cmd.Dir = packageDir
	// 不把 stdout/stderr 接出来 -- IDE 侧目前不消费; 之后要看日志时再加.
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start dlv: %w", err)
	}

	conn, err := waitForDial("127.0.0.1:"+strconv.Itoa(port), 3*time.Second)
	if err != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		return nil, fmt.Errorf("dial dlv: %w", err)
	}

	return &DebugSession{
		cmd:  cmd,
		port: port,
		conn: conn,
		enc:  json.NewEncoder(conn),
		dec:  json.NewDecoder(conn),
	}, nil
}

// Close 优雅停掉 dlv: 先尝试 Detach (附带 kill=true), 再 kill 进程兜底
// Detach 任意错误都忽略 -- 进程都要终止了, 没必要把 RPC 错误返给上层
func (s *DebugSession) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()

	// Detach 的 params 是 {Kill: true}; 锁已经释放, rpcCall 自己会再上锁
	// 但 closed 此时为 true, 所以我们走一条临时的内部路径: 直接 encode/decode
	// 一次而不通过 rpcCall (rpcCall 在 closed 状态下会拒绝).
	s.sendDetachBestEffort()

	if s.conn != nil {
		_ = s.conn.Close()
	}
	if s.cmd != nil && s.cmd.Process != nil {
		// dlv 在 Detach(kill=true) 之后通常会自己退出, 这里再补一刀确保不漏
		_ = s.cmd.Process.Kill()
		_, _ = s.cmd.Process.Wait()
	}
	return nil
}

// sendDetachBestEffort 在 Close 期间尽力发一次 Detach. 任何错误都吞掉.
// 不复用 rpcCall 是因为 Close 已经把 closed 置为 true 让后续 RPC 早退;
// 这里独立写入与读取, 保持 lifecycle 语义清晰.
func (s *DebugSession) sendDetachBestEffort() {
	if s.conn == nil || s.enc == nil || s.dec == nil {
		return
	}
	s.nextRPCID++
	id := s.nextRPCID
	req := rpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  "RPCServer.Detach",
		Params: []interface{}{
			struct {
				Kill bool `json:"Kill"`
			}{Kill: true},
		},
	}
	// 读写都设短超时, Close 不能因为 dlv 卡死就吊住整个 IDE
	_ = s.conn.SetDeadline(time.Now().Add(500 * time.Millisecond))
	if err := s.enc.Encode(&req); err != nil {
		return
	}
	var raw rpcRawResponse
	_ = s.dec.Decode(&raw)
	_ = s.conn.SetDeadline(time.Time{})
}

// Port 返回 dlv headless 监听的端口号, 给上层日志/UI 用
func (s *DebugSession) Port() int { return s.port }

// SetBreakpoint 在 file:line 处下断点, 返回 dlv 分配的 ID
// dlv 不要求 file 是绝对路径但强烈推荐, 否则其内部要靠 packageDir 解析
func (s *DebugSession) SetBreakpoint(file string, line int) (*Breakpoint, error) {
	type bpSpec struct {
		File string `json:"file"`
		Line int    `json:"line"`
	}
	type createIn struct {
		Breakpoint bpSpec `json:"Breakpoint"`
	}
	// dlv API v2 的应答把断点放在 "Breakpoint" 字段下
	type createOut struct {
		Breakpoint struct {
			ID           int    `json:"id"`
			File         string `json:"file"`
			Line         int    `json:"line"`
			FunctionName string `json:"functionName"`
		} `json:"Breakpoint"`
	}
	var out createOut
	if err := s.rpcCall("CreateBreakpoint", createIn{Breakpoint: bpSpec{File: file, Line: line}}, &out); err != nil {
		return nil, err
	}
	return &Breakpoint{
		ID:       out.Breakpoint.ID,
		File:     out.Breakpoint.File,
		Line:     out.Breakpoint.Line,
		Function: out.Breakpoint.FunctionName,
	}, nil
}

// ListBreakpoints 拉取当前所有断点
// dlv v2 应答里还会含一个 "unrecovered-panic" 等内部断点 (ID<0); 这里原样返回,
// 由上层决定是否过滤(IDE 显示侧通常会按 ID>0 筛一遍)
func (s *DebugSession) ListBreakpoints() ([]Breakpoint, error) {
	type listIn struct {
		All bool `json:"All"`
	}
	type bpEnvelope struct {
		ID           int    `json:"id"`
		File         string `json:"file"`
		Line         int    `json:"line"`
		FunctionName string `json:"functionName"`
	}
	type listOut struct {
		Breakpoints []bpEnvelope `json:"Breakpoints"`
	}
	var out listOut
	if err := s.rpcCall("ListBreakpoints", listIn{All: false}, &out); err != nil {
		return nil, err
	}
	bps := make([]Breakpoint, 0, len(out.Breakpoints))
	for _, b := range out.Breakpoints {
		bps = append(bps, Breakpoint{
			ID:       b.ID,
			File:     b.File,
			Line:     b.Line,
			Function: b.FunctionName,
		})
	}
	return bps, nil
}

// Continue 让被调程序运行直到下一个断点/退出. 阻塞直到 dlv 给出 stop state.
// dlv 的 RPCServer.Command 在 v2 下应答为 DebuggerState; 我们只关心当前线程的
// 停止位置. 程序已退出时 Exited=true, File/Line 为空.
func (s *DebugSession) Continue() (*StopState, error) {
	return s.command("continue")
}

// Step 等同于 dlv 的 "next" -- 同一 goroutine 内单步, 不进入函数调用
// 选 next 而非 step-in 是出于 IDE Debug 工具栏最直觉的 "Step Over" 语义;
// 真正的 step-in 等后续 commit 加 StepIn 方法时再用 "step" 名字.
func (s *DebugSession) Step() (*StopState, error) {
	return s.command("next")
}

// command 是 Continue/Step 共享的内部包装. dlv 的 Command RPC 在 API v2 下
// 接受 {"name": "<cmd>"}, 返回 {"State": DebuggerState}.
func (s *DebugSession) command(name string) (*StopState, error) {
	type cmdIn struct {
		Name string `json:"name"`
	}
	// DebuggerState 是个大对象, 这里只挑当前线程位置以及退出信号
	type loc struct {
		File     string `json:"file"`
		Line     int    `json:"line"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	type thread struct {
		Breakpoint *struct {
			ID int `json:"id"`
		} `json:"Breakpoint"`
		File     string `json:"file"`
		Line     int    `json:"line"`
		Function *struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	type stateOut struct {
		State struct {
			Exited            bool    `json:"exited"`
			ExitStatus        int     `json:"exitStatus"`
			CurrentThread     *thread `json:"currentThread"`
			SelectedGoroutine *struct {
				CurrentLoc loc `json:"currentLoc"`
			} `json:"selectedGoroutine"`
		} `json:"State"`
	}
	var out stateOut
	if err := s.rpcCall("Command", cmdIn{Name: name}, &out); err != nil {
		return nil, err
	}
	// Reason 是 dlv 没原生提供的语义, 这里根据上下文推:
	//   Exited=true                                  -> "exited"
	//   name=="continue" 且 currentThread.Breakpoint -> "breakpoint"
	//   其它                                          -> name 原样回写
	st := &StopState{Reason: name}
	if out.State.Exited {
		st.Reason = "exited"
		return st, nil
	}
	if t := out.State.CurrentThread; t != nil {
		st.File = t.File
		st.Line = t.Line
		if t.Function != nil {
			st.Function = t.Function.Name
		}
		if name == "continue" && t.Breakpoint != nil {
			st.Reason = "breakpoint"
		}
	} else if g := out.State.SelectedGoroutine; g != nil {
		// 兜底: 当前线程缺失时用 selected goroutine 的位置
		st.File = g.CurrentLoc.File
		st.Line = g.CurrentLoc.Line
		st.Function = g.CurrentLoc.Function.Name
	}
	return st, nil
}

// rpcCall 是 JSON-RPC 1.0 在 TCP 上的单次 round-trip
// 串行化由 s.mu 提供 -- net/rpc 的 JSON codec 不允许两个 goroutine 同时写一个 conn.
// out 必须是非空指针; 调用方负责字段对应到 Delve 的应答结构.
func (s *DebugSession) rpcCall(method string, params interface{}, out interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("debug session closed")
	}
	if s.conn == nil {
		return errors.New("debug session has no conn")
	}
	s.nextRPCID++
	id := s.nextRPCID

	// Delve 的 net/rpc JSON 框架: params 永远是单元素数组
	req := rpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  "RPCServer." + method,
		Params:  []interface{}{params},
	}
	if err := s.enc.Encode(&req); err != nil {
		return fmt.Errorf("rpc encode %s: %w", method, err)
	}

	var raw rpcRawResponse
	if err := s.dec.Decode(&raw); err != nil {
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("rpc decode %s: connection closed", method)
		}
		return fmt.Errorf("rpc decode %s: %w", method, err)
	}
	if raw.ID != id {
		// net/rpc JSON 在严格场景下保证按序; 真出现错位说明协议被打断
		return fmt.Errorf("rpc id mismatch on %s: got %d want %d", method, raw.ID, id)
	}
	if len(raw.Error) > 0 && string(raw.Error) != "null" {
		// dlv 的 error 字段为 string (api v2 over net/rpc), 但也兼容对象形态
		return fmt.Errorf("rpc %s: %s", method, decodeRPCError(raw.Error))
	}
	if out == nil {
		return nil
	}
	if len(raw.Result) == 0 || string(raw.Result) == "null" {
		return nil
	}
	if err := json.Unmarshal(raw.Result, out); err != nil {
		return fmt.Errorf("rpc %s result decode: %w", method, err)
	}
	return nil
}

// rpcRequest / rpcRawResponse 是协议的薄信封
// 之所以把 result/error 暴露成 json.RawMessage, 是为了让 rpcCall 把最终的
// 类型化 unmarshal 推给调用方持有的 out 指针, 不需要在中间挪两次内存.
type rpcRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      int           `json:"id"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
}

type rpcRawResponse struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  json.RawMessage `json:"error"`
}

// decodeRPCError 把 dlv 可能给的 error 字段转成展示用字符串
// dlv 在 net/rpc 路径下通常返回纯字符串, 但保留对 {"code":..,"message":..} 形态的兼容
func decodeRPCError(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "<empty>"
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var obj struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil && obj.Message != "" {
		if obj.Code != 0 {
			return fmt.Sprintf("%s (code=%d)", obj.Message, obj.Code)
		}
		return obj.Message
	}
	return string(raw)
}

// pickFreePort 让内核分配一个空闲 TCP 端口然后立即释放
// 返回的端口号可以接着传给子进程让它去 listen. 极少数情况下会被竞争走,
// 但本机调试场景里这种竞争实质为零 -- 不值得加重试. 失败时返回 wrapped error.
func pickFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("listen: %w", err)
	}
	defer l.Close()
	addr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("unexpected listener address type %T", l.Addr())
	}
	return addr.Port, nil
}

// waitForDial 在 deadline 之内反复 TCP 连 addr, 直到成功或超时
// 用于等待 dlv headless server 真正开始 listen.
func waitForDial(addr string, deadline time.Duration) (net.Conn, error) {
	end := time.Now().Add(deadline)
	var lastErr error
	for {
		// 单次 dial 设短超时, 避免 syscall 卡住把剩余预算吃光
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			return conn, nil
		}
		lastErr = err
		if time.Now().After(end) {
			return nil, fmt.Errorf("dial %s timed out: %w", addr, lastErr)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
