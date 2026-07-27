package core

import (
	"strings"
	"testing"
	"time"
)

// twoPackageStream 是一段"两个包并行、输出交错"的 go test -json 流.
// 刻意让两个包里各有一个同名 TestAlpha, 并把它们的 output 事件穿插在一起 ——
// 这正是"解析控制台文本 + 事后回填包名"会错配的场景.
// 内含: 子测试(pass + fail)、父测试因子测试失败而 fail、skip、包级汇总行.
const twoPackageStream = `{"Time":"2026-07-27T02:51:02.241002-07:00","Action":"start","Package":"gtj/a"}
{"Time":"2026-07-27T02:51:02.241135-07:00","Action":"start","Package":"gtj/b"}
{"Action":"run","Package":"gtj/a","Test":"TestAlpha"}
{"Action":"output","Package":"gtj/a","Test":"TestAlpha","Output":"=== RUN   TestAlpha\n"}
{"Action":"run","Package":"gtj/b","Test":"TestAlpha"}
{"Action":"output","Package":"gtj/b","Test":"TestAlpha","Output":"=== RUN   TestAlpha\n"}
{"Action":"output","Package":"gtj/a","Test":"TestAlpha","Output":"    a_test.go:6: alpha ran\n"}
{"Action":"output","Package":"gtj/b","Test":"TestAlpha","Output":"    b_test.go:9: beta side\n"}
{"Action":"pass","Package":"gtj/a","Test":"TestAlpha","Elapsed":0.02}
{"Action":"run","Package":"gtj/a","Test":"TestParent"}
{"Action":"output","Package":"gtj/a","Test":"TestParent","Output":"=== RUN   TestParent\n"}
{"Action":"run","Package":"gtj/a","Test":"TestParent/sub_ok"}
{"Action":"output","Package":"gtj/a","Test":"TestParent/sub_ok","Output":"    a_test.go:10: sub ok\n"}
{"Action":"pass","Package":"gtj/a","Test":"TestParent/sub_ok","Elapsed":0}
{"Action":"run","Package":"gtj/a","Test":"TestParent/sub_bad"}
{"Action":"output","Package":"gtj/a","Test":"TestParent/sub_bad","Output":"    a_test.go:11: boom: want 1 got 2\n"}
{"Action":"output","Package":"gtj/a","Test":"TestParent/sub_bad","Output":"--- FAIL: TestParent/sub_bad (0.01s)\n"}
{"Action":"fail","Package":"gtj/a","Test":"TestParent/sub_bad","Elapsed":0.01}
{"Action":"output","Package":"gtj/a","Test":"TestParent","Output":"--- FAIL: TestParent (0.03s)\n"}
{"Action":"fail","Package":"gtj/a","Test":"TestParent","Elapsed":0.03}
{"Action":"run","Package":"gtj/b","Test":"TestSkipped"}
{"Action":"output","Package":"gtj/b","Test":"TestSkipped","Output":"    b_test.go:15: nope\n"}
{"Action":"skip","Package":"gtj/b","Test":"TestSkipped","Elapsed":0}
{"Action":"pass","Package":"gtj/b","Test":"TestAlpha","Elapsed":0.05}
{"Action":"output","Package":"gtj/a","Output":"FAIL\n"}
{"Action":"output","Package":"gtj/a","Output":"FAIL\tgtj/a\t0.314s\n"}
{"Action":"fail","Package":"gtj/a","Elapsed":0.314}
{"Action":"output","Package":"gtj/b","Output":"ok  \tgtj/b\t0.29s\n"}
{"Action":"pass","Package":"gtj/b","Elapsed":0.29}
`

// feedStream 把一段流喂进一个新聚合器
func feedStream(t *testing.T, stream string) *Aggregator {
	t.Helper()
	agg := NewAggregator()
	if err := agg.Feed(strings.NewReader(stream), nil); err != nil {
		t.Fatalf("Feed: unexpected error: %v", err)
	}
	return agg
}

// TestAggregatorPackageOrderAndStatus: 包按首次出现顺序排列, 包级状态和耗时
// 来自包级事件(没有 Test 字段的那条).
func TestAggregatorPackageOrderAndStatus(t *testing.T) {
	agg := feedStream(t, twoPackageStream)

	pkgs := agg.Packages()
	if len(pkgs) != 2 {
		t.Fatalf("packages = %d, want 2", len(pkgs))
	}
	if pkgs[0].Package != "gtj/a" || pkgs[1].Package != "gtj/b" {
		t.Errorf("package order = [%s %s], want [gtj/a gtj/b]", pkgs[0].Package, pkgs[1].Package)
	}
	if pkgs[0].Status != TestStatusFail {
		t.Errorf("gtj/a status = %v, want fail", pkgs[0].Status)
	}
	if pkgs[0].Elapsed != 314*time.Millisecond {
		t.Errorf("gtj/a elapsed = %v, want 314ms", pkgs[0].Elapsed)
	}
	if pkgs[1].Status != TestStatusPass {
		t.Errorf("gtj/b status = %v, want pass", pkgs[1].Status)
	}
	if pkgs[1].Elapsed != 290*time.Millisecond {
		t.Errorf("gtj/b elapsed = %v, want 290ms", pkgs[1].Elapsed)
	}
	if got := agg.Package("gtj/nope"); got != nil {
		t.Errorf("Package(unknown) = %v, want nil", got)
	}
	if len(agg.BuildOutput()) != 0 {
		t.Errorf("BuildOutput = %v, want empty", agg.BuildOutput())
	}
}

// TestAggregatorTestStatusAndElapsed: 每个测试(含子测试)的状态和耗时都取自
// 自己的终态事件, 不受同名测试或交错输出影响.
func TestAggregatorTestStatusAndElapsed(t *testing.T) {
	agg := feedStream(t, twoPackageStream)

	cases := []struct {
		pkg     string
		test    string
		status  TestStatus
		elapsed time.Duration
	}{
		{"gtj/a", "TestAlpha", TestStatusPass, 20 * time.Millisecond},
		{"gtj/a", "TestParent", TestStatusFail, 30 * time.Millisecond},
		{"gtj/a", "TestParent/sub_ok", TestStatusPass, 0},
		{"gtj/a", "TestParent/sub_bad", TestStatusFail, 10 * time.Millisecond},
		{"gtj/b", "TestAlpha", TestStatusPass, 50 * time.Millisecond},
		{"gtj/b", "TestSkipped", TestStatusSkip, 0},
	}
	for _, c := range cases {
		tr := agg.Package(c.pkg).Test(c.test)
		if tr == nil {
			t.Errorf("%s %s: missing", c.pkg, c.test)
			continue
		}
		if tr.Status != c.status {
			t.Errorf("%s %s: status = %v, want %v", c.pkg, c.test, tr.Status, c.status)
		}
		if tr.Elapsed != c.elapsed {
			t.Errorf("%s %s: elapsed = %v, want %v", c.pkg, c.test, tr.Elapsed, c.elapsed)
		}
	}

	// 两个包各自只有自己的测试; gtj/a 的 TestAlpha 不会跑到 gtj/b 里去
	if n := len(agg.Package("gtj/a").Tests); n != 4 {
		t.Errorf("gtj/a tests = %d, want 4", n)
	}
	if n := len(agg.Package("gtj/b").Tests); n != 2 {
		t.Errorf("gtj/b tests = %d, want 2", n)
	}

	passed, failed, skipped := agg.Counts()
	if passed != 3 || failed != 2 || skipped != 1 {
		t.Errorf("Counts = (%d,%d,%d), want (3,2,1)", passed, failed, skipped)
	}
}

// TestAggregatorOutputAttribution: 输出行只落在事件声明的那个测试上 ——
// 交错的同名测试不会互相污染, 子测试的输出不会冒泡进父测试, 包级输出单独存放.
func TestAggregatorOutputAttribution(t *testing.T) {
	agg := feedStream(t, twoPackageStream)
	a := agg.Package("gtj/a")
	b := agg.Package("gtj/b")

	wantAlphaA := []string{"=== RUN   TestAlpha", "    a_test.go:6: alpha ran"}
	if got := a.Test("TestAlpha").Output; !equalLines(got, wantAlphaA) {
		t.Errorf("gtj/a TestAlpha output = %q, want %q", got, wantAlphaA)
	}
	wantAlphaB := []string{"=== RUN   TestAlpha", "    b_test.go:9: beta side"}
	if got := b.Test("TestAlpha").Output; !equalLines(got, wantAlphaB) {
		t.Errorf("gtj/b TestAlpha output = %q, want %q", got, wantAlphaB)
	}

	// 父测试只拿到自己那两行, 子测试的诊断留在子测试上
	wantParent := []string{"=== RUN   TestParent", "--- FAIL: TestParent (0.03s)"}
	if got := a.Test("TestParent").Output; !equalLines(got, wantParent) {
		t.Errorf("TestParent output = %q, want %q", got, wantParent)
	}
	wantBad := []string{
		"    a_test.go:11: boom: want 1 got 2",
		"--- FAIL: TestParent/sub_bad (0.01s)",
	}
	if got := a.Test("TestParent/sub_bad").Output; !equalLines(got, wantBad) {
		t.Errorf("sub_bad output = %q, want %q", got, wantBad)
	}

	// 包级输出("FAIL" / "ok  pkg  0.29s")既不属于任何测试, 也不会被回填
	wantPkgA := []string{"FAIL", "FAIL\tgtj/a\t0.314s"}
	if got := a.Output; !equalLines(got, wantPkgA) {
		t.Errorf("gtj/a package output = %q, want %q", got, wantPkgA)
	}
	wantPkgB := []string{"ok  \tgtj/b\t0.29s"}
	if got := b.Output; !equalLines(got, wantPkgB) {
		t.Errorf("gtj/b package output = %q, want %q", got, wantPkgB)
	}
	if a.Test("") != nil {
		t.Error(`a.Test("") != nil: 包级事件不应该建出一个空名测试`)
	}
}

// TestAggregatorSubtestTree: 子测试挂在父测试下, Roots/Children 给出稳定的树.
func TestAggregatorSubtestTree(t *testing.T) {
	a := feedStream(t, twoPackageStream).Package("gtj/a")

	wantOrder := []string{"TestAlpha", "TestParent", "TestParent/sub_ok", "TestParent/sub_bad"}
	if !equalLines(a.Order, wantOrder) {
		t.Errorf("order = %q, want %q", a.Order, wantOrder)
	}
	if got := names(a.Roots()); !equalLines(got, []string{"TestAlpha", "TestParent"}) {
		t.Errorf("roots = %q, want [TestAlpha TestParent]", got)
	}
	if got := names(a.Children("TestParent")); !equalLines(got, []string{"TestParent/sub_ok", "TestParent/sub_bad"}) {
		t.Errorf("children(TestParent) = %q, want [TestParent/sub_ok TestParent/sub_bad]", got)
	}
	if got := a.Test("TestParent/sub_ok").Parent; got != "TestParent" {
		t.Errorf("sub_ok parent = %q, want TestParent", got)
	}
	if got := a.Test("TestAlpha").Parent; got != "" {
		t.Errorf("TestAlpha parent = %q, want empty", got)
	}
	if got := a.Children("TestAlpha"); len(got) != 0 {
		t.Errorf("children(TestAlpha) = %q, want none", names(got))
	}

	failed := feedStream(t, twoPackageStream).FailedTests()
	want := []TestRef{
		{Package: "gtj/a", Test: "TestParent"},
		{Package: "gtj/a", Test: "TestParent/sub_bad"},
	}
	if len(failed) != len(want) {
		t.Fatalf("FailedTests = %v, want %v", failed, want)
	}
	for i := range want {
		if failed[i] != want[i] {
			t.Errorf("FailedTests[%d] = %v, want %v", i, failed[i], want[i])
		}
	}
}

// TestAggregatorNestedSubtestAncestors: 只看到孙子测试的事件时, 缺失的祖先
// 节点被补出来, 树里不会留下孤儿.
func TestAggregatorNestedSubtestAncestors(t *testing.T) {
	agg := feedStream(t, `{"Action":"fail","Package":"p","Test":"TestX/mid/leaf","Elapsed":0.01}`)
	p := agg.Package("p")
	if !equalLines(p.Order, []string{"TestX", "TestX/mid", "TestX/mid/leaf"}) {
		t.Fatalf("order = %q, want [TestX TestX/mid TestX/mid/leaf]", p.Order)
	}
	if got := names(p.Roots()); !equalLines(got, []string{"TestX"}) {
		t.Errorf("roots = %q, want [TestX]", got)
	}
	if got := p.Test("TestX").Status; got != TestStatusRunning {
		t.Errorf("补出来的祖先 status = %v, want running", got)
	}
	if got := p.Test("TestX/mid/leaf").Parent; got != "TestX/mid" {
		t.Errorf("leaf parent = %q, want TestX/mid", got)
	}
}

// buildErrorStream 混了三种"不是测试事件"的行: 老工具链直接打在流里的裸文本、
// go1.24+ 的 build-output/build-fail 事件、以及一行坏 JSON.
const buildErrorStream = `# gtj/b [gtj/b.test]
b/broken.go:3:28: cannot use "not an int" (untyped string constant) as int value
{"ImportPath":"gtj/b [gtj/b.test]","Action":"build-output","Output":"# gtj/b [gtj/b.test]\n"}
{"ImportPath":"gtj/b [gtj/b.test]","Action":"build-output","Output":"b/broken.go:3:28: cannot use \"not an int\" (untyped string constant) as int value\n"}
{"ImportPath":"gtj/b [gtj/b.test]","Action":"build-fail"}
{"Time":"2026-07-27T02:51:15.035874-07:00","Action":"start","Package":"gtj/b"}
{"Action":"output","Package":"gtj/b","Output":"FAIL\tgtj/b [build failed]\n"}
{"Action":"fail","Package":"gtj/b","Elapsed":0,"FailedBuild":"gtj/b [gtj/b.test]"}
{"Action":"output","Package":"gtj/b",
`

// TestAggregatorBuildOutput: 非事件文本落进 BuildOutput, 不伪造出测试结果;
// 包级 fail 仍然被记下来, 于是"编译失败"= 包 fail + BuildOutput 非空.
func TestAggregatorBuildOutput(t *testing.T) {
	agg := feedStream(t, buildErrorStream)

	want := []string{
		"# gtj/b [gtj/b.test]",
		`b/broken.go:3:28: cannot use "not an int" (untyped string constant) as int value`,
		"# gtj/b [gtj/b.test]",
		`b/broken.go:3:28: cannot use "not an int" (untyped string constant) as int value`,
		`{"Action":"output","Package":"gtj/b",`,
	}
	if got := agg.BuildOutput(); !equalLines(got, want) {
		t.Errorf("BuildOutput = %q,\nwant %q", got, want)
	}

	pkgs := agg.Packages()
	if len(pkgs) != 1 || pkgs[0].Package != "gtj/b" {
		t.Fatalf("packages = %v, want only gtj/b", pkgs)
	}
	if pkgs[0].Status != TestStatusFail {
		t.Errorf("status = %v, want fail", pkgs[0].Status)
	}
	if n := len(pkgs[0].Tests); n != 0 {
		t.Errorf("tests = %d, want 0 (编译失败时一个测试都没跑)", n)
	}
	if got := pkgs[0].Output; !equalLines(got, []string{"FAIL\tgtj/b [build failed]"}) {
		t.Errorf("package output = %q", got)
	}
	passed, failed, skipped := agg.Counts()
	if passed != 0 || failed != 0 || skipped != 0 {
		t.Errorf("Counts = (%d,%d,%d), want all zero", passed, failed, skipped)
	}
}

// TestParseTestEventRejects: 只有"带 Action 的合法 JSON 对象"才算事件.
func TestParseTestEventRejects(t *testing.T) {
	bad := []string{
		"",
		"   ",
		"# gtj/b [gtj/b.test]",
		"b/broken.go:3:28: cannot use x as int",
		`{"Action":"output"`,
		`{"Package":"p","Test":"TestX"}`,
		`{"Action":,}`,
	}
	for _, line := range bad {
		if ev, ok := ParseTestEvent(line); ok {
			t.Errorf("ParseTestEvent(%q) = (%+v, true), want ok=false", line, ev)
		}
	}
	ev, ok := ParseTestEvent(`  {"Action":"pass","Package":"p","Test":"TestX","Elapsed":0.29}  `)
	if !ok {
		t.Fatal("ParseTestEvent: 合法事件被拒")
	}
	if ev.Action != TestActionPass || ev.Package != "p" || ev.Test != "TestX" {
		t.Errorf("event = %+v", ev)
	}
	// 0.29 的 float64 略小于 0.29, 截断会得到 289999999ns; Duration() 四舍五入
	if got := ev.Duration(); got != 290*time.Millisecond {
		t.Errorf("Duration = %v, want 290ms", got)
	}
}

// TestAggregatorBenchAndRunActions: run/pause/cont 只建节点不改状态, bench 记成
// 基准的终态 pass.
func TestAggregatorBenchAndRunActions(t *testing.T) {
	agg := feedStream(t, `{"Action":"run","Package":"p","Test":"TestSlow"}
{"Action":"pause","Package":"p","Test":"TestSlow"}
{"Action":"cont","Package":"p","Test":"TestSlow"}
{"Action":"run","Package":"p","Test":"BenchmarkFast"}
{"Action":"output","Package":"p","Test":"BenchmarkFast","Output":"BenchmarkFast-10  \t1000000\t  1012 ns/op\n"}
{"Action":"bench","Package":"p","Test":"BenchmarkFast","Elapsed":1.5}
`)
	p := agg.Package("p")
	if got := p.Test("TestSlow").Status; got != TestStatusRunning {
		t.Errorf("TestSlow status = %v, want running (没有终态事件)", got)
	}
	bench := p.Test("BenchmarkFast")
	if bench.Status != TestStatusPass || bench.Elapsed != 1500*time.Millisecond {
		t.Errorf("BenchmarkFast = %v/%v, want pass/1.5s", bench.Status, bench.Elapsed)
	}
	if got := p.Status; got != TestStatusRunning {
		t.Errorf("package status = %v, want running (没有包级终态事件)", got)
	}
}

// TestAggregatorFeedCallback: Feed 的回调逐行触发, 原始行原样给出, 非事件行的
// Action 为空.
func TestAggregatorFeedCallback(t *testing.T) {
	agg := NewAggregator()
	var actions []TestAction
	var raws []string
	err := agg.Feed(strings.NewReader(`# build noise
{"Action":"run","Package":"p","Test":"TestX"}

{"Action":"pass","Package":"p","Test":"TestX","Elapsed":0.02}
`), func(ev TestEvent, raw string) {
		actions = append(actions, ev.Action)
		raws = append(raws, raw)
	})
	if err != nil {
		t.Fatalf("Feed: %v", err)
	}
	wantActions := []TestAction{"", TestActionRun, "", TestActionPass}
	if len(actions) != len(wantActions) {
		t.Fatalf("callbacks = %d %v, want %d", len(actions), actions, len(wantActions))
	}
	for i := range wantActions {
		if actions[i] != wantActions[i] {
			t.Errorf("action[%d] = %q, want %q", i, actions[i], wantActions[i])
		}
	}
	if raws[0] != "# build noise" {
		t.Errorf("raw[0] = %q", raws[0])
	}
	// 空行既不进 BuildOutput 也不建节点
	if got := agg.BuildOutput(); !equalLines(got, []string{"# build noise"}) {
		t.Errorf("BuildOutput = %q", got)
	}
}

// TestAggregatorClone: Clone 是深拷贝, 拷贝出来的结果不会被后续事件改动.
func TestAggregatorClone(t *testing.T) {
	agg := feedStream(t, twoPackageStream)
	snap := agg.Clone()

	agg.AddLine(`{"Action":"run","Package":"gtj/c","Test":"TestLater"}`)
	agg.AddLine(`{"Action":"output","Package":"gtj/a","Test":"TestAlpha","Output":"late line\n"}`)
	agg.AddLine(`{"Action":"fail","Package":"gtj/b","Test":"TestAlpha","Elapsed":0.09}`)
	agg.AddLine("late build noise")

	if len(snap.Packages()) != 2 {
		t.Errorf("snapshot packages = %d, want 2", len(snap.Packages()))
	}
	if got := len(snap.Package("gtj/a").Test("TestAlpha").Output); got != 2 {
		t.Errorf("snapshot output lines = %d, want 2", got)
	}
	if got := snap.Package("gtj/b").Test("TestAlpha").Status; got != TestStatusPass {
		t.Errorf("snapshot status = %v, want pass", got)
	}
	if got := len(snap.BuildOutput()); got != 0 {
		t.Errorf("snapshot BuildOutput = %d lines, want 0", got)
	}
	// 原聚合器确实变了(证明上面的断言不是因为改动没生效)
	if got := len(agg.Package("gtj/a").Test("TestAlpha").Output); got != 3 {
		t.Errorf("live output lines = %d, want 3", got)
	}
}

// TestStatusString 固定状态名, 面板和日志都用它
func TestStatusStringNames(t *testing.T) {
	cases := map[TestStatus]string{
		TestStatusRunning: "running",
		TestStatusPass:    "pass",
		TestStatusFail:    "fail",
		TestStatusSkip:    "skip",
		TestStatus(99):    "unknown",
	}
	for st, want := range cases {
		if got := st.String(); got != want {
			t.Errorf("TestStatus(%d).String() = %q, want %q", int(st), got, want)
		}
	}
}

// TestAppendOutputLinesShapes 覆盖 output 文本切行的边界情况
func TestAppendOutputLinesShapes(t *testing.T) {
	if got := appendOutputLines(nil, ""); got != nil {
		t.Errorf("empty output appended %q", got)
	}
	if got := appendOutputLines(nil, "\n"); !equalLines(got, []string{""}) {
		t.Errorf(`"\n" -> %q, want one empty line`, got)
	}
	if got := appendOutputLines(nil, "a\r\nb\n"); !equalLines(got, []string{"a", "b"}) {
		t.Errorf("multi-line -> %q, want [a b]", got)
	}
	if got := appendOutputLines(nil, "  indented"); !equalLines(got, []string{"  indented"}) {
		t.Errorf("partial line -> %q, want ['  indented']", got)
	}
}

// equalLines 比较两个字符串切片(长度 + 逐项), 空切片与 nil 视为相等
func equalLines(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// names 取一串测试结果的名字
func names(list []*TestResult) []string {
	out := make([]string, 0, len(list))
	for _, t := range list {
		out = append(out, t.Name)
	}
	return out
}
