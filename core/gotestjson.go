package core

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
	"time"
)

// `go test -json` (test2json) 事件流解析 + 聚合
//
// 流格式: 每行一个 JSON 对象, 字段
//
//	Time    事件时间戳
//	Action  run|pause|cont|pass|fail|skip|output|bench (go1.24+ 还有 start / build-output / build-fail)
//	Package 包导入路径
//	Test    测试名; 缺省表示这是包级事件
//	Elapsed 耗时(秒, 浮点)
//	Output  一段输出文本(通常正好一行, 带结尾 \n)
//
// 与"解析 go test -v 控制台文本"的做法相比, JSON 流的关键优势是每条输出都自带
// Package/Test 归属: 多个包并行跑时输出交错也不会错配, 因此这里绝不做"事后把包名
// 回填给前面的行"这种猜测 —— 每行输出只挂到它自己声明的那个测试上, 包级输出
// (事件里没有 Test 字段)单独存放.
//
// 子测试在流里以 "Parent/Sub" 的全名出现, 聚合器据此建立父子关系, 树形展开由
// Roots()/Children() 提供. 编译失败等非事件文本(裸文本行, 或 go1.24+ 的
// build-output 事件)统一收进 BuildOutput, 不污染测试结果.
//
// 只做数据层: 不启动进程, 不碰 UI. 进程侧在 ide/gotest.

// TestAction 是事件的 Action 字段
type TestAction string

const (
	TestActionStart  TestAction = "start"  // 包开始跑(go1.24+)
	TestActionRun    TestAction = "run"    // 测试开始
	TestActionPause  TestAction = "pause"  // t.Parallel 让出
	TestActionCont   TestAction = "cont"   // 并行测试恢复
	TestActionPass   TestAction = "pass"   // 通过(终态)
	TestActionFail   TestAction = "fail"   // 失败(终态)
	TestActionSkip   TestAction = "skip"   // 跳过(终态)
	TestActionOutput TestAction = "output" // 一段输出
	TestActionBench  TestAction = "bench"  // 基准结果行(基准的终态)

	TestActionBuildOutput TestAction = "build-output" // 编译诊断文本(go1.24+)
	TestActionBuildFail   TestAction = "build-fail"   // 编译失败(go1.24+)
)

// TestEvent 是流里的一条事件, 字段与 test2json 的输出一一对应
// Elapsed 保留原始的"秒"单位; 折算成 Duration 用 Duration().
type TestEvent struct {
	Time    time.Time  `json:"Time,omitempty"`
	Action  TestAction `json:"Action"`
	Package string     `json:"Package,omitempty"`
	Test    string     `json:"Test,omitempty"`
	Elapsed float64    `json:"Elapsed,omitempty"`
	Output  string     `json:"Output,omitempty"`
	// ImportPath 只出现在 build-output / build-fail 事件上(这类事件没有 Package)
	ImportPath string `json:"ImportPath,omitempty"`
	// FailedBuild 出现在"因为编译失败而 fail"的包级事件上, 值是失败的 ImportPath
	FailedBuild string `json:"FailedBuild,omitempty"`
}

// Duration 把 Elapsed(秒)折算成 time.Duration
// 用四舍五入而非截断: 0.29 的 float64 表示略小于 0.29, 直接截断会得到
// 289999999ns 这种毛刺值.
func (e TestEvent) Duration() time.Duration {
	return time.Duration(math.Round(e.Elapsed * float64(time.Second)))
}

// ParseTestEvent 解析一行事件
// ok=false 表示这一行不是事件: 裸文本(老工具链把编译错误直接打在流里)、坏 JSON、
// 或缺 Action 的对象. 调用方应把这类行当作 build 输出保留而不是丢弃.
func ParseTestEvent(line string) (TestEvent, bool) {
	s := strings.TrimSpace(line)
	// test2json 每行都是一个完整 JSON 对象; 先按首尾字符快速排除裸文本,
	// 省掉为每个编译错误行白跑一次 Unmarshal
	if len(s) < 2 || s[0] != '{' || s[len(s)-1] != '}' {
		return TestEvent{}, false
	}
	var ev TestEvent
	if err := json.Unmarshal([]byte(s), &ev); err != nil {
		return TestEvent{}, false
	}
	if ev.Action == "" {
		return TestEvent{}, false
	}
	return ev, true
}

// TestStatus 是一个测试或一个包的状态
// 零值是 TestStatusRunning: 只收到 run 还没收到终态事件时就是这个状态
// (流被 ctrl-c 掐断时, 半截的测试也停在这里).
type TestStatus int

const (
	TestStatusRunning TestStatus = iota
	TestStatusPass
	TestStatusFail
	TestStatusSkip
)

// String 返回小写状态名
func (s TestStatus) String() string {
	switch s {
	case TestStatusRunning:
		return "running"
	case TestStatusPass:
		return "pass"
	case TestStatusFail:
		return "fail"
	case TestStatusSkip:
		return "skip"
	default:
		return "unknown"
	}
}

// TestResult 是一个测试(或子测试)的聚合结果
// Name 是全名, 子测试形如 "Parent/Sub"; Parent 是直接父测试的全名, 顶层测试为空.
// Output 只包含"事件里 Test == Name"的那些输出行 —— 子测试的输出不会冒泡到父测试.
type TestResult struct {
	Name    string
	Status  TestStatus
	Elapsed time.Duration
	Output  []string
	Parent  string
}

// PackageResult 是一个包的聚合结果
// Tests 以测试全名为键(含子测试); Order 记录首次出现顺序, 用它遍历才能得到
// 稳定输出(map 遍历顺序是随机的).
// Output 是包级输出(事件里没有 Test 字段的那些行, 例如 "ok  pkg  0.31s"),
// 与测试自身的输出严格分开.
type PackageResult struct {
	Package string
	Status  TestStatus
	Elapsed time.Duration
	Tests   map[string]*TestResult
	Order   []string
	Output  []string
}

// Test 按全名取一个测试, 不存在返回 nil
func (p *PackageResult) Test(name string) *TestResult {
	if p == nil {
		return nil
	}
	return p.Tests[name]
}

// TestList 按首次出现顺序返回全部测试(含子测试)
func (p *PackageResult) TestList() []*TestResult {
	if p == nil {
		return nil
	}
	out := make([]*TestResult, 0, len(p.Order))
	for _, name := range p.Order {
		if t := p.Tests[name]; t != nil {
			out = append(out, t)
		}
	}
	return out
}

// Roots 返回顶层测试(Parent == ""), 首次出现顺序
func (p *PackageResult) Roots() []*TestResult { return p.childrenOf("") }

// Children 返回 name 的直接子测试, 首次出现顺序
// 只返回下一层: "A/b/c" 是 "A/b" 的孩子, 不是 "A" 的孩子.
func (p *PackageResult) Children(name string) []*TestResult { return p.childrenOf(name) }

func (p *PackageResult) childrenOf(parent string) []*TestResult {
	if p == nil {
		return nil
	}
	var out []*TestResult
	for _, name := range p.Order {
		if t := p.Tests[name]; t != nil && t.Parent == parent {
			out = append(out, t)
		}
	}
	return out
}

// Counts 统计本包 pass/fail/skip 的数量
// 计入子测试: 一个子测试失败时它的父测试也是 fail, 两条都算 —— 这与面板
// "一行一个测试"的展示口径一致.
func (p *PackageResult) Counts() (passed, failed, skipped int) {
	if p == nil {
		return
	}
	for _, name := range p.Order {
		t := p.Tests[name]
		if t == nil {
			continue
		}
		switch t.Status {
		case TestStatusPass:
			passed++
		case TestStatusFail:
			failed++
		case TestStatusSkip:
			skipped++
		}
	}
	return
}

// touch 取(或建)一个测试节点
// 子测试名形如 "Parent/Sub": 先递归补齐所有祖先节点再挂自己, 这样即使流里
// 缺了父测试的 run 事件, Roots()/Children() 拼出来的树也不会有孤儿.
func (p *PackageResult) touch(name string) *TestResult {
	if p.Tests == nil {
		p.Tests = make(map[string]*TestResult)
	}
	if t, ok := p.Tests[name]; ok {
		return t
	}
	parent := ""
	if i := strings.LastIndexByte(name, '/'); i > 0 {
		parent = name[:i]
		p.touch(parent)
	}
	t := &TestResult{Name: name, Parent: parent}
	p.Tests[name] = t
	p.Order = append(p.Order, name)
	return t
}

// clone 深拷贝一个包结果
func (p *PackageResult) clone() *PackageResult {
	cp := &PackageResult{
		Package: p.Package,
		Status:  p.Status,
		Elapsed: p.Elapsed,
		Tests:   make(map[string]*TestResult, len(p.Tests)),
		Order:   append([]string(nil), p.Order...),
		Output:  append([]string(nil), p.Output...),
	}
	for name, t := range p.Tests {
		ct := *t
		ct.Output = append([]string(nil), t.Output...)
		cp.Tests[name] = &ct
	}
	return cp
}

// TestRef 定位一个测试: 包导入路径 + 测试全名
// 有了包名, "在 ./... 上按裸测试名跑"导致的重名歧义就不存在了.
type TestRef struct {
	Package string
	Test    string
}

// Aggregator 把事件流折叠成按包组织的结果
// 不是并发安全的: 由单个读取协程喂数据, 要把状态交给别的线程(比如 GUI 线程)
// 先 Clone 一份.
type Aggregator struct {
	order []string
	pkgs  map[string]*PackageResult
	build []string
}

// NewAggregator 建一个空聚合器
func NewAggregator() *Aggregator {
	return &Aggregator{pkgs: make(map[string]*PackageResult)}
}

// AddLine 解析并折叠一行
// 返回解析出的事件; 返回值的 Action 为空表示这一行不是事件, 已被收进 BuildOutput.
// 空行直接忽略.
func (a *Aggregator) AddLine(line string) TestEvent {
	if strings.TrimSpace(line) == "" {
		return TestEvent{}
	}
	ev, ok := ParseTestEvent(line)
	if !ok {
		a.build = append(a.build, strings.TrimRight(line, "\r\n"))
		return TestEvent{}
	}
	a.AddEvent(ev)
	return ev
}

// AddEvent 折叠一条已解析的事件
// 归属规则:
//   - build-output / build-fail, 以及任何没有 Package 的事件 -> BuildOutput
//   - 有 Package 没有 Test -> 包级(状态/耗时/包级输出)
//   - 有 Package 有 Test   -> 该测试自己的(状态/耗时/输出)
//
// run/start/pause/cont 只负责把节点建出来(状态留在 Running), 不改状态.
// bench 视为基准的终态并记成 pass: 基准跑完只有这一条结果事件, 失败的基准会
// 单独发 fail.
func (a *Aggregator) AddEvent(ev TestEvent) {
	switch ev.Action {
	case TestActionBuildOutput, TestActionBuildFail:
		// go1.24+ 把编译诊断也塞进 JSON 流: 这类事件只有 ImportPath 没有 Package,
		// 不属于任何测试
		a.build = appendOutputLines(a.build, ev.Output)
		return
	}
	if ev.Package == "" {
		// 没有包归属的输出理论上不该出现, 但也别丢
		a.build = appendOutputLines(a.build, ev.Output)
		return
	}
	pkg := a.pkg(ev.Package)
	if ev.Test == "" {
		switch ev.Action {
		case TestActionOutput:
			pkg.Output = appendOutputLines(pkg.Output, ev.Output)
		case TestActionPass:
			pkg.Status, pkg.Elapsed = TestStatusPass, ev.Duration()
		case TestActionFail:
			pkg.Status, pkg.Elapsed = TestStatusFail, ev.Duration()
		case TestActionSkip:
			pkg.Status, pkg.Elapsed = TestStatusSkip, ev.Duration()
		}
		return
	}
	t := pkg.touch(ev.Test)
	switch ev.Action {
	case TestActionOutput:
		t.Output = appendOutputLines(t.Output, ev.Output)
	case TestActionPass, TestActionBench:
		t.Status, t.Elapsed = TestStatusPass, ev.Duration()
	case TestActionFail:
		t.Status, t.Elapsed = TestStatusFail, ev.Duration()
	case TestActionSkip:
		t.Status, t.Elapsed = TestStatusSkip, ev.Duration()
	}
}

// Feed 从 r 逐行读入整个事件流
// onLine 非 nil 时在每行折叠之后回调一次(raw 是原始行, ev.Action 为空表示这行
// 不是事件), 流式 UI 用它做增量刷新. 单行超过 4MB 会返回错误, 此前折叠的结果
// 仍然保留.
func (a *Aggregator) Feed(r io.Reader, onLine func(ev TestEvent, raw string)) error {
	sc := bufio.NewScanner(r)
	// 一条 output 事件可能带着很长的 t.Log 文本, 默认 64KB 上限不够用
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		raw := sc.Text()
		ev := a.AddLine(raw)
		if onLine != nil {
			onLine(ev, raw)
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("go test -json scan: %w", err)
	}
	return nil
}

// Packages 按首次出现顺序返回所有包结果
func (a *Aggregator) Packages() []*PackageResult {
	out := make([]*PackageResult, 0, len(a.order))
	for _, path := range a.order {
		if p := a.pkgs[path]; p != nil {
			out = append(out, p)
		}
	}
	return out
}

// Package 按导入路径取一个包结果, 不存在返回 nil
func (a *Aggregator) Package(path string) *PackageResult { return a.pkgs[path] }

// BuildOutput 返回所有非事件文本行(编译错误、go 命令自身的诊断)
func (a *Aggregator) BuildOutput() []string { return a.build }

// Counts 汇总所有包的 pass/fail/skip 数量
func (a *Aggregator) Counts() (passed, failed, skipped int) {
	for _, p := range a.Packages() {
		pa, fa, sk := p.Counts()
		passed += pa
		failed += fa
		skipped += sk
	}
	return
}

// FailedTests 按包/测试的首次出现顺序返回所有 fail 的测试(含子测试)
func (a *Aggregator) FailedTests() []TestRef {
	var out []TestRef
	for _, p := range a.Packages() {
		for _, t := range p.TestList() {
			if t.Status == TestStatusFail {
				out = append(out, TestRef{Package: p.Package, Test: t.Name})
			}
		}
	}
	return out
}

// Clone 深拷贝整个聚合结果
// 流式回调拿到的 *Aggregator 属于读取协程, 跨线程交付前先 Clone.
func (a *Aggregator) Clone() *Aggregator {
	cp := &Aggregator{
		order: append([]string(nil), a.order...),
		pkgs:  make(map[string]*PackageResult, len(a.pkgs)),
		build: append([]string(nil), a.build...),
	}
	for path, p := range a.pkgs {
		cp.pkgs[path] = p.clone()
	}
	return cp
}

// pkg 取(或建)一个包结果, 并记录首次出现顺序
func (a *Aggregator) pkg(path string) *PackageResult {
	if a.pkgs == nil {
		a.pkgs = make(map[string]*PackageResult)
	}
	p, ok := a.pkgs[path]
	if !ok {
		p = &PackageResult{Package: path, Tests: make(map[string]*TestResult)}
		a.pkgs[path] = p
		a.order = append(a.order, path)
	}
	return p
}

// appendOutputLines 把一段 output 文本按行追加到 dst
// Output 一般正好是一行(带结尾 \n), 但也可能是多行、或没有结尾换行的片段.
// 行内原样保留(失败输出靠缩进表达层级), 只去掉行尾的 \n / \r.
// 空字符串什么都不追加; "\n" 追加一个空行(失败输出里的空行是有意义的).
func appendOutputLines(dst []string, out string) []string {
	if out == "" {
		return dst
	}
	out = strings.TrimSuffix(out, "\n")
	for _, ln := range strings.Split(out, "\n") {
		dst = append(dst, strings.TrimSuffix(ln, "\r"))
	}
	return dst
}
