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

// TestRPCCall_OnClosedSession: closed=true 时 rpcCall 早退
func TestDlvRPCCall_OnClosedSession(t *testing.T) {
	sess := &DebugSession{closed: true}
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
type fakeRPCServer struct {
	ln      net.Listener
	conn    net.Conn
	br      *bufio.Reader
	t       *testing.T
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
		srv.conn = c
		srv.br = bufio.NewReader(c)
		// 服务循环: 每读一行就调当前 handler
		for {
			line, err := srv.br.ReadBytes('\n')
			if err != nil {
				_ = c.Close()
				return
			}
			var req map[string]interface{}
			if err := json.Unmarshal(line, &req); err != nil {
				continue
			}
			h := srv.current
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
	s.current = fn
}

func (s *fakeRPCServer) close() {
	if s.conn != nil {
		_ = s.conn.Close()
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
	sess := &DebugSession{closed: true}
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
	sess := &DebugSession{closed: true}
	_, err := sess.Eval("x", -1, 0)
	if err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("want closed error, got %v", err)
	}
}

// TestListGoroutines_ClosedSession + TestListLocals_ClosedSession
// 一并覆盖 closed 早退路径, 防止后续重构忘了某条
func TestDlvListGoroutines_ClosedSession(t *testing.T) {
	sess := &DebugSession{closed: true}
	_, err := sess.ListGoroutines()
	if err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("want closed error, got %v", err)
	}
}

func TestDlvListLocals_ClosedSession(t *testing.T) {
	sess := &DebugSession{closed: true}
	_, err := sess.ListLocals(-1, 0)
	if err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("want closed error, got %v", err)
	}
}
