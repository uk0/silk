package core

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

// 本文件与 gitrefs_test.go 共用下面这套假 runner: 每个操作只被断言"拼出了什么 argv",
// 一条真 git 都不跑 —— 既让测试在没有仓库/没有网络的机器上也稳定, 也顺带证明
// GitRunner 这个注入点真的把执行完全隔离了.

// gitArgvRecorder 是注入给 GitOps.Runner 的假执行器
// 记录每次调用的工作目录与 argv, 并回放预置的 stdout / error.
type gitArgvRecorder struct {
	dirs  []string
	calls [][]string
	out   string
	err   error
}

// run 满足 GitRunner 签名
func (r *gitArgvRecorder) run(dir string, args ...string) (string, error) {
	r.dirs = append(r.dirs, dir)
	r.calls = append(r.calls, slices.Clone(args))
	return r.out, r.err
}

// newRecordedGitOps 返回一个只会调用假 runner 的 GitOps
func newRecordedGitOps(out string) (*GitOps, *gitArgvRecorder) {
	rec := &gitArgvRecorder{out: out}
	return &GitOps{Dir: "/repo", Runner: rec.run}, rec
}

// wantOneCall 断言恰好发生了一次调用, 且 argv 与 want 逐项相等
func (r *gitArgvRecorder) wantOneCall(t *testing.T, want ...string) {
	t.Helper()
	if len(r.calls) != 1 {
		t.Fatalf("got %d git calls, want exactly 1: %v", len(r.calls), r.calls)
	}
	if !slices.Equal(r.calls[0], want) {
		t.Fatalf("argv mismatch:\n got: %q\nwant: %q", r.calls[0], want)
	}
	if r.dirs[0] != "/repo" {
		t.Fatalf("dir = %q, want /repo", r.dirs[0])
	}
}

func TestGitOpsArgv(t *testing.T) {
	tests := []struct {
		name string
		call func(*GitOps) error
		want []string
	}{
		{"fetch all", func(g *GitOps) error { return g.Fetch("", false) },
			[]string{"fetch", "--all"}},
		{"fetch prune remote", func(g *GitOps) error { return g.Fetch("origin", true) },
			[]string{"fetch", "--prune", "origin"}},

		{"pull merge", func(g *GitOps) error { return g.Pull("", "", false) },
			[]string{"pull", "--no-rebase"}},
		{"pull rebase remote branch", func(g *GitOps) error { return g.Pull("origin", "main", true) },
			[]string{"pull", "--rebase", "origin", "main"}},
		{"pull remote only", func(g *GitOps) error { return g.Pull("origin", "", false) },
			[]string{"pull", "--no-rebase", "origin"}},

		{"push plain", func(g *GitOps) error { return g.Push("", "", false, false) },
			[]string{"push"}},
		{"push set upstream", func(g *GitOps) error { return g.Push("origin", "feature", true, false) },
			[]string{"push", "--set-upstream", "origin", "feature"}},
		{"push force with lease", func(g *GitOps) error { return g.Push("origin", "", false, true) },
			[]string{"push", "--force-with-lease", "origin"}},

		{"stash push", func(g *GitOps) error { return g.StashPush("", false) },
			[]string{"stash", "push"}},
		{"stash push message and untracked", func(g *GitOps) error { return g.StashPush("  wip  ", true) },
			[]string{"stash", "push", "--include-untracked", "-m", "wip"}},
		{"stash list", func(g *GitOps) error { _, err := g.StashList(); return err },
			[]string{"stash", "list", gitStashFormat}},
		{"stash pop latest", func(g *GitOps) error { return g.StashPop("") },
			[]string{"stash", "pop"}},
		{"stash pop ref", func(g *GitOps) error { return g.StashPop("stash@{2}") },
			[]string{"stash", "pop", "stash@{2}"}},
		{"stash drop", func(g *GitOps) error { return g.StashDrop("stash@{1}") },
			[]string{"stash", "drop", "stash@{1}"}},

		{"rebase start", func(g *GitOps) error { return g.RebaseStart("origin/main") },
			[]string{"rebase", "origin/main"}},
		{"rebase continue", func(g *GitOps) error { return g.RebaseContinue() },
			[]string{"rebase", "--continue"}},
		{"rebase abort", func(g *GitOps) error { return g.RebaseAbort() },
			[]string{"rebase", "--abort"}},

		{"cherry-pick", func(g *GitOps) error { return g.CherryPick("abc1234") },
			[]string{"cherry-pick", "abc1234"}},
		{"cherry-pick continue", func(g *GitOps) error { return g.CherryPickContinue() },
			[]string{"cherry-pick", "--continue"}},
		{"cherry-pick abort", func(g *GitOps) error { return g.CherryPickAbort() },
			[]string{"cherry-pick", "--abort"}},

		{"merge abort", func(g *GitOps) error { return g.MergeAbort() },
			[]string{"merge", "--abort"}},
		{"conflict status", func(g *GitOps) error { _, err := g.ConflictStatus(); return err },
			[]string{"ls-files", "-u", "-z"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g, rec := newRecordedGitOps("")
			if err := tc.call(g); err != nil {
				t.Fatalf("call returned error: %v", err)
			}
			rec.wantOneCall(t, tc.want...)
		})
	}
}

// TestGitOpsRejectsBadArgs 覆盖参数校验: 报错且一条 git 都不发
// 重点是选项注入 —— 叫 "--force" 的分支名或 "-x" 的 revision 绝不能溜进 argv.
func TestGitOpsRejectsBadArgs(t *testing.T) {
	tests := []struct {
		name string
		call func(*GitOps) error
	}{
		{"fetch dash remote", func(g *GitOps) error { return g.Fetch("--all", false) }},
		{"pull dash remote", func(g *GitOps) error { return g.Pull("-x", "", false) }},
		{"pull dash branch", func(g *GitOps) error { return g.Pull("origin", "-x", false) }},
		{"push upstream without branch", func(g *GitOps) error { return g.Push("origin", "", true, false) }},
		{"push upstream without remote", func(g *GitOps) error { return g.Push("", "main", true, false) }},
		{"stash pop dash ref", func(g *GitOps) error { return g.StashPop("-x") }},
		{"stash drop dash ref", func(g *GitOps) error { return g.StashDrop("--all") }},
		{"rebase empty upstream", func(g *GitOps) error { return g.RebaseStart("  ") }},
		{"rebase dash upstream", func(g *GitOps) error { return g.RebaseStart("-i") }},
		{"cherry-pick empty", func(g *GitOps) error { return g.CherryPick("") }},
		{"cherry-pick dash", func(g *GitOps) error { return g.CherryPick("--continue") }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g, rec := newRecordedGitOps("")
			if err := tc.call(g); err == nil {
				t.Fatal("want error, got nil")
			}
			if len(rec.calls) != 0 {
				t.Fatalf("git was invoked despite invalid args: %v", rec.calls)
			}
		})
	}
}

// TestGitOpsPropagatesRunnerError 断言 runner 的错误原样上浮(可用 errors.Is 判定)
func TestGitOpsPropagatesRunnerError(t *testing.T) {
	boom := errors.New("boom")
	g, rec := newRecordedGitOps("")
	rec.err = boom

	if err := g.Fetch("origin", false); !errors.Is(err, boom) {
		t.Fatalf("Fetch error = %v, want %v", err, boom)
	}
	if _, err := g.ConflictStatus(); !errors.Is(err, boom) {
		t.Fatalf("ConflictStatus error = %v, want %v", err, boom)
	}
	if _, err := g.StashList(); !errors.Is(err, boom) {
		t.Fatalf("StashList error = %v, want %v", err, boom)
	}
}

// TestNewGitOpsDefaults 默认句柄不注入 runner(走真实 git), 超时留 0 表示用 gitOpsTimeout
func TestNewGitOpsDefaults(t *testing.T) {
	g := NewGitOps("/tmp/repo")
	if g.Dir != "/tmp/repo" {
		t.Fatalf("Dir = %q", g.Dir)
	}
	if g.Runner != nil {
		t.Fatal("Runner should default to nil (real git)")
	}
	if g.Timeout != 0 {
		t.Fatalf("Timeout = %v, want 0 (means gitOpsTimeout)", g.Timeout)
	}
}

// TestGitNonInteractiveEnviron 非交互保证: 覆盖项追加在最后, 因此即便调用方环境里
// 已经有 GIT_TERMINAL_PROMPT=1 / 中文 locale, 生效值也是我们这套(os/exec 对重复 key 取最后一个).
func TestGitNonInteractiveEnviron(t *testing.T) {
	base := []string{"PATH=/usr/bin", "GIT_TERMINAL_PROMPT=1", "LC_ALL=zh_CN.UTF-8", "GIT_EDITOR=vim"}
	env := gitNonInteractiveEnviron(base)

	// base 必须整段保留(只追加, 不改写)
	if !slices.Equal(env[:len(base)], base) {
		t.Fatalf("base env not preserved: %q", env[:len(base)])
	}
	want := map[string]string{
		"GIT_TERMINAL_PROMPT": "0",
		"GIT_ASKPASS":         "",
		"SSH_ASKPASS":         "",
		"GIT_EDITOR":          "true",
		"GIT_SEQUENCE_EDITOR": "true",
		"LC_ALL":              "C",
		"PATH":                "/usr/bin",
	}
	for k, v := range want {
		if got, ok := lastEnvValue(env, k); !ok || got != v {
			t.Fatalf("%s = %q (present=%v), want %q", k, got, ok, v)
		}
	}
}

// lastEnvValue 取 env 里某个 key 最后一次出现的值(os/exec 去重时保留的那个)
func lastEnvValue(env []string, key string) (string, bool) {
	value, found := "", false
	for _, kv := range env {
		k, v, ok := strings.Cut(kv, "=")
		if ok && k == key {
			value, found = v, true
		}
	}
	return value, found
}

// TestGitStashFormatHasNoDateOption 守一个真实踩过的坑:
// 命令行上一旦出现 --date=, %gd 会改用日期渲染成 "stash@{2026-07-27}",
// ParseGitStashList 就再也取不出 Index, StashPop/StashDrop 也拿不到可用的 ref.
// 所以 gitStashFormat 用 %as 自带日期, StashList 的 argv 里不能有 --date.
func TestGitStashFormatHasNoDateOption(t *testing.T) {
	if strings.Contains(gitStashFormat, "--date") {
		t.Fatalf("gitStashFormat must not carry --date: %q", gitStashFormat)
	}
	g, rec := newRecordedGitOps("")
	if _, err := g.StashList(); err != nil {
		t.Fatalf("StashList: %v", err)
	}
	for _, arg := range rec.calls[0] {
		if strings.HasPrefix(arg, "--date") {
			t.Fatalf("StashList argv must not carry --date: %q", rec.calls[0])
		}
	}
}

func TestParseGitStashList(t *testing.T) {
	// 取自真实 `git stash list --pretty=format:%gd%x1f%gs%x1f%an%x1f%as`
	out := strings.Join([]string{
		"stash@{0}\x1fWIP on main: eba6b3d c1\x1fTest User\x1f2026-07-27",
		"stash@{1}\x1fOn main: my work\x1fTest User\x1f2026-07-26",
		"stash@{oops}\x1fOn main: broken selector\x1fTest User\x1f2026-07-25",
		"missing-fields",
		"",
	}, "\n")

	got := ParseGitStashList(out)
	want := []GitStash{
		{Ref: "stash@{0}", Index: 0, Message: "WIP on main: eba6b3d c1", Author: "Test User", Date: "2026-07-27"},
		{Ref: "stash@{1}", Index: 1, Message: "On main: my work", Author: "Test User", Date: "2026-07-26"},
		{Ref: "stash@{oops}", Index: -1, Message: "On main: broken selector", Author: "Test User", Date: "2026-07-25"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d stashes, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("stash[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestParseGitUnmerged 解析 `git ls-files -u` 的换行形态
// 输入取自一次真实的 content conflict(c.txt 三个 stage 全在)加上一个被引号转义的
// 非 ASCII 路径(只有 stage 2/3, 即双方各自新增), 再混两条畸形记录.
func TestParseGitUnmerged(t *testing.T) {
	out := strings.Join([]string{
		"100644 df967b96a579e45a18b8251732d16804b2e56a55 1\tc.txt",
		"100644 b19a1e93bec1317dc6097229e12afaffbfa74dc2 2\tc.txt",
		"100644 950b81b7eee953d050aa05a641f8e056c85dd1bd 3\tc.txt",
		`100644 1111111111111111111111111111111111111111 2` + "\t" + `"\346\265\213\350\257\225.txt"`,
		`100644 2222222222222222222222222222222222222222 3` + "\t" + `"\346\265\213\350\257\225.txt"`,
		"this record has no tab at all",
		"100644 3333333333333333333333333333333333333333 9\tbogus-stage.txt",
		"100644 4444444444444444444444444444444444444444\tmissing-stage.txt",
		"",
	}, "\n")

	got := ParseGitUnmerged(out)
	if len(got) != 2 {
		t.Fatalf("got %d conflicts, want 2: %+v", len(got), got)
	}

	if got[0].Path != "c.txt" {
		t.Errorf("conflict[0].Path = %q, want c.txt", got[0].Path)
	}
	wantStages := []GitConflictStage{
		{Mode: "100644", Hash: "df967b96a579e45a18b8251732d16804b2e56a55", Stage: 1},
		{Mode: "100644", Hash: "b19a1e93bec1317dc6097229e12afaffbfa74dc2", Stage: 2},
		{Mode: "100644", Hash: "950b81b7eee953d050aa05a641f8e056c85dd1bd", Stage: 3},
	}
	if !slices.Equal(got[0].Stages, wantStages) {
		t.Errorf("conflict[0].Stages = %+v, want %+v", got[0].Stages, wantStages)
	}

	// 非 ASCII 路径在不带 -z 的输出里是 C 风格转义的, 必须被还原
	if got[1].Path != "测试.txt" {
		t.Errorf("conflict[1].Path = %q, want 测试.txt", got[1].Path)
	}
	if len(got[1].Stages) != 2 || got[1].Stages[0].Stage != 2 || got[1].Stages[1].Stage != 3 {
		t.Errorf("conflict[1].Stages = %+v, want stages 2 and 3", got[1].Stages)
	}
}

// TestParseGitUnmergedNUL -z 形态: 只按 NUL 切记录
// 特意放一个文件名里带换行的冲突文件 —— 这正是 -z 存在的理由, 按换行切会把它撕成两条.
func TestParseGitUnmergedNUL(t *testing.T) {
	out := "100644 aaaaaaa 2\tweird\nname.txt\x00" +
		"100644 bbbbbbb 3\tweird\nname.txt\x00" +
		"100644 ccccccc 2\tplain.txt\x00"

	got := ParseGitUnmerged(out)
	if len(got) != 2 {
		t.Fatalf("got %d conflicts, want 2: %+v", len(got), got)
	}
	if got[0].Path != "weird\nname.txt" {
		t.Errorf("conflict[0].Path = %q, want the newline-bearing name intact", got[0].Path)
	}
	if len(got[0].Stages) != 2 {
		t.Errorf("conflict[0].Stages = %+v, want 2 stages", got[0].Stages)
	}
	if got[1].Path != "plain.txt" {
		t.Errorf("conflict[1].Path = %q, want plain.txt", got[1].Path)
	}
}

func TestParseGitUnmergedEmpty(t *testing.T) {
	if got := ParseGitUnmerged(""); got != nil {
		t.Fatalf("no conflicts should parse to nil, got %+v", got)
	}
}

func TestSplitGitRecords(t *testing.T) {
	// 有 NUL 就只按 NUL 切(路径里的换行必须留着)
	got := splitGitRecords("a\nb\x00c\x00")
	if !slices.Equal(got, []string{"a\nb", "c"}) {
		t.Fatalf("NUL split = %q", got)
	}
	// 没有 NUL 才按换行切, 空记录丢掉
	got = splitGitRecords("a\n\nb\n")
	if !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("newline split = %q", got)
	}
}
