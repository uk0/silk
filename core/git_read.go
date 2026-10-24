package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/uk0/silk/internal/processutil"
)

var (
	ErrGitUnavailable     = errors.New("Git executable unavailable")
	ErrGitNotRepository   = errors.New("not a Git working tree")
	ErrGitReadLimit       = errors.New("Git read output exceeds limit")
	ErrGitStatusMalformed = errors.New("malformed Git status output")
)

// GitStatusPorcelainContext is a cancelable, bounded, read-only status query.
// Paths are lossless and relative to the repository root (not necessarily dir).
// It uses porcelain v1 -z: rename records contain destination then source.
// Run off the UI thread; errors never return a partial/clean-looking snapshot.
func GitStatusPorcelainContext(ctx context.Context, dir string) ([]GitStatusEntry, error) {
	out, err := runGitReadContext(ctx, dir, "status", "--porcelain=v1", "-z")
	if err != nil {
		return nil, err
	}
	return parseGitStatusZ(out)
}

func parseGitStatusZ(out string) ([]GitStatusEntry, error) {
	var rows []GitStatusEntry
	for out != "" {
		row, rest, ok := strings.Cut(out, "\x00")
		if !ok || len(row) < 4 || row[2] != ' ' {
			return nil, ErrGitStatusMalformed
		}
		e := GitStatusEntry{Staged: row[0], Unstaged: row[1], Path: row[3:]}
		out = rest
		if e.Staged == 'R' || e.Staged == 'C' || e.Unstaged == 'R' || e.Unstaged == 'C' {
			e.OrigPath, out, ok = strings.Cut(out, "\x00")
			if !ok || e.OrigPath == "" {
				return nil, ErrGitStatusMalformed
			}
		}
		if len(rows) >= 100000 {
			return nil, ErrGitReadLimit
		}
		rows = append(rows, e)
	}
	return rows, nil
}

// GitRepositoryRootContext resolves the working-tree root used by status paths.
// It does not initialize a repository or change process cwd/environment.
func GitRepositoryRootContext(ctx context.Context, dir string) (string, error) {
	out, err := runGitReadContext(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	root := strings.TrimSuffix(out, "\n")
	if root == "" {
		return "", ErrGitNotRepository
	}
	return root, nil
}

// GitShortLogContext is the bounded/cancelable history API. A valid unborn
// branch has no commits; unavailable Git, non-repositories and other failures
// remain errors. Limit must be positive and no greater than 10,000.
func GitShortLogContext(ctx context.Context, dir string, n int) ([]GitCommit, error) {
	if n < 1 || n > 10000 {
		return nil, fmt.Errorf("Git log limit must be between 1 and 10000")
	}
	out, err := runGitReadContext(ctx, dir, "log", fmt.Sprintf("-n%d", n), "--pretty=format:%h%x1f%s%x1f%an%x1f%ad", "--date=short")
	if err != nil {
		return nil, err
	}
	return parseShortLog(out), nil
}

type gitReadCapture struct {
	buffer   bytes.Buffer
	limit    int
	exceeded bool
	cancel   context.CancelFunc
}

func (w *gitReadCapture) Write(p []byte) (int, error) {
	if len(p) > w.limit-w.buffer.Len() {
		n := max(0, w.limit-w.buffer.Len())
		_, _ = w.buffer.Write(p[:n])
		w.exceeded = true
		w.cancel()
		return n, ErrGitReadLimit
	}
	return w.buffer.Write(p)
}
func gitReadEnvironment(env []string) []string {
	out := make([]string, 0, len(env)+3)
	for _, v := range env {
		k, _, _ := strings.Cut(v, "=")
		switch strings.ToUpper(k) {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR", "GIT_PREFIX", "GIT_NAMESPACE", "GIT_OPTIONAL_LOCKS", "GIT_TERMINAL_PROMPT", "LC_ALL":
			continue
		}
		out = append(out, v)
	}
	return append(out, "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")
}
func runGitReadContext(parent context.Context, dir string, args ...string) (string, error) {
	if err := parent.Err(); err != nil {
		return "", err
	}
	path, err := exec.LookPath("git")
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrGitUnavailable, err)
	}
	ctx, cancel := context.WithTimeout(parent, gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, append([]string{"--no-optional-locks", "-c", "core.pager=cat"}, args...)...)
	cmd.Dir = dir
	cmd.Env = gitReadEnvironment(os.Environ())
	return runGitReadCommand(ctx, cancel, cmd, args)
}
func runGitReadCommand(ctx context.Context, cancel context.CancelFunc, cmd *exec.Cmd, args []string) (string, error) {
	out := &gitReadCapture{limit: 8 << 20, cancel: cancel}
	stderr := &gitReadCapture{limit: 64 << 10, cancel: cancel}
	cmd.Stdout, cmd.Stderr = out, stderr
	cmd.WaitDelay = 250 * time.Millisecond
	processutil.NoConsoleWindow(cmd)
	err := cmd.Run()
	if out.exceeded || stderr.exceeded {
		return "", ErrGitReadLimit
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil {
		msg := firstLine(stderr.buffer.String())
		if strings.HasPrefix(msg, "fatal: not a git repository") {
			return "", fmt.Errorf("%w: %s", ErrGitNotRepository, msg)
		}
		if len(args) > 0 && args[0] == "log" && strings.HasPrefix(msg, "fatal: your current branch ") && strings.HasSuffix(msg, "does not have any commits yet") {
			return "", nil
		}
		return "", fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), msg, err)
	}
	return out.buffer.String(), nil
}
