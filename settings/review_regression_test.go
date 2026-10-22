package settings

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uk0/silk/core"
)

func TestSettingsQueuedSnapshotSurvivesWriteFailure(t *testing.T) {
	for _, finalError := range []bool{false, true} {
		t.Run(map[bool]string{false: "recovered", true: "latest error"}[finalError], func(t *testing.T) {
			s := New(filepath.Join(t.TempDir(), "prefs"))
			settingsSet(t, s, "first")
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			defer once.Do(func() { close(release) })
			var calls atomic.Int32
			s.writeSnapshot = func(path string, d *core.TDoc) error {
				if calls.Add(1) == 1 {
					close(entered)
					<-release
					return io.ErrClosedPipe
				}
				if finalError {
					return os.ErrPermission
				}
				return writeSettingsSnapshot(path, d)
			}
			if err := s.RequestSync(); err != nil {
				t.Fatal(err)
			}
			settingsWait(t, entered)
			f := settingsFlight(s)
			settingsSet(t, s, "latest")
			if err := s.RequestSync(); err != nil {
				t.Fatal(err)
			}
			once.Do(func() { close(release) })
			settingsWait(t, f.done)
			if calls.Load() != 2 {
				t.Fatalf("attempts=%d", calls.Load())
			}
			if finalError {
				if !errors.Is(f.err, os.ErrPermission) || !s.IsDirty() {
					t.Fatal(f.err)
				}
			} else {
				if f.err != nil || s.IsDirty() || New(s.Path()).String("key", "") != "latest" {
					t.Fatalf("lost final snapshot: %v", f.err)
				}
			}
		})
	}
}

func TestSettingsEmptyAndSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "managed")
	link := filepath.Join(root, "preferences")
	if err := os.WriteFile(target, nil, 0600); err != nil {
		t.Fatal(err)
	}
	s := New(target)
	settingsSet(t, s, "empty recovered")
	if err := s.Sync(); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	s = New(link)
	if s.String("key", "") != "empty recovered" {
		t.Fatal("link not loaded")
	}
	settingsSet(t, s, "through link")
	if err := s.Sync(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Readlink(link); err != nil {
		t.Fatal("managed link replaced", err)
	}
	if New(target).String("key", "") != "through link" {
		t.Fatal("target not updated")
	}
}

func TestSettingsReloadRepairsBlockedStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prefs")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	s := New(path)
	_ = s.String("key", "")
	if s.Status() == nil {
		t.Fatal("directory should block reads")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("=\nkey = repaired\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.Reload(); err != nil {
		t.Fatal(err)
	}
	if s.Status() != nil || s.String("key", "") != "repaired" {
		t.Fatal("sticky load error")
	}
	settingsSet(t, s, "new")
	if err := s.Reload(); err == nil {
		t.Fatal("reload discarded unsaved edits")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Flush(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestSettingsReloadAfterReadErrorPreservesRuntimeEdits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prefs")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	s := New(path)
	settingsSet(t, s, "runtime")
	if err := s.Reload(); err == nil {
		t.Fatal("unrepaired directory accepted")
	}
	if s.String("key", "") != "runtime" {
		t.Fatal("failed retry lost edits")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("=\nkey = disk\nother = preserved\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.Reload(); err != nil {
		t.Fatal(err)
	}
	if err := s.Sync(); err != nil {
		t.Fatal(err)
	}
	loaded := New(path)
	if loaded.String("key", "") != "runtime" || loaded.String("other", "") != "preserved" {
		t.Fatal("recovery lost disk defaults or runtime edits")
	}
}
