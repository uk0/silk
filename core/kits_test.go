package core

import (
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// validKit 是一份能通过 Validate 的基准Kit, 各用例在它上面改一个字段
func validKit() Kit {
	return Kit{
		Name:      "linux-server",
		GoExe:     "go",
		GoVersion: "1.25.0",
		GOOS:      "linux",
		GOARCH:    "amd64",
		Tags:      []string{"prod", "netgo"},
		Env:       map[string]string{"CGO_ENABLED": "0"},
		OutputDir: "build/release",
		Deploy:    DeployProfile{Kind: DeploySSH, Host: "10.0.0.9", User: "deploy", RemoteDir: "/opt/app"},
	}
}

func TestKitValidate_OK(t *testing.T) {
	if err := validKit().Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	// 本机部署: Kind 留空也应合法
	k := validKit()
	k.Deploy = DeployProfile{}
	if err := k.Validate(); err != nil {
		t.Fatalf("Validate empty deploy kind: %v", err)
	}
	k.Deploy = DeployProfile{Kind: DeployLocal}
	if err := k.Validate(); err != nil {
		t.Fatalf("Validate local deploy: %v", err)
	}
}

func TestKitValidate_Rejects(t *testing.T) {
	cases := []struct {
		name  string
		mut   func(*Kit)
		wants string // 错误信息应包含的片段
	}{
		{"empty name", func(k *Kit) { k.Name = "  " }, "name is empty"},
		{"empty goos", func(k *Kit) { k.GOOS = "" }, "GOOS is empty"},
		{"empty goarch", func(k *Kit) { k.GOARCH = "" }, "GOARCH is empty"},
		{"bad build mode", func(k *Kit) { k.BuildMode = "turbo" }, "unknown build mode"},
		{"tag with space", func(k *Kit) { k.Tags = []string{"a b"} }, "malformed build tag"},
		{"tag with comma", func(k *Kit) { k.Tags = []string{"a,b"} }, "malformed build tag"},
		{"empty tag", func(k *Kit) { k.Tags = []string{""} }, "malformed build tag"},
		{"env key with =", func(k *Kit) { k.Env = map[string]string{"A=B": "1"} }, "malformed env key"},
		{"empty env key", func(k *Kit) { k.Env = map[string]string{"": "1"} }, "malformed env key"},
		{"ssh without host", func(k *Kit) { k.Deploy.Host = "" }, "needs a host"},
		{"ssh without remote dir", func(k *Kit) { k.Deploy.RemoteDir = "" }, "needs a remote dir"},
		{"unknown deploy kind", func(k *Kit) { k.Deploy.Kind = "ftp" }, "unknown deploy kind"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			k := validKit()
			c.mut(&k)
			err := k.Validate()
			if err == nil {
				t.Fatalf("Validate() = nil, want error containing %q", c.wants)
			}
			if !strings.Contains(err.Error(), c.wants) {
				t.Fatalf("Validate() = %v, want error containing %q", err, c.wants)
			}
		})
	}
}

func TestKitValidate_BuildModes(t *testing.T) {
	for _, mode := range []string{"", "default", "exe", "pie", "plugin", "c-shared", "c-archive", "archive", "shared"} {
		k := validKit()
		k.BuildMode = mode
		if err := k.Validate(); err != nil {
			t.Errorf("build mode %q rejected: %v", mode, err)
		}
	}
}

func TestDefaultKit(t *testing.T) {
	k := DefaultKit()
	if k.GOOS != runtime.GOOS || k.GOARCH != runtime.GOARCH {
		t.Errorf("DefaultKit target = %s/%s, want %s/%s", k.GOOS, k.GOARCH, runtime.GOOS, runtime.GOARCH)
	}
	if k.GoExe != "go" {
		t.Errorf("GoExe = %q, want %q", k.GoExe, "go")
	}
	if k.OutputDir != "." {
		t.Errorf("OutputDir = %q, want %q", k.OutputDir, ".")
	}
	if k.Deploy.Kind != DeployLocal {
		t.Errorf("Deploy.Kind = %q, want %q", k.Deploy.Kind, DeployLocal)
	}
	// runtime.Version() 形如 "go1.25.0"; Kit里存的是剥掉前缀的版本号
	if strings.HasPrefix(k.GoVersion, "go") {
		t.Errorf("GoVersion = %q, want the \"go\" prefix stripped", k.GoVersion)
	}
	if err := k.Validate(); err != nil {
		t.Errorf("DefaultKit does not validate: %v", err)
	}
}

// TestDetectKits 需要PATH上有go; 没有就跳过, 不让缺工具链的环境失败
func TestDetectKits(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH; skipping toolchain probe")
	}
	ks, err := DetectKits()
	if err != nil {
		t.Fatalf("DetectKits: %v", err)
	}
	if len(ks) == 0 {
		t.Fatal("DetectKits returned no kits")
	}
	k := ks[0]
	if k.GOOS == "" || k.GOARCH == "" {
		t.Errorf("detected kit target = %s/%s, want both set", k.GOOS, k.GOARCH)
	}
	if k.GoVersion == "" || strings.HasPrefix(k.GoVersion, "go") {
		t.Errorf("detected GoVersion = %q, want a bare version", k.GoVersion)
	}
	if !filepath.IsAbs(k.GoExe) {
		t.Errorf("GoExe = %q, want the absolute path LookPath resolved", k.GoExe)
	}
	if !strings.Contains(k.Name, k.GOOS) || !strings.Contains(k.Name, k.GOARCH) {
		t.Errorf("Name = %q, want it to mention %s/%s", k.Name, k.GOOS, k.GOARCH)
	}
	if err := k.Validate(); err != nil {
		t.Errorf("detected kit does not validate: %v", err)
	}
}

func TestParseGoEnvValues(t *testing.T) {
	got := parseGoEnvValues("darwin\narm64\ngo1.25.0\n", 3)
	want := []string{"darwin", "arm64", "go1.25.0"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseGoEnvValues = %#v, want %#v", got, want)
	}
	// CRLF 也要吃掉
	got = parseGoEnvValues("windows\r\namd64\r\ngo1.24.1\r\n", 3)
	want = []string{"windows", "amd64", "go1.24.1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseGoEnvValues(crlf) = %#v, want %#v", got, want)
	}
	// 行数不足时补空, 长度恒为n
	got = parseGoEnvValues("linux\n", 3)
	want = []string{"linux", "", ""}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseGoEnvValues(short) = %#v, want %#v", got, want)
	}
	if got := parseGoEnvValues("", 0); len(got) != 0 {
		t.Fatalf("parseGoEnvValues(n=0) = %#v, want empty", got)
	}
}

func TestParseBuildTags(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		{"prod", []string{"prod"}},
		{"prod,netgo", []string{"prod", "netgo"}},
		{" prod , netgo ,, ", []string{"prod", "netgo"}},
		{"prod netgo", []string{"prod", "netgo"}},
		{"prod\tnetgo\nosusergo", []string{"prod", "netgo", "osusergo"}},
	}
	for _, c := range cases {
		if got := ParseBuildTags(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("ParseBuildTags(%q) = %#v, want %#v", c.in, got, c.want)
		}
	}
	if got := FormatBuildTags([]string{"prod", "netgo"}); got != "prod,netgo" {
		t.Errorf("FormatBuildTags = %q, want %q", got, "prod,netgo")
	}
	if got := FormatBuildTags(nil); got != "" {
		t.Errorf("FormatBuildTags(nil) = %q, want empty", got)
	}
	// round-trip
	if got := ParseBuildTags(FormatBuildTags([]string{"a", "b"})); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("round-trip = %#v", got)
	}
}

func TestKitClone_Independent(t *testing.T) {
	k := validKit()
	c := k.Clone()
	c.Tags[0] = "mutated"
	c.Env["CGO_ENABLED"] = "1"
	c.Env["EXTRA"] = "1"

	if k.Tags[0] != "prod" {
		t.Errorf("original Tags mutated: %#v", k.Tags)
	}
	if k.Env["CGO_ENABLED"] != "0" {
		t.Errorf("original Env mutated: %#v", k.Env)
	}
	if _, ok := k.Env["EXTRA"]; ok {
		t.Errorf("original Env gained a key: %#v", k.Env)
	}
	// 没有 Tags/Env 的Kit克隆后也不该无端多出容器
	empty := Kit{Name: "x"}.Clone()
	if empty.Tags != nil || empty.Env != nil {
		t.Errorf("Clone of empty kit = %#v, want nil containers", empty)
	}
}

func TestKitBuildArgs(t *testing.T) {
	k := Kit{Name: "k", GOOS: "linux", GOARCH: "amd64"}
	if got := k.BuildArgs(); len(got) != 0 {
		t.Errorf("BuildArgs of plain kit = %#v, want empty", got)
	}

	k.Tags = []string{"prod", "netgo"}
	k.Race = true
	k.Coverage = true
	k.BuildMode = "pie"
	got := k.BuildArgs()
	want := []string{"-tags", "prod,netgo", "-race", "-cover", "-buildmode=pie"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildArgs = %#v\nwant %#v", got, want)
	}

	// "default" 与 "" 一样, 不该出现在命令行上
	k2 := Kit{Name: "k", GOOS: "linux", GOARCH: "amd64", BuildMode: "default"}
	if got := k2.BuildArgs(); len(got) != 0 {
		t.Errorf("BuildArgs with default build mode = %#v, want empty", got)
	}
}

func TestKitBuildEnv(t *testing.T) {
	k := Kit{
		Name: "k", GOOS: "linux", GOARCH: "arm64",
		Env: map[string]string{"CC": "clang", "CGO_ENABLED": "1"},
	}
	got := k.BuildEnv()
	want := []string{"GOOS=linux", "GOARCH=arm64", "CC=clang", "CGO_ENABLED=1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildEnv = %#v\nwant %#v", got, want)
	}
	// 自定义变量按key排序 => 同一个Kit两次输出必须一致
	if !reflect.DeepEqual(k.BuildEnv(), got) {
		t.Error("BuildEnv is not deterministic")
	}
	if got := (Kit{Name: "k"}).BuildEnv(); len(got) != 0 {
		t.Errorf("BuildEnv without target = %#v, want empty", got)
	}
}

// ---------------------------------------------------------------------------
// toolchain 指令 (gomod.go)
// ---------------------------------------------------------------------------

func TestParseGoMod_ToolchainDirective(t *testing.T) {
	src := "module silk/example\n\ngo 1.22\n\ntoolchain go1.24.3\n\nrequire foo v1.0.0\n"
	gm, err := ParseGoMod(src)
	if err != nil {
		t.Fatalf("ParseGoMod: %v", err)
	}
	if gm.Toolchain != "go1.24.3" {
		t.Errorf("Toolchain = %q, want %q", gm.Toolchain, "go1.24.3")
	}
	if gm.GoVersion != "1.22" {
		t.Errorf("GoVersion = %q, want %q (the go directive stays untouched)", gm.GoVersion, "1.22")
	}
	// toolchain 更强: 展示用的版本要跟它
	if got := gm.EffectiveGoVersion(); got != "1.24.3" {
		t.Errorf("EffectiveGoVersion = %q, want %q", got, "1.24.3")
	}
	// 其余指令照旧
	if len(gm.Requires) != 1 || gm.Requires[0].Path != "foo" {
		t.Errorf("Requires = %+v", gm.Requires)
	}
}

func TestParseGoMod_ToolchainVariants(t *testing.T) {
	cases := []struct {
		name      string
		src       string
		toolchain string
		effective string
	}{
		{"absent", "module m\n\ngo 1.21\n", "", "1.21"},
		{"default", "module m\n\ngo 1.21\n\ntoolchain default\n", "default", "1.21"},
		{"trailing comment", "module m\n\ngo 1.21\n\ntoolchain go1.23.5 // pinned\n", "go1.23.5", "1.23.5"},
		{"only toolchain", "module m\n\ntoolchain go1.23.5\n", "go1.23.5", "1.23.5"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gm, err := ParseGoMod(c.src)
			if err != nil {
				t.Fatalf("ParseGoMod: %v", err)
			}
			if gm.Toolchain != c.toolchain {
				t.Errorf("Toolchain = %q, want %q", gm.Toolchain, c.toolchain)
			}
			if got := gm.EffectiveGoVersion(); got != c.effective {
				t.Errorf("EffectiveGoVersion = %q, want %q", got, c.effective)
			}
		})
	}
}

func TestParseGoMod_ToolchainMissingName(t *testing.T) {
	gm, err := ParseGoMod("module m\n\ngo 1.21\n\ntoolchain\n")
	if err == nil {
		t.Fatal("expected an error for a bare toolchain directive")
	}
	if gm == nil {
		t.Fatal("expected a partial result")
	}
	if gm.GoVersion != "1.21" || gm.Toolchain != "" {
		t.Errorf("partial result = %+v", gm)
	}
}

func TestGoMod_EffectiveGoVersion_Nil(t *testing.T) {
	var gm *GoMod
	if got := gm.EffectiveGoVersion(); got != "" {
		t.Errorf("nil GoMod EffectiveGoVersion = %q, want empty", got)
	}
}
