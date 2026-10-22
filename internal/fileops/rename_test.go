package fileops

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestRenameNoReplaceConcurrentDestinations(t *testing.T) {
	root := t.TempDir()
	dst := filepath.Join(root, "target")
	a, b := filepath.Join(root, "a"), filepath.Join(root, "b")
	for _, p := range []string{a, b} {
		if err := os.WriteFile(p, []byte(filepath.Base(p)), 0644); err != nil {
			t.Fatal(err)
		}
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, p := range []string{a, b} {
		wg.Go(func() { <-start; results <- RenameNoReplace(p, dst) })
	}
	close(start)
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else if !os.IsExist(err) {
			t.Fatalf("not conflict: %v", err)
		}
	}
	if success != 1 {
		t.Fatalf("atomic winner count: %d", success)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	loser := "b"
	if string(data) == "b" {
		loser = "a"
	} else if string(data) != "a" {
		t.Fatalf("corrupt target %q", data)
	}
	if got, err := os.ReadFile(filepath.Join(root, loser)); err != nil || string(got) != loser {
		t.Fatalf("loser lost: %q %v", got, err)
	}
}
func TestRenameNoReplaceDirectoryAndDanglingSymlink(t *testing.T) {
	root := t.TempDir()
	src, dst := filepath.Join(root, "src"), filepath.Join(root, "dst")
	if err := os.Mkdir(src, 0755); err != nil {
		t.Fatal(err)
	}
	if err := RenameNoReplace(src, dst); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(src, 0755); err != nil {
		t.Fatal(err)
	}
	if err := RenameNoReplace(src, dst); !os.IsExist(err) {
		t.Fatalf("directory collision: %v", err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(filepath.Join(root, "missing"), link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if err := RenameNoReplace(src, link); !os.IsExist(err) {
		t.Fatalf("dangling link collision: %v", err)
	}
	if _, err := os.Readlink(link); err != nil {
		t.Fatal("symlink replaced", err)
	}
}

func TestCompatibleRenameRequiresUnsupportedAndProtectsExisting(t *testing.T) {
	root := t.TempDir()
	src, dst := filepath.Join(root, "src"), filepath.Join(root, "dst")
	unsupported := func(string, string) error { return ErrRenameUnsupported }
	if err := os.WriteFile(src, []byte("source"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := renameCompatible(src, dst, func(string, string) error { return os.ErrPermission }); !os.IsPermission(err) {
		t.Fatal(err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatal("permission failure mutated source")
	}
	if err := os.WriteFile(dst, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := renameCompatible(src, dst, unsupported); !os.IsExist(err) {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != "original" {
		t.Fatal("existing target replaced")
	}
	if err := os.Remove(dst); err != nil {
		t.Fatal(err)
	}
	if err := renameCompatible(src, dst, unsupported); err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(dst)
	if string(got) != "source" {
		t.Fatal("compatibility rename lost content")
	}
	if err := os.WriteFile(src, []byte("second"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(filepath.Join(root, "missing"), link); err != nil {
		t.Skip(err)
	}
	if err := renameCompatible(src, link, unsupported); !os.IsExist(err) {
		t.Fatal("dangling link replaced", err)
	}
}
