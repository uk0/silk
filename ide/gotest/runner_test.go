package gotest

import (
	"strings"
	"testing"
	"time"

	"github.com/uk0/silk/core"
)

// failedStream: 两个包, m/a 里一个顶层测试失败 + 一个子测试失败(父测试因此也
// fail), m/b 全绿. 用来钉住 rerun-failed 生成的参数.
const failedStream = `{"Action":"run","Package":"m/a","Test":"TestOne"}
{"Action":"fail","Package":"m/a","Test":"TestOne","Elapsed":0.01}
{"Action":"run","Package":"m/a","Test":"TestTwo"}
{"Action":"pass","Package":"m/a","Test":"TestTwo","Elapsed":0.01}
{"Action":"run","Package":"m/a","Test":"TestThree/case a+b"}
{"Action":"fail","Package":"m/a","Test":"TestThree/case a+b","Elapsed":0.01}
{"Action":"fail","Package":"m/a","Test":"TestThree","Elapsed":0.02}
{"Action":"fail","Package":"m/a","Elapsed":0.05}
{"Action":"run","Package":"m/b","Test":"TestOne"}
{"Action":"pass","Package":"m/b","Test":"TestOne","Elapsed":0.01}
{"Action":"pass","Package":"m/b","Elapsed":0.03}
`

// buildFailStream: m/c 编译不过 —— 包 fail 但一个测试都没有.
const buildFailStream = `{"ImportPath":"m/c [m/c.test]","Action":"build-output","Output":"# m/c\n"}
{"ImportPath":"m/c [m/c.test]","Action":"build-fail"}
{"Action":"start","Package":"m/c"}
{"Action":"fail","Package":"m/c","Elapsed":0,"FailedBuild":"m/c [m/c.test]"}
`

func aggregate(t *testing.T, stream string) *core.Aggregator {
	t.Helper()
	agg := core.NewAggregator()
	if err := agg.Feed(strings.NewReader(stream), nil); err != nil {
		t.Fatalf("Feed: %v", err)
	}
	return agg
}

// TestArgsShapes: Args 是纯函数, 参数顺序和默认包固定下来.
func TestArgsShapes(t *testing.T) {
	cases := []struct {
		name string
		opts Options
		want []string
	}{
		{
			name: "default whole module",
			opts: Options{},
			want: []string{"test", "-json", "./..."},
		},
		{
			name: "package qualified single test",
			opts: Options{Packages: []string{"github.com/uk0/silk/ged"}, Run: "^TestFoo$", Count1: true},
			want: []string{"test", "-json", "-count=1", "-run", "^TestFoo$", "github.com/uk0/silk/ged"},
		},
		{
			name: "several packages, no filter",
			opts: Options{Packages: []string{"./core/", "./ged/"}},
			want: []string{"test", "-json", "./core/", "./ged/"},
		},
	}
	for _, c := range cases {
		if got := Args(c.opts); !equalArgs(got, c.want) {
			t.Errorf("%s: Args = %q, want %q", c.name, got, c.want)
		}
	}
}

// TestRunRegexpAnchoring: -run 的 pattern 按 "/" 分段匹配, 所以子测试路径必须
// 逐段锚定, 且每段的正则元字符要转义.
func TestRunRegexpAnchoring(t *testing.T) {
	cases := map[string]string{
		"":                   "",
		"TestFoo":            "^TestFoo$",
		"TestFoo/sub":        "^TestFoo$/^sub$",
		"TestFoo/a+b":        "^TestFoo$/^a\\+b$",
		"TestFoo/mid/leaf":   "^TestFoo$/^mid$/^leaf$",
		"TestParse(1.2)/x|y": "^TestParse\\(1\\.2\\)$/^x\\|y$",
	}
	for in, want := range cases {
		if got := RunRegexp(in); got != want {
			t.Errorf("RunRegexp(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestRerunTestPackageQualified: 单测试重跑必须带上它所在的包, 同名测试才不会
// 在 ./... 上撞车.
func TestRerunTestPackageQualified(t *testing.T) {
	o := RerunTest("github.com/uk0/silk/core", "TestParent/sub_bad")
	want := []string{
		"test", "-json", "-count=1",
		"-run", "^TestParent$/^sub_bad$",
		"github.com/uk0/silk/core",
	}
	if got := Args(o); !equalArgs(got, want) {
		t.Errorf("Args = %q, want %q", got, want)
	}
	if !o.Count1 {
		t.Error("Count1 = false: 没有 -count=1 时缓存会直接回放上一次结果")
	}
	// 没有包名时退回整模块 —— 明确记下这个退化行为
	if got := Args(RerunTest("", "TestX")); !equalArgs(got, []string{"test", "-json", "-count=1", "-run", "^TestX$", "./..."}) {
		t.Errorf("empty package Args = %q", got)
	}
}

// TestRerunFailedArgs: 每个有失败的包生成一条包内定向的重跑参数; 失败的子测试
// 收敛到它的顶层测试名.
func TestRerunFailedArgs(t *testing.T) {
	opts := RerunFailed(aggregate(t, failedStream))
	if len(opts) != 1 {
		t.Fatalf("options = %d %+v, want 1 (只有 m/a 有失败)", len(opts), opts)
	}
	if !equalArgs(opts[0].Packages, []string{"m/a"}) {
		t.Errorf("packages = %q, want [m/a]", opts[0].Packages)
	}
	if opts[0].Run != "^(TestOne|TestThree)$" {
		t.Errorf("run = %q, want ^(TestOne|TestThree)$", opts[0].Run)
	}
	want := []string{"test", "-json", "-count=1", "-run", "^(TestOne|TestThree)$", "m/a"}
	if got := Args(opts[0]); !equalArgs(got, want) {
		t.Errorf("Args = %q, want %q", got, want)
	}
	if got := RerunFailed(nil); got != nil {
		t.Errorf("RerunFailed(nil) = %v, want nil", got)
	}
	if got := RerunFailed(aggregate(t, `{"Action":"pass","Package":"m/b","Elapsed":0.01}`)); got != nil {
		t.Errorf("all-green RerunFailed = %v, want nil", got)
	}
}

// TestRerunFailedBuildFailureRerunsWholePackage: 编译失败的包没有任何失败测试,
// 只能整包重跑(不带 -run).
func TestRerunFailedBuildFailureRerunsWholePackage(t *testing.T) {
	agg := aggregate(t, buildFailStream)
	if len(agg.BuildOutput()) == 0 {
		t.Fatal("BuildOutput 为空: 编译诊断丢了")
	}
	opts := RerunFailed(agg)
	if len(opts) != 1 {
		t.Fatalf("options = %+v, want 1", opts)
	}
	if opts[0].Run != "" {
		t.Errorf("run = %q, want empty", opts[0].Run)
	}
	want := []string{"test", "-json", "-count=1", "m/c"}
	if got := Args(opts[0]); !equalArgs(got, want) {
		t.Errorf("Args = %q, want %q", got, want)
	}
}

// TestStreamDeliversUpdates: 流式回调逐行触发, 原始行原样带出, 非 JSON 行的
// Action 为空并落进 BuildOutput; 回调看到的始终是同一个聚合器, 且最后一行时
// 状态已经完整.
func TestStreamDeliversUpdates(t *testing.T) {
	src := "# m/a build noise\n" + failedStream
	agg := core.NewAggregator()
	var got []Update
	// 增量性只能在回调里当场看: Update.Agg 是活的聚合器而不是快照, 流跑完之后
	// 它已经是终态了.
	seenRunning := false
	if err := stream(agg, strings.NewReader(src), func(u Update) {
		got = append(got, u)
		if u.Event.Action == core.TestActionRun && u.Event.Package == "m/a" && u.Event.Test == "TestOne" {
			// run 事件折叠之后节点已经建出来, 但还没有终态
			seenRunning = u.Agg.Package("m/a").Test("TestOne").Status == core.TestStatusRunning
		}
	}); err != nil {
		t.Fatalf("stream: %v", err)
	}
	if !seenRunning {
		t.Error("run 事件的那次回调里 TestOne 不是 running: 增量状态没有及时可见")
	}

	lines := strings.Split(strings.TrimRight(src, "\n"), "\n")
	if len(got) != len(lines) {
		t.Fatalf("updates = %d, want %d (一行一条)", len(got), len(lines))
	}
	if got[0].Raw != "# m/a build noise" || got[0].Event.Action != "" {
		t.Errorf("update[0] = %q/%q, want the raw noise line with an empty action", got[0].Raw, got[0].Event.Action)
	}
	if got[1].Event.Action != core.TestActionRun || got[1].Event.Test != "TestOne" {
		t.Errorf("update[1] event = %+v, want run TestOne", got[1].Event)
	}
	for i, u := range got {
		if u.Agg != agg {
			t.Fatalf("update[%d].Agg is not the live aggregator", i)
		}
		if u.Raw != lines[i] {
			t.Errorf("update[%d].Raw = %q, want %q", i, u.Raw, lines[i])
		}
	}

	// 流跑完时全部状态就位
	passed, failed, skipped := agg.Counts()
	if passed != 2 || failed != 3 || skipped != 0 {
		t.Errorf("Counts = (%d,%d,%d), want (2,3,0)", passed, failed, skipped)
	}
	if got := agg.Package("m/a").Elapsed; got != 50*time.Millisecond {
		t.Errorf("m/a elapsed = %v, want 50ms", got)
	}
	if got := agg.BuildOutput(); len(got) != 1 || got[0] != "# m/a build noise" {
		t.Errorf("BuildOutput = %q", got)
	}
	// stream 的回调可以是 nil
	if err := stream(core.NewAggregator(), strings.NewReader(failedStream), nil); err != nil {
		t.Errorf("stream with nil callback: %v", err)
	}
}

// TestSplitLinesTrailingNewline 固定 stderr 切行的边界
func TestSplitLinesTrailingNewline(t *testing.T) {
	if got := splitLines(""); got != nil {
		t.Errorf("splitLines(\"\") = %q, want nil", got)
	}
	if got := splitLines("\n"); got != nil {
		t.Errorf("splitLines(\"\\n\") = %q, want nil", got)
	}
	if got := splitLines("a\nb\n"); !equalArgs(got, []string{"a", "b"}) {
		t.Errorf("splitLines = %q, want [a b]", got)
	}
}

// equalArgs 比较两个字符串切片
func equalArgs(got, want []string) bool {
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
