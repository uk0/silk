package core

import (
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

// TestParseGoListJSON_StderrPollutionBreaksIt pins WHY RunGoList must keep
// stdout and stderr apart. It used to call CombinedOutput, so on a machine that
// still had to fetch dependencies `go list` prefixed the stream with
// "go: downloading ..." and the decoder failed on the first character:
//
//	go list parse: invalid character 'g' looking for beginning of value
//
// Worse, the loop aborts on the first error, so one harmless progress line
// dropped EVERY package. Windows CI hit exactly this on a cold module cache.
func TestParseGoListJSON_StderrPollutionBreaksIt(t *testing.T) {
	clean := `{"Dir":"/x","ImportPath":"example.com/x","Name":"x"}`

	pkgs, err := ParseGoListJSON(clean)
	if err != nil || len(pkgs) != 1 {
		t.Fatalf("clean stdout should parse: pkgs=%d err=%v", len(pkgs), err)
	}

	polluted := "go: downloading github.com/foo/bar v1.2.3\n" + clean
	pkgs, err = ParseGoListJSON(polluted)
	if err == nil {
		t.Fatal("stderr mixed into stdout should fail to parse; " +
			"if this ever passes, the guarantee below is no longer needed")
	}
	if len(pkgs) != 0 {
		t.Errorf("got %d packages from polluted input, want 0 — "+
			"the parser aborts on the first error, which is why separation matters", len(pkgs))
	}
	if !strings.Contains(err.Error(), "invalid character 'g'") {
		t.Errorf("unexpected error, want the 'g' from \"go: downloading\": %v", err)
	}
}

// 一份手写的多包样本, 模拟 `go list -json ./...` 的真实输出形态:
// - 顶层不是数组, 而是多个 pretty-printed 对象首尾相接
// - 第一个对象带 Module 字段
// - 第二个对象带 TestGoFiles / XTestGoFiles
// - 第三个对象既没有测试文件也没有 Module(模拟 stdlib 或 GOPATH 包)
const sampleGoListJSON = `{
    "Dir": "/Users/dev/silk/core",
    "ImportPath": "github.com/uk0/silk/core",
    "Name": "core",
    "GoFiles": [
        "core.go",
        "doc.go"
    ],
    "Module": {
        "Path": "silk",
        "Main": true,
        "Dir": "/Users/dev/silk"
    }
}
{
    "Dir": "/Users/dev/silk/gui",
    "ImportPath": "github.com/uk0/silk/gui",
    "Name": "gui",
    "GoFiles": [
        "accordion.go",
        "action.go"
    ],
    "TestGoFiles": [
        "accordion_test.go"
    ],
    "XTestGoFiles": [
        "gui_external_test.go"
    ],
    "Module": {
        "Path": "silk",
        "Main": true,
        "Dir": "/Users/dev/silk"
    }
}
{
    "Dir": "/usr/local/go/src/fmt",
    "ImportPath": "fmt",
    "Name": "fmt",
    "GoFiles": [
        "doc.go",
        "format.go"
    ]
}
`

func TestParseGoListJSON_Sample(t *testing.T) {
	pkgs, err := ParseGoListJSON(sampleGoListJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pkgs) != 3 {
		t.Fatalf("packages len = %d, want 3 (%+v)", len(pkgs), pkgs)
	}

	// pkg 0: silk/core, 有 Module, 没有测试文件
	p0 := pkgs[0]
	if p0.ImportPath != "github.com/uk0/silk/core" {
		t.Errorf("pkg0 ImportPath = %q, want %q", p0.ImportPath, "github.com/uk0/silk/core")
	}
	if p0.Name != "core" {
		t.Errorf("pkg0 Name = %q, want %q", p0.Name, "core")
	}
	if p0.Dir != "/Users/dev/silk/core" {
		t.Errorf("pkg0 Dir = %q, want %q", p0.Dir, "/Users/dev/silk/core")
	}
	wantGo0 := []string{"core.go", "doc.go"}
	if !reflect.DeepEqual(p0.GoFiles, wantGo0) {
		t.Errorf("pkg0 GoFiles = %v, want %v", p0.GoFiles, wantGo0)
	}
	if len(p0.TestGoFiles) != 0 {
		t.Errorf("pkg0 TestGoFiles = %v, want empty", p0.TestGoFiles)
	}
	if p0.Module == nil {
		t.Fatalf("pkg0 Module is nil, want populated")
	}
	wantMod := GoListModule{Path: "silk", Main: true, Dir: "/Users/dev/silk"}
	if *p0.Module != wantMod {
		t.Errorf("pkg0 Module = %+v, want %+v", *p0.Module, wantMod)
	}

	// pkg 1: silk/gui, 有 TestGoFiles 和 XTestGoFiles
	p1 := pkgs[1]
	if p1.ImportPath != "github.com/uk0/silk/gui" {
		t.Errorf("pkg1 ImportPath = %q, want %q", p1.ImportPath, "github.com/uk0/silk/gui")
	}
	wantTest1 := []string{"accordion_test.go"}
	if !reflect.DeepEqual(p1.TestGoFiles, wantTest1) {
		t.Errorf("pkg1 TestGoFiles = %v, want %v", p1.TestGoFiles, wantTest1)
	}
	wantXTest1 := []string{"gui_external_test.go"}
	if !reflect.DeepEqual(p1.XTestGoFiles, wantXTest1) {
		t.Errorf("pkg1 XTestGoFiles = %v, want %v", p1.XTestGoFiles, wantXTest1)
	}

	// pkg 2: fmt, 没有 Module 字段, 指针应为 nil
	p2 := pkgs[2]
	if p2.ImportPath != "fmt" {
		t.Errorf("pkg2 ImportPath = %q, want %q", p2.ImportPath, "fmt")
	}
	if p2.Module != nil {
		t.Errorf("pkg2 Module = %+v, want nil", p2.Module)
	}
	if len(p2.TestGoFiles) != 0 || len(p2.XTestGoFiles) != 0 {
		t.Errorf("pkg2 should have no test files, got TestGoFiles=%v XTestGoFiles=%v", p2.TestGoFiles, p2.XTestGoFiles)
	}
}

func TestParseGoListJSON_Empty(t *testing.T) {
	pkgs, err := ParseGoListJSON("")
	if err != nil {
		t.Fatalf("empty input: unexpected error: %v", err)
	}
	if len(pkgs) != 0 {
		t.Errorf("empty input: pkgs len = %d, want 0", len(pkgs))
	}

	// 仅有空白也应被当作空输入
	pkgs, err = ParseGoListJSON("   \n\t\n  ")
	if err != nil {
		t.Fatalf("whitespace input: unexpected error: %v", err)
	}
	if len(pkgs) != 0 {
		t.Errorf("whitespace input: pkgs len = %d, want 0", len(pkgs))
	}
}

func TestParseGoListJSON_Malformed(t *testing.T) {
	// 第一个对象完整, 第二个对象缺了收尾的 "}" -> 应拿到 pkg[0] 并报错
	malformed := `{
    "ImportPath": "github.com/uk0/silk/core",
    "Name": "core",
    "GoFiles": ["core.go"]
}
{
    "ImportPath": "github.com/uk0/silk/gui",
    "Name": "gui",
    "GoFiles": ["accordion.go"
`
	pkgs, err := ParseGoListJSON(malformed)
	if err == nil {
		t.Fatalf("expected non-nil error for malformed input, got nil")
	}
	if len(pkgs) < 1 {
		t.Fatalf("expected at least 1 valid package before the broken one, got %d", len(pkgs))
	}
	if pkgs[0].ImportPath != "github.com/uk0/silk/core" {
		t.Errorf("pkgs[0] ImportPath = %q, want %q", pkgs[0].ImportPath, "github.com/uk0/silk/core")
	}
}

// 烟雾测试: 若环境里有 go, 跑一次真实的 `go list -json ./...` 并断言有 package
// 出现. 没有 go 时跳过 (例如沙箱 CI 镜像没装 Go).
func TestRunGoList_Smoke(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH, skipping smoke test")
	}
	out, diag, err := RunGoList(".", "./...")
	if err != nil {
		t.Fatalf("RunGoList failed: %v\nstdout:\n%s\nstderr:\n%s", err, out, diag)
	}
	// stdout must be pure JSON: nothing from stderr may leak into it, or the
	// decoder trips over the first "go: downloading ..." line.
	if trimmed := strings.TrimSpace(out); trimmed != "" && !strings.HasPrefix(trimmed, "{") {
		t.Fatalf("stdout does not start with a JSON object — stderr leaked in?\ngot: %.120s", trimmed)
	}
	pkgs, err := ParseGoListJSON(out)
	if err != nil {
		t.Fatalf("ParseGoListJSON failed: %v", err)
	}
	if len(pkgs) == 0 {
		t.Fatalf("expected at least 1 package from `go list -json ./...`, got 0")
	}
}
