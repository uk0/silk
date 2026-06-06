package core

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GoWork 描述一个go.work文件中我们关心的内容
// 仅覆盖IDE所需的最小子集: go版本, use目录列表, replace
// 未支持的指令: toolchain, godebug, godebug(...)块 (这些在实际go.work中较少出现且
// 不影响"哪些模块在工作区里"这一核心信号)
//
// Replaces 复用 gomod.go 中的 GoModReplace: go.work 的 replace 指令与
// go.mod 完全同形态 (from [ver] => to [ver]), 共用同一结构能让上层调用方
// 用同一套渲染/校验逻辑处理两边的替换规则, 因此这里不再定义平行类型.
type GoWork struct {
	GoVersion string
	Uses      []string // 相对或绝对的模块目录
	Replaces  []GoModReplace
}

// ParseGoWork 解析go.work文本
// 容忍块状的use/replace, 自动剥离 "// comments", 跳过空白行
// 当某行格式不规范时, 返回部分结果和包装后的error, 不会panic
func ParseGoWork(src string) (*GoWork, error) {
	gw := &GoWork{}
	var errs []string

	lines := strings.Split(src, "\n")

	// 当前是否在block内, 以及block的类型
	const (
		blockNone = iota
		blockUse
		blockReplace
	)
	block := blockNone

	for i, raw := range lines {
		// 顶层指令需要先剥注释; use/replace的子句保留原始内容,
		// 让子解析器自行处理可能的行尾注释
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

		// 处于block内: 按block类型解析每一行
		if block == blockUse {
			p, err := parseGoWorkUse(trimmed)
			if err != nil {
				errs = append(errs, fmt.Sprintf("line %d: %v", i+1, err))
				continue
			}
			gw.Uses = append(gw.Uses, p)
			continue
		}
		if block == blockReplace {
			rep, err := parseGoModReplace(trimmed)
			if err != nil {
				errs = append(errs, fmt.Sprintf("line %d: %v", i+1, err))
				continue
			}
			gw.Replaces = append(gw.Replaces, rep)
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
		case "go":
			if len(fields) < 2 {
				errs = append(errs, fmt.Sprintf("line %d: go directive missing version", i+1))
				continue
			}
			gw.GoVersion = fields[1]
		case "use":
			// "use ("  开block, 否则单行
			if len(fields) >= 2 && fields[1] == "(" {
				block = blockUse
				continue
			}
			// 单行use: 把"use"剥掉后取剩余路径
			rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "use"))
			p, err := parseGoWorkUse(rest)
			if err != nil {
				errs = append(errs, fmt.Sprintf("line %d: %v", i+1, err))
				continue
			}
			gw.Uses = append(gw.Uses, p)
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
			gw.Replaces = append(gw.Replaces, rep)
		default:
			// 忽略其他指令(toolchain, godebug, ...)
		}
	}

	if len(errs) > 0 {
		return gw, fmt.Errorf("go.work parse: %s", strings.Join(errs, "; "))
	}
	return gw, nil
}

// FindGoWork 从startDir向上查找go.work
// 找到则返回其绝对路径和true, 否则返回""和false
func FindGoWork(startDir string) (string, bool) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", false
	}
	for {
		path := filepath.Join(dir, "go.work")
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

// LoadGoWork 从startDir向上查找go.work并解析
// 找不到时返回wrapped error; 解析出错时仍可能返回部分结果
func LoadGoWork(startDir string) (*GoWork, error) {
	path, ok := FindGoWork(startDir)
	if !ok {
		return nil, errors.New("go.work not found")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return ParseGoWork(string(data))
}

// parseGoWorkUse 解析一行use的内容(不含 "use" 关键字)
// 形如: "./mod-a" 或 "./mod-a  // trailing comment"
func parseGoWorkUse(s string) (string, error) {
	body, _ := stripGoModComment(s)
	body = strings.TrimSpace(body)
	if body == "" {
		return "", fmt.Errorf("malformed use: empty path")
	}
	fields := strings.Fields(body)
	if len(fields) != 1 {
		return "", fmt.Errorf("malformed use: %q", s)
	}
	return trimQuotes(fields[0]), nil
}
