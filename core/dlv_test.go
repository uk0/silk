package core

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// 单元测试聚焦于纯协议 + 端口辅助, 不会拉起 dlv 子进程.
// 烟雾测试 (LaunchDebug + Continue) 单独一节, 在 dlv 不在 PATH 时 Skip.

// TestPickFreePort_Bindable: pickFreePort 给出的端口必须可被立即 bind.
// 如果实现真把 listener 留着, 这里就会 EADDRINUSE.
func TestDlvPickFreePort_Bindable(t *testing.T) {
	port, err := pickFreePort()
	if err != nil {
		t.Fatalf("pickFreePort: %v", err)
	}
	if port <= 0 || port > 65535 {
		t.Fatalf("port out of range: %d", port)
	}
	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("re-bind %d: %v", port, err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}
}

// TestPickFreePort_Distinct: 连续两次拿应该 (绝大概率) 给不同端口.
// 即便偶发相等也不算 bug, 但极不可能, 当作 sanity check 足够.
func TestDlvPickFreePort_Distinct(t *testing.T) {
	a, err := pickFreePort()
	if err != nil {
		t.Fatalf("pickFreePort a: %v", err)
	}
	b, err := pickFreePort()
	if err != nil {
		t.Fatalf("pickFreePort b: %v", err)
	}
	if a == b {
		t.Logf("port a==b==%d (rare but allowed)", a)
	}
}

// TestDecodeRPCError_StringForm: dlv 在 net/rpc 路径下通常返回字符串 error
func TestDlvDecodeRPCError_StringForm(t *testing.T) {
	raw := json.RawMessage(`"could not find file foo.go"`)
	got := decodeRPCError(raw)
	want := "could not find file foo.go"
	if got != want {
		t.Errorf("decodeRPCError = %q, want %q", got, want)
	}
}

// TestDecodeRPCError_ObjectForm: 兼容对象形态 {"code":..,"message":..}
func TestDlvDecodeRPCError_ObjectForm(t *testing.T) {
	raw := json.RawMessage(`{"code":42,"message":"boom"}`)
	got := decodeRPCError(raw)
	if !strings.Contains(got, "boom") || !strings.Contains(got, "42") {
		t.Errorf("decodeRPCError = %q, want both 'boom' and '42'", got)
	}

	// 没有 code 的对象, 只输出 message
	raw2 := json.RawMessage(`{"message":"plain"}`)
	if got := decodeRPCError(raw2); got != "plain" {
		t.Errorf("decodeRPCError no-code = %q, want 'plain'", got)
	}
}

// TestDecodeRPCError_Fallback: 既不是 string 也不是带 message 的对象, 原样字符串化
func TestDlvDecodeRPCError_Fallback(t *testing.T) {
	raw := json.RawMessage(`[1,2,3]`)
	got := decodeRPCError(raw)
	if got != "[1,2,3]" {
		t.Errorf("decodeRPCError fallback = %q, want '[1,2,3]'", got)
	}
}

// TestRPCCall_RoundTrip 用一个本地 fake server 验证:
//   1. encode 出去的请求是 {"jsonrpc":"2.0","id":N,"method":"RPCServer.X","params":[<params>]}
//   2. 收到 {"id":N,"result":{...}} 后 result 能解到调用方提供的 out 指针
//   3. id 单调递增
// 这等价于一次 RPCServer.CreateBreakpoint 的协议形态, 但不依赖 dlv.
func TestDlvRPCCall_RoundTrip(t *testing.T) {
	srv, clientConn := newFakeRPCServer(t)
	defer srv.close()

	sess := &DebugSession{
		conn: clientConn,
		enc:  json.NewEncoder(clientConn),
		dec:  json.NewDecoder(clientConn),
	}

	// fake server 协议: 读一行 JSON, 取 id, 回 {"id":id,"result":{"Breakpoint":{"id":7,"file":"f.go","line":42,"functionName":"main.main"}},"error":null}
	srv.handle(func(req map[string]interface{}) string {
		id := int(req["id"].(float64))
		method, _ := req["method"].(string)
		if method != "RPCServer.CreateBreakpoint" {
			return fmt.Sprintf(`{"id":%d,"result":null,"error":"unexpected method %s"}`, id, method)
		}
		// params 必须是数组且首元素带 Breakpoint
		params := req["params"].([]interface{})
		first := params[0].(map[string]interface{})
		bp := first["Breakpoint"].(map[string]interface{})
		if int(bp["line"].(float64)) != 42 {
			return fmt.Sprintf(`{"id":%d,"result":null,"error":"line wrong"}`, id)
		}
		return fmt.Sprintf(
			`{"id":%d,"result":{"Breakpoint":{"id":7,"file":"f.go","line":42,"functionName":"main.main"}},"error":null}`,
			id,
		)
	})

	type bpSpec struct {
		File string `json:"file"`
		Line int    `json:"line"`
	}
	type in struct {
		Breakpoint bpSpec `json:"Breakpoint"`
	}
	type out struct {
		Breakpoint struct {
			ID           int    `json:"id"`
			File         string `json:"file"`
			Line         int    `json:"line"`
			FunctionName string `json:"functionName"`
		} `json:"Breakpoint"`
	}

	var got out
	if err := sess.rpcCall("CreateBreakpoint", in{Breakpoint: bpSpec{File: "f.go", Line: 42}}, &got); err != nil {
		t.Fatalf("rpcCall: %v", err)
	}
	if got.Breakpoint.ID != 7 || got.Breakpoint.Line != 42 || got.Breakpoint.FunctionName != "main.main" {
		t.Errorf("unexpected result: %+v", got)
	}

	// 再调一次, id 应当递增到 2
	srv.handle(func(req map[string]interface{}) string {
		id := int(req["id"].(float64))
		if id != 2 {
			return fmt.Sprintf(`{"id":%d,"result":null,"error":"expected id 2 got %d"}`, id, id)
		}
		return fmt.Sprintf(`{"id":%d,"result":null,"error":null}`, id)
	})
	if err := sess.rpcCall("CreateBreakpoint", in{Breakpoint: bpSpec{File: "g.go", Line: 1}}, &got); err != nil {
		t.Fatalf("rpcCall second: %v", err)
	}
}

// TestRPCCall_ErrorPath: fake server 返回 error 字段 -> rpcCall 必须返回 wrapped error,
// 且不污染 out 指针.
func TestDlvRPCCall_ErrorPath(t *testing.T) {
	srv, clientConn := newFakeRPCServer(t)
	defer srv.close()

	sess := &DebugSession{
		conn: clientConn,
		enc:  json.NewEncoder(clientConn),
		dec:  json.NewDecoder(clientConn),
	}

	srv.handle(func(req map[string]interface{}) string {
		id := int(req["id"].(float64))
		return fmt.Sprintf(`{"id":%d,"result":null,"error":"could not find file nope.go"}`, id)
	})

	type out struct {
		Breakpoint struct {
			ID int `json:"id"`
		} `json:"Breakpoint"`
	}
	var got out
	err := sess.rpcCall("CreateBreakpoint", map[string]interface{}{}, &got)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "could not find file nope.go") {
		t.Errorf("err = %v, want it to contain dlv error message", err)
	}
	if got.Breakpoint.ID != 0 {
		t.Errorf("out should be untouched on error, got %+v", got)
	}
}

// TestRPCCall_IDMismatch: server 故意返回错号 id -> 客户端报错
func TestDlvRPCCall_IDMismatch(t *testing.T) {
	srv, clientConn := newFakeRPCServer(t)
	defer srv.close()

	sess := &DebugSession{
		conn: clientConn,
		enc:  json.NewEncoder(clientConn),
		dec:  json.NewDecoder(clientConn),
	}

	srv.handle(func(req map[string]interface{}) string {
		// 不论真实 id 是多少, 都回 999
		return `{"id":999,"result":null,"error":null}`
	})

	err := sess.rpcCall("AnyMethod", struct{}{}, nil)
	if err == nil || !strings.Contains(err.Error(), "id mismatch") {
		t.Fatalf("want id mismatch error, got %v", err)
	}
}

// closedSession 构造一个已打上 closed 标记的 session, 供各 RPC 的 closed 早退
// 测试复用. closed 是 atomic.Bool, 没法用结构体字面量初始化, 统一走这个小工厂.
func closedSession() *DebugSession {
	sess := &DebugSession{}
	sess.closed.Store(true)
	return sess
}

// TestRPCCall_OnClosedSession: closed=true 时 rpcCall 早退
func TestDlvRPCCall_OnClosedSession(t *testing.T) {
	sess := closedSession()
	err := sess.rpcCall("X", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("want closed error, got %v", err)
	}
}

// TestRPCCall_ConnEOF: 服务器立即断连接 -> 客户端返回 connection closed
func TestDlvRPCCall_ConnEOF(t *testing.T) {
	srv, clientConn := newFakeRPCServer(t)
	// 让 server 收到第一条请求后立刻断开
	srv.handle(func(req map[string]interface{}) string {
		return "" // 空字符串 = 不写, 直接关
	})

	sess := &DebugSession{
		conn: clientConn,
		enc:  json.NewEncoder(clientConn),
		dec:  json.NewDecoder(clientConn),
	}
	err := sess.rpcCall("X", struct{}{}, nil)
	if err == nil {
		t.Fatal("expected error after server EOF")
	}
	if !strings.Contains(err.Error(), "connection closed") && !strings.Contains(err.Error(), "EOF") {
		t.Errorf("err = %v, want EOF/closed indication", err)
	}
	srv.close()
}

// fakeRPCServer 在 localhost 起一个最朴素的 newline-delimited JSON-RPC 服务器
// 仅供 rpcCall 协议测试使用. handle(fn) 注册一个处理回调; 一次连接每条请求都
// 走最近一次注册的回调. 写回 "" 表示不应答, 直接关闭连接 (模拟 dlv 崩溃).
// conn 由 accept goroutine 在 Accept 之后才写入, 而 close()/handle() 在测试
// goroutine 上并发读写 -- 共享字段统一用 mu 护住, 否则 -race 会抓到 hand-off 竞态.
type fakeRPCServer struct {
	ln net.Listener
	t  *testing.T

	mu      sync.Mutex // guards conn + current (accept goroutine vs 测试 goroutine)
	conn    net.Conn
	current func(map[string]interface{}) string
}

func newFakeRPCServer(t *testing.T) (*fakeRPCServer, net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &fakeRPCServer{ln: ln, t: t}
	// 启动 accept goroutine
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		srv.mu.Lock()
		srv.conn = c
		srv.mu.Unlock()
		br := bufio.NewReader(c)
		// 服务循环: 每读一行就调当前 handler
		for {
			line, err := br.ReadBytes('\n')
			if err != nil {
				_ = c.Close()
				return
			}
			var req map[string]interface{}
			if err := json.Unmarshal(line, &req); err != nil {
				continue
			}
			srv.mu.Lock()
			h := srv.current
			srv.mu.Unlock()
			if h == nil {
				continue
			}
			resp := h(req)
			if resp == "" {
				_ = c.Close()
				return
			}
			if !strings.HasSuffix(resp, "\n") {
				resp += "\n"
			}
			if _, err := c.Write([]byte(resp)); err != nil {
				return
			}
		}
	}()
	clientConn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return srv, clientConn
}

func (s *fakeRPCServer) handle(fn func(map[string]interface{}) string) {
	s.mu.Lock()
	s.current = fn
	s.mu.Unlock()
}

func (s *fakeRPCServer) close() {
	s.mu.Lock()
	conn := s.conn
	s.mu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
	if s.ln != nil {
		_ = s.ln.Close()
	}
}

// TestWaitForDial_Timeout: 不开任何 server, waitForDial 必须按预算超时
func TestDlvWaitForDial_Timeout(t *testing.T) {
	port, err := pickFreePort()
	if err != nil {
		t.Fatalf("pickFreePort: %v", err)
	}
	// 已经被释放, 没人 listen, 必然超时
	start := time.Now()
	_, err = waitForDial(fmt.Sprintf("127.0.0.1:%d", port), 250*time.Millisecond)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if elapsed > 2*time.Second {
		t.Errorf("waitForDial took %v, too slow", elapsed)
	}
}

// TestWaitForDial_Success: 提前一会 listen, waitForDial 应当能连上
func TestDlvWaitForDial_Success(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		_ = c.Close()
	}()
	conn, err := waitForDial(ln.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatalf("waitForDial: %v", err)
	}
	_ = conn.Close()
}

// -----------------------------------------------------------------------------
// 烟雾测试: 真的拉起 dlv 在一个临时 main.go 上调试.
// 没有 dlv 时 Skip; 失败时打印更多上下文 (端口/进程状态).
// 这是 ~1s 级的慢测试, 但 -run 过滤后默认不会跑很多次.
// -----------------------------------------------------------------------------

const smokeMainGo = `package main

import "fmt"

func main() {
	x := 1
	x++
	fmt.Println("hello", x)
}
`

func TestDebugSmoke_LaunchSetBreakpointContinue(t *testing.T) {
	if _, err := exec.LookPath("dlv"); err != nil {
		t.Skip("dlv not on PATH, skipping debug smoke test")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH, skipping debug smoke test")
	}

	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(mainPath, []byte(smokeMainGo), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	// 写一个 go.mod 让 dlv 不会因为 GOPATH 模式而抱怨
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module dlvsmoke\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	sess, err := LaunchDebug(dir, nil)
	if err != nil {
		t.Fatalf("LaunchDebug: %v", err)
	}
	defer func() {
		if cerr := sess.Close(); cerr != nil {
			t.Logf("Close: %v", cerr)
		}
	}()

	// 第 6 行 (x := 1) 是 main 函数的第一条可执行语句; 在这行下断点
	bp, err := sess.SetBreakpoint(mainPath, 6)
	if err != nil {
		t.Fatalf("SetBreakpoint: %v", err)
	}
	if bp.ID == 0 {
		t.Errorf("breakpoint id = 0, want non-zero")
	}
	if bp.Line != 6 {
		t.Errorf("breakpoint line = %d, want 6", bp.Line)
	}

	bps, err := sess.ListBreakpoints()
	if err != nil {
		t.Fatalf("ListBreakpoints: %v", err)
	}
	found := false
	for _, b := range bps {
		if b.ID == bp.ID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("breakpoint id %d not in ListBreakpoints: %+v", bp.ID, bps)
	}

	st, err := sess.Continue()
	if err != nil {
		t.Fatalf("Continue: %v", err)
	}
	if st.Reason != "breakpoint" {
		t.Errorf("StopState.Reason = %q, want 'breakpoint' (full state: %+v)", st.Reason, st)
	}
	if st.Line != 6 {
		t.Errorf("StopState.Line = %d, want 6", st.Line)
	}
}

// 几个意外但合理的边界, 不依赖 dlv:
// LaunchDebug 在 dlv 缺席时必须立即报错而不是 hang
func TestLaunchDebug_NoDlv(t *testing.T) {
	if _, err := exec.LookPath("dlv"); err == nil {
		t.Skip("dlv is on PATH; this test verifies the missing-dlv error path")
	}
	_, err := LaunchDebug(t.TempDir(), nil)
	if err == nil {
		t.Fatal("expected error when dlv missing, got nil")
	}
	if !strings.Contains(err.Error(), "PATH") && !strings.Contains(err.Error(), "dlv") {
		t.Errorf("error %v does not mention dlv/PATH", err)
	}
}

// 防御性: rpcCall 不应该在 nil conn / nil enc 上崩
func TestDlvRPCCall_NilConn(t *testing.T) {
	sess := &DebugSession{}
	err := sess.rpcCall("X", nil, nil)
	if err == nil {
		t.Fatal("expected error on nil conn")
	}
}

// io.EOF 兼容性 -- 防止上游改 errors.Is 路径时漏掉
func TestDlvRPCCall_EOFWrapping(t *testing.T) {
	// 直接构造 wrapped EOF, 确认 errors.Is 链有效 (与 rpcCall 内部相同)
	wrapped := fmt.Errorf("rpc decode X: %w", io.EOF)
	if !errors.Is(wrapped, io.EOF) {
		t.Fatal("io.EOF wrapping broken")
	}
}

// -----------------------------------------------------------------------------
// Stacktrace / ListGoroutines / ListLocals / Eval -- protocol-level tests
// 都用 fake server: 验证请求形态 + 应答解码, 不依赖 dlv 子进程.
// -----------------------------------------------------------------------------

// newSessionWithFakeServer 是一个测试小工厂, 把 newFakeRPCServer + DebugSession
// 装在一起, 让每个新方法的协议测试更短.
func newSessionWithFakeServer(t *testing.T) (*fakeRPCServer, *DebugSession) {
	t.Helper()
	srv, clientConn := newFakeRPCServer(t)
	sess := &DebugSession{
		conn: clientConn,
		enc:  json.NewEncoder(clientConn),
		dec:  json.NewDecoder(clientConn),
	}
	return srv, sess
}

// TestStacktrace_Decode: 预编程 fake server 回三帧, 断言 method + params + 解码
func TestDlvStacktrace_Decode(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()

	srv.handle(func(req map[string]interface{}) string {
		id := int(req["id"].(float64))
		if req["method"].(string) != "RPCServer.Stacktrace" {
			return fmt.Sprintf(`{"id":%d,"result":null,"error":"wrong method %s"}`, id, req["method"])
		}
		// params 是 [ {Id,Depth,Full,Defers} ]
		first := req["params"].([]interface{})[0].(map[string]interface{})
		if int(first["Id"].(float64)) != -1 {
			return fmt.Sprintf(`{"id":%d,"result":null,"error":"want Id=-1"}`, id)
		}
		if int(first["Depth"].(float64)) != 50 {
			return fmt.Sprintf(`{"id":%d,"result":null,"error":"want Depth=50"}`, id)
		}
		if first["Full"].(bool) != false {
			return fmt.Sprintf(`{"id":%d,"result":null,"error":"want Full=false"}`, id)
		}
		return fmt.Sprintf(`{"id":%d,"result":{"Locations":[
			{"Location":{"file":"a.go","line":10,"function":{"name":"main.main"}}},
			{"Location":{"file":"b.go","line":20,"function":{"name":"runtime.gopark"}}},
			{"Location":{"file":"c.go","line":30,"function":{"name":"runtime.goexit"}}}
		]},"error":null}`, id)
	})

	frames, err := sess.Stacktrace(-1, 50)
	if err != nil {
		t.Fatalf("Stacktrace: %v", err)
	}
	if len(frames) != 3 {
		t.Fatalf("want 3 frames, got %d: %+v", len(frames), frames)
	}
	want := []StackFrame{
		{File: "a.go", Line: 10, Function: "main.main"},
		{File: "b.go", Line: 20, Function: "runtime.gopark"},
		{File: "c.go", Line: 30, Function: "runtime.goexit"},
	}
	for i, w := range want {
		if frames[i] != w {
			t.Errorf("frame[%d] = %+v, want %+v", i, frames[i], w)
		}
	}
}

// TestStacktrace_ErrorPath: server 报错 -> 方法返回 error 且不返回部分结果
func TestDlvStacktrace_ErrorPath(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()

	srv.handle(func(req map[string]interface{}) string {
		id := int(req["id"].(float64))
		return fmt.Sprintf(`{"id":%d,"result":null,"error":"goroutine not found"}`, id)
	})

	frames, err := sess.Stacktrace(99, 10)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "goroutine not found") {
		t.Errorf("err = %v, want it to contain dlv error", err)
	}
	if frames != nil {
		t.Errorf("frames should be nil on error, got %+v", frames)
	}
}

// TestStacktrace_ClosedSession: pre-call Close, Stacktrace 必须返回 closed 错误
func TestDlvStacktrace_ClosedSession(t *testing.T) {
	sess := closedSession()
	_, err := sess.Stacktrace(-1, 10)
	if err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("want closed error, got %v", err)
	}
}

// TestListGoroutines_Decode: 验证 method/params + UserCurrentLoc 优先回落策略
func TestDlvListGoroutines_Decode(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()

	srv.handle(func(req map[string]interface{}) string {
		id := int(req["id"].(float64))
		if req["method"].(string) != "RPCServer.ListGoroutines" {
			return fmt.Sprintf(`{"id":%d,"result":null,"error":"wrong method %s"}`, id, req["method"])
		}
		first := req["params"].([]interface{})[0].(map[string]interface{})
		if int(first["Start"].(float64)) != 0 || int(first["Count"].(float64)) != 0 {
			return fmt.Sprintf(`{"id":%d,"result":null,"error":"want Start=0 Count=0"}`, id)
		}
		// goroutine 1: 有 user loc; goroutine 2: user loc 空, 回落到 currentLoc;
		// goroutine 3: 都有, 取 user loc.
		return fmt.Sprintf(`{"id":%d,"result":{"Goroutines":[
			{"id":1,"userCurrentLoc":{"file":"u.go","line":5,"function":{"name":"main.run"}},"currentLoc":{"file":"runtime.go","line":1}},
			{"id":2,"userCurrentLoc":{"file":"","line":0},"currentLoc":{"file":"runtime.go","line":7,"function":{"name":"runtime.gopark"}}},
			{"id":3,"userCurrentLoc":{"file":"u3.go","line":42,"function":{"name":"main.foo"}},"currentLoc":{"file":"runtime.go","line":99}}
		]},"error":null}`, id)
	})

	gs, err := sess.ListGoroutines()
	if err != nil {
		t.Fatalf("ListGoroutines: %v", err)
	}
	if len(gs) != 3 {
		t.Fatalf("want 3 goroutines, got %d: %+v", len(gs), gs)
	}
	want := []Goroutine{
		{ID: 1, File: "u.go", Line: 5, Function: "main.run"},
		{ID: 2, File: "runtime.go", Line: 7, Function: "runtime.gopark"},
		{ID: 3, File: "u3.go", Line: 42, Function: "main.foo"},
	}
	for i, w := range want {
		if gs[i] != w {
			t.Errorf("goroutine[%d] = %+v, want %+v", i, gs[i], w)
		}
	}
}

// TestListGoroutines_ErrorPath
func TestDlvListGoroutines_ErrorPath(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()

	srv.handle(func(req map[string]interface{}) string {
		id := int(req["id"].(float64))
		return fmt.Sprintf(`{"id":%d,"result":null,"error":"not stopped"}`, id)
	})

	gs, err := sess.ListGoroutines()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not stopped") {
		t.Errorf("err = %v, want dlv message", err)
	}
	if gs != nil {
		t.Errorf("gs should be nil on error, got %+v", gs)
	}
}

// TestListLocals_Decode: 验证 EvalScope + LoadConfig 形态, 解 3 个变量
func TestDlvListLocals_Decode(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()

	srv.handle(func(req map[string]interface{}) string {
		id := int(req["id"].(float64))
		if req["method"].(string) != "RPCServer.ListLocalVars" {
			return fmt.Sprintf(`{"id":%d,"result":null,"error":"wrong method %s"}`, id, req["method"])
		}
		first := req["params"].([]interface{})[0].(map[string]interface{})
		scope := first["Scope"].(map[string]interface{})
		if int(scope["GoroutineID"].(float64)) != -1 || int(scope["Frame"].(float64)) != 0 {
			return fmt.Sprintf(`{"id":%d,"result":null,"error":"want GoroutineID=-1 Frame=0"}`, id)
		}
		cfg := first["Cfg"].(map[string]interface{})
		// 默认上限的几个关键字段
		if int(cfg["MaxStringLen"].(float64)) != 256 {
			return fmt.Sprintf(`{"id":%d,"result":null,"error":"want MaxStringLen=256"}`, id)
		}
		if int(cfg["MaxArrayValues"].(float64)) != 64 {
			return fmt.Sprintf(`{"id":%d,"result":null,"error":"want MaxArrayValues=64"}`, id)
		}
		if int(cfg["MaxStructFields"].(float64)) != -1 {
			return fmt.Sprintf(`{"id":%d,"result":null,"error":"want MaxStructFields=-1"}`, id)
		}
		return fmt.Sprintf(`{"id":%d,"result":{"Variables":[
			{"name":"x","type":"int","value":"42"},
			{"name":"s","type":"string","value":"hello"},
			{"name":"p","type":"*int","value":"0xc000010050"}
		]},"error":null}`, id)
	})

	vs, err := sess.ListLocals(-1, 0)
	if err != nil {
		t.Fatalf("ListLocals: %v", err)
	}
	if len(vs) != 3 {
		t.Fatalf("want 3 vars, got %d: %+v", len(vs), vs)
	}
	want := []Variable{
		{Name: "x", Type: "int", Value: "42"},
		{Name: "s", Type: "string", Value: "hello"},
		{Name: "p", Type: "*int", Value: "0xc000010050"},
	}
	for i, w := range want {
		if vs[i] != w {
			t.Errorf("var[%d] = %+v, want %+v", i, vs[i], w)
		}
	}
}

// TestListLocals_ErrorPath
func TestDlvListLocals_ErrorPath(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()

	srv.handle(func(req map[string]interface{}) string {
		id := int(req["id"].(float64))
		return fmt.Sprintf(`{"id":%d,"result":null,"error":"frame out of range"}`, id)
	})

	vs, err := sess.ListLocals(-1, 99)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "frame out of range") {
		t.Errorf("err = %v, want dlv message", err)
	}
	if vs != nil {
		t.Errorf("vs should be nil on error, got %+v", vs)
	}
}

// TestEval_Decode: 验证 Expr/Scope 形态, 解一个 Variable
func TestDlvEval_Decode(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()

	srv.handle(func(req map[string]interface{}) string {
		id := int(req["id"].(float64))
		if req["method"].(string) != "RPCServer.Eval" {
			return fmt.Sprintf(`{"id":%d,"result":null,"error":"wrong method %s"}`, id, req["method"])
		}
		first := req["params"].([]interface{})[0].(map[string]interface{})
		if first["Expr"].(string) != "x+1" {
			return fmt.Sprintf(`{"id":%d,"result":null,"error":"want Expr=x+1"}`, id)
		}
		scope := first["Scope"].(map[string]interface{})
		if int(scope["GoroutineID"].(float64)) != -1 || int(scope["Frame"].(float64)) != 0 {
			return fmt.Sprintf(`{"id":%d,"result":null,"error":"want GoroutineID=-1 Frame=0"}`, id)
		}
		return fmt.Sprintf(`{"id":%d,"result":{"Variable":{"name":"x+1","type":"int","value":"43"}},"error":null}`, id)
	})

	v, err := sess.Eval("x+1", -1, 0)
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}
	want := Variable{Name: "x+1", Type: "int", Value: "43"}
	if v != want {
		t.Errorf("v = %+v, want %+v", v, want)
	}
}

// TestEval_ErrorPath: 表达式不存在 -> dlv 返回 error, Eval 也返回 error
func TestDlvEval_ErrorPath(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()

	srv.handle(func(req map[string]interface{}) string {
		id := int(req["id"].(float64))
		return fmt.Sprintf(`{"id":%d,"result":null,"error":"could not find symbol value for nope"}`, id)
	})

	v, err := sess.Eval("nope", -1, 0)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "could not find symbol") {
		t.Errorf("err = %v, want dlv message", err)
	}
	if v != (Variable{}) {
		t.Errorf("v should be zero on error, got %+v", v)
	}
}

// TestEval_ClosedSession: Close 后再 Eval 应该立刻返回 closed 错误
func TestDlvEval_ClosedSession(t *testing.T) {
	sess := closedSession()
	_, err := sess.Eval("x", -1, 0)
	if err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("want closed error, got %v", err)
	}
}

// TestListGoroutines_ClosedSession + TestListLocals_ClosedSession
// 一并覆盖 closed 早退路径, 防止后续重构忘了某条
func TestDlvListGoroutines_ClosedSession(t *testing.T) {
	sess := closedSession()
	_, err := sess.ListGoroutines()
	if err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("want closed error, got %v", err)
	}
}

func TestDlvListLocals_ClosedSession(t *testing.T) {
	sess := closedSession()
	_, err := sess.ListLocals(-1, 0)
	if err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("want closed error, got %v", err)
	}
}

// -----------------------------------------------------------------------------
// SetVariable / SetConditionalBreakpoint / Restart -- protocol-level tests
// 同样用 fake server, 只验证请求形态 + 应答解码, 不依赖 dlv 子进程.
// -----------------------------------------------------------------------------

// TestSetVariable_Decode: 断言 method=RPCServer.Set, params 带正确的
// Scope/Symbol/Value, 应答空 result -> 方法返回 nil.
func TestDlvSetVariable_Decode(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()

	srv.handle(func(req map[string]interface{}) string {
		id := int(req["id"].(float64))
		if req["method"].(string) != "RPCServer.Set" {
			return fmt.Sprintf(`{"id":%d,"result":null,"error":"wrong method %s"}`, id, req["method"])
		}
		first := req["params"].([]interface{})[0].(map[string]interface{})
		if first["Symbol"].(string) != "x" {
			return fmt.Sprintf(`{"id":%d,"result":null,"error":"want Symbol=x"}`, id)
		}
		if first["Value"].(string) != "99" {
			return fmt.Sprintf(`{"id":%d,"result":null,"error":"want Value=99"}`, id)
		}
		scope := first["Scope"].(map[string]interface{})
		if int(scope["GoroutineID"].(float64)) != -1 || int(scope["Frame"].(float64)) != 0 {
			return fmt.Sprintf(`{"id":%d,"result":null,"error":"want GoroutineID=-1 Frame=0"}`, id)
		}
		// RPCServer.Set 在 dlv 里应答空 -- 给个 null result
		return fmt.Sprintf(`{"id":%d,"result":null,"error":null}`, id)
	})

	if err := sess.SetVariable("x", "99", -1, 0); err != nil {
		t.Fatalf("SetVariable: %v", err)
	}
}

// TestSetVariable_ErrorPath: dlv 回 error (类型不符/符号不存在) -> 方法返回 error
func TestDlvSetVariable_ErrorPath(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()

	srv.handle(func(req map[string]interface{}) string {
		id := int(req["id"].(float64))
		return fmt.Sprintf(`{"id":%d,"result":null,"error":"literal string can not be assigned to int"}`, id)
	})

	err := sess.SetVariable("x", `"oops"`, -1, 0)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "can not be assigned") {
		t.Errorf("err = %v, want dlv message", err)
	}
}

// TestSetVariable_ClosedSession: Close 后再调必须返回 closed 错误, 不 panic
func TestDlvSetVariable_ClosedSession(t *testing.T) {
	sess := closedSession()
	err := sess.SetVariable("x", "1", -1, 0)
	if err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("want closed error, got %v", err)
	}
}

// TestSetConditionalBreakpoint_Decode: 断言 Breakpoint 对象带 cond + file/line,
// 应答返回带 id 的断点 -> 方法回传该 id.
func TestDlvSetConditionalBreakpoint_Decode(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()

	srv.handle(func(req map[string]interface{}) string {
		id := int(req["id"].(float64))
		if req["method"].(string) != "RPCServer.CreateBreakpoint" {
			return fmt.Sprintf(`{"id":%d,"result":null,"error":"wrong method %s"}`, id, req["method"])
		}
		first := req["params"].([]interface{})[0].(map[string]interface{})
		bp := first["Breakpoint"].(map[string]interface{})
		if bp["cond"] == nil || bp["cond"].(string) != "i == 3" {
			return fmt.Sprintf(`{"id":%d,"result":null,"error":"want cond='i == 3', got %v"}`, id, bp["cond"])
		}
		if bp["file"].(string) != "main.go" || int(bp["line"].(float64)) != 12 {
			return fmt.Sprintf(`{"id":%d,"result":null,"error":"want file=main.go line=12"}`, id)
		}
		return fmt.Sprintf(
			`{"id":%d,"result":{"Breakpoint":{"id":5,"file":"main.go","line":12,"functionName":"main.loop"}},"error":null}`,
			id,
		)
	})

	bp, err := sess.SetConditionalBreakpoint("main.go", 12, "i == 3")
	if err != nil {
		t.Fatalf("SetConditionalBreakpoint: %v", err)
	}
	if bp.ID != 5 || bp.Line != 12 || bp.File != "main.go" || bp.Function != "main.loop" {
		t.Errorf("unexpected breakpoint: %+v", bp)
	}
}

// TestSetConditionalBreakpoint_ClosedSession
func TestDlvSetConditionalBreakpoint_ClosedSession(t *testing.T) {
	sess := closedSession()
	_, err := sess.SetConditionalBreakpoint("main.go", 12, "i == 3")
	if err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("want closed error, got %v", err)
	}
}

// TestSetBreakpoint_NoCondField: 重构后无条件断点必须仍然不在线缆上带 cond 字段
// (omitempty 保证). 这把 createBreakpoint 重构对既有 SetBreakpoint 行为的等价性
// 钉死, 防止回归.
func TestDlvSetBreakpoint_NoCondField(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()

	srv.handle(func(req map[string]interface{}) string {
		id := int(req["id"].(float64))
		first := req["params"].([]interface{})[0].(map[string]interface{})
		bp := first["Breakpoint"].(map[string]interface{})
		// 无条件断点: cond 键必须根本不存在
		if _, present := bp["cond"]; present {
			return fmt.Sprintf(`{"id":%d,"result":null,"error":"cond should be omitted on unconditional bp"}`, id)
		}
		if int(bp["line"].(float64)) != 6 {
			return fmt.Sprintf(`{"id":%d,"result":null,"error":"line wrong"}`, id)
		}
		return fmt.Sprintf(
			`{"id":%d,"result":{"Breakpoint":{"id":3,"file":"main.go","line":6,"functionName":"main.main"}},"error":null}`,
			id,
		)
	})

	bp, err := sess.SetBreakpoint("main.go", 6)
	if err != nil {
		t.Fatalf("SetBreakpoint: %v", err)
	}
	if bp.ID != 3 || bp.Line != 6 {
		t.Errorf("unexpected breakpoint: %+v", bp)
	}
}

// TestRestart_Empty: method=RPCServer.Restart, params 带 Position=""/ResetArgs=false,
// 应答 DiscardedBreakpoints 为空 -> 方法返回 nil.
func TestDlvRestart_Empty(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()

	srv.handle(func(req map[string]interface{}) string {
		id := int(req["id"].(float64))
		if req["method"].(string) != "RPCServer.Restart" {
			return fmt.Sprintf(`{"id":%d,"result":null,"error":"wrong method %s"}`, id, req["method"])
		}
		first := req["params"].([]interface{})[0].(map[string]interface{})
		if first["Position"].(string) != "" {
			return fmt.Sprintf(`{"id":%d,"result":null,"error":"want Position=''"}`, id)
		}
		if first["ResetArgs"].(bool) != false {
			return fmt.Sprintf(`{"id":%d,"result":null,"error":"want ResetArgs=false"}`, id)
		}
		return fmt.Sprintf(`{"id":%d,"result":{"DiscardedBreakpoints":[]},"error":null}`, id)
	})

	if err := sess.Restart(); err != nil {
		t.Fatalf("Restart: %v", err)
	}
}

// TestRestart_DiscardedNonEmpty: 即便 dlv 报告丢了断点, Restart 仍返回 nil 且不 panic
// (重启成功, 个别断点丢失只 Warn, 见 dlv.go 文档).
func TestDlvRestart_DiscardedNonEmpty(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()

	srv.handle(func(req map[string]interface{}) string {
		id := int(req["id"].(float64))
		return fmt.Sprintf(`{"id":%d,"result":{"DiscardedBreakpoints":[
			{"reason":"could not rebind"},
			{"reason":"file gone"}
		]},"error":null}`, id)
	})

	if err := sess.Restart(); err != nil {
		t.Fatalf("Restart with discarded bps should still be nil, got %v", err)
	}
}

// TestRestart_ErrorPath: dlv 报错 -> 方法返回 error
func TestDlvRestart_ErrorPath(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()

	srv.handle(func(req map[string]interface{}) string {
		id := int(req["id"].(float64))
		return fmt.Sprintf(`{"id":%d,"result":null,"error":"can not restart core dump"}`, id)
	})

	err := sess.Restart()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "can not restart") {
		t.Errorf("err = %v, want dlv message", err)
	}
}

// TestRestart_ClosedSession
func TestDlvRestart_ClosedSession(t *testing.T) {
	sess := closedSession()
	if err := sess.Restart(); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("want closed error, got %v", err)
	}
}

// -----------------------------------------------------------------------------
// Next / StepInto / StepOut -- step-over/into/out, protocol-level tests.
// 三个动作只在 dlv 的 Command name 上有差别 (next/step/stepOut); 都用 fake
// server 断言发出的 name, 并验证返回的 StopState 解码自应答的 currentThread.
// -----------------------------------------------------------------------------

// stepCmdServer 预编程 fake server: 断言 method=RPCServer.Command 且 name==want,
// 然后回一个停在 file:line(fn) 的 DebuggerState. 复用在三个 step 动作的测试里.
func stepCmdServer(t *testing.T, srv *fakeRPCServer, want, file string, line int, fn string) {
	t.Helper()
	srv.handle(func(req map[string]interface{}) string {
		id := int(req["id"].(float64))
		if req["method"].(string) != "RPCServer.Command" {
			return fmt.Sprintf(`{"id":%d,"result":null,"error":"wrong method %s"}`, id, req["method"])
		}
		first := req["params"].([]interface{})[0].(map[string]interface{})
		if first["name"] == nil || first["name"].(string) != want {
			return fmt.Sprintf(`{"id":%d,"result":null,"error":"want name=%s, got %v"}`, id, want, first["name"])
		}
		return fmt.Sprintf(
			`{"id":%d,"result":{"State":{"exited":false,"currentThread":{"file":%q,"line":%d,"function":{"name":%q}}}},"error":null}`,
			id, file, line, fn,
		)
	})
}

// TestNext_Decode: Next 必须发 {name:"next"}, 返回应答里的 File/Line/Function.
func TestDlvNext_Decode(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()

	stepCmdServer(t, srv, "next", "n.go", 11, "main.next")
	st, err := sess.Next()
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	want := &StopState{Reason: "next", File: "n.go", Line: 11, Function: "main.next"}
	if *st != *want {
		t.Errorf("Next state = %+v, want %+v", *st, *want)
	}
}

// TestStepInto_Decode: StepInto 必须发 {name:"step"}.
func TestDlvStepInto_Decode(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()

	stepCmdServer(t, srv, "step", "i.go", 22, "main.callee")
	st, err := sess.StepInto()
	if err != nil {
		t.Fatalf("StepInto: %v", err)
	}
	want := &StopState{Reason: "step", File: "i.go", Line: 22, Function: "main.callee"}
	if *st != *want {
		t.Errorf("StepInto state = %+v, want %+v", *st, *want)
	}
}

// TestStepOut_Decode: StepOut 必须发 {name:"stepOut"}.
func TestDlvStepOut_Decode(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()

	stepCmdServer(t, srv, "stepOut", "o.go", 33, "main.caller")
	st, err := sess.StepOut()
	if err != nil {
		t.Fatalf("StepOut: %v", err)
	}
	want := &StopState{Reason: "stepOut", File: "o.go", Line: 33, Function: "main.caller"}
	if *st != *want {
		t.Errorf("StepOut state = %+v, want %+v", *st, *want)
	}
}

// TestStep_IsNextSynonym: Step (历史别名) 必须仍发 {name:"next"}, 与 Next 等价.
// 钉死兼容性 -- silkide 仍可能引用 Step.
func TestDlvStep_IsNextSynonym(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()

	stepCmdServer(t, srv, "next", "s.go", 6, "main.main")
	st, err := sess.Step()
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	want := &StopState{Reason: "next", File: "s.go", Line: 6, Function: "main.main"}
	if *st != *want {
		t.Errorf("Step state = %+v, want %+v", *st, *want)
	}
}

// TestStepActions_ClosedSession: Step/Next/StepInto/StepOut 在 closed session 上
// 都必须返回 closed 错误且不 panic.
func TestDlvStepActions_ClosedSession(t *testing.T) {
	type stepFn struct {
		name string
		call func(*DebugSession) (*StopState, error)
	}
	for _, f := range []stepFn{
		{"Step", (*DebugSession).Step},
		{"Next", (*DebugSession).Next},
		{"StepInto", (*DebugSession).StepInto},
		{"StepOut", (*DebugSession).StepOut},
	} {
		sess := closedSession()
		_, err := f.call(sess)
		if err == nil || !strings.Contains(err.Error(), "closed") {
			t.Fatalf("%s on closed session: want closed error, got %v", f.name, err)
		}
	}
}

// -----------------------------------------------------------------------------
// Close vs 阻塞中的 rpcCall -- 死锁回归测试
// -----------------------------------------------------------------------------

// TestDlvClose_WhileBlockedInContinue: 一个后台 goroutine 的 Continue 持着 s.mu
// 阻塞在 Decode 上 (fake server 收下请求但永不应答, 模拟 debuggee 一直在跑),
// 此时调 Close. 修复前 Close 一上来就等 s.mu, 永远拿不到 -> IDE 点 Stop 冻死;
// 修复后 Close 先关 conn 唤醒 Decode, 必须立刻返回, Continue 侧拿到统一的
// errSessionClosed (而不是底层 read 错误).
func TestDlvClose_WhileBlockedInContinue(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()

	gotReq := make(chan struct{})
	release := make(chan struct{})
	defer close(release) // 收尾时放行 handler, 让 accept goroutine 退出
	srv.handle(func(req map[string]interface{}) string {
		close(gotReq)
		<-release // 扣住请求不应答
		return ""
	})

	contErr := make(chan error, 1)
	go func() {
		_, err := sess.Continue()
		contErr <- err
	}()

	// 等 fake server 真收到 Continue 请求, 确保 rpcCall 已持锁阻塞在 Decode 上
	select {
	case <-gotReq:
	case <-time.After(3 * time.Second):
		t.Fatal("fake server never received the Continue request")
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- sess.Close() }()

	// Close 必须立刻返回, 不许等 Continue 的应答 (修复前这里 3s 超时)
	select {
	case err := <-closeDone:
		if err != nil {
			t.Errorf("Close: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Close deadlocked while Continue was blocked in Decode")
	}

	// Continue 侧必须被唤醒, 且拿到干净的 session closed 错误
	select {
	case err := <-contErr:
		if err == nil {
			t.Fatal("Continue returned nil after Close, want session closed error")
		}
		if !errors.Is(err, errSessionClosed) {
			t.Errorf("Continue err = %v, want errSessionClosed", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Continue did not unblock after Close")
	}
}
