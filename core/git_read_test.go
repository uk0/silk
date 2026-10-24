package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestGitStatusExactPaths(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not installed")
	}
	dir, old := setupRepo(t)
	names := []string{"中文 文件.txt", "space name.txt", "[name].txt"}
	if runtime.GOOS != "windows" {
		names = append(names, "literal -> name.txt", "tab\tname.txt", "line\nname.txt", `quote"name.txt`)
	}
	for _, name := range names {
		writeFile(t, dir, name, "test\n")
	}
	dest := "重命名 文件.txt"
	if err := os.Rename(filepath.Join(dir, old), filepath.Join(dir, dest)); err != nil {
		t.Fatal(err)
	}
	mustGit(t, dir, "add", "--", old, dest)
	rows, err := GitStatusPorcelain(dir)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]GitStatusEntry{}
	for _, row := range rows {
		by[row.Path] = row
	}
	for _, name := range names {
		if _, ok := by[name]; !ok {
			t.Errorf("exact path %q lost in %+v", name, rows)
		}
	}
	if by[dest].OrigPath != old {
		t.Fatalf("rename %q -> %q lost: %+v", old, dest, rows)
	}
}

func TestGitReadContextClassificationAndScope(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := GitStatusPorcelainContext(ctx, t.TempDir()); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := GitShortLogContext(context.Background(), t.TempDir(), 0); err == nil {
		t.Fatal("unbounded history accepted")
	}
	if !GitAvailable() {
		t.Skip("real Git unavailable")
	}
	dir, _ := setupRepo(t)
	sub := filepath.Join(dir, "sub folder")
	if err := os.Mkdir(sub, 0700); err != nil {
		t.Fatal(err)
	}
	foreign, _ := setupRepo(t)
	t.Setenv("GIT_DIR", filepath.Join(foreign, ".git"))
	t.Setenv("GIT_WORK_TREE", foreign)
	root, err := GitRepositoryRootContext(context.Background(), sub)
	if err != nil {
		t.Fatal(err)
	}
	realDir, _ := filepath.EvalSymlinks(dir)
	realRoot, _ := filepath.EvalSymlinks(root)
	if realDir != realRoot {
		t.Fatalf("wrong root %q != %q", root, dir)
	}
	if _, err := GitRepositoryRootContext(context.Background(), t.TempDir()); !errors.Is(err, ErrGitNotRepository) {
		t.Fatalf("nonrepo: %v", err)
	}
	if err := os.Unsetenv("GIT_DIR"); err != nil {
		t.Fatal(err)
	}
	if err := os.Unsetenv("GIT_WORK_TREE"); err != nil {
		t.Fatal(err)
	}
	empty := t.TempDir()
	mustGit(t, empty, "init")
	if rows, err := GitShortLogContext(context.Background(), empty, 100); err != nil || len(rows) != 0 {
		t.Fatalf("unborn: %v %v", rows, err)
	}
}

func TestGitReadMissingAndMalformed(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := GitStatusPorcelainContext(context.Background(), t.TempDir()); !errors.Is(err, ErrGitUnavailable) {
		t.Fatal(err)
	}
	for _, input := range []string{" M x", "M x\x00", "R  to\x00", "R  to\x00\x00", "?? \x00"} {
		if rows, err := parseGitStatusZ(input); !errors.Is(err, ErrGitStatusMalformed) || rows != nil {
			t.Fatalf("accepted %q: %v %v", input, rows, err)
		}
	}
	rows, err := parseGitStatusZ("?? literal -> name\x00R  new\nname\x00old\tname\x00")
	if err != nil || len(rows) != 2 || rows[0].OrigPath != "" || rows[1].OrigPath != "old\tname" {
		t.Fatalf("%+v %v", rows, err)
	}
	if rows, err := parseGitStatusZ(strings.Repeat("?? x\x00", 100001)); !errors.Is(err, ErrGitReadLimit) || rows != nil {
		t.Fatal(len(rows), err)
	}
	long := strings.Repeat("x", 100000)
	commits := parseShortLog("abc\x1f" + long + "\x1fauthor\x1f2026-01-01\n")
	if len(commits) != 1 || commits[0].Subject != long {
		t.Fatal("long subject silently dropped")
	}
}

func TestGitReadHelperProcess(t *testing.T) {
	mode := os.Getenv("SILK_GIT_READ_HELPER")
	if mode == "" {
		return
	}
	if mode == "overflow" {
		for i := 0; i < 9; i++ {
			fmt.Print(strings.Repeat("x", 1<<20))
		}
		os.Exit(0)
	}
	if mode == "wait" {
		_ = os.WriteFile(os.Getenv("SILK_GIT_READY"), []byte("ready"), 0600)
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}
	os.Exit(2)
}
func TestGitReadCommandCancellationAndBounds(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"overflow", "wait"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			ready := filepath.Join(t.TempDir(), "ready")
			cmd := exec.CommandContext(ctx, exe, "-test.run=^TestGitReadHelperProcess$")
			cmd.Env = append(os.Environ(), "SILK_GIT_READ_HELPER="+mode, "SILK_GIT_READY="+ready)
			done := make(chan error, 1)
			go func() {
				out, err := runGitReadCommand(ctx, cancel, cmd, []string{"test"})
				if out != "" {
					done <- fmt.Errorf("partial output returned")
					return
				}
				done <- err
			}()
			if mode == "wait" {
				deadline := time.Now().Add(5 * time.Second)
				for {
					if _, err := os.Stat(ready); err == nil {
						break
					}
					if time.Now().After(deadline) {
						t.Fatal("helper never started")
					}
					time.Sleep(10 * time.Millisecond)
				}
				cancel()
			}
			select {
			case err := <-done:
				want := ErrGitReadLimit
				if mode == "wait" {
					want = context.Canceled
				}
				if !errors.Is(err, want) {
					t.Fatal(err)
				}
			case <-time.After(6 * time.Second):
				t.Fatal("query did not finish")
			}
			if cmd.ProcessState == nil {
				t.Fatal("child was not waited")
			}
		})
	}
}
