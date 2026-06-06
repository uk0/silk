package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 一份覆盖块/单行use与replace的真实样本go.work
const sampleGoWork = `// top-level comment
go 1.22

use (
    ./mod-a
    ./mod-b   // trailing comment
    ./mod-c
)

use ./single-mod

replace example.com/foo => ./local/foo

replace (
    example.com/bar v1.2.3 => example.com/bar-fork v1.2.4
    example.com/baz => ../baz-local
)
`

func TestParseGoWork_Sample(t *testing.T) {
	gw, err := ParseGoWork(sampleGoWork)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gw.GoVersion != "1.22" {
		t.Errorf("go version = %q, want %q", gw.GoVersion, "1.22")
	}

	wantUses := []string{"./mod-a", "./mod-b", "./mod-c", "./single-mod"}
	if len(gw.Uses) != len(wantUses) {
		t.Fatalf("uses len = %d, want %d (%+v)", len(gw.Uses), len(wantUses), gw.Uses)
	}
	for i, w := range wantUses {
		if gw.Uses[i] != w {
			t.Errorf("uses[%d] = %q, want %q", i, gw.Uses[i], w)
		}
	}

	wantReps := []GoModReplace{
		{From: "example.com/foo", FromVer: "", To: "./local/foo", ToVer: ""},
		{From: "example.com/bar", FromVer: "v1.2.3", To: "example.com/bar-fork", ToVer: "v1.2.4"},
		{From: "example.com/baz", FromVer: "", To: "../baz-local", ToVer: ""},
	}
	if len(gw.Replaces) != len(wantReps) {
		t.Fatalf("replaces len = %d, want %d (%+v)", len(gw.Replaces), len(wantReps), gw.Replaces)
	}
	for i, w := range wantReps {
		got := gw.Replaces[i]
		if got != w {
			t.Errorf("replaces[%d] = %+v, want %+v", i, got, w)
		}
	}
}

func TestParseGoWork_Malformed(t *testing.T) {
	// use块里混入一行有两段路径(畸形), replace右侧版本号是垃圾数据
	bad := `go 1.22

use (
    ./ok-mod
    ./bad mod extra
)

replace example.com/x v1.0.0 => example.com/z bogus
`
	gw, err := ParseGoWork(bad)
	if err == nil {
		t.Fatal("expected error for malformed lines, got nil")
	}
	if gw == nil {
		t.Fatal("expected partial result, got nil")
	}
	if gw.GoVersion != "1.22" {
		t.Errorf("go version = %q", gw.GoVersion)
	}
	// 合法那条use应仍然被收录
	if len(gw.Uses) != 1 || gw.Uses[0] != "./ok-mod" {
		t.Errorf("expected 1 good use, got %+v", gw.Uses)
	}
	// 不合法的replace不应被收录
	for _, rep := range gw.Replaces {
		if rep.To == "example.com/z" && rep.ToVer == "bogus" {
			t.Errorf("malformed replace should not be accepted: %+v", rep)
		}
	}
}

func TestParseGoWork_CommentsAndBlanks(t *testing.T) {
	src := `// header comment
go 1.22   // trailing

// blank below

use   ./only-one   // trailing on use
`
	gw, err := ParseGoWork(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gw.GoVersion != "1.22" {
		t.Errorf("go version = %q", gw.GoVersion)
	}
	if len(gw.Uses) != 1 || gw.Uses[0] != "./only-one" {
		t.Errorf("uses = %+v", gw.Uses)
	}
}

func TestParseGoWork_IgnoresUnknownDirectives(t *testing.T) {
	// toolchain/godebug 等指令应被静默忽略, 不产生error
	src := `go 1.22

toolchain go1.22.1

use ./m

godebug default=go1.21
`
	gw, err := ParseGoWork(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gw.GoVersion != "1.22" {
		t.Errorf("go version = %q", gw.GoVersion)
	}
	if len(gw.Uses) != 1 || gw.Uses[0] != "./m" {
		t.Errorf("uses = %+v", gw.Uses)
	}
}

func TestFindGoWork(t *testing.T) {
	root := t.TempDir()
	// 在root放一个go.work, 在嵌套子目录里查找应该向上找到它
	workPath := filepath.Join(root, "go.work")
	if err := os.WriteFile(workPath, []byte("go 1.22\n\nuse ./m\n"), 0644); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(deep, 0755); err != nil {
		t.Fatal(err)
	}

	got, ok := FindGoWork(deep)
	if !ok {
		t.Fatal("FindGoWork: not found")
	}
	// 解析符号链接以应对/tmp -> /private/tmp 等情况
	gotResolved, _ := filepath.EvalSymlinks(got)
	wantResolved, _ := filepath.EvalSymlinks(workPath)
	if gotResolved != wantResolved {
		t.Errorf("FindGoWork path = %q, want %q", gotResolved, wantResolved)
	}

	// 文件系统根上方没有go.work的常见情形
	rootOfFS := string(filepath.Separator)
	if _, exists := FindGoWork(rootOfFS); !exists {
		// 期望: 文件系统根没有go.work, 返回false
	} else {
		t.Log("filesystem root unexpectedly contains go.work; skipping no-ancestor check")
	}
}

func TestLoadGoWork(t *testing.T) {
	root := t.TempDir()
	workPath := filepath.Join(root, "go.work")
	body := "go 1.22\n\nuse (\n    ./mod-a\n    ./mod-b\n)\n\nreplace example.com/foo => ./local/foo\n"
	if err := os.WriteFile(workPath, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(root, "x", "y")
	if err := os.MkdirAll(deep, 0755); err != nil {
		t.Fatal(err)
	}

	gw, err := LoadGoWork(deep)
	if err != nil {
		t.Fatalf("LoadGoWork: %v", err)
	}
	if gw.GoVersion != "1.22" {
		t.Errorf("go = %q", gw.GoVersion)
	}
	wantUses := []string{"./mod-a", "./mod-b"}
	if len(gw.Uses) != len(wantUses) {
		t.Fatalf("uses = %+v, want %+v", gw.Uses, wantUses)
	}
	for i, w := range wantUses {
		if gw.Uses[i] != w {
			t.Errorf("uses[%d] = %q, want %q", i, gw.Uses[i], w)
		}
	}
	if len(gw.Replaces) != 1 || gw.Replaces[0].From != "example.com/foo" || gw.Replaces[0].To != "./local/foo" {
		t.Errorf("replaces = %+v", gw.Replaces)
	}

	// 不存在go.work时, LoadGoWork应返回error
	isolated := t.TempDir()
	if p, ok := FindGoWork(isolated); ok {
		t.Logf("ancestor go.work found at %s; skipping not-found check", p)
		return
	}
	if _, err := LoadGoWork(isolated); err == nil {
		t.Error("LoadGoWork: expected error for missing go.work, got nil")
	} else if !strings.Contains(err.Error(), "not found") {
		t.Errorf("LoadGoWork error = %v; want contains \"not found\"", err)
	}
}
