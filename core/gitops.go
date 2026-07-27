package core

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// git 工作流操作的封装(core/git.go 的后半场)
// core/git.go 覆盖只读查询与提交流程(status/diff/branch/log/blame/stage/commit);
// 这里补上 IDE 的 Git 面板还缺的另一半: fetch/pull/push、stash、rebase、cherry-pick、
// merge --abort, 以及"当前冲突了哪些文件、每个文件有哪几个 stage"的查询.
// 分支与远程的增删查在同包的 core/gitrefs.go, 与本文件共用这里的 GitOps/GitRunner.
//
// 三条贯穿本文件的设计约束:
//
//  1. 绝不交互. execGit 给每条命令套上 gitNonInteractiveEnv —— 缺凭据、想开编辑器时
//     git 立刻失败, 而不是挂在那里等一个根本不存在的终端(IDE 里没有 tty 可用).
//     rebase 一律不带 -i, 所以不存在"掉进交互式 todo 列表出不来"的状态.
//  2. 解析与执行分离. 所有 Parse* 都是纯函数(吃字符串, 吐结构体, 不碰进程), 可直接单测;
//     命令拼装统一走 GitOps.run, 由可注入的 Runner 决定是真跑 git 还是被测试拦下,
//     因此 argv 本身也是可断言的.
//  3. 参数不做选项注入. 分支名/远程名/URL/revision 都会被拼进 argv, 一个 "-" 开头的值
//     会被 git 当成选项, 所以统一过 validateGitArg.
//
// 全部走 stdlib(os/exec, os, strings, strconv, bytes, context, fmt, time),
// 每个解析都是"跳过畸形并继续", 永远不 panic.

// gitOpsTimeout 是单条 git 命令的默认超时上限
// 比 core/git.go 的 gitTimeout(10s, 只用于本地查询)宽得多 —— fetch/pull/push 要走网络,
// 10 秒远不够用. 但仍然设上限: 非交互模式下 git 不会等输入, 可网络本身能卡到天荒地老.
const gitOpsTimeout = 120 * time.Second

// gitNonInteractiveEnv 是所有 git 调用共享的"禁止交互"环境覆盖
// 追加在进程环境之后, 因此同名变量以这里的值为准(os/exec 对重复 key 取最后一个).
var gitNonInteractiveEnv = []string{
	// 核心一条: 不许向终端索要用户名/口令, 缺凭据直接非零退出.
	"GIT_TERMINAL_PROMPT=0",
	// 置空 askpass 助手, 否则 git 可能改去弹一个 GUI 凭据框(同样是无人应答的阻塞).
	"GIT_ASKPASS=",
	"SSH_ASKPASS=",
	// merge/commit/rebase --continue/cherry-pick 会想开编辑器写信息; true 立刻成功退出,
	// git 于是沿用默认信息继续走, 不阻塞.
	"GIT_EDITOR=true",
	"GIT_SEQUENCE_EDITOR=true",
	// 固定 C locale: %(upstream:track) 的 "ahead/behind/gone" 是会被本地化的字符串,
	// parseGitTrack 按英文形态解析, 这里把语言钉死, 顺带让报错也是英文便于诊断.
	"LC_ALL=C",
}

// GitRunner 是执行一条 git 命令的抽象
// dir 是工作目录, args 是不含 "git" 本身的 argv, 返回 stdout 与错误.
// 生产路径是 GitOps.execGit(真的 fork git); 测试注入一个记录 argv 的假实现,
// 于是"每个操作拼出什么命令"可以被断言, 且测试完全不需要真仓库.
type GitRunner func(dir string, args ...string) (string, error)

// GitOps 是绑定在某个工作树上的 git 操作句柄
// Runner 为 nil 时走 execGit(真实 git, 非交互); Timeout 为 0 时用 gitOpsTimeout.
// 本身无状态(除这三个字段), 可以随手 new, 也可以在面板里长期持有.
type GitOps struct {
	Dir     string
	Runner  GitRunner     // nil 表示用内置的非交互 execGit
	Timeout time.Duration // <=0 表示用 gitOpsTimeout
}

// NewGitOps 返回一个绑定 dir 的默认 GitOps(真实 git + 默认超时)
func NewGitOps(dir string) *GitOps {
	return &GitOps{Dir: dir}
}

// run 是所有操作唯一的出口: 优先用注入的 Runner, 否则走 execGit
func (g *GitOps) run(args ...string) (string, error) {
	if g.Runner != nil {
		return g.Runner(g.Dir, args...)
	}
	return g.execGit(g.Dir, args...)
}

// execGit 是默认 runner: 在 dir 下以非交互方式跑一条 git
// 与 core/git.go 的 runGit 同一套路(cmd.Dir 而非 -C, context 超时, 分开捕获
// stdout/stderr, 报错带上 stderr 首行), 额外做两件事: 追加 gitNonInteractiveEnv,
// 以及不接 Stdin —— exec.Cmd 的 Stdin 留 nil 即 /dev/null, 任何想读终端的 git 立刻拿到 EOF.
func (g *GitOps) execGit(dir string, args ...string) (string, error) {
	timeout := g.Timeout
	if timeout <= 0 {
		timeout = gitOpsTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = gitNonInteractiveEnviron(os.Environ())
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// 超时优先报出来, 让上层知道是被 context 掐断而非 git 自己报错
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

// gitNonInteractiveEnviron 在 base 之后追加 gitNonInteractiveEnv
// 单列成纯函数便于直接单测: 即便调用方环境里已经有 GIT_TERMINAL_PROMPT=1,
// 追加在后面的 0 才是生效值(os/exec 的 dedupEnv 对重复 key 取最后一个).
func gitNonInteractiveEnviron(base []string) []string {
	env := make([]string, 0, len(base)+len(gitNonInteractiveEnv))
	env = append(env, base...)
	env = append(env, gitNonInteractiveEnv...)
	return env
}

// validateGitArg 挡掉空值和 "-" 开头的值
// 分支名/远程名/URL/revision 都会原样拼进 argv, 一个叫 "--force" 的分支名会被 git
// 当成选项解析(选项注入). 这里在拼命令之前就快速失败, 且失败时一条 git 都不会跑.
func validateGitArg(kind, v string) error {
	if strings.TrimSpace(v) == "" {
		return fmt.Errorf("git: empty %s", kind)
	}
	if strings.HasPrefix(v, "-") {
		return fmt.Errorf("git: invalid %s %q: must not start with '-'", kind, v)
	}
	return nil
}

// Fetch 从远程抓取(`git fetch [--prune] <remote>|--all`)
// remote 为空表示抓全部远程(--all). prune 为真时顺带删掉本地那些上游已消失的
// 远程跟踪引用 —— 这正是 ListBranches 里 Gone 标记的来源.
func (g *GitOps) Fetch(remote string, prune bool) error {
	args := []string{"fetch"}
	if prune {
		args = append(args, "--prune")
	}
	if remote == "" {
		args = append(args, "--all")
	} else {
		if err := validateGitArg("remote", remote); err != nil {
			return err
		}
		args = append(args, remote)
	}
	_, err := g.run(args...)
	return err
}

// Pull 拉取并合入(`git pull --rebase|--no-rebase [<remote> [<branch>]]`)
// 刻意总是显式给出 --rebase / --no-rebase: 二者都不给时, 若本地与上游已分叉且用户没配
// pull.rebase, 新版 git 会直接 fatal("need to specify how to reconcile divergent branches"),
// 由调用方明确选一种反而更可预期. remote 为空表示用当前分支配置的上游;
// branch 只有在给了 remote 时才有意义.
func (g *GitOps) Pull(remote, branch string, rebase bool) error {
	args := []string{"pull"}
	if rebase {
		args = append(args, "--rebase")
	} else {
		args = append(args, "--no-rebase")
	}
	if remote != "" {
		if err := validateGitArg("remote", remote); err != nil {
			return err
		}
		args = append(args, remote)
		if branch != "" {
			if err := validateGitArg("branch name", branch); err != nil {
				return err
			}
			args = append(args, branch)
		}
	}
	_, err := g.run(args...)
	return err
}

// Push 推送(`git push [--set-upstream] [--force-with-lease] [<remote> [<branch>]]`)
// setUpstream 即 "第一次推一个新分支并记住上游", 因此要求 remote 与 branch 都给全 ——
// `git push --set-upstream` 缺 refspec 时 git 自己也会报错, 这里提前挡掉.
// force 走 --force-with-lease 而不是 --force: 只有远程还停在我们上次见到的位置时才覆盖,
// 别人期间推过东西就拒绝, 不会静默吃掉他人的提交.
func (g *GitOps) Push(remote, branch string, setUpstream, force bool) error {
	if setUpstream && (remote == "" || branch == "") {
		return fmt.Errorf("git push: --set-upstream needs both remote and branch")
	}
	args := []string{"push"}
	if setUpstream {
		args = append(args, "--set-upstream")
	}
	if force {
		args = append(args, "--force-with-lease")
	}
	if remote != "" {
		if err := validateGitArg("remote", remote); err != nil {
			return err
		}
		args = append(args, remote)
		if branch != "" {
			if err := validateGitArg("branch name", branch); err != nil {
				return err
			}
			args = append(args, branch)
		}
	}
	_, err := g.run(args...)
	return err
}

// GitStash 是 `git stash list` 的一条记录
// Ref 是可直接喂给 StashPop/StashDrop 的选择子("stash@{0}");
// Index 是从 Ref 里取出的序号(0 是最新), 取不出来时为 -1.
// Message 是 reflog 主题: 自动保存形如 "WIP on main: abc1234 subject",
// 带 -m 保存则形如 "On main: <自定义信息>".
type GitStash struct {
	Ref     string
	Index   int
	Message string
	Author  string
	Date    string
}

// gitStashFormat 是 StashList 用的 --pretty 串
// %gd=短 reflog 选择子, %gs=reflog 主题, %an=作者, %as=作者日期(YYYY-MM-DD).
// 刻意用 %as 而不是 "%ad + --date=short": 一旦命令行上出现 --date=,
// %gd 会跟着改用日期形态渲染成 "stash@{2026-07-27}", Index 就再也解析不出来了.
const gitStashFormat = "--pretty=format:%gd%x1f%gs%x1f%an%x1f%as"

// StashList 列出全部 stash(`git stash list`)
func (g *GitOps) StashList() ([]GitStash, error) {
	out, err := g.run("stash", "list", gitStashFormat)
	if err != nil {
		return nil, err
	}
	return ParseGitStashList(out), nil
}

// ParseGitStashList 解析 gitStashFormat 的 0x1f 分隔输出(纯函数)
// 用 0x1f(unit separator)分隔四个字段, 避免 stash 信息里的普通标点导致误切.
// 空行跳过, 字段数不齐的行跳过, 序号解析失败时 Index 记 -1 而不丢掉整条.
func ParseGitStashList(out string) []GitStash {
	var stashes []GitStash
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\x1f")
		if len(parts) < 4 {
			continue
		}
		s := GitStash{
			Ref:     parts[0],
			Index:   -1,
			Message: parts[1],
			Author:  parts[2],
			Date:    parts[3],
		}
		if n, ok := parseGitStashIndex(parts[0]); ok {
			s.Index = n
		}
		stashes = append(stashes, s)
	}
	return stashes
}

// parseGitStashIndex 从 "stash@{3}" 里取出 3
// 取花括号里的十进制数; 形态不符或不是非负整数时返回 (0,false).
func parseGitStashIndex(ref string) (int, bool) {
	lo := strings.IndexByte(ref, '{')
	hi := strings.LastIndexByte(ref, '}')
	if lo < 0 || hi <= lo {
		return 0, false
	}
	n, err := strconv.Atoi(ref[lo+1 : hi])
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// StashPush 把当前改动存起来并还原工作树(`git stash push [-u] [-m <message>]`)
// message 为空(或全空白)时不带 -m, 让 git 生成默认的 "WIP on <branch>" 信息.
// includeUntracked 为真时连未跟踪文件一起收走(--include-untracked), 否则它们留在原地.
func (g *GitOps) StashPush(message string, includeUntracked bool) error {
	args := []string{"stash", "push"}
	if includeUntracked {
		args = append(args, "--include-untracked")
	}
	if m := strings.TrimSpace(message); m != "" {
		args = append(args, "-m", m)
	}
	_, err := g.run(args...)
	return err
}

// StashPop 弹出一个 stash 并从栈上删掉它(`git stash pop [<ref>]`)
// ref 为空表示最新的 stash(stash@{0}). 有冲突时 git 非零退出且保留该 stash,
// 错误原样返回, 冲突文件可以用 ConflictStatus 查.
func (g *GitOps) StashPop(ref string) error {
	args := []string{"stash", "pop"}
	if ref != "" {
		if err := validateGitArg("stash ref", ref); err != nil {
			return err
		}
		args = append(args, ref)
	}
	_, err := g.run(args...)
	return err
}

// StashDrop 丢弃一个 stash(`git stash drop [<ref>]`), ref 为空表示最新的那个
func (g *GitOps) StashDrop(ref string) error {
	args := []string{"stash", "drop"}
	if ref != "" {
		if err := validateGitArg("stash ref", ref); err != nil {
			return err
		}
		args = append(args, ref)
	}
	_, err := g.run(args...)
	return err
}

// RebaseStart 把当前分支变基到 upstream 上(`git rebase <upstream>`)
// 刻意不支持 -i: 交互式变基要一个能编辑 todo 列表的终端, 在 IDE 里只会变成挂死.
// 遇到冲突时 git 非零退出并停在"变基中"状态, 接着用 RebaseContinue/RebaseAbort 收场.
func (g *GitOps) RebaseStart(upstream string) error {
	if err := validateGitArg("upstream", upstream); err != nil {
		return err
	}
	_, err := g.run("rebase", upstream)
	return err
}

// RebaseContinue 解决冲突后继续变基(`git rebase --continue`)
// 依赖 GIT_EDITOR=true: 需要确认提交信息时不会卡在编辑器上, 直接沿用现有信息.
func (g *GitOps) RebaseContinue() error {
	_, err := g.run("rebase", "--continue")
	return err
}

// RebaseAbort 放弃变基并回到起点(`git rebase --abort`)
func (g *GitOps) RebaseAbort() error {
	_, err := g.run("rebase", "--abort")
	return err
}

// CherryPick 把一个提交摘到当前分支(`git cherry-pick <rev>`)
// rev 可以是 SHA、分支名或任何 revision 表达式. 冲突时非零退出并停在"摘取中"状态,
// 用 CherryPickContinue/CherryPickAbort 收场.
func (g *GitOps) CherryPick(rev string) error {
	if err := validateGitArg("revision", rev); err != nil {
		return err
	}
	_, err := g.run("cherry-pick", rev)
	return err
}

// CherryPickContinue 解决冲突后继续摘取(`git cherry-pick --continue`)
func (g *GitOps) CherryPickContinue() error {
	_, err := g.run("cherry-pick", "--continue")
	return err
}

// CherryPickAbort 放弃摘取并回到起点(`git cherry-pick --abort`)
func (g *GitOps) CherryPickAbort() error {
	_, err := g.run("cherry-pick", "--abort")
	return err
}

// MergeAbort 放弃正在进行的合并并还原工作树(`git merge --abort`)
// 没有合并在进行时 git 非零退出, 错误原样返回.
func (g *GitOps) MergeAbort() error {
	_, err := g.run("merge", "--abort")
	return err
}

// GitConflictStage 是一个冲突文件在某一 stage 上的 blob(`git ls-files -u` 的一行)
// Stage 取 1/2/3: 1=base(共同祖先), 2=ours(当前分支), 3=theirs(被合入的一侧).
// 缺哪个 stage 本身就是信息: 缺 1 是双方各自新增(add/add), 缺 2 是被我们删掉(deleted by us),
// 缺 3 是被对方删掉(deleted by them). Hash 可以直接喂给 GitShow 取出该版本内容.
type GitConflictStage struct {
	Mode  string // 文件模式, 如 "100644"
	Hash  string // blob SHA
	Stage int
}

// GitConflict 汇总一个未合并路径的全部 stage 记录
type GitConflict struct {
	Path   string
	Stages []GitConflictStage
}

// ConflictStatus 列出当前所有未合并(冲突)的路径(`git ls-files -u -z`)
// 合并/变基/摘取/stash pop 冲突后用它驱动冲突列表与三方合并编辑器.
// 无冲突时返回 (nil, nil) —— 这是正常状态, 不是错误.
// 加 -z 让路径按原样 NUL 分隔输出, 彻底绕开引号与转义.
func (g *GitOps) ConflictStatus() ([]GitConflict, error) {
	out, err := g.run("ls-files", "-u", "-z")
	if err != nil {
		return nil, err
	}
	return ParseGitUnmerged(out), nil
}

// ParseGitUnmerged 解析 `git ls-files -u` 的输出(纯函数)
// 每条记录形如 "<mode> <sha> <stage>\t<path>", 同一个路径会出现 1~3 条(stage 1/2/3);
// 这里按路径归并, 保持首次出现的顺序, 同一路径的 Stages 按输入顺序追加.
// 记录分隔符兼容两种形态(见 splitGitRecords): 带 -z 是 NUL, 不带是换行.
// 不带 -z 时含特殊字符的路径会被 git 加引号并做 C 风格转义, 统一过 DecodeGitQuotedPath.
// 畸形记录(没有 TAB、字段数不对、stage 不在 1..3)跳过并继续, 永远不 panic.
func ParseGitUnmerged(out string) []GitConflict {
	var conflicts []GitConflict
	at := make(map[string]int)
	for _, rec := range splitGitRecords(out) {
		meta, path, ok := strings.Cut(rec, "\t")
		if !ok {
			continue
		}
		fields := strings.Fields(meta)
		if len(fields) != 3 {
			continue
		}
		stage, err := strconv.Atoi(fields[2])
		if err != nil || stage < 1 || stage > 3 {
			continue
		}
		path = DecodeGitQuotedPath(path)
		if path == "" {
			continue
		}
		idx, seen := at[path]
		if !seen {
			conflicts = append(conflicts, GitConflict{Path: path})
			idx = len(conflicts) - 1
			at[path] = idx
		}
		conflicts[idx].Stages = append(conflicts[idx].Stages, GitConflictStage{
			Mode:  fields[0],
			Hash:  fields[1],
			Stage: stage,
		})
	}
	return conflicts
}

// splitGitRecords 把 git 的记录流切成若干条, 空记录丢掉
// 只要输出里出现过 NUL 就认定是 -z 形态, 于是只按 NUL 切 —— 不能顺手也按换行切:
// -z 下路径是原样输出的, 而文件名里放一个换行在 Unix 上完全合法, 那样切会把一条撕成两条.
// 没有 NUL 时才按换行切.
func splitGitRecords(out string) []string {
	sep := "\n"
	if strings.IndexByte(out, 0) >= 0 {
		sep = "\x00"
	}
	parts := strings.Split(out, sep)
	recs := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		recs = append(recs, p)
	}
	return recs
}
