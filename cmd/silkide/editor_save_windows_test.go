//go:build windows

package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/uk0/silk/gui"
	"golang.org/x/sys/windows"
)

// File.ReadAllText polling in an external QA process opens with FileShare.Read:
// even a reader can deny an editor write. Distinguish that native OS rejection
// from a lost Ctrl+S, and never mark the rejected buffer saved or discard it.
func TestEditorSaveWindowsReaderSharing(t *testing.T) {
	for _, tc := range []struct {
		name   string
		share  uint32
		blocks bool
	}{
		{"reader_denies_write", windows.FILE_SHARE_READ, true},
		{"cooperative_reader", windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "document.txt")
			const original, edited = "original\n", "unsaved 中文 😀\n"
			if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
				t.Fatal(err)
			}
			ed := gui.NewCodeEditor()
			t.Cleanup(ed.CancelGitStatus)
			ed.SetText(original)
			ed.MarkSaved()
			ed.ReplaceAllText(edited)
			revision := ed.TextRevision()
			oldToast, oldCanvas := silkideToast, globalCanvas
			globalCanvas = nil
			var levels []gui.ToastLevel
			silkideToast = func(_ string, level gui.ToastLevel) { levels = append(levels, level) }
			t.Cleanup(func() { silkideToast, globalCanvas = oldToast, oldCanvas })
			name, err := windows.UTF16PtrFromString(path)
			if err != nil {
				t.Fatal(err)
			}
			handle, err := windows.CreateFile(name, windows.GENERIC_READ, tc.share, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if handle != windows.InvalidHandle {
					windows.CloseHandle(handle)
				}
			}()
			if tc.blocks {
				f, err := os.OpenFile(path, os.O_WRONLY, 0)
				if f != nil {
					f.Close()
				}
				if !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
					t.Fatalf("not a native sharing violation: %v", err)
				}
			}
			saved := saveEditorToDisk(ed, path)
			if saved == tc.blocks {
				t.Fatalf("saved=%v with blocks=%v", saved, tc.blocks)
			}
			if ed.Text() != edited || ed.TextRevision() != revision {
				t.Fatal("save changed the text or revision")
			}
			if tc.blocks {
				if !ed.Modified() || diskText(t, path) != original {
					t.Fatal("failed save lost dirty state or changed disk")
				}
				if len(levels) != 1 || levels[0] != gui.ToastError {
					t.Fatalf("missing failure feedback: %v", levels)
				}
				if err := windows.CloseHandle(handle); err != nil {
					t.Fatal(err)
				}
				handle = windows.InvalidHandle
				if !saveEditorToDisk(ed, path) {
					t.Fatal("save did not recover after reader released its handle")
				}
			}
			if diskText(t, path) != edited || ed.Modified() {
				t.Fatal("successful save did not persist and mark the exact buffer clean")
			}
			if levels[len(levels)-1] != gui.ToastSuccess {
				t.Fatalf("missing success feedback: %v", levels)
			}
		})
	}
}
