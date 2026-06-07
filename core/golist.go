package core

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"encoding/json"
)

// GoListPackage 描述 `go list -json ./...` 输出中我们关心的字段
// 仅覆盖 IDE 所需的最小子集: 目录, 导入路径, 包名, GoFiles, 测试文件, 所属 Module
// 字段还可扩展(Imports, Deps, CompiledGoFiles, EmbedFiles 等), 但当前 IDE 还
// 没有使用场景, 先保持精简, 按需要再加
type GoListPackage struct {
	Dir          string
	ImportPath   string
	Name         string
	GoFiles      []string
	TestGoFiles  []string
	XTestGoFiles []string
	Module       *GoListModule // 指针: nil 表示输出里没有 Module 字段(GOPATH 模式或 stdlib)
}

// GoListModule 是 GoListPackage.Module 的内嵌子集
// 仅取 IDE 需要的三个字段; Path/Dir 用来区分主模块和依赖模块,
// Main 用来标识"是不是当前工作区里的那一个 module"
type GoListModule struct {
	Path string
	Main bool
	Dir  string
}

// ParseGoListJSON 解析 `go list -json ./...` 的输出
// `go list` 不输出 JSON 数组, 而是把每个 package 的对象逐个 pretty-print 拼接,
// 所以这里用 json.Decoder 在一个 strings.Reader 上循环 Decode, 直到 io.EOF.
// 遇到单个对象解析失败时跳过该对象继续, 最后把所有错误打包到一个 wrapped error 里返回,
// 调用方仍能拿到此前已成功解析的 package 切片. 永远不 panic.
func ParseGoListJSON(src string) ([]GoListPackage, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return nil, nil
	}

	dec := json.NewDecoder(strings.NewReader(src))
	var pkgs []GoListPackage
	var errs []string
	for {
		var pkg GoListPackage
		err := dec.Decode(&pkg)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			errs = append(errs, err.Error())
			// 单个对象出错时 json.Decoder 的内部读位置可能停在半截 token 上,
			// 继续 Decode 通常会一路报错; 这里直接终止循环, 把之前解析成功的
			// 对象连同错误一起返回, 上层据此决定是否回退
			break
		}
		pkgs = append(pkgs, pkg)
	}

	if len(errs) > 0 {
		return pkgs, fmt.Errorf("go list parse: %s", strings.Join(errs, "; "))
	}
	return pkgs, nil
}

// RunGoList 在 dir 目录下执行 `go list -json` 并把合并后的 stdout+stderr 文本返回
// args 为空时默认补 "./..."(扫描当前模块所有 package)
// 之所以保留 stderr: 当模块状态有问题(缺依赖, 语法错误)时 `go list` 仍会
// 给部分 package 输出 JSON, 但同时把诊断信息打到 stderr; 上层若拿不到 stderr
// 就无从展示给用户. 因此即使 cmd.Run 失败也会返回已收集到的输出.
func RunGoList(dir string, args ...string) (string, error) {
	if len(args) == 0 {
		args = []string{"./..."}
	}
	full := append([]string{"list", "-json"}, args...)
	cmd := exec.Command("go", full...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("go list %s: %w", strings.Join(args, " "), err)
	}
	return string(out), nil
}

// LoadGoListJSON 是 RunGoList + ParseGoListJSON 的便捷封装
// 即使 go list 本身报错也会尝试解析其 stdout(go list 有"边报错边出 JSON"的行为)
// 这样能在依赖缺失时仍把可用的 package 信息提交给 IDE
func LoadGoListJSON(dir string, args ...string) ([]GoListPackage, error) {
	out, runErr := RunGoList(dir, args...)
	pkgs, parseErr := ParseGoListJSON(out)
	if runErr != nil && parseErr != nil {
		return pkgs, fmt.Errorf("%w; %v", runErr, parseErr)
	}
	if runErr != nil {
		return pkgs, runErr
	}
	if parseErr != nil {
		return pkgs, parseErr
	}
	return pkgs, nil
}
