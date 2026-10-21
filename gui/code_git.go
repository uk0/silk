package gui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/uk0/silk/diagnostic"
	"github.com/uk0/silk/internal/processutil"
)

type GitLineStatus int

const (
	GitUnchanged GitLineStatus = iota
	GitAdded
	GitModified
	GitDeleted
)
const gitDiffOutputLimit = 2 << 20
const gitDiffMarkerLimit = 100000
const gitDiffTimeout = 2 * time.Second

var errGitOutputLimit = errors.New("git output exceeds capture limit")
var errGitHunkInvalid = errors.New("invalid git hunk")

// GitDiff is the synchronous compatibility API. It suppresses errors; new
// non-UI callers should use GitDiffContext. Editor setters never call this API.
func GitDiff(filePath string) map[int]GitLineStatus {
	result, _ := GitDiffContext(context.Background(), filePath)
	if result == nil {
		result = map[int]GitLineStatus{}
	}
	return result
}

// GitDiffContext computes saved working-tree vs index markers, not unsaved
// buffer or staged-vs-HEAD changes. It is synchronous and must run off the UI.
// The whole request has a two-second budget, bounded captured output/markers,
// literal pathspecs and no external diff/textconv. An ordinary non-repository is
// an empty successful result; other failures preserve a stable code and cause.
func GitDiffContext(parent context.Context, filePath string) (map[int]GitLineStatus, error) {
	if err := parent.Err(); err != nil {
		return nil, err
	}
	if filePath == "" {
		return map[int]GitLineStatus{}, nil
	}
	path, err := filepath.Abs(filePath)
	if err != nil {
		return nil, gitDiffError(diagnostic.GitDiffFailed, filePath, err)
	}
	path = filepath.Clean(path)
	ctx, cancel := context.WithTimeout(parent, gitDiffTimeout)
	defer cancel()
	run := func(args ...string) ([]byte, []byte, error) {
		cmd := exec.CommandContext(ctx, "git", append([]string{"--literal-pathspecs", "-c", "core.pager=cat", "-c", "core.quotePath=false"}, args...)...)
		cmd.Dir = filepath.Dir(path)
		cmd.Env = gitDiffEnvironment(os.Environ())
		return runGitDiffCommand(ctx, cancel, cmd)
	}
	stdout, stderr, err := run("rev-parse", "--is-inside-work-tree")
	if err != nil {
		// Fix locale before identifying this documented, non-error editor state.
		if ctx.Err() == nil && !errors.Is(err, errGitOutputLimit) && strings.HasPrefix(strings.TrimSpace(string(stderr)), "fatal: not a git repository") {
			return map[int]GitLineStatus{}, nil
		}
		return nil, gitCommandError(parent, ctx, path, stderr, err)
	}
	if strings.TrimSpace(string(stdout)) != "true" {
		return map[int]GitLineStatus{}, nil
	}
	stdout, stderr, err = run("diff", "--no-ext-diff", "--no-textconv", "--ignore-submodules", "--unified=0", "--no-color", "--", filepath.Base(path))
	if err != nil {
		return nil, gitCommandError(parent, ctx, path, stderr, err)
	}
	status, err := parseGitLineStatuses(ctx, string(stdout))
	if err != nil {
		if parent.Err() != nil {
			return nil, parent.Err()
		}
		code := diagnostic.GitDiffLimit
		if ctx.Err() != nil || errors.Is(err, errGitHunkInvalid) {
			code = diagnostic.GitDiffFailed
		}
		return nil, gitDiffError(code, path, err)
	}
	return status, nil
}
func gitDiffEnvironment(env []string) []string {
	out := make([]string, 0, len(env)+4)
	for _, v := range env {
		k, _, _ := strings.Cut(v, "=")
		switch strings.ToUpper(k) {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR", "GIT_PREFIX", "GIT_NAMESPACE", "LC_ALL", "GIT_OPTIONAL_LOCKS", "GIT_TERMINAL_PROMPT":
			continue
		}
		out = append(out, v)
	}
	return append(out, "LC_ALL=C", "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0")
}

// os/exec owns each capture buffer's single writer. Run joins all pipe-copy
// goroutines before any buffer read. Overflow cancels the direct child; Run
// waits/reaps it. WaitDelay bounds inherited pipe handles after child exit.
type gitDiffCapture struct {
	buffer   bytes.Buffer
	limit    int
	overflow bool
	cancel   context.CancelFunc
}

func (w *gitDiffCapture) Write(p []byte) (int, error) {
	if len(p) > w.limit-w.buffer.Len() {
		remaining := max(0, w.limit-w.buffer.Len())
		_, _ = w.buffer.Write(p[:remaining])
		w.overflow = true
		w.cancel()
		return remaining, errGitOutputLimit
	}
	return w.buffer.Write(p)
}
func runGitDiffCommand(ctx context.Context, cancel context.CancelFunc, cmd *exec.Cmd) ([]byte, []byte, error) {
	out := &gitDiffCapture{limit: gitDiffOutputLimit, cancel: cancel}
	errout := &gitDiffCapture{limit: 64 << 10, cancel: cancel}
	cmd.Stdout, cmd.Stderr = out, errout
	cmd.WaitDelay = 250 * time.Millisecond
	processutil.NoConsoleWindow(cmd)
	err := cmd.Run()
	if out.overflow || errout.overflow {
		err = errGitOutputLimit
	} else if ctx.Err() != nil {
		err = ctx.Err()
	}
	return out.buffer.Bytes(), errout.buffer.Bytes(), err
}
func gitDiffError(code diagnostic.Code, path string, err error) error {
	return diagnostic.New(code, "", err).At(diagnostic.Location{File: path})
}
func gitCommandError(parent, ctx context.Context, path string, stderr []byte, err error) error {
	if parent.Err() != nil {
		return parent.Err()
	}
	code := diagnostic.GitDiffFailed
	if errors.Is(err, errGitOutputLimit) {
		code = diagnostic.GitDiffLimit
	} else if errors.Is(err, exec.ErrNotFound) {
		code = diagnostic.GitUnavailable
	}
	if ctx.Err() != nil && code != diagnostic.GitDiffLimit {
		err = ctx.Err()
	}
	if message := strings.TrimSpace(string(stderr)); message != "" {
		err = fmt.Errorf("%w: %s", err, message)
	}
	return gitDiffError(code, path, err)
}

func parseGitLineStatuses(ctx context.Context, diff string) (map[int]GitLineStatus, error) {
	result := make(map[int]GitLineStatus)
	count := 0
	for line := range strings.SplitSeq(diff, "\n") {
		if !strings.HasPrefix(line, "@@ ") {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		oldStart, oldCount, newStart, newCount := parseHunkHeader(line)
		if oldStart < 0 || oldCount < 0 || newStart < 0 || newCount < 0 {
			return nil, errGitHunkInvalid
		}
		if oldCount > gitDiffMarkerLimit || newCount > gitDiffMarkerLimit || count > gitDiffMarkerLimit-newCount {
			return nil, errors.New("git hunk exceeds marker limit")
		}
		count += max(1, newCount)
		if count > gitDiffMarkerLimit {
			return nil, errors.New("git marker count exceeds limit")
		}
		if newCount > 0 && (newStart == 0 || newStart > int(^uint(0)>>1)-newCount) {
			return nil, errGitHunkInvalid
		}
		status := GitModified
		if oldCount == 0 {
			status = GitAdded
		}
		if newCount == 0 && oldCount > 0 {
			result[max(1, newStart)] = GitDeleted
			continue
		}
		for i := 0; i < newCount; i++ {
			result[newStart+i] = status
		}
	}
	return result, nil
}

// Invalid headers/ranges return -1 sentinels, not unbounded allocation counts.
func parseHunkHeader(header string) (oldStart, oldCount, newStart, newCount int) {
	parts := strings.SplitN(header, "@@", 3)
	if len(parts) != 3 {
		return -1, -1, -1, -1
	}
	fields := strings.Fields(parts[1])
	if len(fields) != 2 || !strings.HasPrefix(fields[0], "-") || !strings.HasPrefix(fields[1], "+") {
		return -1, -1, -1, -1
	}
	oldStart, oldCount = parseRange(fields[0][1:])
	newStart, newCount = parseRange(fields[1][1:])
	return
}
func parseRange(s string) (start, count int) {
	a, b, has := strings.Cut(s, ",")
	if !has {
		b = "1"
	}
	valid := func(s string) bool {
		if s == "" {
			return false
		}
		for _, r := range s {
			if r < '0' || r > '9' {
				return false
			}
		}
		return true
	}
	if !valid(a) || !valid(b) {
		return -1, -1
	}
	start, err := strconv.Atoi(a)
	if err != nil {
		return -1, -1
	}
	count, err = strconv.Atoi(b)
	if err != nil {
		return -1, -1
	}
	return
}
