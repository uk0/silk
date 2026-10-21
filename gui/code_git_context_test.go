package gui

import (
	"bytes"
	"context"
	"errors"
	"github.com/uk0/silk/diagnostic"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func gitGutterFixture(t *testing.T) (string, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	root := t.TempDir()
	path := filepath.Join(root, "source.go")
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	if err := os.WriteFile(path, []byte("package p\nvar Value = 1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run("add", "--", "source.go")
	run("-c", "user.name=Silk QA", "-c", "user.email=qa@example.invalid", "commit", "-qm", "baseline")
	if err := os.WriteFile(path, []byte("package p\nvar Value = 2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return root, path
}
func TestGitDiffUsesFileRepositoryNotProcessWorkingDirectory(t *testing.T) {
	_, path := gitGutterFixture(t)
	got := GitDiff(path)
	if got[2] != GitModified {
		t.Fatalf("external-project gutter missing; got %v", got)
	}
}

func TestGitDiffContextStatesAndLiteralPaths(t *testing.T) {
	root, path := gitGutterFixture(t)
	// A basename containing glob metacharacters must remain a literal pathspec.
	named := filepath.Join(root, "[literal].go")
	if err := os.Rename(path, named); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, out)
		}
	}
	git("add", "-A")
	git("-c", "user.name=Silk QA", "-c", "user.email=qa@example.invalid", "commit", "-qm", "literal")
	if err := os.WriteFile(named, []byte("package p\nvar Value = 3\n"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_DIR", filepath.Join(root, "not-a-repo"))
	got, err := GitDiffContext(context.Background(), named)
	if err != nil || got[2] != GitModified {
		t.Fatal("literal or ambient repo identity", got, err)
	}
	empty, err := GitDiffContext(context.Background(), filepath.Join(t.TempDir(), "file.go"))
	if err != nil || len(empty) != 0 {
		t.Fatal("non-repository should be quiet", empty, err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := GitDiffContext(canceled, named); !errors.Is(err, context.Canceled) {
		t.Fatal("pre-canceled", err)
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := GitDiffContext(context.Background(), named); diagnostic.CodeOf(err) != diagnostic.GitUnavailable {
		t.Fatal("missing git code", err)
	}
}
func TestGitDiffHunkBoundsAndStatuses(t *testing.T) {
	got, err := parseGitLineStatuses(context.Background(), "@@ -0,0 +1,2 @@\n@@ -4,2 +6,0 @@\n@@ -8 +9 @@\n")
	if err != nil || got[1] != GitAdded || got[2] != GitAdded || got[6] != GitDeleted || got[9] != GitModified {
		t.Fatal(got, err)
	}
	for _, h := range []string{"@@ -1 +1,100001 @@", "@@ -a +3 @@", "@@ -1 +-1 @@", "@@ -1 +9223372036854775807,1 @@", "@@ -1,1 +0,1 @@"} {
		if _, err := parseGitLineStatuses(context.Background(), h); err == nil {
			t.Fatal("accepted invalid/oversized hunk", h)
		}
	}
}

func TestGitCommandChild(t *testing.T) {
	if len(os.Args) < 2 {
		return
	}
	switch os.Args[len(os.Args)-1] {
	case "silk-git-flood":
		chunk := bytes.Repeat([]byte("x"), 64<<10)
		for i := 0; i < 1000; i++ {
			if _, err := os.Stdout.Write(chunk); err != nil {
				os.Exit(0)
			}
		}
		os.Exit(0)
	case "silk-git-stall":
		for {
			time.Sleep(time.Second)
		}
	}
}
func TestGitCommandBoundsOutputAndReapsCanceledChild(t *testing.T) {
	for _, mode := range []string{"silk-git-flood", "silk-git-stall"} {
		t.Run(mode, func(t *testing.T) {
			timeout := 500 * time.Millisecond
			if mode == "silk-git-flood" {
				timeout = 5 * time.Second
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestGitCommandChild$", "--", mode)
			start := time.Now()
			out, stderr, err := runGitDiffCommand(ctx, cancel, cmd)
			if time.Since(start) > timeout+2*time.Second {
				t.Fatal("command exceeded bounded wait")
			}
			if cmd.ProcessState == nil {
				t.Fatal("child not waited/reaped")
			}
			if len(out) > gitDiffOutputLimit || len(stderr) > 64<<10 {
				t.Fatal("unbounded capture")
			}
			if mode == "silk-git-flood" && !errors.Is(err, errGitOutputLimit) {
				t.Fatal("missing overflow", len(out), err)
			}
			if mode == "silk-git-stall" && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal("missing cancellation", err)
			}
		})
	}
}
