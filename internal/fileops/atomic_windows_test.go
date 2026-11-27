package fileops

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func holdWithoutDeleteShare(t *testing.T, path string) func() {
	t.Helper()
	name, err := snapshotWindowsPath(path)
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.CreateFile(&name[0], windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil,
		windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	release := func() {
		if h != windows.InvalidHandle {
			if err := windows.CloseHandle(h); err != nil {
				t.Error(err)
			}
			h = windows.InvalidHandle
		}
	}
	t.Cleanup(release)
	return release
}

func TestWindowsAtomicSharingRetry(t *testing.T) {
	for _, scenario := range []string{"release", "external-change", "persistent"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "actual-file.go")
			if err := os.WriteFile(path, []byte("first"), 0600); err != nil {
				t.Fatal(err)
			}
			release := holdWithoutDeleteShare(t, path)
			encoded, attempted := 0, 0
			err := writeAtomic(path, 0600, func(w io.Writer) error {
				encoded++
				_, err := io.WriteString(w, "saved")
				return err
			}, func(dir, pattern string) (atomicOutput, error) {
				return os.CreateTemp(dir, pattern)
			}, func(from, to string) error {
				attempted++
				err := ReplaceSnapshotFile(from, to)
				if attempted == 1 {
					if !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
						t.Fatalf("wanted live sharing conflict on first rename, got %v", err)
					}
					switch scenario {
					case "release":
						release()
					case "external-change":
						// Equal length and restored mtime test the fingerprint,
						// rather than a size or modification time shortcut.
						fi, statErr := os.Stat(path)
						if statErr != nil {
							t.Fatal(statErr)
						}
						if changeErr := os.WriteFile(path, []byte("third"), 0600); changeErr != nil {
							t.Fatal(changeErr)
						}
						if timeErr := os.Chtimes(path, fi.ModTime(), fi.ModTime()); timeErr != nil {
							t.Fatal(timeErr)
						}
						release()
					}
				}
				return err
			})
			want := "first"
			switch scenario {
			case "release":
				want = "saved"
				if err != nil || attempted != 2 {
					t.Errorf("released lock: attempts=%d err=%v", attempted, err)
				}
			case "external-change":
				want = "third"
				if !errors.Is(err, ErrChanged) || attempted != 1 {
					t.Errorf("changed target: attempts=%d err=%v", attempted, err)
				}
			case "persistent":
				if !errors.Is(err, windows.ERROR_SHARING_VIOLATION) || attempted < 2 || attempted > 6 {
					t.Errorf("permanent lock: attempts=%d err=%v", attempted, err)
				}
			}
			if got, readErr := os.ReadFile(path); readErr != nil || string(got) != want {
				t.Errorf("disk = %q, error=%v; want %q", got, readErr, want)
			}
			if left, globErr := filepath.Glob(filepath.Join(dir, ".silk-write-*")); globErr != nil || len(left) != 0 || encoded != 1 {
				t.Errorf("staged files=%v glob=%v encode count=%d", left, globErr, encoded)
			}
		})
	}
}
