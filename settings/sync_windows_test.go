//go:build windows

package settings

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/uk0/silk/diagnostic"
	"golang.org/x/sys/windows"
)

func TestSettingsWindowsLockedDestinationPreservesOldFileAndRetries(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "prefs"))
	settingsSet(t, s, "old")
	if err := s.Sync(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	name, err := windows.UTF16PtrFromString(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	closed := false
	defer func() {
		if !closed {
			_ = windows.CloseHandle(h)
		}
	}()
	settingsSet(t, s, "new")
	if err = s.Sync(); diagnostic.CodeOf(err) != diagnostic.SettingsWrite {
		t.Fatalf("locked replacement must fail: %v", err)
	}
	if !s.IsDirty() {
		t.Fatal("locked replacement cleared dirty state")
	}
	after, err := os.ReadFile(s.Path())
	if err != nil || !bytes.Equal(after, before) {
		t.Fatalf("old file changed: %v", err)
	}
	tmp, _ := filepath.Glob(filepath.Join(filepath.Dir(s.Path()), ".silk-settings-*"))
	if len(tmp) != 0 {
		t.Fatalf("temp leaked: %v", tmp)
	}
	if err = windows.CloseHandle(h); err != nil {
		t.Fatal(err)
	}
	closed = true
	if err = s.Sync(); err != nil {
		t.Fatal(err)
	}
	if New(s.Path()).String("key", "") != "new" || s.IsDirty() {
		t.Fatal("retry did not install pending value")
	}
}
