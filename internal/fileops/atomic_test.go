package fileops

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func atomicFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "设备 document.txt")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertAtomicState(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || string(got) != want {
		t.Fatalf("destination=%q, %v; want %q", got, err, want)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".silk-write-") {
			t.Fatalf("temporary output leaked: %s", e.Name())
		}
	}
}

func TestAtomicWritePreservesModeAndSnapshotReader(t *testing.T) {
	path := atomicFixture(t)
	before, _ := os.Stat(path)
	held, err := OpenSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	if err := WriteFileAtomic(path, []byte("new😀\r\n"), 0644); err != nil {
		t.Fatal(err)
	}
	assertAtomicState(t, path, "new😀\r\n")
	after, _ := os.Stat(path)
	if before.Mode().Perm() != after.Mode().Perm() {
		t.Fatalf("mode %v -> %v", before.Mode(), after.Mode())
	}
	old, err := io.ReadAll(held)
	if err != nil || string(old) != "original" {
		t.Fatalf("held reader=%q, %v", old, err)
	}
}

func TestAtomicWriteAbsentAndRelativeTarget(t *testing.T) {
	t.Chdir(t.TempDir())
	path := "新文件.txt"
	if err := WriteFileAtomic(path, []byte("first"), 0644); err != nil {
		t.Fatal(err)
	}
	assertAtomicState(t, path, "first")
	if err := WriteFileAtomic(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	assertAtomicState(t, path, "")
}

type failingAtomicOutput struct {
	*os.File
	stage  string
	cause  error
	closes *int
}

func (f *failingAtomicOutput) Chmod(mode os.FileMode) error {
	if f.stage == "chmod" {
		return f.cause
	}
	return f.File.Chmod(mode)
}
func (f *failingAtomicOutput) Write(p []byte) (int, error) {
	if f.stage == "write" {
		_, _ = f.File.Write(p[:1])
		return 1, f.cause
	}
	if f.stage == "short-write" {
		return 0, nil
	}
	return f.File.Write(p)
}
func (f *failingAtomicOutput) Sync() error {
	if f.stage == "sync" {
		return f.cause
	}
	return f.File.Sync()
}
func (f *failingAtomicOutput) Close() error {
	*f.closes++
	err := f.File.Close()
	if f.stage == "close" {
		return errors.Join(err, f.cause)
	}
	return err
}

func TestAtomicWriteEveryFailurePreservesOriginal(t *testing.T) {
	for _, stage := range []string{"create", "chmod", "encode", "write", "short-write", "sync", "close", "replace"} {
		t.Run(stage, func(t *testing.T) {
			path := atomicFixture(t)
			before, _ := os.Stat(path)
			cause, closes, replacements := errors.New("injected "+stage), 0, 0
			create := func(dir, prefix string) (atomicOutput, error) {
				if stage == "create" {
					return nil, cause
				}
				f, err := os.CreateTemp(dir, prefix)
				if err != nil {
					return nil, err
				}
				return &failingAtomicOutput{f, stage, cause, &closes}, nil
			}
			err := writeAtomic(path, 0644, func(w io.Writer) error {
				if stage == "encode" {
					_, _ = io.WriteString(w, "half")
					return cause
				}
				_, err := io.WriteString(w, "replacement")
				return err
			}, create, func(string, string) error { replacements++; return cause })
			want := cause
			if stage == "short-write" {
				want = io.ErrShortWrite
			}
			if !errors.Is(err, want) {
				t.Fatalf("error=%v, want %v", err, want)
			}
			if stage != "create" && closes != 1 {
				t.Fatalf("close count=%d", closes)
			}
			if stage != "replace" && replacements != 0 {
				t.Fatal("failed stage reached replacement")
			}
			after, _ := os.Stat(path)
			if !os.SameFile(before, after) || before.Mode() != after.Mode() {
				t.Fatal("failed write replaced original inode or permissions")
			}
			assertAtomicState(t, path, "original")
		})
	}
}

func TestAtomicWriteDetectsChangesDuringEncode(t *testing.T) {
	for _, kind := range []string{"bytes-same-stat", "replace-identical", "delete", "new-target", "mode"} {
		t.Run(kind, func(t *testing.T) {
			path := atomicFixture(t)
			before, _ := os.Stat(path)
			if kind == "new-target" {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			}
			want := "original"
			err := WriteAtomic(path, 0644, func(w io.Writer) error {
				switch kind {
				case "bytes-same-stat":
					want = "external"
					if err := os.WriteFile(path, []byte(want), 0600); err != nil {
						return err
					}
					if err := os.Chtimes(path, before.ModTime(), before.ModTime()); err != nil {
						return err
					}
				case "replace-identical":
					other := path + ".external"
					if err := os.WriteFile(other, []byte(want), before.Mode().Perm()); err != nil {
						return err
					}
					if err := ReplaceSnapshotFile(other, path); err != nil {
						return err
					}
				case "delete":
					if err := os.Remove(path); err != nil {
						return err
					}
				case "new-target":
					if err := os.WriteFile(path, []byte(want), 0600); err != nil {
						return err
					}
				case "mode":
					if err := os.Chmod(path, 0400); err != nil {
						return err
					}
					t.Cleanup(func() { _ = os.Chmod(path, 0600) })
				}
				_, err := io.WriteString(w, "ours")
				return err
			})
			if !errors.Is(err, ErrChanged) {
				t.Fatalf("changed destination accepted: %v", err)
			}
			if kind == "delete" {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("deleted file resurrected: %v", err)
				}
			} else {
				assertAtomicState(t, path, want)
			}
		})
	}
}

func TestAtomicWriteRejectsNonRegularAndReadOnly(t *testing.T) {
	for _, kind := range []string{"directory", "readonly", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			path := atomicFixture(t)
			switch kind {
			case "directory":
				path += ".dir"
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			case "readonly":
				if err := os.Chmod(path, 0400); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(path, 0600) })
			case "symlink":
				target := path
				path += ".link"
				if err := os.Symlink(target, path); err != nil {
					t.Skipf("symlink unavailable: %v", err)
				}
			}
			called := false
			err := WriteAtomic(path, 0644, func(io.Writer) error { called = true; return nil })
			if err == nil || called {
				t.Fatalf("unsafe destination accepted: %v, encode=%v", err, called)
			}
			if kind != "directory" {
				assertAtomicState(t, path, "original")
			}
		})
	}
}

func TestAtomicWriteEncodePanicCleansUp(t *testing.T) {
	path := atomicFixture(t)
	func() {
		defer func() {
			if recover() != "encoder panic" {
				t.Error("encoder panic swallowed")
			}
		}()
		_ = WriteAtomic(path, 0644, func(w io.Writer) error {
			_, _ = io.WriteString(w, "partial")
			panic("encoder panic")
		})
	}()
	assertAtomicState(t, path, "original")
}

func TestAtomicWriteDoesNotTrustEncoderIgnoringWriteError(t *testing.T) {
	path := atomicFixture(t)
	closes := 0
	cause := errors.New("physical write failed")
	err := writeAtomic(path, 0644, func(w io.Writer) error {
		_, _ = w.Write([]byte("partial"))
		return nil // broken encoder must not cause successful replacement
	}, func(dir, prefix string) (atomicOutput, error) {
		f, err := os.CreateTemp(dir, prefix)
		if err != nil {
			return nil, err
		}
		return &failingAtomicOutput{f, "write", cause, &closes}, nil
	}, func(string, string) error { t.Fatal("failed write reached replacement"); return nil })
	if !errors.Is(err, cause) || closes != 1 {
		t.Fatalf("error=%v closes=%d", err, closes)
	}
	assertAtomicState(t, path, "original")
}

func TestAtomicWriteInvalidArguments(t *testing.T) {
	if err := WriteAtomic("", 0644, func(io.Writer) error { return nil }); !errors.Is(err, os.ErrInvalid) {
		t.Fatal(err)
	}
	if err := WriteAtomic(filepath.Join(t.TempDir(), "file"), 0644, nil); !errors.Is(err, os.ErrInvalid) {
		t.Fatal(err)
	}
}
