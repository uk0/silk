package core

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestGitDiffContextCancellationAndOptions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := GitDiffFileContext(ctx, t.TempDir(), "x"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := GitShowCommitContext(ctx, t.TempDir(), "HEAD"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := GitDiffFileContext(nil, t.TempDir(), "x"); err == nil {
		t.Fatal("nil context accepted")
	}
	if _, err := GitShowCommitContext(nil, t.TempDir(), "HEAD"); err == nil {
		t.Fatal("nil context accepted")
	}
	if _, err := GitShowCommitContext(context.Background(), t.TempDir(), "--output=unexpected"); err == nil {
		t.Fatal("revision interpreted as an option")
	}
	if !GitAvailable() {
		t.Skip("real Git unavailable")
	}
	dir, file := setupRepo(t)
	mustGit(t, dir, "config", "diff.external", "silk-invalid-external-helper")
	mustGit(t, dir, "config", "color.ui", "always")
	mustGit(t, dir, "config", "diff.qa.textconv", "silk-invalid-textconv-helper")
	writeFile(t, dir, ".gitattributes", file+" diff=qa\n")
	writeFile(t, dir, file, "new exact content\n")
	out, err := GitDiffFileContext(context.Background(), dir, file)
	if err != nil || !strings.Contains(out, "+new exact content") || strings.Contains(out, "\x1b[") {
		t.Fatalf("diff used external helpers/color: %v %q", err, out)
	}
	mustGit(t, dir, "add", "--", file)
	mustGit(t, dir, "commit", "-m", "context diff")
	out, err = GitShowCommitContext(context.Background(), dir, "HEAD")
	if err != nil || !strings.Contains(out, "+new exact content") || strings.Contains(out, "\x1b[") {
		t.Fatalf("show used external helpers/color: %v %q", err, out)
	}
}
func TestGitDiffContextOutputBound(t *testing.T) {
	if !GitAvailable() {
		t.Skip("real Git unavailable")
	}
	dir, file := setupRepo(t)
	writeFile(t, dir, file, strings.Repeat("x", 9<<20)+"\n")
	out, err := GitDiffFileContext(context.Background(), dir, file)
	if !errors.Is(err, ErrGitReadLimit) || out != "" {
		t.Fatalf("oversized diff was not rejected explicitly: %v bytes=%d", err, len(out))
	}
}
