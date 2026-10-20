package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAppendExistingToolDirs(t *testing.T) {
	root := t.TempDir()
	preferred := filepath.Join(root, "preferred")
	fallback := filepath.Join(root, "fallback")
	for _, dir := range []string{preferred, fallback} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	file := filepath.Join(root, "file")
	os.WriteFile(file, []byte("not a directory"), 0600)
	dirs := []string{fallback, filepath.Join(fallback, "."), preferred, "relative", filepath.Join(root, "missing"), file}
	want := preferred + string(os.PathListSeparator) + fallback
	if got := appendExistingToolDirs(preferred, dirs, false); got != want {
		t.Fatalf("PATH=%q want %q", got, want)
	}
	if got := appendExistingToolDirs(want, dirs, false); got != want {
		t.Fatalf("non-idempotent PATH %q", got)
	}
	if got := appendExistingToolDirs("", []string{fallback}, false); got != fallback {
		t.Fatalf("empty PATH introduces current directory: %q", got)
	}
	if got := appendExistingToolDirs(strings.ToUpper(fallback), []string{fallback}, true); got != strings.ToUpper(fallback) {
		t.Fatal("Windows case-insensitive duplicate")
	}
}

func TestIDEProcessToolPathFindsGoWithoutChangingPriority(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	os.Mkdir(bin, 0700)
	name := "go"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	fake := filepath.Join(bin, name)
	if err := os.WriteFile(fake, []byte("test fixture only"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOROOT", root)
	t.Setenv("GOPATH", filepath.Join(root, "nonexistent"))
	t.Setenv("PATH", "")
	configureIDEProcessToolPath()
	if got, err := exec.LookPath("go"); err != nil || got != fake {
		t.Fatalf("desktop Go resolution=%q %v", got, err)
	}
	first := os.Getenv("PATH")
	configureIDEProcessToolPath()
	if os.Getenv("PATH") != first {
		t.Fatal("repeated setup appends duplicates")
	}
	// An explicit user PATH remains authoritative over the fallback install.
	preferred := filepath.Join(root, "preferred")
	os.Mkdir(preferred, 0700)
	chosen := filepath.Join(preferred, name)
	os.WriteFile(chosen, []byte("fixture"), 0700)
	t.Setenv("PATH", preferred)
	configureIDEProcessToolPath()
	if got, err := exec.LookPath("go"); err != nil || got != chosen {
		t.Fatalf("explicit Go lost priority: %q %v", got, err)
	}
}
