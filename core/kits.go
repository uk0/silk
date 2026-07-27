package core

// Kit 是"用哪套工具链, 为哪个目标平台, 带哪些开关构建, 结果交付到哪里"的完整描述,
// 对应 Qt Creator 的 Kit + Build Configuration + Deploy Configuration 三件套.
//
// 这里只放数据和纯函数: 探测(DetectKits)会起子进程, 校验(Validate)/渲染
// 构建参数(BuildArgs/BuildEnv)都不碰文件系统, 便于上层在任何线程调用.
// 工程级的持久化不在本包, 见 ide/kits.Store.

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"sort"
	"strings"
)

// 部署方式
const (
	DeployLocal = "local" // 产物留在本机 OutputDir
	DeploySSH   = "ssh"   // 产物推到 User@Host:RemoteDir
)

// DeployProfile 描述产物的交付目标
// Kind 为 "" 时按 DeployLocal 处理; Host/User/RemoteDir 只对 DeploySSH 有意义
type DeployProfile struct {
	Kind      string `json:"kind"`
	Host      string `json:"host,omitempty"`
	User      string `json:"user,omitempty"`
	RemoteDir string `json:"remote_dir,omitempty"`
}

// Kit 一套具名的构建配置
type Kit struct {
	Name      string            `json:"name"`
	GoExe     string            `json:"go_exe"`     // go 可执行文件路径, 默认 "go"
	GoVersion string            `json:"go_version"` // 不带 "go" 前缀, 如 "1.25.0"
	GOOS      string            `json:"goos"`
	GOARCH    string            `json:"goarch"`
	Tags      []string          `json:"tags,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	Race      bool              `json:"race,omitempty"`
	Coverage  bool              `json:"coverage,omitempty"`
	BuildMode string            `json:"build_mode,omitempty"` // go build -buildmode, "" = 默认
	OutputDir string            `json:"output_dir,omitempty"`
	Deploy    DeployProfile     `json:"deploy"`
}

// goBuildModes 是 `go help buildmode` 列出的取值; "" 表示不传 -buildmode
var goBuildModes = map[string]bool{
	"":          true,
	"archive":   true,
	"c-archive": true,
	"c-shared":  true,
	"default":   true,
	"exe":       true,
	"pie":       true,
	"plugin":    true,
	"shared":    true,
}

// DefaultKit 返回零配置的本机Kit: 用PATH上的go, 目标平台是当前平台,
// 产物留在当前目录. 它不起子进程, 因此可以在Init/热路径/测试里随意调用
func DefaultKit() Kit {
	return Kit{
		Name:      "default",
		GoExe:     "go",
		GoVersion: trimGoVersionPrefix(runtime.Version()),
		GOOS:      runtime.GOOS,
		GOARCH:    runtime.GOARCH,
		OutputDir: ".",
		Deploy:    DeployProfile{Kind: DeployLocal},
	}
}

// DetectKits 探测本机可用的工具链
// 当前只认PATH上的 go: 用 `go env GOOS GOARCH GOVERSION` 问出它的目标平台和版本,
// 折成一个本机Kit返回. PATH上没有go时返回 (nil, error), 调用方据此退回 DefaultKit
func DetectKits() ([]Kit, error) {
	exe, err := exec.LookPath("go")
	if err != nil {
		return nil, fmt.Errorf("detect kits: go not found on PATH: %w", err)
	}
	out, err := exec.Command(exe, "env", "GOOS", "GOARCH", "GOVERSION").Output()
	if err != nil {
		return nil, fmt.Errorf("detect kits: go env: %w", err)
	}

	vals := parseGoEnvValues(string(out), 3)
	k := DefaultKit()
	k.GoExe = exe
	if vals[0] != "" {
		k.GOOS = vals[0]
	}
	if vals[1] != "" {
		k.GOARCH = vals[1]
	}
	if v := trimGoVersionPrefix(vals[2]); v != "" {
		k.GoVersion = v
	}
	k.Name = fmt.Sprintf("Go %s %s/%s", k.GoVersion, k.GOOS, k.GOARCH)
	return []Kit{k}, nil
}

// parseGoEnvValues 解析 `go env K1 K2 ...` 的输出
// go 每行输出一个值, 顺序与参数一致, 值为空时输出空行;
// 少于 n 行时补 "", 多于 n 行时忽略多余部分, 因此返回值长度恒为 n
func parseGoEnvValues(out string, n int) []string {
	vals := make([]string, n)
	if n <= 0 {
		return vals
	}
	lines := strings.Split(out, "\n")
	for i := 0; i < n && i < len(lines); i++ {
		vals[i] = strings.TrimSpace(lines[i])
	}
	return vals
}

// Validate 检查Kit是否足以驱动一次构建
// 只校验构建系统真正会读的字段: 选择用的Name, 目标平台, -buildmode取值,
// build tag / 环境变量的格式, 以及部署配置自身是否自洽
func (k Kit) Validate() error {
	if strings.TrimSpace(k.Name) == "" {
		return errors.New("kit: name is empty")
	}
	if k.GOOS == "" {
		return fmt.Errorf("kit %q: GOOS is empty", k.Name)
	}
	if k.GOARCH == "" {
		return fmt.Errorf("kit %q: GOARCH is empty", k.Name)
	}
	if !goBuildModes[k.BuildMode] {
		return fmt.Errorf("kit %q: unknown build mode %q", k.Name, k.BuildMode)
	}
	for _, t := range k.Tags {
		if t == "" || strings.ContainsAny(t, " \t,") {
			return fmt.Errorf("kit %q: malformed build tag %q", k.Name, t)
		}
	}
	for key := range k.Env {
		if key == "" || strings.ContainsAny(key, "= \t") {
			return fmt.Errorf("kit %q: malformed env key %q", k.Name, key)
		}
	}
	switch k.Deploy.Kind {
	case "", DeployLocal:
		// 本机部署只用 OutputDir, 其余字段留着也无害
	case DeploySSH:
		if k.Deploy.Host == "" {
			return fmt.Errorf("kit %q: ssh deploy needs a host", k.Name)
		}
		if k.Deploy.RemoteDir == "" {
			return fmt.Errorf("kit %q: ssh deploy needs a remote dir", k.Name)
		}
	default:
		return fmt.Errorf("kit %q: unknown deploy kind %q", k.Name, k.Deploy.Kind)
	}
	return nil
}

// Clone 深拷贝Kit: Tags切片和Env映射都是新的
// 存储层/面板对外交付Kit时都走它, 免得调用方改到共享的底层容器
func (k Kit) Clone() Kit {
	out := k
	if len(k.Tags) > 0 {
		out.Tags = make([]string, len(k.Tags))
		copy(out.Tags, k.Tags)
	}
	if len(k.Env) > 0 {
		out.Env = make(map[string]string, len(k.Env))
		for key, v := range k.Env {
			out.Env[key] = v
		}
	}
	return out
}

// BuildArgs 渲染该Kit对应的 `go build` 参数, 顺序固定:
// -tags, -race, -cover, -buildmode. 包路径和 -o 由调用方按 OutputDir 自行拼
func (k Kit) BuildArgs() []string {
	var args []string
	if len(k.Tags) > 0 {
		args = append(args, "-tags", FormatBuildTags(k.Tags))
	}
	if k.Race {
		args = append(args, "-race")
	}
	if k.Coverage {
		args = append(args, "-cover")
	}
	if k.BuildMode != "" && k.BuildMode != "default" {
		args = append(args, "-buildmode="+k.BuildMode)
	}
	return args
}

// BuildEnv 渲染该Kit要追加到 exec.Cmd.Env 的 "K=V" 列表
// GOOS/GOARCH 在前, 其余自定义变量按key排序, 保证同一个Kit每次输出一致
func (k Kit) BuildEnv() []string {
	env := make([]string, 0, len(k.Env)+2)
	if k.GOOS != "" {
		env = append(env, "GOOS="+k.GOOS)
	}
	if k.GOARCH != "" {
		env = append(env, "GOARCH="+k.GOARCH)
	}
	keys := make([]string, 0, len(k.Env))
	for key := range k.Env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		env = append(env, key+"="+k.Env[key])
	}
	return env
}

// ParseBuildTags 把用户输入的 build tag 串拆成列表
// 逗号和空白都当分隔符, 空片段丢弃, 顺序保留
func ParseBuildTags(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
	if len(fields) == 0 {
		return nil
	}
	return fields
}

// FormatBuildTags 按 `go build -tags` 的形式把列表拼回一行
func FormatBuildTags(tags []string) string {
	return strings.Join(tags, ",")
}
