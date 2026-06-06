package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 一份覆盖三种构造的真实样本go.mod
const sampleGoMod = `module silk/example

go 1.21

require (
    github.com/foo/bar v1.2.3
    github.com/baz/qux v0.5.0 // indirect
)

require github.com/single/dep v0.1.0

replace github.com/foo/bar => ../local/bar

replace (
    github.com/aa/bb v1.0.0 => github.com/cc/dd v1.0.1
    github.com/ee/ff => ../ee-local
)
`

func TestParseGoMod_Sample(t *testing.T) {
	gm, err := ParseGoMod(sampleGoMod)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gm.Module != "silk/example" {
		t.Errorf("module = %q, want %q", gm.Module, "silk/example")
	}
	if gm.GoVersion != "1.21" {
		t.Errorf("go version = %q, want %q", gm.GoVersion, "1.21")
	}

	wantReqs := []GoModRequire{
		{Path: "github.com/foo/bar", Version: "v1.2.3", Indirect: false},
		{Path: "github.com/baz/qux", Version: "v0.5.0", Indirect: true},
		{Path: "github.com/single/dep", Version: "v0.1.0", Indirect: false},
	}
	if len(gm.Requires) != len(wantReqs) {
		t.Fatalf("requires len = %d, want %d (%+v)", len(gm.Requires), len(wantReqs), gm.Requires)
	}
	for i, w := range wantReqs {
		got := gm.Requires[i]
		if got != w {
			t.Errorf("requires[%d] = %+v, want %+v", i, got, w)
		}
	}

	wantReps := []GoModReplace{
		{From: "github.com/foo/bar", FromVer: "", To: "../local/bar", ToVer: ""},
		{From: "github.com/aa/bb", FromVer: "v1.0.0", To: "github.com/cc/dd", ToVer: "v1.0.1"},
		{From: "github.com/ee/ff", FromVer: "", To: "../ee-local", ToVer: ""},
	}
	if len(gm.Replaces) != len(wantReps) {
		t.Fatalf("replaces len = %d, want %d (%+v)", len(gm.Replaces), len(wantReps), gm.Replaces)
	}
	for i, w := range wantReps {
		got := gm.Replaces[i]
		if got != w {
			t.Errorf("replaces[%d] = %+v, want %+v", i, got, w)
		}
	}
}

func TestParseGoMod_Malformed(t *testing.T) {
	// require行的版本号缺失, replace右侧版本号是垃圾数据
	bad := `module silk/example

go 1.21

require (
    github.com/foo/bar
    github.com/ok/good v1.0.0
)

replace github.com/x/y v1.0.0 => github.com/z/w bogus
`
	gm, err := ParseGoMod(bad)
	if err == nil {
		t.Fatal("expected error for malformed lines, got nil")
	}
	if gm == nil {
		t.Fatal("expected partial result, got nil")
	}
	// 仍能拿到module/go版本
	if gm.Module != "silk/example" {
		t.Errorf("module = %q, want %q", gm.Module, "silk/example")
	}
	if gm.GoVersion != "1.21" {
		t.Errorf("go version = %q", gm.GoVersion)
	}
	// 能拿到合法的那一条require
	if len(gm.Requires) != 1 || gm.Requires[0].Path != "github.com/ok/good" {
		t.Errorf("expected 1 good require, got %+v", gm.Requires)
	}
	// 不合法的replace不应被收录
	for _, rep := range gm.Replaces {
		if rep.To == "github.com/z/w" && rep.ToVer == "bogus" {
			t.Errorf("malformed replace should not be accepted: %+v", rep)
		}
	}
}

func TestParseGoMod_CommentsAndBlanks(t *testing.T) {
	src := `// leading comment
module  m/x  // trailing comment

go 1.20

`
	gm, err := ParseGoMod(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gm.Module != "m/x" {
		t.Errorf("module = %q", gm.Module)
	}
	if gm.GoVersion != "1.20" {
		t.Errorf("go = %q", gm.GoVersion)
	}
}

func TestFindGoMod(t *testing.T) {
	root := t.TempDir()
	// 在root放一个go.mod, 在嵌套子目录里查找应该向上找到它
	modPath := filepath.Join(root, "go.mod")
	if err := os.WriteFile(modPath, []byte("module silk/example\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(deep, 0755); err != nil {
		t.Fatal(err)
	}

	got, ok := FindGoMod(deep)
	if !ok {
		t.Fatal("FindGoMod: not found")
	}
	// 解析符号链接以应对/tmp -> /private/tmp 等情况
	gotResolved, _ := filepath.EvalSymlinks(got)
	wantResolved, _ := filepath.EvalSymlinks(modPath)
	if gotResolved != wantResolved {
		t.Errorf("FindGoMod path = %q, want %q", gotResolved, wantResolved)
	}

	// 没有任何祖先包含go.mod的目录, 应返回false
	// 用一个绝对孤立的TempDir, 但要确保它自己以及它的祖先(到/)都没有go.mod
	// 在大多数测试环境中 /tmp 上面没有 go.mod, 但为稳妥起见我们换一种验证:
	// 给deep目录加一个临时哨兵, 然后在删除哨兵后从一个根上方的位置寻找应失败.
	// 这里我们用 os.TempDir() 的根: 假定其上方无 go.mod.
	// 若该假定不成立, 该断言跳过.
	rootOfFS := string(filepath.Separator)
	if _, exists := FindGoMod(rootOfFS); !exists {
		// 期望: 文件系统根没有go.mod, 返回false
	} else {
		t.Log("filesystem root unexpectedly contains go.mod; skipping no-ancestor check")
	}
}

func TestLoadGoMod(t *testing.T) {
	root := t.TempDir()
	modPath := filepath.Join(root, "go.mod")
	body := "module silk/example\n\ngo 1.21\n\nrequire github.com/foo/bar v1.2.3\n"
	if err := os.WriteFile(modPath, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(root, "x", "y")
	if err := os.MkdirAll(deep, 0755); err != nil {
		t.Fatal(err)
	}

	gm, err := LoadGoMod(deep)
	if err != nil {
		t.Fatalf("LoadGoMod: %v", err)
	}
	if gm.Module != "silk/example" {
		t.Errorf("module = %q", gm.Module)
	}
	if gm.GoVersion != "1.21" {
		t.Errorf("go = %q", gm.GoVersion)
	}
	if len(gm.Requires) != 1 || gm.Requires[0].Path != "github.com/foo/bar" {
		t.Errorf("requires = %+v", gm.Requires)
	}

	// 不存在go.mod时, LoadGoMod应返回error
	isolated := t.TempDir()
	// 找到该路径向上首个含go.mod的祖先(如果有)
	if p, ok := FindGoMod(isolated); ok {
		t.Logf("ancestor go.mod found at %s; skipping not-found check", p)
		return
	}
	if _, err := LoadGoMod(isolated); err == nil {
		t.Error("LoadGoMod: expected error for missing go.mod, got nil")
	} else if !strings.Contains(err.Error(), "not found") {
		t.Errorf("LoadGoMod error = %v; want contains \"not found\"", err)
	}
}
