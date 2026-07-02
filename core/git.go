package core

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// git CLI 的最小封装
// 设计目标: 让 IDE 查询仓库状态(diff/status/branch/log)并驱动提交流程
// (stage/unstage/commit), 而无需每个调用方自己重写 os/exec 的样板.
// 与已合并的 core.ParseUnifiedDiff 配套使用:
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

// GitBlameLine 是 `git blame` 里最终文件的一行归属信息
// Hash 是该行最后一次改动的提交; Author 是该提交作者; Line 是 1-based 行号;
// Content 是该行源码文本(不含末尾换行).
type GitBlameLine struct {
	Hash    string // commit hash, 已缩短为 8 位便于 gutter 展示
	Author  string
	Line    int    // 最终文件里的 1-based 行号
	Content string // 该行源码文本
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
	return parseShortLog(out), nil
}

// GitLogFile 返回单个文件最近 n 条提交摘要(`git log -n <n> -- <file>`)
// 即"文件历史"视图. 与 GitShortLog 共用完全相同的 0x1f 字段格式和 parseShortLog
// 解析, 区别仅在末尾加了 `-- <file>` 把日志限定到该文件. 单行字段不齐时跳过.
func GitLogFile(dir, file string, n int) ([]GitCommit, error) {
	out, err := runGit(dir, "log",
		fmt.Sprintf("-n%d", n),
		"--pretty=format:%h%x1f%s%x1f%an%x1f%ad",
		"--date=short",
		"--", file)
	if err != nil {
		return nil, err
	}
	return parseShortLog(out), nil
}

// parseShortLog 解析 `--pretty=format:%h%x1f%s%x1f%an%x1f%ad` 的 0x1f 分隔输出
// 由 GitShortLog 和 GitLogFile 共用: 用 0x1f(unit separator)分隔
// hash/subject/author/date 四个字段, 避免 subject 里出现普通分隔符导致误切.
// 空行跳过, 单行字段数不足时跳过该行继续, 永远不 panic.
func parseShortLog(out string) []GitCommit {
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
	return commits
}

// GitBlame 返回文件每一行的最后改动归属(`git blame --line-porcelain -- <file>`)
// 即"git blame" gutter / 行注释视图. 用 --line-porcelain 这种稳定的机器格式:
// 每一行(最终文件的一行)对应一个 block —— 先是一行头
//
//	<40-hex> <orig-line> <final-line> [<num-lines>]
//
// 接着若干 "key value" 元数据行(author / author-time / ...), 最后一行 TAB 开头
// 即该行源码文本. 这里只取 hash(头行第一个字段)、author(`author ` 行)和那条
// TAB 内容行. Hash 缩短到 8 位便于 gutter 展示.
// 健壮解析: 头行字段不齐、缺 author、缺内容行的畸形 block 直接跳过不收集, 永不 panic.
func GitBlame(dir, file string) ([]GitBlameLine, error) {
	out, err := runGit(dir, "blame", "--line-porcelain", "--", file)
	if err != nil {
		return nil, err
	}

	var lines []GitBlameLine
	var cur GitBlameLine
	var haveHeader bool // 已读到本 block 的头行
	sc := bufio.NewScanner(strings.NewReader(out))
	// blame 的源码行可能很长, 调大 Scanner 缓冲上限到 1MiB 防止 ErrTooLong.
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		// TAB 开头的是该 block 的源码内容行, 也是一个 block 的收尾.
		if strings.HasPrefix(line, "\t") {
			if haveHeader {
				cur.Content = line[1:]
				lines = append(lines, cur)
			}
			// 不论是否成功收集, 都重置, 等待下一个 block 的头行.
			cur = GitBlameLine{}
			haveHeader = false
			continue
		}
		// 元数据行 "author Foo Bar"
		if strings.HasPrefix(line, "author ") {
			if haveHeader {
				cur.Author = line[len("author "):]
			}
			continue
		}
		// 其余非头行的元数据(author-time/committer/summary/...)直接忽略;
		// 头行是唯一以 40-hex 开头并带数字字段的行, 用字段数+全 hex 判定.
		fields := strings.Fields(line)
		if len(fields) >= 3 && isHex40(fields[0]) {
			finalLine, perr := atoiStrict(fields[2])
			if perr != nil {
				// 头行 final-line 不是合法整数 —— 畸形, 跳过该 block.
				haveHeader = false
				continue
			}
			cur = GitBlameLine{
				Hash: shortHash(fields[0]),
				Line: finalLine,
			}
			haveHeader = true
		}
		// 其它行(不匹配任何已知形态)忽略, 继续.
	}
	return lines, nil
}

// GitShow 返回文件在某个修订版本下的内容(`git show <rev>:<file>`)
// 用于把工作副本与任意提交对比, 或展示"原始"内容. rev 可为 "HEAD"、分支名、SHA 等.
// 空文件返回 ("", nil); rev 或路径非法时 git 非零退出, 原样返回带诊断的 error.
func GitShow(dir, rev, file string) (string, error) {
	return runGit(dir, "show", rev+":"+file)
}

// GitRevParse 把一个 ref(如 "HEAD"、分支名)解析为完整 SHA(`git rev-parse <ref>`)
// 返回去掉首尾空白的 40-hex SHA. 未知 ref 时 git 非零退出, 返回带诊断的 error.
func GitRevParse(dir, ref string) (string, error) {
	out, err := runGit(dir, "rev-parse", ref)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// GitStage 把给定路径加入暂存区(`git add -- <paths...>`)
// 只暂存显式给出的路径(用 `--` 与选项分隔, 刻意不用 `git add .`), 供 Git Changes
// 面板逐文件勾选暂存. paths 为空时直接返回 nil 不调用 git —— 空 pathspec 的
// `git add` 会报错, 且"没选任何文件"本就该是 no-op.
func GitStage(dir string, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	args := append([]string{"add", "--"}, paths...)
	_, err := runGit(dir, args...)
	return err
}

// GitUnstage 把给定路径移出暂存区但保留工作树改动(`git reset HEAD -- <paths...>`)
// 与 GitStage 相反: 只把 index 里这些路径还原成 HEAD 版本, 不动工作副本.
// paths 为空时直接返回 nil 不调用 git.
func GitUnstage(dir string, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	args := append([]string{"reset", "HEAD", "--"}, paths...)
	_, err := runGit(dir, args...)
	return err
}

// GitStageAll 暂存工作树里的全部改动(`git add -A`: 新增/修改/删除/未跟踪都进 index)
// 这是本封装里唯一的批量暂存形式 —— 需要"全部暂存"用它, 精确逐文件暂存用 GitStage.
func GitStageAll(dir string) error {
	_, err := runGit(dir, "add", "-A")
	return err
}

// GitCommitChanges 用给定信息提交(`git commit -m <message>`)并返回新提交的短 hash
// (命名避开同名的 GitCommit 结构体 —— 那是 git log 的提交摘要类型).
// message 去空白后为空时直接返回 error 快速失败(git 本身也会拒绝空信息, 这里提前挡掉).
// 若暂存区为空, git commit 以 "nothing to commit" 非零退出, runGit 会把它转成带命令
// 上下文的 error 原样返回(与 GitRevParse 遇未知 ref 同一套路), 不 panic.
// 提交成功后再跑 `git rev-parse --short HEAD` 取回短 hash.
func GitCommitChanges(dir, message string) (string, error) {
	if strings.TrimSpace(message) == "" {
		return "", fmt.Errorf("git commit: empty commit message")
	}
	if _, err := runGit(dir, "commit", "-m", message); err != nil {
		return "", err
	}
	out, err := runGit(dir, "rev-parse", "--short", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// GitHasStagedChanges 报告 index 相对 HEAD 是否有已暂存改动
// 走 `git diff --cached --quiet`: 退出码 0 表示无暂存改动, 退出码 1 表示有 ——
// 这个 1 是"存在 diff"的信号而非报错. 因此这里把退出码 1 映射成 (true, nil),
// 退出码 0 映射成 (false, nil); 只有其它退出码(如非仓库的 128)或超时才当真正的
// error 返回. 供 UI 据此决定 Commit 按钮是否可点. 不 panic.
func GitHasStagedChanges(dir string) (bool, error) {
	if _, err := runGit(dir, "diff", "--cached", "--quiet"); err != nil {
		// runGit 用 %w 包了底层 err; 底层是 *exec.ExitError 且退出码为 1 时,
		// 那是"有暂存改动"的正常信号, 不是错误.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return true, nil
		}
		return false, err
	}
	// 退出码 0: index 与 HEAD 一致, 无暂存改动
	return false, nil
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

// isHex40 判定 s 是否恰好是 40 位十六进制(blame line-porcelain 头行的提交 SHA)
// 用来把头行从其它元数据行里区分出来, 避免误把 "author-time 123" 当成头行.
func isHex40(s string) bool {
	if len(s) != 40 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// shortHash 把 40-hex 提交 SHA 缩短到 8 位便于 gutter 展示; 短于 8 位时原样返回.
func shortHash(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

// atoiStrict 解析一个十进制整数, 仅是 strconv.Atoi 的薄封装
// 单列出来是为了让 blame 头行解析的意图(严格整数, 失败即视为畸形)更清楚.
func atoiStrict(s string) (int, error) {
	return strconv.Atoi(s)
}

// ErrNotGitRepo 是供上层用 errors.Is 区分"非 git 仓库"场景的哨兵错误
// IsGitRepo 已能做布尔判定, 这里仅为需要 error 语义的调用方保留一个稳定值.
var ErrNotGitRepo = errors.New("not a git repository")
