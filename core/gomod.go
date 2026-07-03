package core

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GoMod 描述一个go.mod文件中我们关心的内容
// 仅覆盖IDE所需的最小子集: module路径, go版本, require, replace
// 未支持的指令: retract, exclude, toolchain, godebug (这些在实际项目中较少见)
type GoMod struct {
	Module    string
	GoVersion string
	Requires  []GoModRequire
	Replaces  []GoModReplace
}

// GoModRequire 一行require信息
type GoModRequire struct {
	Path     string
	Version  string
	Indirect bool
}

// GoModReplace 一行replace信息
// 形如 "replace A v1 => B v2", 其中版本号可省略
type GoModReplace struct {
	From    string
	FromVer string // 可选, 可能为""
	To      string
	ToVer   string // 可选, 可能为""
}

// ParseGoMod 解析go.mod文本
// 容忍块状的require/replace, 自动剥离 "// comments", 跳过空白行
// 当某行格式不规范时, 返回部分结果和包装后的error, 不会panic
func ParseGoMod(src string) (*GoMod, error) {
	gm := &GoMod{}
	var errs []string

	lines := strings.Split(src, "\n")

	// 当前是否在block内, 以及block的类型
	const (
		blockNone = iota
		blockRequire
		blockReplace
	)
	block := blockNone

	for i, raw := range lines {
		// 顶层指令需要先剥注释; require/replace的子句保留原始内容,
		// 让子解析器自行处理 "// indirect" 等行尾注释
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		// 整行注释直接跳过
		if strings.HasPrefix(trimmed, "//") {
			continue
		}

		// block结束
		if trimmed == ")" {
			block = blockNone
			continue
		}

		// 处于block内: 按block类型解析每一行(保留行尾注释让子解析器读indirect)
		if block == blockRequire {
			req, err := parseGoModRequire(trimmed)
			if err != nil {
				errs = append(errs, fmt.Sprintf("line %d: %v", i+1, err))
				continue
			}
			gm.Requires = append(gm.Requires, req)
			continue
		}
		if block == blockReplace {
			rep, err := parseGoModReplace(trimmed)
			if err != nil {
				errs = append(errs, fmt.Sprintf("line %d: %v", i+1, err))
				continue
			}
			gm.Replaces = append(gm.Replaces, rep)
			continue
		}

		// 顶层: 先剥行尾注释再分析关键字
		line, _ := stripGoModComment(trimmed)
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// 顶层指令
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "module":
			if len(fields) < 2 {
				errs = append(errs, fmt.Sprintf("line %d: module directive missing path", i+1))
				continue
			}
			gm.Module = trimQuotes(fields[1])
		case "go":
			if len(fields) < 2 {
				errs = append(errs, fmt.Sprintf("line %d: go directive missing version", i+1))
				continue
			}
			gm.GoVersion = fields[1]
		case "require":
			// "require ("  开block, 否则单行
			if len(fields) >= 2 && fields[1] == "(" {
				block = blockRequire
				continue
			}
			// 单行require: 把"require"剥掉以后, 保留行尾注释(供indirect识别)
			rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "require"))
			req, err := parseGoModRequire(rest)
			if err != nil {
				errs = append(errs, fmt.Sprintf("line %d: %v", i+1, err))
				continue
			}
			gm.Requires = append(gm.Requires, req)
		case "replace":
			if len(fields) >= 2 && fields[1] == "(" {
				block = blockReplace
				continue
			}
			rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "replace"))
			rep, err := parseGoModReplace(rest)
			if err != nil {
				errs = append(errs, fmt.Sprintf("line %d: %v", i+1, err))
				continue
			}
			gm.Replaces = append(gm.Replaces, rep)
		default:
			// 忽略其他指令(exclude, retract, toolchain, ...)
		}
	}

	if len(errs) > 0 {
		return gm, fmt.Errorf("go.mod parse: %s", strings.Join(errs, "; "))
	}
	return gm, nil
}

// FindGoMod 从startDir向上查找go.mod
// 找到则返回其绝对路径和true, 否则返回""和false
func FindGoMod(startDir string) (string, bool) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", false
	}
	for {
		path := filepath.Join(dir, "go.mod")
		fi, err := os.Stat(path)
		if err == nil && !fi.IsDir() {
			return path, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// LoadGoMod 从startDir向上查找go.mod并解析
// 找不到时返回wrapped error; 解析出错时仍可能返回部分结果
func LoadGoMod(startDir string) (*GoMod, error) {
	path, ok := FindGoMod(startDir)
	if !ok {
		return nil, errors.New("go.mod not found")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return ParseGoMod(string(data))
}

// stripGoModComment 去掉行内 "// comment" 部分
// 返回去掉注释的内容和注释自身(不含 "//")
func stripGoModComment(s string) (string, string) {
	idx := strings.Index(s, "//")
	if idx < 0 {
		return s, ""
	}
	return s[:idx], strings.TrimSpace(s[idx+2:])
}

// trimQuotes 剥掉两端的双引号
func trimQuotes(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// parseGoModRequire 解析一行require的内容(不含 "require" 关键字)
// 形如: "github.com/foo/bar v1.2.3" 或 "github.com/foo/bar v1.2.3 // indirect"
func parseGoModRequire(s string) (GoModRequire, error) {
	body, cmt := stripGoModComment(s)
	body = strings.TrimSpace(body)
	fields := strings.Fields(body)
	if len(fields) < 2 {
		return GoModRequire{}, fmt.Errorf("malformed require: %q", s)
	}
	if !looksLikeVersion(fields[1]) {
		return GoModRequire{}, fmt.Errorf("malformed require version: %q", fields[1])
	}
	req := GoModRequire{
		Path:    trimQuotes(fields[0]),
		Version: fields[1],
	}
	if strings.Contains(cmt, "indirect") {
		req.Indirect = true
	}
	return req, nil
}

// parseGoModReplace 解析一行replace的内容(不含 "replace" 关键字)
// 形如:
//
//	"github.com/foo/bar => ../local/bar"
//	"github.com/foo/bar v1.0.0 => github.com/baz/qux v1.0.1"
func parseGoModReplace(s string) (GoModReplace, error) {
	body, _ := stripGoModComment(s)
	body = strings.TrimSpace(body)
	idx := strings.Index(body, "=>")
	if idx < 0 {
		return GoModReplace{}, fmt.Errorf("malformed replace (missing =>): %q", s)
	}
	left := strings.Fields(strings.TrimSpace(body[:idx]))
	right := strings.Fields(strings.TrimSpace(body[idx+2:]))
	if len(left) == 0 || len(right) == 0 {
		return GoModReplace{}, fmt.Errorf("malformed replace: %q", s)
	}

	rep := GoModReplace{From: trimQuotes(left[0])}
	switch len(left) {
	case 1:
		// "A => ..."
	case 2:
		if !looksLikeVersion(left[1]) {
			return rep, fmt.Errorf("malformed replace from-version: %q", left[1])
		}
		rep.FromVer = left[1]
	default:
		return rep, fmt.Errorf("malformed replace lhs: %q", s)
	}

	rep.To = trimQuotes(right[0])
	switch len(right) {
	case 1:
		// "... => B"
	case 2:
		if !looksLikeVersion(right[1]) {
			return rep, fmt.Errorf("malformed replace to-version: %q", right[1])
		}
		rep.ToVer = right[1]
	default:
		return rep, fmt.Errorf("malformed replace rhs: %q", s)
	}

	return rep, nil
}

// looksLikeVersion 简单识别模块版本号
// 接受 "v" 开头(如 v1.2.3, v0.0.0-20230101000000-abcdef)
// 或 SemVer 风格 (1.2.3)
func looksLikeVersion(s string) bool {
	if s == "" {
		return false
	}
	if s[0] == 'v' && len(s) > 1 {
		return true
	}
	// 退而求其次, 至少要含数字和点
	hasDigit := false
	hasDot := false
	for _, r := range s {
		if r >= '0' && r <= '9' {
			hasDigit = true
		}
		if r == '.' {
			hasDot = true
		}
	}
	return hasDigit && hasDot
}
