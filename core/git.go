package core

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// git CLI 的最小封装
// 设计目标: 让 IDE 查询仓库状态(diff/status/branch/log)而无需每个调用方
// 自己重写 os/exec 的样板. 与已合并的 core.ParseUnifiedDiff 配套使用:
// GitDiffHead/GitDiffFile 产出 unified diff 文本, 交给 ParseUnifiedDiff 渲染.
// 全部走 stdlib(os/exec, strings, bufio, fmt, errors, bytes, context, time),
// 每个解析都是"跳过并收集"或干净返回 error, 永远不 panic.

// gitTimeout 是单条 git 命令的最长执行时间
// 套一层 context 超时, 避免某个挂死的 git(比如等待凭据输入)拖死 IDE.
const gitTimeout = 10 * time.Second

// GitStatusEntry 是 `git status --porcelain=v1` 的一行
// Staged 是 X 列(index 状态), Unstaged 是 Y 列(worktree 状态),
// 取值如 'M','A','D','R','?',' ' 等. 重命名时 OrigPath 为箭头左侧的旧路径.
type GitStatusEntry struct {
	Staged   byte
	Unstaged byte
	Path     string
	OrigPath string
}

// GitCommit 是 `git log` 的一条提交摘要
type GitCommit struct {
	Hash    string
	Subject string
	Author  string
	Date    string
}

// GitAvailable 报告 PATH 上是否能找到 git 可执行文件
func GitAvailable() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// IsGitRepo 报告 dir 是否在某个 git 工作树内
// 通过 `git rev-parse --is-inside-work-tree` 退出码为 0 且输出 "true" 判定.
func IsGitRepo(dir string) bool {
	out, err := runGit(dir, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) == "true"
}

// GitDiffHead 返回 dir 工作树相对 HEAD 的完整 unified diff(`git diff HEAD`)
// 无改动时返回 ("", nil). 输出文本可直接喂给 ParseUnifiedDiff.
func GitDiffHead(dir string) (string, error) {
	return runGit(dir, "diff", "HEAD")
}

// GitDiffFile 返回单个文件相对 HEAD 的 unified diff(`git diff HEAD -- <file>`)
// 文件无改动时返回 ("", nil).
func GitDiffFile(dir, file string) (string, error) {
	return runGit(dir, "diff", "HEAD", "--", file)
}

// GitStatusPorcelain 解析 `git status --porcelain=v1` 的输出
// 每行前两个字符是 X(staged)/Y(unstaged)两列状态, 第 4 个字符起是路径.
// 重命名行形如 "R  old -> new", 此时 Path 取箭头右侧新路径, OrigPath 取左侧旧路径.
// 路径含特殊字符时 git 会给整段加双引号并做 C 风格转义; 这里只做最小处理:
// 若路径被双引号包裹, 仅剥掉首尾引号, 不做完整的 C 风格反转义(超出当前范围).
// 单行畸形(长度不足)时跳过该行继续, 永远不 panic.
func GitStatusPorcelain(dir string) ([]GitStatusEntry, error) {
	out, err := runGit(dir, "status", "--porcelain=v1")
	if err != nil {
		return nil, err
	}

	var entries []GitStatusEntry
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		// porcelain v1: 前两列是状态, 第 3 列是空格分隔符, 第 4 列起是路径.
		// 不足 4 个字符的行无法构成有效条目, 跳过.
		if len(line) < 4 {
			continue
		}
		entry := GitStatusEntry{
			Staged:   line[0],
			Unstaged: line[1],
		}
		rest := line[3:]
		// 重命名/复制: "old -> new", Path 记新路径, OrigPath 记旧路径
		if idx := strings.Index(rest, " -> "); idx >= 0 {
			entry.OrigPath = stripGitQuotes(rest[:idx])
			entry.Path = stripGitQuotes(rest[idx+len(" -> "):])
		} else {
			entry.Path = stripGitQuotes(rest)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// GitCurrentBranch 返回当前分支名(`git rev-parse --abbrev-ref HEAD`)
// 处于 detached HEAD 时 git 输出 "HEAD", 这里原样透传.
func GitCurrentBranch(dir string) (string, error) {
	out, err := runGit(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// GitShortLog 返回最近 n 条提交摘要(`git log -n <n>`)
// 用 0x1f(unit separator)分隔 hash/subject/author/date 四个字段, 避免 subject
// 里出现普通分隔符导致误切. 单行字段数不足时跳过该行继续, 永远不 panic.
func GitShortLog(dir string, n int) ([]GitCommit, error) {
	// %x1f 是 0x1f 单元分隔符; --date=short 输出 YYYY-MM-DD
	out, err := runGit(dir, "log",
		fmt.Sprintf("-n%d", n),
		"--pretty=format:%h%x1f%s%x1f%an%x1f%ad",
		"--date=short")
	if err != nil {
		return nil, err
	}

	var commits []GitCommit
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\x1f")
		if len(parts) < 4 {
			// 字段不齐, 跳过这条而不是把半截数据塞进结果
			continue
		}
		commits = append(commits, GitCommit{
			Hash:    parts[0],
			Subject: parts[1],
			Author:  parts[2],
			Date:    parts[3],
		})
	}
	return commits, nil
}

// runGit 是所有 git 调用的共享私有助手
// 统一用 cmd.Dir 指定工作目录(不混用 -C), 套 context 超时防止挂死,
// 分开捕获 stdout/stderr. 命令非零退出时返回的 error 带上 stderr 的首行,
// 方便上层直接展示诊断信息. 永远不 panic.
func runGit(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// 超时优先报出来, 让上层知道是被 context 掐断而非 git 自身报错
		if ctx.Err() != nil {
			return stdout.String(), fmt.Errorf("git %s: %w", strings.Join(args, " "), ctx.Err())
		}
		if msg := firstLine(stderr.String()); msg != "" {
			return stdout.String(), fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), msg, err)
		}
		return stdout.String(), fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return stdout.String(), nil
}

// firstLine 取字符串的第一行(去掉首尾空白), 用于把 git 的多行 stderr 压成一行
func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return strings.TrimSpace(s[:idx])
	}
	return s
}

// stripGitQuotes 最小处理 git porcelain 的引号路径
// 仅当整段被双引号包裹时剥掉首尾引号; 不做 C 风格反转义(超出当前范围).
func stripGitQuotes(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// ErrNotGitRepo 是供上层用 errors.Is 区分"非 git 仓库"场景的哨兵错误
// IsGitRepo 已能做布尔判定, 这里仅为需要 error 语义的调用方保留一个稳定值.
var ErrNotGitRepo = errors.New("not a git repository")
