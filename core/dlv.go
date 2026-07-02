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
	"sync/atomic"
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

	// closed 独立于 mu (atomic): Close 必须能在不等 s.mu 的情况下打上标记 --
	// 一个 rpcCall 可能正持着 s.mu 阻塞在 Decode 上 (debuggee 运行中), 等锁即死锁.
	closed atomic.Bool

	mu        sync.Mutex
	conn      net.Conn
	enc       *json.Encoder
	dec       *json.Decoder
	nextRPCID int
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

// Close 停掉 dlv: 标记 closed -> 关连接 -> 尽力 Detach -> kill 进程兜底
// 关键顺序: Close 不能一上来就等 s.mu -- 后台的 rpcCall (典型是 Continue 在等
// stop state) 可能正持有 s.mu 阻塞在 Decode 上, 先等锁就是死锁 (IDE 里表现为
// 点 Stop 冻住主线程). 因此:
//  1. closed 用独立的 atomic 打标记, 不经过 s.mu;
//  2. 先 conn.Close() -- net.Conn 并发安全, 会让阻塞中的 Decode 立即带错返回,
//     对应的 rpcCall 看到 closed 后回 errSessionClosed 并释放 s.mu;
//  3. 之后再拿 s.mu 做剩余清理. 此时 Detach 大概率失败 (conn 已关) -- 无妨,
//     Process.Kill 兜底保证 dlv 子进程一定被终止.
//
// Detach 任意错误都忽略 -- 进程都要终止了, 没必要把 RPC 错误返给上层
func (s *DebugSession) Close() error {
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}

	// 先断连接, 唤醒可能阻塞在 Decode 上的 rpcCall, 让它释放 s.mu.
	// conn 在构造之后不再变更, 这里无锁读是安全的.
	if s.conn != nil {
		_ = s.conn.Close()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Detach 的 params 是 {Kill: true}. 不通过 rpcCall (closed 状态下它会拒绝);
	// conn 已关时这次读写会立刻失败, 属于预期内的 best-effort.
	s.sendDetachBestEffort()

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
	// cond="" 即无条件断点 -- omitempty 让 cond 字段在这种情况下根本不出现在
	// 线缆上, 与重构前 SetBreakpoint 发出的 JSON 逐字节一致, 不改变既有行为.
	return s.createBreakpoint(file, line, "")
}

// SetConditionalBreakpoint 在 file:line 下一个带 Go 表达式条件的断点
// 只有当 cond 在该行求值为 true 时 dlv 才会真正停下 (例如 "i == 3" / "err != nil").
// 这是 IDE "右键断点 -> 编辑条件" 的后端; cond 的语法与 Eval 表达式一致.
// 返回值形态与 SetBreakpoint 对齐 (同样是 *Breakpoint), 两者共用 createBreakpoint.
func (s *DebugSession) SetConditionalBreakpoint(file string, line int, cond string) (*Breakpoint, error) {
	return s.createBreakpoint(file, line, cond)
}

// createBreakpoint 是 SetBreakpoint / SetConditionalBreakpoint 共享的私有实现
// 走 RPCServer.CreateBreakpoint, 把 cond 透传到 Breakpoint.Cond. cond 为空串时
// 借 omitempty 省掉该字段, 等价于无条件断点. dlv API v2 的应答把断点放在
// "Breakpoint" 字段下.
func (s *DebugSession) createBreakpoint(file string, line int, cond string) (*Breakpoint, error) {
	type bpSpec struct {
		File string `json:"file"`
		Line int    `json:"line"`
		Cond string `json:"cond,omitempty"`
	}
	type createIn struct {
		Breakpoint bpSpec `json:"Breakpoint"`
	}
	type createOut struct {
		Breakpoint struct {
			ID           int    `json:"id"`
			File         string `json:"file"`
			Line         int    `json:"line"`
			FunctionName string `json:"functionName"`
		} `json:"Breakpoint"`
	}
	var out createOut
	if err := s.rpcCall("CreateBreakpoint", createIn{Breakpoint: bpSpec{File: file, Line: line, Cond: cond}}, &out); err != nil {
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

// Step 是 Next 的同义词 (step-over), 历史遗留方法, 保留以兼容已引用它的调用方
// (silkide 等). 与 Next 一样发 dlv 的 "next": 同一 goroutine 内单步, 不进入
// 函数调用. 新代码应优先用 Next; Step 不再扩展语义.
func (s *DebugSession) Step() (*StopState, error) {
	return s.command("next")
}

// Next 是 IDE Debug 工具栏的 "Step Over": 执行当前行, 跨过 (不进入) 行内的函数
// 调用. 对应 dlv 的 "next". 与 Step 等价 -- Step 是历史别名, 两者发同一条命令.
func (s *DebugSession) Next() (*StopState, error) {
	return s.command("next")
}

// StepInto 是 "Step Into": 进入当前行所调用的函数. 对应 dlv 的 "step".
// 当前行没有函数调用时 dlv 退化为一次 next.
func (s *DebugSession) StepInto() (*StopState, error) {
	return s.command("step")
}

// StepOut 是 "Step Out": 运行到当前函数返回 (跳出当前帧). 对应 dlv 的 "stepOut".
func (s *DebugSession) StepOut() (*StopState, error) {
	return s.command("stepOut")
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

// StackFrame 是 goroutine 调用栈上的一帧
// Delve Stackframe 还带 Locals/Arguments/FrameOffset 等; IDE 当前只画 File/Line/Function
type StackFrame struct {
	File     string
	Line     int
	Function string
}

// Goroutine 是 dlv 看到的一个用户 goroutine
// CurrentLoc 是 PC 当前所在位置, UserCurrentLoc 是去掉 runtime 帧之后的用户视角位置.
// IDE 显示用的是后者 -- 用户更关心自己的代码, 不在乎卡在 runtime.gopark.
type Goroutine struct {
	ID       int
	File     string
	Line     int
	Function string
}

// Variable 是 dlv Eval/ListLocalVars 应答的极简投影
// Delve 的 Variable 还有 Kind/Addr/Children/Len/Cap/Flags/... 一大堆字段,
// IDE 第一版的悬浮 + watch panel 只需要 Name/Type/Value, 其它留给后续 commit.
type Variable struct {
	Name  string
	Type  string
	Value string
}

// loadConfig 是 dlv 在 Eval/Stacktrace(Full)/ListLocalVars 里要求的取值上限
// 默认值的取舍:
//   - MaxStringLen 256       够看大多数字符串, 又不会一次拉回 MB 级数据
//   - MaxArrayValues 64      切片/数组前 64 个元素够 IDE 预览
//   - MaxStructFields -1     字段不限制 (struct 字段数一般有限)
//   - MaxVariableRecurse 1   嵌套展开 1 层, 再深由用户在 UI 上点开
type loadConfig struct {
	FollowPointers     bool `json:"FollowPointers"`
	MaxVariableRecurse int  `json:"MaxVariableRecurse"`
	MaxStringLen       int  `json:"MaxStringLen"`
	MaxArrayValues     int  `json:"MaxArrayValues"`
	MaxStructFields    int  `json:"MaxStructFields"`
}

func defaultLoadConfig() loadConfig {
	return loadConfig{
		FollowPointers:     true,
		MaxVariableRecurse: 1,
		MaxStringLen:       256,
		MaxArrayValues:     64,
		MaxStructFields:    -1,
	}
}

// evalScope 是 dlv EvalScope: 选哪一个 goroutine 的哪一帧
// GoroutineID = -1 表示当前(SelectedGoroutine), Frame 0 是栈顶
type evalScope struct {
	GoroutineID int `json:"GoroutineID"`
	Frame       int `json:"Frame"`
}

// rpcLocation / rpcVariable 是 dlv 应答的薄信封, 只挑我们暴露的字段
// 这些类型不出包, 上层永远拿到的是 StackFrame / Variable / Goroutine.
type rpcLocation struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Function *struct {
		Name string `json:"name"`
	} `json:"function"`
}

type rpcVariable struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

// Stacktrace 返回当前/指定 goroutine 的调用栈
// goroutineID = -1 表示当前 SelectedGoroutine. depth 是最多取几帧 (栈顶起算).
// Full=false 让 dlv 不带 Locals/Arguments -- 我们这里只画位置, 想看局部变量走
// ListLocals. 这样应答体积更小.
func (s *DebugSession) Stacktrace(goroutineID, depth int) ([]StackFrame, error) {
	type stackIn struct {
		ID     int  `json:"Id"`
		Depth  int  `json:"Depth"`
		Full   bool `json:"Full"`
		Defers bool `json:"Defers"`
	}
	type rpcFrame struct {
		Location rpcLocation `json:"Location"`
	}
	type stackOut struct {
		Locations []rpcFrame `json:"Locations"`
	}
	var out stackOut
	if err := s.rpcCall("Stacktrace", stackIn{ID: goroutineID, Depth: depth, Full: false, Defers: false}, &out); err != nil {
		return nil, err
	}
	frames := make([]StackFrame, 0, len(out.Locations))
	for _, f := range out.Locations {
		fr := StackFrame{File: f.Location.File, Line: f.Location.Line}
		if f.Location.Function != nil {
			fr.Function = f.Location.Function.Name
		}
		frames = append(frames, fr)
	}
	return frames, nil
}

// ListGoroutines 拉取所有 goroutine
// 不分页 (Start=0, Count=0 在 dlv 里语义是 "全部"); 一旦 IDE 真要分页再加.
// 取 UserCurrentLoc 而非 CurrentLoc -- 用户关心自己写的代码而不是 runtime 帧.
func (s *DebugSession) ListGoroutines() ([]Goroutine, error) {
	type listIn struct {
		Start int `json:"Start"`
		Count int `json:"Count"`
	}
	type rpcGoroutine struct {
		ID             int         `json:"id"`
		CurrentLoc     rpcLocation `json:"currentLoc"`
		UserCurrentLoc rpcLocation `json:"userCurrentLoc"`
	}
	type listOut struct {
		Goroutines []rpcGoroutine `json:"Goroutines"`
		// Nextg int -- 分页游标, 当前不消费
	}
	var out listOut
	if err := s.rpcCall("ListGoroutines", listIn{Start: 0, Count: 0}, &out); err != nil {
		return nil, err
	}
	gs := make([]Goroutine, 0, len(out.Goroutines))
	for _, g := range out.Goroutines {
		// 优先 UserCurrentLoc; 如果 File 为空 (纯 runtime goroutine) 回落到 CurrentLoc
		loc := g.UserCurrentLoc
		if loc.File == "" {
			loc = g.CurrentLoc
		}
		out := Goroutine{ID: g.ID, File: loc.File, Line: loc.Line}
		if loc.Function != nil {
			out.Function = loc.Function.Name
		}
		gs = append(gs, out)
	}
	return gs, nil
}

// ListLocals 拉取指定 frame 的局部变量
// goroutineID=-1 -> 当前 goroutine; frame=0 -> 栈顶. 不含函数参数 (那是
// ListFunctionArgs), 想要全量参数+局部以后再加 ListArgs 方法.
func (s *DebugSession) ListLocals(goroutineID, frame int) ([]Variable, error) {
	type listIn struct {
		Scope evalScope  `json:"Scope"`
		Cfg   loadConfig `json:"Cfg"`
	}
	type listOut struct {
		Variables []rpcVariable `json:"Variables"`
	}
	var out listOut
	in := listIn{
		Scope: evalScope{GoroutineID: goroutineID, Frame: frame},
		Cfg:   defaultLoadConfig(),
	}
	if err := s.rpcCall("ListLocalVars", in, &out); err != nil {
		return nil, err
	}
	vs := make([]Variable, 0, len(out.Variables))
	for _, v := range out.Variables {
		vs = append(vs, Variable{Name: v.Name, Type: v.Type, Value: v.Value})
	}
	return vs, nil
}

// Eval 在 (goroutine, frame) 作用域下求值一个 Go 表达式
// 表达式形态遵循 dlv 文档: 支持局部/包级变量 + 成员/索引/解引用, 不支持函数调用.
// 这是 hover-to-inspect 和 watch panel 的基础.
func (s *DebugSession) Eval(expr string, goroutineID, frame int) (Variable, error) {
	type evalIn struct {
		Scope evalScope  `json:"Scope"`
		Expr  string     `json:"Expr"`
		Cfg   loadConfig `json:"Cfg"`
	}
	type evalOut struct {
		Variable rpcVariable `json:"Variable"`
	}
	var out evalOut
	in := evalIn{
		Scope: evalScope{GoroutineID: goroutineID, Frame: frame},
		Expr:  expr,
		Cfg:   defaultLoadConfig(),
	}
	if err := s.rpcCall("Eval", in, &out); err != nil {
		return Variable{}, err
	}
	return Variable{
		Name:  out.Variable.Name,
		Type:  out.Variable.Type,
		Value: out.Variable.Value,
	}, nil
}

// SetVariable 把 (goroutine, frame) 作用域下的某个变量 symbol 赋成 value
// 这是 IDE 变量面板 "双击改值" 动作的后端.走 dlv 的 RPCServer.Set, 参数
// {Scope: EvalScope, Symbol, Value}, 应答为空 (无 result). symbol 是变量名
// (例如 "x" 或 "p.Field"), value 是 Go 字面量字符串 (dlv 自己解析, 例如 "42"
// / "\"hi\"" / "true"). 类型不匹配或符号不存在时 dlv 回 error, 这里原样 wrap.
// goroutineID=-1 表示当前 SelectedGoroutine, frame=0 是栈顶.
func (s *DebugSession) SetVariable(symbol, value string, goroutineID, frame int) error {
	type setIn struct {
		Scope  evalScope `json:"Scope"`
		Symbol string    `json:"Symbol"`
		Value  string    `json:"Value"`
	}
	in := setIn{
		Scope:  evalScope{GoroutineID: goroutineID, Frame: frame},
		Symbol: symbol,
		Value:  value,
	}
	// RPCServer.Set 没有 result body; out 传 nil, rpcCall 只校验 error 字段
	return s.rpcCall("Set", in, nil)
}

// Restart 把被调进程从头重跑一遍, 不重启 dlv 进程本身
// 走 RPCServer.Restart, 普通重启传 {Position:"", ResetArgs:false}:
// Position 空表示从入口重新开始 (非空时是 checkpoint/位置, record/replay 才用到),
// ResetArgs=false 保留原命令行参数. 断点默认跨 Restart 存活 -- dlv 会把它们
// 重新绑到新进程上, 所以重启后不必重新 SetBreakpoint.
// 应答里的 DiscardedBreakpoints 列出那些重新绑定失败而被丢弃的断点 (一般为空);
// 非空时仅 Warn 一条, 不当成错误 -- 重启本身已经成功, 个别断点丢失不该让调用方失败.
func (s *DebugSession) Restart() error {
	type restartIn struct {
		Position  string   `json:"Position"`
		ResetArgs bool     `json:"ResetArgs"`
		NewArgs   []string `json:"NewArgs,omitempty"`
		Rerecord  bool     `json:"Rerecord"`
	}
	// dlv API v2 的应答把丢弃的断点放在 "DiscardedBreakpoints" 字段下;
	// 每个元素带一个被丢弃的断点和原因, 这里只数个数用于 Warn.
	type restartOut struct {
		DiscardedBreakpoints []struct {
			Reason string `json:"reason"`
		} `json:"DiscardedBreakpoints"`
	}
	var out restartOut
	in := restartIn{Position: "", ResetArgs: false}
	if err := s.rpcCall("Restart", in, &out); err != nil {
		return err
	}
	if n := len(out.DiscardedBreakpoints); n > 0 {
		Warn(fmt.Sprintf("dlv restart discarded %d breakpoint(s)", n))
	}
	return nil
}

// errSessionClosed 是 session 已 Close 之后一切 RPC 的统一返回错误
// Close 会先关 conn 把阻塞中的 Decode 唤醒 -- rpcCall 醒来看到 closed 时也回它,
// 不把底层 "use of closed network connection" 之类的 read 错误抛给调用方.
var errSessionClosed = errors.New("debug session closed")

// rpcCall 是 JSON-RPC 1.0 在 TCP 上的单次 round-trip
// 串行化由 s.mu 提供 -- net/rpc 的 JSON codec 不允许两个 goroutine 同时写一个 conn.
// closed 在入口查一次, encode/decode 出错后再查一次: 后者对应 Close 并发关掉
// conn 把本调用从阻塞 IO 里唤醒的场景, 统一回 errSessionClosed.
// out 必须是非空指针; 调用方负责字段对应到 Delve 的应答结构.
func (s *DebugSession) rpcCall(method string, params interface{}, out interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed.Load() {
		return errSessionClosed
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
		if s.closed.Load() {
			return errSessionClosed
		}
		return fmt.Errorf("rpc encode %s: %w", method, err)
	}

	var raw rpcRawResponse
	if err := s.dec.Decode(&raw); err != nil {
		if s.closed.Load() {
			return errSessionClosed
		}
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
