package core

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// 进阶 Delve 能力的协议测试: 断点属性/清除/修改, goroutine 与帧作用域,
// 变量懒展开, goroutine 分页, debuggee 输出泵.
//
// 一律不拉起真的 dlv: 复用 dlv_test.go 里的 fakeRPCServer (newline-JSON over TCP)
// 作为注入点, 用 canned JSON 驱动"请求编码 + 应答解码"两个方向.
// 断言分两类:
//   - 编码方向: dlvRouteRPC 记录每次请求, 测试 goroutine 事后检查 method 与 params;
//   - 解码方向: canned result 走完 rpcCall, 检查投影出来的模型字段.
// 输出泵那几个测试连 server 都不需要 -- 直接喂 io.Reader.

// -----------------------------------------------------------------------------
// 测试脚手架: 按 method 分派 canned 应答 + 记录请求
// -----------------------------------------------------------------------------

// dlvCall 是 fake server 收到的一次请求 (method 已去掉 "RPCServer." 前缀)
type dlvCall struct {
	method string
	params map[string]interface{}
}

// dlvRecorder 收集请求, 供测试 goroutine 事后断言
// handler 跑在 fake server 自己的 goroutine 上, 那里不能调 t.Fatalf, 所以断言
// 一律搬到测试 goroutine 上做 -- handler 只负责记录和回 canned 数据.
type dlvRecorder struct {
	mu    sync.Mutex
	calls []dlvCall
}

func (r *dlvRecorder) add(c dlvCall) {
	r.mu.Lock()
	r.calls = append(r.calls, c)
	r.mu.Unlock()
}

func (r *dlvRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

// methods 返回按发生顺序的方法名, 用来断言"调了哪几个 RPC / 顺序对不对"
func (r *dlvRecorder) methods() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.calls))
	for _, c := range r.calls {
		out = append(out, c.method)
	}
	return out
}

// call 取第 i 次请求; 越界直接 Fatal, 免得后面对着零值断言出一堆误导性失败
func (r *dlvRecorder) call(t *testing.T, i int) dlvCall {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if i >= len(r.calls) {
		t.Fatalf("want at least %d call(s), got %d: %v", i+1, len(r.calls), r.calls)
	}
	return r.calls[i]
}

// dlvRouteRPC 让 fake server 按方法名回 canned result
// routes 的 key 是去掉 "RPCServer." 前缀的方法名, value 是 result 字段的 JSON
// 片段 ("null" = 空应答). 没登记的方法一律回 error, 于是"发错了 RPC"必然让被测
// 方法失败, 测试不会静默通过.
func dlvRouteRPC(t *testing.T, srv *fakeRPCServer, routes map[string]string) *dlvRecorder {
	t.Helper()
	rec := &dlvRecorder{}
	srv.handle(func(req map[string]interface{}) string {
		id := int(req["id"].(float64))
		method, _ := req["method"].(string)
		method = strings.TrimPrefix(method, "RPCServer.")
		var params map[string]interface{}
		if arr, ok := req["params"].([]interface{}); ok && len(arr) > 0 {
			params, _ = arr[0].(map[string]interface{})
		}
		rec.add(dlvCall{method: method, params: params})
		result, ok := routes[method]
		if !ok {
			return fmt.Sprintf(`{"id":%d,"result":null,"error":"unexpected method %s"}`, id, method)
		}
		return fmt.Sprintf(`{"id":%d,"result":%s,"error":null}`, id, result)
	})
	return rec
}

// 下面四个取字段的小工具都在测试 goroutine 上跑, 缺字段/类型不对直接 Fatal

func dlvObjField(t *testing.T, m map[string]interface{}, key string) map[string]interface{} {
	t.Helper()
	v, ok := m[key].(map[string]interface{})
	if !ok {
		t.Fatalf("field %q is not an object in %v", key, m)
	}
	return v
}

func dlvNumField(t *testing.T, m map[string]interface{}, key string) float64 {
	t.Helper()
	v, ok := m[key].(float64)
	if !ok {
		t.Fatalf("field %q is not a number in %v", key, m)
	}
	return v
}

func dlvStrField(t *testing.T, m map[string]interface{}, key string) string {
	t.Helper()
	v, ok := m[key].(string)
	if !ok {
		t.Fatalf("field %q is not a string in %v", key, m)
	}
	return v
}

func dlvBoolField(t *testing.T, m map[string]interface{}, key string) bool {
	t.Helper()
	v, ok := m[key].(bool)
	if !ok {
		t.Fatalf("field %q is not a bool in %v", key, m)
	}
	return v
}

// -----------------------------------------------------------------------------
// 断点属性: 条件 / 命中次数 / logpoint / enabled / verified
// -----------------------------------------------------------------------------

// TestDlvBreakpointDecode_RichFields: ListBreakpoints 的应答带上 dlv 的全套断点
// 字段时, 模型必须解出 Cond/HitCount/Tracepoint/LogMessage/Enabled/Verified.
// 三条数据分别覆盖: 普通条件断点(已绑定) / logpoint(disabled, 用 "continue" 键) /
// 用 "tracepoint" 别名 + addrs 表达已绑定.
func TestDlvBreakpointDecode_RichFields(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()

	dlvRouteRPC(t, srv, map[string]string{
		"ListBreakpoints": `{"Breakpoints":[
			{"id":1,"file":"main.go","line":12,"functionName":"main.loop",
			 "cond":"i == 3","totalHitCount":7,"addr":4198400},
			{"id":2,"file":"main.go","line":20,"continue":true,
			 "variables":["i","err"],"disabled":true},
			{"id":3,"file":"b.go","line":5,"tracepoint":true,"addrs":[4198500,4198600]}
		]}`,
	})

	bps, err := sess.ListBreakpoints()
	if err != nil {
		t.Fatalf("ListBreakpoints: %v", err)
	}
	if len(bps) != 3 {
		t.Fatalf("want 3 breakpoints, got %d: %+v", len(bps), bps)
	}

	want := []Breakpoint{
		{ID: 1, File: "main.go", Line: 12, Function: "main.loop",
			Cond: "i == 3", HitCount: 7, Enabled: true, Verified: true},
		{ID: 2, File: "main.go", Line: 20,
			Tracepoint: true, LogMessage: "i", Enabled: false, Verified: false},
		{ID: 3, File: "b.go", Line: 5,
			Tracepoint: true, Enabled: true, Verified: true},
	}
	for i, w := range want {
		if bps[i] != w {
			t.Errorf("bp[%d] = %+v\nwant %+v", i, bps[i], w)
		}
	}
}

// TestDlvBreakpointDecode_HitCountMapIgnored: dlv 的应答里 "hitCount" 是
// map[goroutine]count, 我们的 HitCount 取的是标量 "totalHitCount".
// 这个测试钉死两者不能搞混 -- 一旦把标量绑到 "hitCount" 上, 整个应答解码就会炸.
func TestDlvBreakpointDecode_HitCountMapIgnored(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()

	dlvRouteRPC(t, srv, map[string]string{
		"ListBreakpoints": `{"Breakpoints":[
			{"id":1,"file":"main.go","line":9,"hitCount":{"5":2,"9":4},"totalHitCount":6}
		]}`,
	})

	bps, err := sess.ListBreakpoints()
	if err != nil {
		t.Fatalf("ListBreakpoints: %v", err)
	}
	if len(bps) != 1 || bps[0].HitCount != 6 {
		t.Fatalf("want HitCount=6 from totalHitCount, got %+v", bps)
	}
}

// TestDlvAmendBreakpoint_Encode: AmendBreakpoint 必须发 RPCServer.AmendBreakpoint,
// 把条件/logpoint/禁用位放进 Breakpoint 对象; 只读的 HitCount 不能出现在线缆上.
func TestDlvAmendBreakpoint_Encode(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()

	rec := dlvRouteRPC(t, srv, map[string]string{"AmendBreakpoint": "null"})

	bp := &Breakpoint{
		ID: 4, File: "main.go", Line: 12,
		Cond: "i == 3", Tracepoint: true, LogMessage: "i",
		HitCount: 99,    // 只读: 不应该被写回
		Enabled:  false, // -> disabled:true
	}
	if err := sess.AmendBreakpoint(bp); err != nil {
		t.Fatalf("AmendBreakpoint: %v", err)
	}

	c := rec.call(t, 0)
	if c.method != "AmendBreakpoint" {
		t.Fatalf("method = %q, want AmendBreakpoint", c.method)
	}
	obj := dlvObjField(t, c.params, "Breakpoint")
	if got := dlvNumField(t, obj, "id"); got != 4 {
		t.Errorf("id = %v, want 4", got)
	}
	if got := dlvStrField(t, obj, "cond"); got != "i == 3" {
		t.Errorf("cond = %q, want 'i == 3'", got)
	}
	if !dlvBoolField(t, obj, "continue") {
		t.Error(`want continue=true (dlv 的 tracepoint 标志)`)
	}
	if !dlvBoolField(t, obj, "disabled") {
		t.Error("want disabled=true for Enabled=false")
	}
	vars, ok := obj["variables"].([]interface{})
	if !ok || len(vars) != 1 || vars[0] != "i" {
		t.Errorf(`variables = %v, want ["i"] (logpoint 打印表达式)`, obj["variables"])
	}
	if _, present := obj["totalHitCount"]; present {
		t.Error("totalHitCount must never be sent -- HitCount is read-only")
	}
}

// TestDlvAmendBreakpoint_EnabledOmitsDisabled: Enabled=true 时线缆上不该出现
// disabled 键 (omitempty), 与 dlv 默认状态一致.
func TestDlvAmendBreakpoint_EnabledOmitsDisabled(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()

	rec := dlvRouteRPC(t, srv, map[string]string{"AmendBreakpoint": "null"})
	if err := sess.AmendBreakpoint(&Breakpoint{ID: 2, Enabled: true}); err != nil {
		t.Fatalf("AmendBreakpoint: %v", err)
	}
	obj := dlvObjField(t, rec.call(t, 0).params, "Breakpoint")
	if _, present := obj["disabled"]; present {
		t.Errorf("disabled should be omitted when Enabled=true, got %v", obj)
	}
	if _, present := obj["cond"]; present {
		t.Errorf("cond should be omitted when empty, got %v", obj)
	}
}

// TestDlvAmendBreakpoint_Rejects: nil / 非法 ID 必须在本地就被拒, 一个字节都不发.
// 否则 dlv 会拿 id=0 报一个含义不清的错误.
func TestDlvAmendBreakpoint_Rejects(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()
	rec := dlvRouteRPC(t, srv, map[string]string{"AmendBreakpoint": "null"})

	if err := sess.AmendBreakpoint(nil); err == nil {
		t.Error("AmendBreakpoint(nil) should fail")
	}
	for _, id := range []int{0, -1} {
		if err := sess.AmendBreakpoint(&Breakpoint{ID: id}); err == nil {
			t.Errorf("AmendBreakpoint(id=%d) should fail", id)
		}
	}
	if n := rec.count(); n != 0 {
		t.Errorf("%d RPC(s) sent for invalid amends, want 0", n)
	}
}

// TestDlvClearBreakpoint_Encode: 按 ID 删 -> RPCServer.ClearBreakpoint {Id}
func TestDlvClearBreakpoint_Encode(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()

	rec := dlvRouteRPC(t, srv, map[string]string{
		"ClearBreakpoint": `{"Breakpoint":{"id":7,"file":"main.go","line":9}}`,
	})
	if err := sess.ClearBreakpoint(7); err != nil {
		t.Fatalf("ClearBreakpoint: %v", err)
	}
	c := rec.call(t, 0)
	if c.method != "ClearBreakpoint" {
		t.Fatalf("method = %q, want ClearBreakpoint", c.method)
	}
	if got := dlvNumField(t, c.params, "Id"); got != 7 {
		t.Errorf("Id = %v, want 7", got)
	}
}

// TestDlvClearBreakpoint_ErrorPath: dlv 说断点不存在 -> 原样上抛
func TestDlvClearBreakpoint_ErrorPath(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()

	srv.handle(func(req map[string]interface{}) string {
		id := int(req["id"].(float64))
		return fmt.Sprintf(`{"id":%d,"result":null,"error":"no breakpoint with ID 42"}`, id)
	})
	err := sess.ClearBreakpoint(42)
	if err == nil || !strings.Contains(err.Error(), "no breakpoint with ID 42") {
		t.Fatalf("err = %v, want dlv message", err)
	}
}

// TestDlvClearBreakpointByLocation_ResolvesID: 编辑器只知道 file:line ->
// 先 ListBreakpoints 找 ID, 再 ClearBreakpoint. 顺序和参数都钉住.
func TestDlvClearBreakpointByLocation_ResolvesID(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()

	rec := dlvRouteRPC(t, srv, map[string]string{
		"ListBreakpoints": `{"Breakpoints":[
			{"id":8,"file":"/w/main.go","line":10},
			{"id":9,"file":"/w/main.go","line":12},
			{"id":10,"file":"/w/other.go","line":12}
		]}`,
		"ClearBreakpoint": "null",
	})

	if err := sess.ClearBreakpointByLocation("/w/main.go", 12); err != nil {
		t.Fatalf("ClearBreakpointByLocation: %v", err)
	}
	if got := rec.methods(); !reflect.DeepEqual(got, []string{"ListBreakpoints", "ClearBreakpoint"}) {
		t.Fatalf("call sequence = %v, want [ListBreakpoints ClearBreakpoint]", got)
	}
	if id := dlvNumField(t, rec.call(t, 1).params, "Id"); id != 9 {
		t.Errorf("cleared Id = %v, want 9 (file+line 都要匹配)", id)
	}
}

// TestDlvClearBreakpointByLocation_RelativePath: dlv 一律回绝对路径, IDE 手里可能
// 是相对路径 -- 后缀匹配必须认出这是同一个文件.
func TestDlvClearBreakpointByLocation_RelativePath(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()

	rec := dlvRouteRPC(t, srv, map[string]string{
		"ListBreakpoints": `{"Breakpoints":[{"id":5,"file":"/w/proj/pkg/main.go","line":30}]}`,
		"ClearBreakpoint": "null",
	})
	if err := sess.ClearBreakpointByLocation("pkg/main.go", 30); err != nil {
		t.Fatalf("ClearBreakpointByLocation: %v", err)
	}
	if id := dlvNumField(t, rec.call(t, 1).params, "Id"); id != 5 {
		t.Errorf("cleared Id = %v, want 5", id)
	}
}

// TestDlvClearBreakpointByLocation_NotFound: 该行没断点 -> 报错, 且不发 Clear
func TestDlvClearBreakpointByLocation_NotFound(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()

	rec := dlvRouteRPC(t, srv, map[string]string{
		"ListBreakpoints": `{"Breakpoints":[{"id":8,"file":"/w/main.go","line":10}]}`,
		"ClearBreakpoint": "null",
	})
	err := sess.ClearBreakpointByLocation("/w/main.go", 99)
	if err == nil || !strings.Contains(err.Error(), "no breakpoint at") {
		t.Fatalf("err = %v, want 'no breakpoint at ...'", err)
	}
	if got := rec.methods(); !reflect.DeepEqual(got, []string{"ListBreakpoints"}) {
		t.Errorf("calls = %v, want only ListBreakpoints", got)
	}
}

// TestDlvToggleBreakpoint_CreatesWhenAbsent: gutter 点在空行上 -> List 之后 Create
func TestDlvToggleBreakpoint_CreatesWhenAbsent(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()

	rec := dlvRouteRPC(t, srv, map[string]string{
		"ListBreakpoints":  `{"Breakpoints":[{"id":1,"file":"/w/main.go","line":10}]}`,
		"CreateBreakpoint": `{"Breakpoint":{"id":2,"file":"/w/main.go","line":18,"functionName":"main.run","addr":4198400}}`,
	})

	bp, err := sess.ToggleBreakpoint("/w/main.go", 18)
	if err != nil {
		t.Fatalf("ToggleBreakpoint: %v", err)
	}
	if bp == nil {
		t.Fatal("want a created breakpoint, got nil")
	}
	if bp.ID != 2 || bp.Line != 18 || bp.Function != "main.run" || !bp.Enabled || !bp.Verified {
		t.Errorf("created bp = %+v", *bp)
	}
	if got := rec.methods(); !reflect.DeepEqual(got, []string{"ListBreakpoints", "CreateBreakpoint"}) {
		t.Errorf("calls = %v, want [ListBreakpoints CreateBreakpoint]", got)
	}
}

// TestDlvToggleBreakpoint_ClearsWhenPresent: 同一行再点一次 -> 删掉, 返回 (nil, nil)
func TestDlvToggleBreakpoint_ClearsWhenPresent(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()

	rec := dlvRouteRPC(t, srv, map[string]string{
		"ListBreakpoints": `{"Breakpoints":[{"id":3,"file":"/w/main.go","line":18}]}`,
		"ClearBreakpoint": "null",
	})

	bp, err := sess.ToggleBreakpoint("/w/main.go", 18)
	if err != nil {
		t.Fatalf("ToggleBreakpoint: %v", err)
	}
	if bp != nil {
		t.Errorf("want nil breakpoint on the clear path, got %+v", *bp)
	}
	if got := rec.methods(); !reflect.DeepEqual(got, []string{"ListBreakpoints", "ClearBreakpoint"}) {
		t.Fatalf("calls = %v, want [ListBreakpoints ClearBreakpoint]", got)
	}
	if id := dlvNumField(t, rec.call(t, 1).params, "Id"); id != 3 {
		t.Errorf("cleared Id = %v, want 3", id)
	}
}

// TestDlvToggleBreakpoint_SkipsInternalBreakpoint: dlv 内部断点 (ID<0, 例如
// unrecovered-panic) 落在同一行时不能被用户 toggle 掉 -- 应当照常新建.
func TestDlvToggleBreakpoint_SkipsInternalBreakpoint(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()

	rec := dlvRouteRPC(t, srv, map[string]string{
		"ListBreakpoints":  `{"Breakpoints":[{"id":-1,"file":"/w/main.go","line":18}]}`,
		"CreateBreakpoint": `{"Breakpoint":{"id":4,"file":"/w/main.go","line":18}}`,
	})
	bp, err := sess.ToggleBreakpoint("/w/main.go", 18)
	if err != nil {
		t.Fatalf("ToggleBreakpoint: %v", err)
	}
	if bp == nil || bp.ID != 4 {
		t.Fatalf("want newly created bp id=4, got %+v", bp)
	}
	if got := rec.methods(); !reflect.DeepEqual(got, []string{"ListBreakpoints", "CreateBreakpoint"}) {
		t.Errorf("calls = %v, want [ListBreakpoints CreateBreakpoint]", got)
	}
}

// TestDlvSameSourceFile: 路径匹配规则的直接单测
func TestDlvSameSourceFile(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"/w/main.go", "/w/main.go", true},
		{"/w/./main.go", "/w/main.go", true},
		{"main.go", "./main.go", true},
		{"/w/proj/pkg/main.go", "pkg/main.go", true},
		{"pkg/main.go", "/w/proj/pkg/main.go", true},
		{"/w/proj/pkg/barmain.go", "main.go", false}, // 后缀必须落在分隔符上
		{"/w/a/main.go", "/w/b/main.go", false},
		{"a/main.go", "b/main.go", false},
		{"/w/main.go", "", false},

		// Windows 形态: dlv 回带盘符的反斜杠路径, IDE 手里是正斜杠相对路径.
		// 这几条在 POSIX 上也必须成立 -- 分隔符与盘符的判定不依赖运行平台,
		// 否则整组用例只在 mac 上是绿的, Windows 上按位置清断点全线失效.
		{`C:\w\proj\pkg\main.go`, "pkg/main.go", true},
		{`C:\w\proj\pkg\main.go`, `pkg\main.go`, true},
		{`C:\w\a\main.go`, `C:\w\a\main.go`, true},
		{`C:\w\a\main.go`, `C:\w\b\main.go`, false},
		{`C:\w\proj\pkg\barmain.go`, "main.go", false},
		// 无盘符的根路径 -- dlv 在 Linux debuggee 上就是这么回的,
		// filepath.IsAbs 在 Windows 上不认它, 曾因此两边都判成相对路径.
		{"/w/proj/pkg/main.go", `pkg\main.go`, true},
	}
	for _, c := range cases {
		if got := sameSourceFile(c.a, c.b); got != c.want {
			t.Errorf("sameSourceFile(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

// TestDlvSameSourceFileIsCaseInsensitiveOnWindows: dlv 回 "C:\\W\\Main.go" 而
// IDE 记 "c:/w/main.go" 是常态, 在大小写不敏感的文件系统上这必须算同一个文件.
func TestDlvSameSourceFileIsCaseInsensitiveOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("大小写折叠只在 windows 上生效")
	}
	if !sameSourceFile(`C:\W\PROJ\Main.go`, "c:/w/proj/main.go") {
		t.Error("同一个文件因大小写不同被判成两个")
	}
}

// -----------------------------------------------------------------------------
// goroutine / frame 上下文
// -----------------------------------------------------------------------------

// TestDlvSwitchGoroutine_Encode: 必须发 Command{name:"switchGoroutine", goroutineID}
// 并把选择记在 session 上.
func TestDlvSwitchGoroutine_Encode(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()

	if got := sess.SelectedGoroutine(); got != -1 {
		t.Fatalf("SelectedGoroutine() = %d before switching, want -1", got)
	}

	rec := dlvRouteRPC(t, srv, map[string]string{
		"Command": `{"State":{"exited":false,"currentThread":{"file":"g.go","line":3,"function":{"name":"main.worker"}}}}`,
	})
	if err := sess.SwitchGoroutine(17); err != nil {
		t.Fatalf("SwitchGoroutine: %v", err)
	}

	c := rec.call(t, 0)
	if c.method != "Command" {
		t.Fatalf("method = %q, want Command", c.method)
	}
	if got := dlvStrField(t, c.params, "name"); got != "switchGoroutine" {
		t.Errorf("name = %q, want switchGoroutine", got)
	}
	if got := dlvNumField(t, c.params, "goroutineID"); got != 17 {
		t.Errorf("goroutineID = %v, want 17", got)
	}
	if got := sess.SelectedGoroutine(); got != 17 {
		t.Errorf("SelectedGoroutine() = %d after switch, want 17", got)
	}
}

// TestDlvSwitchGoroutine_ErrorKeepsSelection: dlv 拒绝切换时本地选择不能被改坏
func TestDlvSwitchGoroutine_ErrorKeepsSelection(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()

	srv.handle(func(req map[string]interface{}) string {
		id := int(req["id"].(float64))
		return fmt.Sprintf(`{"id":%d,"result":null,"error":"unknown goroutine 999"}`, id)
	})
	if err := sess.SwitchGoroutine(999); err == nil {
		t.Fatal("expected error from dlv")
	}
	if got := sess.SelectedGoroutine(); got != -1 {
		t.Errorf("SelectedGoroutine() = %d after failed switch, want -1", got)
	}
}

// TestDlvCommand_OmitsGoroutineIDWhenUnset: 普通 continue/next 的线缆形态不能因为
// 新增 goroutineID 字段而变化 (omitempty), 否则老 dlv 行为可能被改.
func TestDlvCommand_OmitsGoroutineIDWhenUnset(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()

	rec := dlvRouteRPC(t, srv, map[string]string{
		"Command": `{"State":{"exited":false,"currentThread":{"file":"a.go","line":1}}}`,
	})
	if _, err := sess.Continue(); err != nil {
		t.Fatalf("Continue: %v", err)
	}
	if _, present := rec.call(t, 0).params["goroutineID"]; present {
		t.Error("goroutineID must be omitted for plain continue")
	}
}

// TestDlvScopedCalls_FollowSelectedGoroutine: SwitchGoroutine 之后, 所有传"当前"
// (负数) 的作用域方法都必须把选中的 goroutine 发出去 -- 这就是变量/调用栈跟着
// goroutine 面板换上下文的机制.
func TestDlvScopedCalls_FollowSelectedGoroutine(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()

	rec := dlvRouteRPC(t, srv, map[string]string{
		"Command":          `{"State":{"exited":false,"currentThread":{"file":"g.go","line":3}}}`,
		"ListLocalVars":    `{"Variables":[]}`,
		"ListFunctionArgs": `{"Args":[]}`,
		"Eval":             `{"Variable":{"name":"x","type":"int","value":"1"}}`,
		"Set":              "null",
		"Stacktrace":       `{"Locations":[]}`,
	})

	if err := sess.SwitchGoroutine(17); err != nil {
		t.Fatalf("SwitchGoroutine: %v", err)
	}
	if _, err := sess.ListLocals(-1, 2); err != nil {
		t.Fatalf("ListLocals: %v", err)
	}
	if _, err := sess.ListArgs(-1, 2); err != nil {
		t.Fatalf("ListArgs: %v", err)
	}
	if _, err := sess.Eval("x", -1, 2); err != nil {
		t.Fatalf("Eval: %v", err)
	}
	if err := sess.SetVariable("x", "1", -1, 2); err != nil {
		t.Fatalf("SetVariable: %v", err)
	}
	if _, err := sess.Stacktrace(-1, 20); err != nil {
		t.Fatalf("Stacktrace: %v", err)
	}

	// call 0 是 switchGoroutine; 1..4 带 Scope, 5 是 Stacktrace (字段名 Id)
	for i := 1; i <= 4; i++ {
		c := rec.call(t, i)
		scope := dlvObjField(t, c.params, "Scope")
		if got := dlvNumField(t, scope, "GoroutineID"); got != 17 {
			t.Errorf("%s Scope.GoroutineID = %v, want 17", c.method, got)
		}
		if got := dlvNumField(t, scope, "Frame"); got != 2 {
			t.Errorf("%s Scope.Frame = %v, want 2", c.method, got)
		}
	}
	st := rec.call(t, 5)
	if got := dlvNumField(t, st.params, "Id"); got != 17 {
		t.Errorf("Stacktrace Id = %v, want 17", got)
	}
}

// TestDlvScopedCalls_ExplicitGoroutineWins: 点名某个 goroutine 时不受 SwitchGoroutine
// 影响 -- 面板要能在不改变全局选择的情况下窥视另一个 goroutine.
func TestDlvScopedCalls_ExplicitGoroutineWins(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()

	rec := dlvRouteRPC(t, srv, map[string]string{
		"Command":       `{"State":{"exited":false,"currentThread":{"file":"g.go","line":3}}}`,
		"ListLocalVars": `{"Variables":[]}`,
	})
	if err := sess.SwitchGoroutine(17); err != nil {
		t.Fatalf("SwitchGoroutine: %v", err)
	}
	if _, err := sess.ListLocals(5, 0); err != nil {
		t.Fatalf("ListLocals: %v", err)
	}
	scope := dlvObjField(t, rec.call(t, 1).params, "Scope")
	if got := dlvNumField(t, scope, "GoroutineID"); got != 5 {
		t.Errorf("Scope.GoroutineID = %v, want the explicit 5", got)
	}
}

// -----------------------------------------------------------------------------
// 函数参数 + 变量懒展开
// -----------------------------------------------------------------------------

// TestDlvListArgs_Decode: ListArgs 走 RPCServer.ListFunctionArgs, 应答字段是 "Args"
// (不是 ListLocalVars 的 "Variables"), 作用域按传入的 goroutine/frame.
func TestDlvListArgs_Decode(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()

	rec := dlvRouteRPC(t, srv, map[string]string{
		"ListFunctionArgs": fmt.Sprintf(`{"Args":[
			{"name":"n","type":"int","value":"7","kind":%d,"addr":824633786448},
			{"name":"names","type":"[]string","value":"","kind":%d,"len":3,"cap":4}
		]}`, int(reflect.Int), int(reflect.Slice)),
	})

	args, err := sess.ListArgs(9, 1)
	if err != nil {
		t.Fatalf("ListArgs: %v", err)
	}
	c := rec.call(t, 0)
	if c.method != "ListFunctionArgs" {
		t.Fatalf("method = %q, want ListFunctionArgs", c.method)
	}
	scope := dlvObjField(t, c.params, "Scope")
	if got := dlvNumField(t, scope, "GoroutineID"); got != 9 {
		t.Errorf("Scope.GoroutineID = %v, want 9", got)
	}
	if got := dlvNumField(t, scope, "Frame"); got != 1 {
		t.Errorf("Scope.Frame = %v, want 1", got)
	}

	want := []Variable{
		{Name: "n", Type: "int", Value: "7", Kind: reflect.Int.String(), Addr: 824633786448},
		{Name: "names", Type: "[]string", Kind: reflect.Slice.String(), Len: 3, Cap: 4},
	}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("args = %+v\nwant %+v", args, want)
	}
}

// TestDlvListArgs_EmptyIsNonNil: 没有参数时返回空 slice 而不是 nil,
// 与 ListLocals 的既有行为一致 (UI 侧可以直接 range).
func TestDlvListArgs_EmptyIsNonNil(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()

	dlvRouteRPC(t, srv, map[string]string{"ListFunctionArgs": `{"Args":[]}`})
	args, err := sess.ListArgs(-1, 0)
	if err != nil {
		t.Fatalf("ListArgs: %v", err)
	}
	if args == nil {
		t.Fatal("args = nil, want empty non-nil slice")
	}
	if len(args) != 0 {
		t.Errorf("args = %+v, want empty", args)
	}
}

// TestDlvLoadVariable_LazyChildren: 懒展开的核心用例 -- depth 透传到
// LoadConfig.MaxVariableRecurse, 应答里的嵌套 children 递归解成 Variable 树.
func TestDlvLoadVariable_LazyChildren(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()

	rec := dlvRouteRPC(t, srv, map[string]string{
		"Eval": fmt.Sprintf(`{"Variable":{
			"name":"cfg","type":"main.Config","value":"","kind":%d,"addr":100,
			"children":[
				{"name":"Name","type":"string","value":"silk","kind":%d,"len":4,"addr":108},
				{"name":"Inner","type":"main.Inner","value":"","kind":%d,"addr":116,
				 "children":[{"name":"N","type":"int","value":"42","kind":%d,"addr":120}]}
			]}}`,
			int(reflect.Struct), int(reflect.String), int(reflect.Struct), int(reflect.Int)),
	})

	v, err := sess.LoadVariable("cfg", 9, 2, 2)
	if err != nil {
		t.Fatalf("LoadVariable: %v", err)
	}

	c := rec.call(t, 0)
	if c.method != "Eval" {
		t.Fatalf("method = %q, want Eval", c.method)
	}
	if got := dlvStrField(t, c.params, "Expr"); got != "cfg" {
		t.Errorf("Expr = %q, want cfg", got)
	}
	scope := dlvObjField(t, c.params, "Scope")
	if got := dlvNumField(t, scope, "GoroutineID"); got != 9 {
		t.Errorf("Scope.GoroutineID = %v, want 9", got)
	}
	if got := dlvNumField(t, scope, "Frame"); got != 2 {
		t.Errorf("Scope.Frame = %v, want 2", got)
	}
	cfg := dlvObjField(t, c.params, "Cfg")
	if got := dlvNumField(t, cfg, "MaxVariableRecurse"); got != 2 {
		t.Errorf("MaxVariableRecurse = %v, want the requested depth 2", got)
	}
	// 其余上限保持默认, 免得懒展开顺手把字符串/数组截断策略改了
	if got := dlvNumField(t, cfg, "MaxStringLen"); got != 256 {
		t.Errorf("MaxStringLen = %v, want 256", got)
	}

	want := Variable{
		Name: "cfg", Type: "main.Config", Kind: reflect.Struct.String(), Addr: 100,
		Children: []Variable{
			{Name: "Name", Type: "string", Value: "silk", Kind: reflect.String.String(), Len: 4, Addr: 108},
			{Name: "Inner", Type: "main.Inner", Kind: reflect.Struct.String(), Addr: 116,
				Children: []Variable{
					{Name: "N", Type: "int", Value: "42", Kind: reflect.Int.String(), Addr: 120},
				}},
		},
	}
	if !reflect.DeepEqual(v, want) {
		t.Errorf("LoadVariable =\n%+v\nwant\n%+v", v, want)
	}
}

// TestDlvLoadVariable_DepthZeroAndNegative: depth=0 表示"只要这一层";
// depth<0 回落到默认的 1 层.
func TestDlvLoadVariable_DepthZeroAndNegative(t *testing.T) {
	for _, tc := range []struct {
		depth int
		want  float64
	}{{0, 0}, {-1, 1}, {3, 3}} {
		srv, sess := newSessionWithFakeServer(t)
		rec := dlvRouteRPC(t, srv, map[string]string{
			"Eval": `{"Variable":{"name":"x","type":"int","value":"1"}}`,
		})
		if _, err := sess.LoadVariable("x", -1, 0, tc.depth); err != nil {
			t.Fatalf("LoadVariable(depth=%d): %v", tc.depth, err)
		}
		cfg := dlvObjField(t, rec.call(t, 0).params, "Cfg")
		if got := dlvNumField(t, cfg, "MaxVariableRecurse"); got != tc.want {
			t.Errorf("depth=%d -> MaxVariableRecurse=%v, want %v", tc.depth, got, tc.want)
		}
		srv.close()
	}
}

// TestDlvEval_DecodesChildren: Eval 也走同一套投影, 所以 hover/watch 拿到的变量
// 现在带 Kind/Len/Children -- 老调用方看到的 Name/Type/Value 不变.
func TestDlvEval_DecodesChildren(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()

	dlvRouteRPC(t, srv, map[string]string{
		"Eval": fmt.Sprintf(`{"Variable":{"name":"xs","type":"[]int","value":"","kind":%d,"len":5,"cap":8,
			"children":[
				{"name":"","type":"int","value":"1","kind":%d},
				{"name":"","type":"int","value":"2","kind":%d}
			]}}`, int(reflect.Slice), int(reflect.Int), int(reflect.Int)),
	})

	v, err := sess.Eval("xs", -1, 0)
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}
	if v.Kind != reflect.Slice.String() || v.Len != 5 || v.Cap != 8 {
		t.Errorf("kind/len/cap = %q/%d/%d, want slice/5/8", v.Kind, v.Len, v.Cap)
	}
	// Len 是真实长度, 可能大于已加载的 children 数 (MaxArrayValues 截断)
	if len(v.Children) != 2 {
		t.Fatalf("want 2 loaded children, got %d", len(v.Children))
	}
	if v.Children[0].Value != "1" || v.Children[1].Value != "2" {
		t.Errorf("children = %+v", v.Children)
	}
}

// TestDlvVariable_NoChildrenStaysNil: 没有 children 的变量 Children 必须是 nil --
// 这是 UI 判断"还没展开/没有子项"的唯一形态.
func TestDlvVariable_NoChildrenStaysNil(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()

	dlvRouteRPC(t, srv, map[string]string{
		"Eval": `{"Variable":{"name":"x","type":"int","value":"1","children":[]}}`,
	})
	v, err := sess.Eval("x", -1, 0)
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}
	if v.Children != nil {
		t.Errorf("Children = %+v, want nil", v.Children)
	}
}

// TestDlvKindName: 数值 kind -> 名字; 0 (reflect.Invalid) 映射成空串而不是 "invalid"
func TestDlvKindName(t *testing.T) {
	for _, k := range []reflect.Kind{reflect.Int, reflect.String, reflect.Slice, reflect.Map, reflect.Ptr, reflect.Struct} {
		if got := kindName(uint(k)); got != k.String() {
			t.Errorf("kindName(%d) = %q, want %q", k, got, k.String())
		}
	}
	if got := kindName(0); got != "" {
		t.Errorf("kindName(0) = %q, want empty (dlv 用 Invalid 表示未知)", got)
	}
}

// -----------------------------------------------------------------------------
// goroutine 分页
// -----------------------------------------------------------------------------

// TestDlvListGoroutinesPage_Encode: Start/Count 透传, Nextg 作为下一页游标返回
func TestDlvListGoroutinesPage_Encode(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()

	rec := dlvRouteRPC(t, srv, map[string]string{
		"ListGoroutines": `{"Goroutines":[
			{"id":21,"userCurrentLoc":{"file":"u.go","line":5,"function":{"name":"main.run"}},"currentLoc":{"file":"r.go","line":1}},
			{"id":22,"userCurrentLoc":{"file":"","line":0},"currentLoc":{"file":"r.go","line":9,"function":{"name":"runtime.gopark"}}}
		],"Nextg":30}`,
	})

	gs, next, err := sess.ListGoroutinesPage(20, 10)
	if err != nil {
		t.Fatalf("ListGoroutinesPage: %v", err)
	}
	c := rec.call(t, 0)
	if c.method != "ListGoroutines" {
		t.Fatalf("method = %q, want ListGoroutines", c.method)
	}
	if got := dlvNumField(t, c.params, "Start"); got != 20 {
		t.Errorf("Start = %v, want 20", got)
	}
	if got := dlvNumField(t, c.params, "Count"); got != 10 {
		t.Errorf("Count = %v, want 10", got)
	}
	if next != 30 {
		t.Errorf("next cursor = %d, want 30", next)
	}
	want := []Goroutine{
		{ID: 21, File: "u.go", Line: 5, Function: "main.run"},
		{ID: 22, File: "r.go", Line: 9, Function: "runtime.gopark"},
	}
	if !reflect.DeepEqual(gs, want) {
		t.Errorf("goroutines = %+v\nwant %+v", gs, want)
	}
}

// TestDlvListGoroutinesPage_LastPage: 应答没有 Nextg -> 游标 0 (到底了)
func TestDlvListGoroutinesPage_LastPage(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()

	dlvRouteRPC(t, srv, map[string]string{
		"ListGoroutines": `{"Goroutines":[{"id":1,"currentLoc":{"file":"a.go","line":1}}]}`,
	})
	_, next, err := sess.ListGoroutinesPage(0, 100)
	if err != nil {
		t.Fatalf("ListGoroutinesPage: %v", err)
	}
	if next != 0 {
		t.Errorf("next = %d, want 0 on the last page", next)
	}
}

// TestDlvListGoroutines_StillUnpaged: 老签名必须继续发 Start=0/Count=0 (= 全部),
// 且忽略游标 -- 已有调用方 (silkide) 依赖这个行为.
func TestDlvListGoroutines_StillUnpaged(t *testing.T) {
	srv, sess := newSessionWithFakeServer(t)
	defer srv.close()

	rec := dlvRouteRPC(t, srv, map[string]string{
		"ListGoroutines": `{"Goroutines":[{"id":1,"currentLoc":{"file":"a.go","line":1}}],"Nextg":5}`,
	})
	gs, err := sess.ListGoroutines()
	if err != nil {
		t.Fatalf("ListGoroutines: %v", err)
	}
	if len(gs) != 1 {
		t.Fatalf("want 1 goroutine, got %d", len(gs))
	}
	c := rec.call(t, 0)
	if dlvNumField(t, c.params, "Start") != 0 || dlvNumField(t, c.params, "Count") != 0 {
		t.Errorf("params = %v, want Start=0 Count=0", c.params)
	}
}

// -----------------------------------------------------------------------------
// debuggee / dlv 输出
// -----------------------------------------------------------------------------

// TestDlvPumpOutput_Lines: 按行切分, 去掉 \r\n, 末尾没有换行的一段也要投递出去
func TestDlvPumpOutput_Lines(t *testing.T) {
	sess := &DebugSession{}
	var (
		mu   sync.Mutex
		got  []string
		done = make(chan struct{})
	)
	sess.OnOutput(func(stream, line string) {
		mu.Lock()
		got = append(got, stream+":"+line)
		mu.Unlock()
	})
	go func() {
		sess.pumpOutput("stdout", strings.NewReader("first\r\nsecond\n\ntail-without-newline"))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("pumpOutput did not finish")
	}

	mu.Lock()
	defer mu.Unlock()
	want := []string{"stdout:first", "stdout:second", "stdout:", "stdout:tail-without-newline"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("lines = %v\nwant %v", got, want)
	}
}

// TestDlvOnOutput_BacklogReplay: OnOutput 只能在 LaunchDebug 返回之后注册, 所以
// 注册之前的行必须被缓存下来并按序补发 -- 否则永远看不到 dlv 的启动/构建输出.
func TestDlvOnOutput_BacklogReplay(t *testing.T) {
	sess := &DebugSession{}
	sess.emitOutput("stdout", "API server listening at: 127.0.0.1:1234")
	sess.emitOutput("stderr", "build failed")

	var got []string
	sess.OnOutput(func(stream, line string) { got = append(got, stream+":"+line) })

	want := []string{"stdout:API server listening at: 127.0.0.1:1234", "stderr:build failed"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("replayed = %v\nwant %v", got, want)
	}

	// 注册之后的行直接走回调, 不再进 backlog
	sess.emitOutput("stdout", "live")
	if len(got) != 3 || got[2] != "stdout:live" {
		t.Errorf("after registration got %v", got)
	}
}

// TestDlvOnOutput_BacklogBounded: 没人订阅时 backlog 不能无限涨
func TestDlvOnOutput_BacklogBounded(t *testing.T) {
	sess := &DebugSession{}
	for i := 0; i < maxOutputBacklog+50; i++ {
		sess.emitOutput("stdout", fmt.Sprintf("line %d", i))
	}
	n := 0
	sess.OnOutput(func(stream, line string) { n++ })
	if n != maxOutputBacklog {
		t.Errorf("replayed %d lines, want the cap %d", n, maxOutputBacklog)
	}
	// backlog 补发后应当被清空, 再注册一次不会重复收到
	again := 0
	sess.OnOutput(func(stream, line string) { again++ })
	if again != 0 {
		t.Errorf("second registration replayed %d lines, want 0", again)
	}
}

// TestDlvOnOutput_Unsubscribe: OnOutput(nil) 取消订阅, 之后的行重新进 backlog
func TestDlvOnOutput_Unsubscribe(t *testing.T) {
	sess := &DebugSession{}
	n := 0
	sess.OnOutput(func(stream, line string) { n++ })
	sess.emitOutput("stdout", "one")
	sess.OnOutput(nil)
	sess.emitOutput("stdout", "two") // 无订阅者: 不能 panic, 进 backlog

	if n != 1 {
		t.Fatalf("callback fired %d time(s), want 1", n)
	}
	var got []string
	sess.OnOutput(func(stream, line string) { got = append(got, line) })
	if !reflect.DeepEqual(got, []string{"two"}) {
		t.Errorf("replayed %v, want [two]", got)
	}
}

// TestDlvCloseOutputPipes_StopsPump: Close 要能让阻塞在 Read 上的输出泵退出,
// 否则每次 Stop 都漏一个 goroutine + 一对 fd.
func TestDlvCloseOutputPipes_StopsPump(t *testing.T) {
	pr, pw := io.Pipe()
	sess := &DebugSession{}
	sess.trackPipe(pr)

	lines := make(chan string, 4)
	sess.OnOutput(func(stream, line string) { lines <- stream + ":" + line })

	done := make(chan struct{})
	go func() {
		sess.pumpOutput("stderr", pr)
		close(done)
	}()

	if _, err := pw.Write([]byte("running\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	select {
	case got := <-lines:
		if got != "stderr:running" {
			t.Fatalf("line = %q", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("pump never delivered the line")
	}

	sess.closeOutputPipes()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("pumpOutput still blocked after closeOutputPipes")
	}
}

// -----------------------------------------------------------------------------
// closed session: 新增的每个 RPC 都必须早退, 不能 panic
// -----------------------------------------------------------------------------

func TestDlvAdvancedCalls_ClosedSession(t *testing.T) {
	calls := []struct {
		name string
		call func(*DebugSession) error
	}{
		{"ClearBreakpoint", func(s *DebugSession) error { return s.ClearBreakpoint(1) }},
		{"ClearBreakpointByLocation", func(s *DebugSession) error { return s.ClearBreakpointByLocation("a.go", 1) }},
		{"ToggleBreakpoint", func(s *DebugSession) error { _, err := s.ToggleBreakpoint("a.go", 1); return err }},
		{"AmendBreakpoint", func(s *DebugSession) error { return s.AmendBreakpoint(&Breakpoint{ID: 1}) }},
		{"ListArgs", func(s *DebugSession) error { _, err := s.ListArgs(-1, 0); return err }},
		{"LoadVariable", func(s *DebugSession) error { _, err := s.LoadVariable("x", -1, 0, 1); return err }},
		{"SwitchGoroutine", func(s *DebugSession) error { return s.SwitchGoroutine(3) }},
		{"ListGoroutinesPage", func(s *DebugSession) error { _, _, err := s.ListGoroutinesPage(0, 10); return err }},
	}
	for _, c := range calls {
		sess := closedSession()
		err := c.call(sess)
		if err == nil || !strings.Contains(err.Error(), "closed") {
			t.Errorf("%s on closed session: err = %v, want session closed", c.name, err)
		}
	}
}

// TestDlvBreakpointWire_RoundTrip: 模型 -> 线缆 -> 模型 的往返不丢字段
// (LogMessage 借 dlv 的 variables 传, Enabled 借 disabled 取反, 这两处最容易写反).
func TestDlvBreakpointWire_RoundTrip(t *testing.T) {
	in := Breakpoint{
		ID: 3, File: "main.go", Line: 42,
		Cond: "n > 1", Tracepoint: true, LogMessage: "n", Enabled: true,
	}
	raw, err := json.Marshal(breakpointToRPC(in))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var wire rpcBreakpoint
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got := wire.toModel()
	// Verified/HitCount 是 dlv 单方向报告的, 往返里保持零值
	if got != in {
		t.Errorf("round trip = %+v\nwant %+v\nwire: %s", got, in, raw)
	}

	// dlv 用大写 "Cond" 的旧版本也要能解 (encoding/json 大小写不敏感)
	var upper rpcBreakpoint
	if err := json.Unmarshal([]byte(`{"id":1,"Cond":"x==1","Disabled":true}`), &upper); err != nil {
		t.Fatalf("unmarshal upper-case: %v", err)
	}
	bp := upper.toModel()
	if bp.Cond != "x==1" || bp.Enabled {
		t.Errorf("upper-case decode = %+v", bp)
	}
}
