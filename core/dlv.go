package core

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Delve (`dlv`) headless 调试器的进程控制器
// 这一版覆盖 IDE Debug 工具栏 + 断点/调用栈/变量面板需要的命令,
// 不打算把 Delve 的整张 RPC 表都包过来. Wired 的方法:
//   - RPCServer.CreateBreakpoint / AmendBreakpoint / ClearBreakpoint / ListBreakpoints
//   - RPCServer.Command         (continue / next / step / stepOut / switchGoroutine)
//   - RPCServer.Stacktrace / ListGoroutines
//   - RPCServer.ListLocalVars / ListFunctionArgs / Eval / Set
//   - RPCServer.Restart
//   - RPCServer.Detach          (Close 时尽力调用一次)
// 其它(Disassemble, ListRegisters, ExamineMemory, Attach, core dump) 留给后续
// commit, 等 UI 真要用到的时候再加. 第三方库一律不引入, stdlib only.
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

	// selGoroutine 是 IDE 通过 SwitchGoroutine 选中的 goroutine.
	// 0 = 没选过, 各作用域方法回落到 dlv 自己的 "current goroutine" (-1);
	// Go 的 goroutine id 从 1 起, 所以 0 可以安全地当"未选"哨兵 (DebugSession
	// 也可以被结构体字面量构造, 不能依赖构造函数把它初始化成 -1).
	// 用 atomic 而不是挂在 s.mu 下: 读它之后紧接着就要发 RPC(要拿 s.mu), 不能嵌套.
	selGoroutine atomic.Int64

	// 输出泵: dlv 进程 (以及跟它共享 fd 的 debuggee) 的 stdout/stderr 行.
	// outMu 只护这三个字段, 与 s.mu 无关 -- 输出回调不该被 RPC 串行化卡住.
	outMu      sync.Mutex
	onOutput   func(stream, line string)
	outBacklog []debugOutputLine
	outPipes   []io.Closer
}

// Breakpoint 是用户/我们在源码某一行下的断点
// 除 ID/File/Line/Function 之外还带上 IDE 断点面板要编辑的属性; 它们与 Delve
// api.Breakpoint 的映射见 rpcBreakpoint. 结构体保持可比较 (全是标量),
// 上层可以直接用 == 判断两个断点是否等价.
type Breakpoint struct {
	ID       int
	File     string
	Line     int
	Function string // 可选

	// Cond 是 Go 表达式条件, 空串表示无条件断点 (dlv Breakpoint.Cond).
	// 只有 cond 求值为 true 时才真正停下, 例如 "i == 3" / "err != nil".
	Cond string
	// HitCount 是 dlv 报告的累计命中次数 (totalHitCount), 只读:
	// AmendBreakpoint 不会把它写回去 (dlv 侧不接受重置).
	HitCount uint64
	// Tracepoint=true 时这个断点是一个 logpoint: dlv 命中后打印信息并自动继续,
	// 不把程序停住. 线缆上 dlv 把这个标志叫 "continue".
	Tracepoint bool
	// LogMessage 是 logpoint 命中时要 dlv 求值并打印的表达式, 映射到 dlv
	// Breakpoint.Variables 的第一个元素; 空串表示只打印命中位置本身.
	LogMessage string
	// Enabled=false 对应 dlv 的 disabled 断点: 仍在列表里但不生效.
	// 注意零值 -- 自己构造 Breakpoint 交给 AmendBreakpoint 时记得显式置 true,
	// 从 dlv 解码出来的断点一定带正确的 Enabled.
	Enabled bool
	// Verified=true 表示 dlv 真的把这个断点绑到了至少一个地址上 (addr/addrs 非空).
	// 行号落在没有代码的位置时 dlv 通常直接报错而不是给一个未绑定断点, 所以实践中
	// 它几乎总是 true; 保留该字段让 UI 能表达"待绑定/已失效"状态.
	Verified bool
}

// rpcBreakpoint 是 Delve api.Breakpoint 的薄信封, 只挑我们暴露的字段
// 创建/修改/列举三条路径共用它, 保证三处的线缆形态一致.
// json tag 一律小写: encoding/json 匹配字段时大小写不敏感, 所以既吃 dlv 的
// "Cond"/"totalHitCount" 也吃 "cond"; 反方向 dlv 用同一套 json 解码, 同理.
// omitempty 让零值字段根本不出现在线缆上 -- 无条件断点因此与重构前逐字节一致.
type rpcBreakpoint struct {
	ID           int    `json:"id,omitempty"`
	File         string `json:"file,omitempty"`
	Line         int    `json:"line,omitempty"`
	FunctionName string `json:"functionName,omitempty"`
	Cond         string `json:"cond,omitempty"`
	// Variables 是 dlv 在 tracepoint 命中时求值并打印的表达式列表
	Variables []string `json:"variables,omitempty"`
	// Tracepoint 在 dlv 线缆上的字段名是 "continue"; 少数前端/版本写成
	// "tracepoint", 解码时两个键都收下 (见 toModel), 编码只发 "continue".
	Tracepoint    bool `json:"continue,omitempty"`
	TracepointAlt bool `json:"tracepoint,omitempty"`
	Disabled      bool `json:"disabled,omitempty"`
	// Addr/Addrs 只在解码方向有意义: 非空即说明断点已绑到指令地址上
	Addr  uint64   `json:"addr,omitempty"`
	Addrs []uint64 `json:"addrs,omitempty"`
	// TotalHitCount 是累计命中次数. 千万不要用 "hitCount" 这个键 --
	// dlv 那边它是 map[goroutine]count, 解到标量上会让整个应答解码失败.
	TotalHitCount uint64 `json:"totalHitCount,omitempty"`
}

// toModel 把线缆断点投影成对外的 Breakpoint
func (b rpcBreakpoint) toModel() Breakpoint {
	bp := Breakpoint{
		ID:         b.ID,
		File:       b.File,
		Line:       b.Line,
		Function:   b.FunctionName,
		Cond:       b.Cond,
		HitCount:   b.TotalHitCount,
		Tracepoint: b.Tracepoint || b.TracepointAlt,
		Enabled:    !b.Disabled,
		Verified:   b.Addr != 0 || len(b.Addrs) > 0,
	}
	if len(b.Variables) > 0 {
		bp.LogMessage = b.Variables[0]
	}
	return bp
}

// breakpointToRPC 是 toModel 的反方向, 供 CreateBreakpoint / AmendBreakpoint 用
func breakpointToRPC(bp Breakpoint) rpcBreakpoint {
	out := rpcBreakpoint{
		ID:         bp.ID,
		File:       bp.File,
		Line:       bp.Line,
		Cond:       bp.Cond,
		Tracepoint: bp.Tracepoint,
		Disabled:   !bp.Enabled,
	}
	if bp.LogMessage != "" {
		out.Variables = []string{bp.LogMessage}
	}
	return out
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
	// 把 dlv 的 stdout/stderr 接成管道: `dlv debug` 下被调试程序继承同一对 fd,
	// 所以这两条流里既有 dlv 自己的日志 (build 报错 / API server listening),
	// 也有 debuggee 的 print 输出 -- 上层用 OnOutput 订阅.
	// 拿不到管道不算致命: 退化成旧行为(输出被丢弃)继续启动, 不为了看日志把整次
	// 调试搞失败. 一旦接了管道就必须一直有人读, 否则 debuggee 写满管道会被挂住;
	// pumpOutput 的 goroutine 承担这个责任 (没人订阅时丢进有上限的 backlog).
	sess := &DebugSession{cmd: cmd, port: port}
	stdout, outErr := cmd.StdoutPipe()
	stderr, errErr := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start dlv: %w", err)
	}
	if outErr == nil {
		sess.trackPipe(stdout)
		go sess.pumpOutput("stdout", stdout)
	}
	if errErr == nil {
		sess.trackPipe(stderr)
		go sess.pumpOutput("stderr", stderr)
	}

	// dlv debug runs `go build` BEFORE it opens the listen port; a cold
	// cgo/cairo build easily exceeds a few seconds, so give it a generous
	// budget. The caller runs LaunchDebug off the UI thread, so this wait
	// never freezes the IDE.
	conn, err := waitForDial("127.0.0.1:"+strconv.Itoa(port), 60*time.Second)
	if err != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		sess.closeOutputPipes()
		return nil, fmt.Errorf("dial dlv: %w", err)
	}

	sess.conn = conn
	sess.enc = json.NewEncoder(conn)
	sess.dec = json.NewDecoder(conn)
	return sess, nil
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
	// 进程已经没了, 读端也可以收了 -- 让 pumpOutput 的 goroutine 退出, 不留 fd.
	// 放在 Kill/Wait 之后: 提前关读端会让还活着的 dlv 写 stdout 时吃 EPIPE.
	s.closeOutputPipes()
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

// -----------------------------------------------------------------------------
// debuggee / dlv 输出 -- OnOutput 订阅 stdout/stderr 行
// -----------------------------------------------------------------------------

// maxOutputBacklog 是 OnOutput 尚未注册时最多缓存多少行
// dlv 一起来就会打 "API server listening at: ..." 之类的行, 而 OnOutput 只能在
// LaunchDebug 返回之后才注册 -- 不缓存就必然漏掉开头几行 (包括 build 失败的原因).
// 上限用来防止无人订阅时内存无限涨; 溢出的行直接丢弃 (调试控制台不是日志系统).
const maxOutputBacklog = 512

// debugOutputLine 是一行还没被消费的调试输出
type debugOutputLine struct {
	stream string
	text   string
}

// OnOutput 注册 dlv 进程 (以及跟它共享 fd 的被调试程序) 的输出行回调
// stream 是 "stdout" 或 "stderr", line 已去掉行尾的 \r\n. 注册之前积压的行会在
// 注册时按序补发一遍, 所以晚注册也看得到 dlv 的启动输出. fn 传 nil 表示取消订阅
// (此时新行重新进 backlog).
// 回调在读取 goroutine 上同步执行: 不要在里面做阻塞的事, UI 侧应当转成一次异步刷新.
func (s *DebugSession) OnOutput(fn func(stream, line string)) {
	s.outMu.Lock()
	s.onOutput = fn
	var backlog []debugOutputLine
	if fn != nil {
		backlog, s.outBacklog = s.outBacklog, nil
	}
	s.outMu.Unlock()
	// 补发放在锁外: 回调里可能反手再调 OnOutput/发 RPC, 持锁调用容易自锁
	for _, l := range backlog {
		fn(l.stream, l.text)
	}
}

// emitOutput 把一行输出交给订阅者; 没有订阅者时进 backlog
func (s *DebugSession) emitOutput(stream, line string) {
	s.outMu.Lock()
	fn := s.onOutput
	if fn == nil {
		if len(s.outBacklog) < maxOutputBacklog {
			s.outBacklog = append(s.outBacklog, debugOutputLine{stream: stream, text: line})
		}
		s.outMu.Unlock()
		return
	}
	s.outMu.Unlock()
	fn(stream, line)
}

// pumpOutput 按行读 r 直到 EOF/出错, 每行调一次 emitOutput
// 用 bufio.Reader 而不是 Scanner: 被调程序完全可能打出超过 Scanner 默认 64KiB
// 上限的一行 (dump 一个大结构体), Scanner 那时会直接放弃剩下的整条流.
// 最后一段没有换行的内容也会被投递出去 (进程被 kill 时很常见).
func (s *DebugSession) pumpOutput(stream string, r io.Reader) {
	br := bufio.NewReader(r)
	for {
		line, err := br.ReadString('\n')
		if line != "" {
			s.emitOutput(stream, strings.TrimRight(line, "\r\n"))
		}
		if err != nil {
			return
		}
	}
}

// trackPipe 记下一个要在 Close 时关掉的管道读端
func (s *DebugSession) trackPipe(c io.Closer) {
	s.outMu.Lock()
	s.outPipes = append(s.outPipes, c)
	s.outMu.Unlock()
}

// closeOutputPipes 关掉所有管道读端, 让阻塞在 Read 上的 pumpOutput 立刻返回
func (s *DebugSession) closeOutputPipes() {
	s.outMu.Lock()
	pipes := s.outPipes
	s.outPipes = nil
	s.outMu.Unlock()
	for _, p := range pipes {
		_ = p.Close()
	}
}

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
	type createIn struct {
		Breakpoint rpcBreakpoint `json:"Breakpoint"`
	}
	type createOut struct {
		Breakpoint rpcBreakpoint `json:"Breakpoint"`
	}
	var out createOut
	spec := rpcBreakpoint{File: file, Line: line, Cond: cond}
	if err := s.rpcCall("CreateBreakpoint", createIn{Breakpoint: spec}, &out); err != nil {
		return nil, err
	}
	bp := out.Breakpoint.toModel()
	return &bp, nil
}

// ClearBreakpoint 按 dlv 分配的 ID 删除一个断点
// 走 RPCServer.ClearBreakpoint (params {Id}). 断点不存在时 dlv 回错误, 原样上抛 --
// UI 侧删一个已经没了的断点应当当成"已经是目标状态", 由调用方决定是否忽略.
// 应答里带着被删掉的断点, 我们不消费.
func (s *DebugSession) ClearBreakpoint(id int) error {
	type clearIn struct {
		ID int `json:"Id"`
	}
	return s.rpcCall("ClearBreakpoint", clearIn{ID: id}, nil)
}

// ClearBreakpointByLocation 删除 file:line 上的断点
// dlv 没有按位置删的 RPC, 这里先 ListBreakpoints 再按位置找 ID -- 编辑器 gutter
// 只知道文件和行号, 不该逼 UI 自己维护一张 id 表. 该行没有断点时返回错误.
// 路径比较见 sameSourceFile: dlv 一律回绝对路径, 而 IDE 手里可能是相对路径.
func (s *DebugSession) ClearBreakpointByLocation(file string, line int) error {
	bp, err := s.findBreakpointAt(file, line)
	if err != nil {
		return err
	}
	if bp == nil {
		return fmt.Errorf("no breakpoint at %s:%d", file, line)
	}
	return s.ClearBreakpoint(bp.ID)
}

// ToggleBreakpoint 是编辑器 gutter 点击的后端: 该行没断点就下一个, 已有就删掉
// 新建时返回创建出来的断点; 删除时返回 (nil, nil) -- 调用方用 nil 判断"这次是删".
func (s *DebugSession) ToggleBreakpoint(file string, line int) (*Breakpoint, error) {
	bp, err := s.findBreakpointAt(file, line)
	if err != nil {
		return nil, err
	}
	if bp != nil {
		return nil, s.ClearBreakpoint(bp.ID)
	}
	return s.SetBreakpoint(file, line)
}

// AmendBreakpoint 把 bp 的可编辑状态写回 dlv (条件 / logpoint / 启用位)
// 走 RPCServer.AmendBreakpoint, bp.ID 必须是 dlv 已知的断点 ID.
// 语义是"整体替换"而不是"局部合并": bp 里为零值的字段会把 dlv 上对应的设置清掉
// (Cond="" 即取消条件), 所以正确用法是从 SetBreakpoint/ListBreakpoints 拿到对象,
// 改字段, 再整个传回来. File/Line 不能靠 amend 改 (dlv 忽略), 换行请删了重下.
// HitCount 是只读的, 不会被写回.
func (s *DebugSession) AmendBreakpoint(bp *Breakpoint) error {
	if bp == nil {
		return errors.New("amend breakpoint: nil breakpoint")
	}
	if bp.ID <= 0 {
		return fmt.Errorf("amend breakpoint: invalid id %d", bp.ID)
	}
	type amendIn struct {
		Breakpoint rpcBreakpoint `json:"Breakpoint"`
	}
	return s.rpcCall("AmendBreakpoint", amendIn{Breakpoint: breakpointToRPC(*bp)}, nil)
}

// findBreakpointAt 在当前断点列表里找 file:line 上的用户断点
// 找不到返回 (nil, nil) -- 这是正常情况 (toggle 的"新建"分支), 不是错误.
// 只看 ID>0: dlv 自己那些内部断点 (unrecovered-panic 等) ID 为负, 不能被用户删.
func (s *DebugSession) findBreakpointAt(file string, line int) (*Breakpoint, error) {
	bps, err := s.ListBreakpoints()
	if err != nil {
		return nil, err
	}
	for i := range bps {
		if bps[i].ID > 0 && bps[i].Line == line && sameSourceFile(bps[i].File, file) {
			return &bps[i], nil
		}
	}
	return nil, nil
}

// sameSourceFile 判断两个源文件路径是否指同一个文件
// 先按 filepath.Clean 逐字符比; 不等时再退一步看"一个是不是另一个的路径后缀" --
// dlv 返回的是编译期的绝对路径, 而 IDE 手里可能是相对项目根的相对路径, 对同一个
// 文件两者不会字面相等. 只在"一个绝对一个相对"时才做后缀匹配, 且要求边界落在
// 路径分隔符上, 避免 a/foo.go 匹配到 b/barfoo.go.
func sameSourceFile(a, b string) bool {
	ca, cb := filepath.Clean(a), filepath.Clean(b)
	if ca == cb {
		return true
	}
	absA, absB := filepath.IsAbs(ca), filepath.IsAbs(cb)
	if absA == absB {
		return false
	}
	if absB {
		ca, cb = cb, ca // 统一成 ca 绝对 / cb 相对
	}
	return strings.HasSuffix(ca, string(filepath.Separator)+cb)
}

// ListBreakpoints 拉取当前所有断点
// dlv v2 应答里还会含一个 "unrecovered-panic" 等内部断点 (ID<0); 这里原样返回,
// 由上层决定是否过滤(IDE 显示侧通常会按 ID>0 筛一遍)
func (s *DebugSession) ListBreakpoints() ([]Breakpoint, error) {
	type listIn struct {
		All bool `json:"All"`
	}
	type listOut struct {
		Breakpoints []rpcBreakpoint `json:"Breakpoints"`
	}
	var out listOut
	if err := s.rpcCall("ListBreakpoints", listIn{All: false}, &out); err != nil {
		return nil, err
	}
	bps := make([]Breakpoint, 0, len(out.Breakpoints))
	for _, b := range out.Breakpoints {
		bps = append(bps, b.toModel())
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

// SwitchGoroutine 把 dlv 的"当前 goroutine"切到 id, 并把这个选择记在 session 上
// 之后 Stacktrace/ListLocals/ListArgs/Eval/LoadVariable/SetVariable 传负数
// goroutineID (即"当前") 时都解析到 id -- 这是 IDE goroutine 面板双击一行之后
// 变量/调用栈跟着换上下文的机制. 走 RPCServer.Command 的 "switchGoroutine" 子命令,
// 要求程序处于停止状态; dlv 报错时不改动本地选择.
func (s *DebugSession) SwitchGoroutine(id int64) error {
	if _, err := s.commandOn("switchGoroutine", id); err != nil {
		return err
	}
	s.selGoroutine.Store(id)
	return nil
}

// SelectedGoroutine 返回 SwitchGoroutine 记下的 goroutine id
// 没切过时返回 -1, 也就是 dlv 的"当前 goroutine".
func (s *DebugSession) SelectedGoroutine() int64 {
	if id := s.selGoroutine.Load(); id > 0 {
		return id
	}
	return -1
}

// scopeGoroutine 把调用方给的 goroutineID 解析成真正发给 dlv 的值
// id>=0 原样透传 (调用方点名了某个 goroutine); id<0 表示"当前", 此时若
// SwitchGoroutine 选过一个就用它, 否则回 -1 交给 dlv 自己决定.
func (s *DebugSession) scopeGoroutine(id int64) int64 {
	if id >= 0 {
		return id
	}
	return s.SelectedGoroutine()
}

// command 是 Continue/Step 共享的内部包装, 不指定 goroutine
func (s *DebugSession) command(name string) (*StopState, error) {
	return s.commandOn(name, 0)
}

// commandOn 是 command 的带 goroutine 版本. dlv 的 Command RPC 在 API v2 下
// 接受 {"name": "<cmd>", "goroutineID": <id>}, 返回 {"State": DebuggerState}.
// goroutineID 传 0 表示"不指定", 靠 omitempty 从线缆上消失 -- 于是 continue/next
// 这些老命令发出的 JSON 与加这个字段之前逐字节一致.
func (s *DebugSession) commandOn(name string, goroutineID int64) (*StopState, error) {
	type cmdIn struct {
		Name        string `json:"name"`
		GoroutineID int64  `json:"goroutineID,omitempty"`
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
	if err := s.rpcCall("Command", cmdIn{Name: name, GoroutineID: goroutineID}, &out); err != nil {
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

// Variable 是 dlv Eval/ListLocalVars/ListFunctionArgs 应答的投影
// 除 Name/Type/Value 之外带上变量树要用的元信息: Kind 决定 UI 画什么图标 / 能不能
// 展开, Len/Cap 让 slice/map 在不展开的情况下也能显示规模, Addr 供"跳到内存"用,
// Children 是已经加载回来的子变量.
// 注意 Children==nil 有两种含义 (没有子项 / 还没加载): 需要展开时调 LoadVariable
// 再要一层, 不要把 nil 当成"确定没有子项". 这就是懒展开的契约 -- 变量树默认只
// 加载一层, 用户点开哪个节点才为那个节点付一次 RPC.
type Variable struct {
	Name  string
	Type  string
	Value string

	// Kind 是 Go reflect.Kind 的名字 ("struct"/"slice"/"map"/"ptr"/...);
	// dlv 线缆上给的是数值, 这里转成名字. 空串表示 dlv 没报(或报了 Invalid).
	Kind string
	// Addr 是变量地址, 0 表示没有地址 (常量/寄存器里的值/未加载).
	Addr uint64
	// Len 是字符串/slice/map/array/chan 的元素个数 (字符串是字节数), 其它类型为 0.
	// 它是"真实长度", 可能大于 len(Children) -- LoadConfig.MaxArrayValues 会截断.
	Len int
	// Cap 是 slice 容量, 其它类型为 0.
	Cap int
	// Children 是已加载的子变量: struct 的字段 / slice 的元素 / ptr 的目标;
	// map 按 dlv 的约定是 key,value 交替排列.
	Children []Variable
}

// rpcVariable 是 dlv api.Variable 的薄信封
// Len/Cap 在线缆上是 int64, Kind 是 reflect.Kind 的数值; Children 递归引用自身.
type rpcVariable struct {
	Name     string        `json:"name"`
	Type     string        `json:"type"`
	Value    string        `json:"value"`
	Kind     uint          `json:"kind"`
	Addr     uint64        `json:"addr"`
	Len      int64         `json:"len"`
	Cap      int64         `json:"cap"`
	Children []rpcVariable `json:"children"`
}

// convertVariable 递归把线缆变量投影成模型变量
// 子变量为空时 Children 保持 nil (而不是空 slice), 让"没展开"在 UI 侧只有一种形态.
func convertVariable(v rpcVariable) Variable {
	out := Variable{
		Name:  v.Name,
		Type:  v.Type,
		Value: v.Value,
		Kind:  kindName(v.Kind),
		Addr:  v.Addr,
		Len:   int(v.Len),
		Cap:   int(v.Cap),
	}
	if len(v.Children) > 0 {
		out.Children = make([]Variable, 0, len(v.Children))
		for _, c := range v.Children {
			out.Children = append(out.Children, convertVariable(c))
		}
	}
	return out
}

// convertVariables 投影一组线缆变量; 返回值恒非 nil, 与重构前 ListLocals 一致
func convertVariables(in []rpcVariable) []Variable {
	out := make([]Variable, 0, len(in))
	for _, v := range in {
		out = append(out, convertVariable(v))
	}
	return out
}

// kindName 把线缆上的数值 kind 转成 Go 里的类别名
// 0 是 reflect.Invalid -- dlv 用它表示"未知/未加载", 这里映射成空串而不是
// "invalid", 免得 UI 把它当成一个真实类别显示出来.
func kindName(k uint) string {
	if k == 0 {
		return ""
	}
	return reflect.Kind(k).String()
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

// loadConfigDepth 是默认上限 + 指定的递归展开层数
// depth<0 表示"用默认的 1 层"; depth==0 表示只加载这一层, 不展开任何子项.
func loadConfigDepth(depth int) loadConfig {
	cfg := defaultLoadConfig()
	if depth >= 0 {
		cfg.MaxVariableRecurse = depth
	}
	return cfg
}

// evalScope 是 dlv EvalScope: 选哪一个 goroutine 的哪一帧
// GoroutineID = -1 表示当前(SelectedGoroutine), Frame 0 是栈顶.
// dlv 侧这个字段是 int64 (goroutine id 会长到 int32 装不下), 这里对齐.
type evalScope struct {
	GoroutineID int64 `json:"GoroutineID"`
	Frame       int   `json:"Frame"`
}

// rpcLocation 是 dlv 应答里位置的薄信封, 只挑我们暴露的字段
// 这些类型不出包, 上层永远拿到的是 StackFrame / Variable / Goroutine.
type rpcLocation struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Function *struct {
		Name string `json:"name"`
	} `json:"function"`
}

// Stacktrace 返回当前/指定 goroutine 的调用栈
// goroutineID < 0 表示"当前 goroutine": 若 SwitchGoroutine 选过一个就用它, 否则
// 交给 dlv 的 SelectedGoroutine. depth 是最多取几帧 (栈顶起算).
// Full=false 让 dlv 不带 Locals/Arguments -- 我们这里只画位置, 想看局部变量走
// ListLocals/ListArgs. 这样应答体积更小.
func (s *DebugSession) Stacktrace(goroutineID, depth int) ([]StackFrame, error) {
	type stackIn struct {
		ID     int64 `json:"Id"`
		Depth  int   `json:"Depth"`
		Full   bool  `json:"Full"`
		Defers bool  `json:"Defers"`
	}
	type rpcFrame struct {
		Location rpcLocation `json:"Location"`
	}
	type stackOut struct {
		Locations []rpcFrame `json:"Locations"`
	}
	var out stackOut
	in := stackIn{ID: s.scopeGoroutine(int64(goroutineID)), Depth: depth, Full: false, Defers: false}
	if err := s.rpcCall("Stacktrace", in, &out); err != nil {
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

// ListGoroutines 拉取所有 goroutine (不分页)
// 等价于 ListGoroutinesPage(0, 0) -- dlv 里 Count=0 的语义就是"全部".
// 取 UserCurrentLoc 而非 CurrentLoc -- 用户关心自己写的代码而不是 runtime 帧.
func (s *DebugSession) ListGoroutines() ([]Goroutine, error) {
	gs, _, err := s.ListGoroutinesPage(0, 0)
	return gs, err
}

// ListGoroutinesPage 分页拉 goroutine 列表
// start 是起始游标 (第一页传 0), count 是本页最多几条 (0 = 全部).
// 第二个返回值是下一页的游标 (dlv 的 Nextg): 0 表示已经到底了, 非 0 时把它当成
// 下一次调用的 start. 一个真实服务进程 goroutine 上万, 面板靠这个避免一次把整张
// 表拉回来 (也避免 dlv 一次给我们几 MB 的 JSON).
func (s *DebugSession) ListGoroutinesPage(start, count int) ([]Goroutine, int, error) {
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
		Nextg      int            `json:"Nextg"` // 下一页游标, 0 = 没有下一页
	}
	var out listOut
	if err := s.rpcCall("ListGoroutines", listIn{Start: start, Count: count}, &out); err != nil {
		return nil, 0, err
	}
	gs := make([]Goroutine, 0, len(out.Goroutines))
	for _, g := range out.Goroutines {
		// 优先 UserCurrentLoc; 如果 File 为空 (纯 runtime goroutine) 回落到 CurrentLoc
		loc := g.UserCurrentLoc
		if loc.File == "" {
			loc = g.CurrentLoc
		}
		item := Goroutine{ID: g.ID, File: loc.File, Line: loc.Line}
		if loc.Function != nil {
			item.Function = loc.Function.Name
		}
		gs = append(gs, item)
	}
	return gs, out.Nextg, nil
}

// ListLocals 拉取指定 (goroutine, frame) 的局部变量
// goroutineID<0 -> 当前 goroutine (SwitchGoroutine 选过则是它); frame=0 -> 栈顶.
// 只含局部变量; 函数参数走 ListArgs -- dlv 把两者拆成了两个 RPC.
func (s *DebugSession) ListLocals(goroutineID, frame int) ([]Variable, error) {
	return s.listVars("ListLocalVars", int64(goroutineID), frame)
}

// ListArgs 拉取指定 (goroutine, frame) 的函数参数
// 与 ListLocals 一起才是一帧的完整变量视图 (Qt Creator 的 Locals and Expressions
// 里 arguments 是单独一组). goroutineID<0 的含义同 ListLocals.
func (s *DebugSession) ListArgs(goroutineID int64, frame int) ([]Variable, error) {
	return s.listVars("ListFunctionArgs", goroutineID, frame)
}

// listVars 是 ListLocalVars / ListFunctionArgs 共享的实现
// 两个 RPC 的入参形态完全一样; 应答的字段名不一样 -- ListLocalVarsOut 用
// "Variables", ListFunctionArgsOut 用 "Args", 所以这里两个键都解, 取非空的那个.
func (s *DebugSession) listVars(method string, goroutineID int64, frame int) ([]Variable, error) {
	type listIn struct {
		Scope evalScope  `json:"Scope"`
		Cfg   loadConfig `json:"Cfg"`
	}
	type listOut struct {
		Variables []rpcVariable `json:"Variables"`
		Args      []rpcVariable `json:"Args"`
	}
	var out listOut
	in := listIn{
		Scope: evalScope{GoroutineID: s.scopeGoroutine(goroutineID), Frame: frame},
		Cfg:   defaultLoadConfig(),
	}
	if err := s.rpcCall(method, in, &out); err != nil {
		return nil, err
	}
	vars := out.Variables
	if len(vars) == 0 {
		vars = out.Args
	}
	return convertVariables(vars), nil
}

// Eval 在 (goroutine, frame) 作用域下求值一个 Go 表达式
// 表达式形态遵循 dlv 文档: 支持局部/包级变量 + 成员/索引/解引用, 不支持函数调用.
// 这是 hover-to-inspect 和 watch panel 的基础. 默认只展开一层嵌套, 要更深走
// LoadVariable.
func (s *DebugSession) Eval(expr string, goroutineID, frame int) (Variable, error) {
	return s.eval(expr, int64(goroutineID), frame, defaultLoadConfig())
}

// LoadVariable 是变量树"懒展开"的后端: 用户点开某个节点时才把那一层子变量拉回来
// expr 是节点的完整路径表达式 ("x" / "x.Field" / "s[3]"), depth 直接映射到 dlv 的
// LoadConfig.MaxVariableRecurse: 0 = 只加载这个节点本身, 1 = 连它的直接子项,
// 越大展开越深; depth<0 用默认的 1. 其它上限 (字符串长度/数组元素数) 与 Eval 一致.
// 典型用法: 面板拿到 Children==nil 的节点 -> 用户点开 -> LoadVariable(路径, g, f, 1)
// -> 用返回值的 Children 填充这一层. 深度别给大值, dlv 是同步的, 一次深展开会把
// 整个 session 卡住 (同一 session 上的 RPC 是串行的).
func (s *DebugSession) LoadVariable(expr string, goroutineID int64, frame, depth int) (Variable, error) {
	return s.eval(expr, goroutineID, frame, loadConfigDepth(depth))
}

// eval 是 Eval / LoadVariable 共享的实现, 唯一差别是 LoadConfig
func (s *DebugSession) eval(expr string, goroutineID int64, frame int, cfg loadConfig) (Variable, error) {
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
		Scope: evalScope{GoroutineID: s.scopeGoroutine(goroutineID), Frame: frame},
		Expr:  expr,
		Cfg:   cfg,
	}
	if err := s.rpcCall("Eval", in, &out); err != nil {
		return Variable{}, err
	}
	return convertVariable(out.Variable), nil
}

// SetVariable 把 (goroutine, frame) 作用域下的某个变量 symbol 赋成 value
// 这是 IDE 变量面板 "双击改值" 动作的后端.走 dlv 的 RPCServer.Set, 参数
// {Scope: EvalScope, Symbol, Value}, 应答为空 (无 result). symbol 是变量名
// (例如 "x" 或 "p.Field"), value 是 Go 字面量字符串 (dlv 自己解析, 例如 "42"
// / "\"hi\"" / "true"). 类型不匹配或符号不存在时 dlv 回 error, 这里原样 wrap.
// goroutineID<0 表示当前 goroutine (SwitchGoroutine 选过则是它), frame=0 是栈顶.
func (s *DebugSession) SetVariable(symbol, value string, goroutineID, frame int) error {
	type setIn struct {
		Scope  evalScope `json:"Scope"`
		Symbol string    `json:"Symbol"`
		Value  string    `json:"Value"`
	}
	in := setIn{
		Scope:  evalScope{GoroutineID: s.scopeGoroutine(int64(goroutineID)), Frame: frame},
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

// ErrSessionClosed 是 session 已 Close 之后一切 RPC 的统一返回错误
// Close 会先关 conn 把阻塞中的 Decode 唤醒 -- rpcCall 醒来看到 closed 时也回它,
// 不把底层 "use of closed network connection" 之类的 read 错误抛给调用方.
// 导出以便上层 (silkide) 用 errors.Is 区分"用户主动 Stop"和真实错误, 前者不弹错误提示.
var ErrSessionClosed = errors.New("debug session closed")

// errSessionClosed 是内部别名, 保持既有引用不变.
var errSessionClosed = ErrSessionClosed

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
