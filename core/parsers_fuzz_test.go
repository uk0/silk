package core

// 集中放置 core 的五个公开 parser 的 fuzz 测试以及配套的 seed-corpus 回归测试.
//
// 每个 parser 都遵循同一份合同:
//   - 任何输入都 *不可* panic
//   - 出错时仍可能返回非 nil 的部分结果 (skip-and-collect)
//   - 出错时 error 也非 nil
// 这里围绕这份合同写五个对应的 Fuzz 目标. 同时为每个 fuzz target 提供一个
// TestXxxSeedsDoNotPanic, 让 seed corpus 在常规 `go test` 下也能跑到, 避免必须
// 带 -fuzz 才能起防回归作用.
//
// Go 1.18+ 的 testing.F 在没有 -fuzz 时会把 f.Add 注册的种子当作普通子用例执行,
// 但为了在不依赖该行为的前提下也明确捕获崩溃, 这里再写一遍 seed runner.

import (
	"bufio"
	"fmt"
	"strings"
	"testing"
)

// ---------- ParseGoMod ----------

// parseGoModSeeds 给 fuzz 和 seed runner 共用的种子语料.
// 覆盖: 空输入 / 最小合法 / require / 完全乱码 / block / replace.
var parseGoModSeeds = []string{
	"",
	"module silk\n\ngo 1.21\n",
	"module silk\n\ngo 1.21\n\nrequire foo v1.0.0\n",
	"garbage\nnot valid\nmodule x",
	"require (\n  foo v1\n)\n",
	"module silk\nreplace foo => ../local\n",
	// 行尾 indirect 注释, 触发 indirect 标记路径
	"require github.com/baz/qux v0.5.0 // indirect\n",
	// 异形输入: 只有右括号 / 只有左括号 / 仅含注释
	")\n",
	"require (\n",
	"// only a comment\n",
}

func runParseGoMod(t *testing.T, src string) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ParseGoMod panicked on %q: %v", src, r)
		}
	}()
	m, err := ParseGoMod(src)
	// 合同: 函数永远返回 *GoMod (即使出错也是部分结果).
	if m == nil {
		t.Fatalf("ParseGoMod returned nil *GoMod for %q (err=%v); contract says result is always non-nil", src, err)
	}
	// 顺手访问字段, 确保切片元素可用而不是 nil 指针.
	for _, r := range m.Requires {
		_ = r.Path
		_ = r.Version
	}
	for _, r := range m.Replaces {
		_ = r.From
		_ = r.To
	}
}

func FuzzParseGoMod(f *testing.F) {
	for _, s := range parseGoModSeeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		runParseGoMod(t, src)
	})
}

func TestParseGoModSeedsDoNotPanic(t *testing.T) {
	for i, s := range parseGoModSeeds {
		t.Run(fmt.Sprintf("seed%d", i), func(t *testing.T) {
			runParseGoMod(t, s)
		})
	}
}

// ---------- ParseGoWork ----------

var parseGoWorkSeeds = []string{
	"",
	"go 1.21\n",
	"go 1.21\nuse ./a\n",
	"use (\n  ./a\n  ./b\n)\n",
	"replace foo => ../local\n",
	"garbage\nuse\n)\n",
	// 多余的 use 行 / 空 use
	"use\n",
	"use ./mod with space // comment\n",
	"// just a comment\n)\n",
}

func runParseGoWork(t *testing.T, src string) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ParseGoWork panicked on %q: %v", src, r)
		}
	}()
	w, err := ParseGoWork(src)
	if w == nil {
		t.Fatalf("ParseGoWork returned nil *GoWork for %q (err=%v); contract says result is always non-nil", src, err)
	}
	for _, u := range w.Uses {
		_ = u
	}
	for _, r := range w.Replaces {
		_ = r.From
		_ = r.To
	}
}

func FuzzParseGoWork(f *testing.F) {
	for _, s := range parseGoWorkSeeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		runParseGoWork(t, src)
	})
}

func TestParseGoWorkSeedsDoNotPanic(t *testing.T) {
	for i, s := range parseGoWorkSeeds {
		t.Run(fmt.Sprintf("seed%d", i), func(t *testing.T) {
			runParseGoWork(t, s)
		})
	}
}

// ---------- ParseGoListJSON ----------

// `go list -json ./...` 输出是若干 pretty-printed 对象拼接, 没有数组包装.
// 种子覆盖: 空 / 单对象 / 多对象拼接 / 截断 / 完全乱码 / 非对象.
var parseGoListJSONSeeds = []string{
	"",
	`{"ImportPath":"silk","Name":"silk","Dir":"/tmp","GoFiles":["a.go"]}`,
	`{"ImportPath":"silk","Module":{"Path":"silk","Main":true,"Dir":"/tmp"}}`,
	// 两个对象流式拼接
	`{"ImportPath":"a"}` + "\n" + `{"ImportPath":"b"}`,
	// 截断 JSON
	`{"ImportPath":"silk","Name":"si`,
	// 完全乱码
	"not json at all\n}}}",
	// 数组形式 (go list 不会输出, 但 fuzz 应优雅处理)
	`[{"ImportPath":"a"}]`,
	// 仅空白
	"   \t\n   ",
	// 嵌套 Module 但 Module 为 null
	`{"ImportPath":"x","Module":null}`,
}

func runParseGoListJSON(t *testing.T, src string) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ParseGoListJSON panicked on %q: %v", src, r)
		}
	}()
	pkgs, _ := ParseGoListJSON(src)
	// pkgs 允许为 nil (空输入或解析失败前已退出循环), 重点是不 panic.
	for _, p := range pkgs {
		_ = p.ImportPath
		_ = p.Name
		_ = p.Dir
		if p.Module != nil {
			_ = p.Module.Path
		}
	}
}

func FuzzParseGoListJSON(f *testing.F) {
	for _, s := range parseGoListJSONSeeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		runParseGoListJSON(t, src)
	})
}

func TestParseGoListJSONSeedsDoNotPanic(t *testing.T) {
	for i, s := range parseGoListJSONSeeds {
		t.Run(fmt.Sprintf("seed%d", i), func(t *testing.T) {
			runParseGoListJSON(t, s)
		})
	}
}

// ---------- ParseCoverage ----------

var parseCoverageSeeds = []string{
	"",
	"mode: set\n",
	"mode: count\nsilk/ged/foo.go:10.13,15.2 3 1\nsilk/ged/foo.go:17.2,17.10 1 0\n",
	// 缺 header (容忍, 视作 set 模式)
	"silk/ged/foo.go:10.13,15.2 3 1\n",
	// 完全乱码
	"hello world\n",
	// 缺 count 字段
	"github.com/uk0/silk/x.go:1.1,2.2 1\n",
	// 数字字段坏掉
	"github.com/uk0/silk/x.go:1.1,2.2 abc 1\n",
	"github.com/uk0/silk/x.go:1.1,2.2 1 def\n",
	// 行号倒置 / 负数
	"github.com/uk0/silk/x.go:5.1,2.2 1 1\n",
	// Windows 风格盘符路径 (头部含 ':')
	`C:\src\foo.go:1.1,2.2 1 1` + "\n",
	// 缺 ',' / 缺 '.'
	"github.com/uk0/silk/x.go:1.1 2.2 1 1\n",
	"github.com/uk0/silk/x.go:11,22 1 1\n",
}

func runParseCoverage(t *testing.T, src string) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ParseCoverage panicked on %q: %v", src, r)
		}
	}()
	mode, blocks, _ := ParseCoverage(src)
	// mode 必然非空字符串 (parser 给的默认是 "set")
	if mode == "" {
		t.Fatalf("ParseCoverage returned empty mode for %q; contract says default is %q", src, "set")
	}
	for _, b := range blocks {
		_ = b.File
		_ = b.StartLine
		_ = b.EndLine
	}
}

func FuzzParseCoverage(f *testing.F) {
	for _, s := range parseCoverageSeeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		runParseCoverage(t, src)
	})
}

func TestParseCoverageSeedsDoNotPanic(t *testing.T) {
	for i, s := range parseCoverageSeeds {
		t.Run(fmt.Sprintf("seed%d", i), func(t *testing.T) {
			runParseCoverage(t, s)
		})
	}
}

// ---------- ReadLSPMessage ----------

// 构造一个合法的 framing 串供 seed 使用.
func lspFrame(body string) string {
	return fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)
}

var readLSPMessageSeeds = []string{
	// 完整合法消息
	lspFrame(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`),
	// notification (无 id)
	lspFrame(`{"jsonrpc":"2.0","method":"didChange","params":{}}`),
	// 空 body, Content-Length: 0
	"Content-Length: 0\r\n\r\n",
	// Content-Length 是负数
	"Content-Length: -1\r\n\r\n",
	// Content-Length 非数字
	"Content-Length: abc\r\n\r\n",
	// 完全缺失 Content-Length
	"Content-Type: application/json\r\n\r\n{}",
	// 头部行没有 ':'
	"Bogus header\r\n\r\n{}",
	// 行分隔符不是 \r\n
	"Content-Length: 2\n\n{}",
	// 声明 100 字节但 body 只有 2 字节
	"Content-Length: 100\r\n\r\n{}",
	// 非 JSON body
	lspFrame("not json"),
	// 空输入
	"",
	// 仅 header 分隔符
	"\r\n",
}

func runReadLSPMessage(t *testing.T, src string) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ReadLSPMessage panicked on %q: %v", src, r)
		}
	}()
	r := bufio.NewReader(strings.NewReader(src))
	m, err := ReadLSPMessage(r)
	// 合同: 出错时 m 必为 nil; 成功时 m 非 nil 且 err 为 nil.
	if err != nil && m != nil {
		t.Fatalf("ReadLSPMessage returned non-nil message AND non-nil error for %q: m=%+v err=%v", src, m, err)
	}
	if err == nil && m == nil {
		t.Fatalf("ReadLSPMessage returned nil message AND nil error for %q", src)
	}
}

func FuzzReadLSPMessage(f *testing.F) {
	for _, s := range readLSPMessageSeeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		runReadLSPMessage(t, src)
	})
}

func TestReadLSPMessageSeedsDoNotPanic(t *testing.T) {
	for i, s := range readLSPMessageSeeds {
		t.Run(fmt.Sprintf("seed%d", i), func(t *testing.T) {
			runReadLSPMessage(t, s)
		})
	}
}
