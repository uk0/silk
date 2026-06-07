package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupRepo 在一个全新的 t.TempDir() 里建一个真实 git 仓库并落一个首提交
// 故意不复用 silk 仓库本身(我们正处在它的某个 worktree 里), 用 TempDir 保证
// 测试与外部仓库状态完全隔离, 不会被嵌套仓库/worktree 状态干扰.
// 返回仓库目录和初始提交里那个文件的相对路径.
func setupRepo(t *testing.T) (dir, file string) {
	t.Helper()
	dir = t.TempDir()

	// init 后把 user.email/user.name 设到本地配置, 否则 commit 会因缺身份失败.
	// 用 git config --local 写进 .git/config, 不污染全局.
	mustGit(t, dir, "init")
	mustGit(t, dir, "config", "--local", "user.email", "test@example.com")
	mustGit(t, dir, "config", "--local", "user.name", "Test User")

	file = "hello.txt"
	writeFile(t, dir, file, "line one\nline two\n")
	mustGit(t, dir, "add", file)
	mustGit(t, dir, "commit", "-m", "initial commit")
	return dir, file
}

// mustGit 在 dir 下跑一条 git 命令, 失败即 Fatal(带上合并输出便于排查)
func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// writeFile 把内容写到 dir/rel
func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, rel), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func TestGitAvailable(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not installed")
	}
	// 在 dev box 上应当可用; 这里只断言不 panic 且返回 true
	if !GitAvailable() {
		t.Fatal("GitAvailable returned false on a box that has git")
	}
}

func TestIsGitRepo(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not installed")
	}
	dir, _ := setupRepo(t)
	if !IsGitRepo(dir) {
		t.Errorf("IsGitRepo(%q) = false, want true", dir)
	}

	// 一个全新的空 TempDir 不是仓库
	bare := t.TempDir()
	if IsGitRepo(bare) {
		t.Errorf("IsGitRepo(%q) = true for non-repo tempdir, want false", bare)
	}
}

func TestGitCurrentBranch(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not installed")
	}
	dir, _ := setupRepo(t)
	branch, err := GitCurrentBranch(dir)
	if err != nil {
		t.Fatalf("GitCurrentBranch: %v", err)
	}
	// 不硬编码 main vs master(git 默认分支名因版本/配置而异), 只要非空即可
	if branch == "" {
		t.Error("GitCurrentBranch returned empty branch name")
	}
}

func TestGitDiffHead(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not installed")
	}
	dir, file := setupRepo(t)

	// 干净工作树: 无 diff
	clean, err := GitDiffHead(dir)
	if err != nil {
		t.Fatalf("GitDiffHead (clean): %v", err)
	}
	if strings.TrimSpace(clean) != "" {
		t.Errorf("GitDiffHead on clean tree = %q, want empty", clean)
	}

	// 改动已跟踪文件后应当出现 diff
	writeFile(t, dir, file, "line one\nline two CHANGED\nline three\n")
	diff, err := GitDiffHead(dir)
	if err != nil {
		t.Fatalf("GitDiffHead (dirty): %v", err)
	}
	if !strings.Contains(diff, "CHANGED") {
		t.Errorf("GitDiffHead output missing changed line; got:\n%s", diff)
	}

	// 集成 sanity: 输出能被已合并的 ParseUnifiedDiff 解析
	files, perr := ParseUnifiedDiff(diff)
	if perr != nil {
		t.Fatalf("ParseUnifiedDiff(GitDiffHead): %v", perr)
	}
	if len(files) != 1 {
		t.Fatalf("ParseUnifiedDiff returned %d files, want 1", len(files))
	}
	if got := files[0].NewPath; got != file {
		t.Errorf("parsed NewPath = %q, want %q", got, file)
	}

	// 单文件 diff 也应当包含改动
	fdiff, err := GitDiffFile(dir, file)
	if err != nil {
		t.Fatalf("GitDiffFile: %v", err)
	}
	if !strings.Contains(fdiff, "CHANGED") {
		t.Errorf("GitDiffFile output missing changed line; got:\n%s", fdiff)
	}
}

func TestGitStatusPorcelain(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not installed")
	}
	dir, file := setupRepo(t)

	// 改一个已跟踪文件 + 新增一个未跟踪文件
	writeFile(t, dir, file, "line one\nmodified\n")
	writeFile(t, dir, "untracked.txt", "brand new\n")

	entries, err := GitStatusPorcelain(dir)
	if err != nil {
		t.Fatalf("GitStatusPorcelain: %v", err)
	}

	byPath := map[string]GitStatusEntry{}
	for _, e := range entries {
		byPath[e.Path] = e
	}

	// 已跟踪但未暂存的改动: Y(Unstaged)列应为 'M'
	mod, ok := byPath[file]
	if !ok {
		t.Fatalf("status missing entry for %q; entries=%+v", file, entries)
	}
	if mod.Unstaged != 'M' {
		t.Errorf("entry for %q Unstaged = %q, want 'M'", file, string(mod.Unstaged))
	}

	// 未跟踪文件: 两列都应为 '?'
	un, ok := byPath["untracked.txt"]
	if !ok {
		t.Fatalf("status missing entry for untracked.txt; entries=%+v", entries)
	}
	if un.Staged != '?' || un.Unstaged != '?' {
		t.Errorf("untracked entry = %q%q, want '??'", string(un.Staged), string(un.Unstaged))
	}
}

func TestGitStatusPorcelainRename(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not installed")
	}
	dir, file := setupRepo(t)

	// 暂存一次重命名, 让 porcelain 输出 "R  old -> new" 形式
	mustGit(t, dir, "mv", file, "renamed.txt")

	entries, err := GitStatusPorcelain(dir)
	if err != nil {
		t.Fatalf("GitStatusPorcelain (rename): %v", err)
	}

	var found bool
	for _, e := range entries {
		if e.Path == "renamed.txt" {
			found = true
			if e.Staged != 'R' {
				t.Errorf("rename entry Staged = %q, want 'R'", string(e.Staged))
			}
			if e.OrigPath != file {
				t.Errorf("rename entry OrigPath = %q, want %q", e.OrigPath, file)
			}
		}
	}
	if !found {
		t.Errorf("status missing renamed.txt entry; entries=%+v", entries)
	}
}

func TestGitShortLog(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not installed")
	}
	dir, _ := setupRepo(t)

	commits, err := GitShortLog(dir, 5)
	if err != nil {
		t.Fatalf("GitShortLog: %v", err)
	}
	if len(commits) != 1 {
		t.Fatalf("GitShortLog returned %d commits, want 1", len(commits))
	}
	c := commits[0]
	if c.Subject != "initial commit" {
		t.Errorf("commit Subject = %q, want %q", c.Subject, "initial commit")
	}
	if c.Hash == "" {
		t.Error("commit Hash is empty")
	}
	if c.Author != "Test User" {
		t.Errorf("commit Author = %q, want %q", c.Author, "Test User")
	}
	if c.Date == "" {
		t.Error("commit Date is empty")
	}
}

func TestRunGitBogusSubcommandErrors(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not installed")
	}
	dir, _ := setupRepo(t)

	// 超时路径无法稳定地人为触发挂死; 这里验证非零退出被转成 error 而非 panic.
	// runGit 是私有的, 同包测试可直接调用.
	out, err := runGit(dir, "this-is-not-a-real-git-subcommand")
	if err == nil {
		t.Fatalf("runGit with bogus subcommand returned nil error; out=%q", out)
	}
	// error 应当带上命令上下文(stderr 首行被并入)
	if !strings.Contains(err.Error(), "git this-is-not-a-real-git-subcommand") {
		t.Errorf("error missing command context: %v", err)
	}
}
