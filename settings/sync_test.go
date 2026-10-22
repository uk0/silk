package settings

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/diagnostic"
)

func settingsWait(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatal("settings worker did not finish")
	}
}
func settingsSet(t *testing.T, s *Settings, value string) {
	t.Helper()
	if err := s.SetString("key", value); err != nil {
		t.Fatal(err)
	}
}
func settingsFlight(s *Settings) *syncFlight { s.mu.Lock(); defer s.mu.Unlock(); return s.flight }

func TestSettingsAsyncCoalescesAndDoesNotLockReadersBehindIO(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prefs")
	s := New(path)
	settingsSet(t, s, "old")
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }); _ = s.Sync() })
	var calls atomic.Int32
	s.writeSnapshot = func(path string, d *core.TDoc) error {
		if calls.Add(1) == 1 {
			close(started)
			<-release
		}
		return writeSettingsSnapshot(path, d)
	}
	if err := s.RequestSync(); err != nil {
		t.Fatal(err)
	}
	settingsWait(t, started)
	responsive := make(chan struct{})
	go func() {
		defer close(responsive)
		for i := 0; i < 10000; i++ {
			_ = s.String("key", "")
			_ = s.SetInt("revision", int64(i))
			_ = s.RequestSync()
		}
		_ = s.SetString("key", "latest")
		_ = s.RequestSync()
	}()
	settingsWait(t, responsive)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := s.Flush(ctx); !errors.Is(err, context.DeadlineExceeded) || diagnostic.CodeOf(err) != diagnostic.SettingsWait {
		t.Fatalf("Flush=%v", err)
	}
	f := settingsFlight(s)
	if f == nil || !s.IsDirty() {
		t.Fatal("timeout reported actual writer finished")
	}
	select {
	case <-f.done:
		t.Fatal("writer was not joined")
	default:
	}
	once.Do(func() { close(release) })
	settingsWait(t, f.done)
	if f.err != nil || s.IsDirty() || calls.Load() != 2 {
		t.Fatalf("err=%v dirty=%v writes=%d", f.err, s.IsDirty(), calls.Load())
	}
	reloaded := New(path)
	if reloaded.String("key", "") != "latest" || reloaded.Int("revision", -1) != 9999 {
		t.Fatal("coalescing lost latest values")
	}
}

func TestSettingsOlderSnapshotCannotClearNewerDirtyEdits(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "prefs"))
	settingsSet(t, s, "old")
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }); _ = s.Sync() })
	s.writeSnapshot = func(path string, d *core.TDoc) error {
		select {
		case <-started:
		default:
			close(started)
		}
		<-release
		return writeSettingsSnapshot(path, d)
	}
	_ = s.RequestSync()
	settingsWait(t, started)
	f := settingsFlight(s)
	settingsSet(t, s, "new") // deliberately no RequestSync
	once.Do(func() { close(release) })
	settingsWait(t, f.done)
	if !s.IsDirty() || New(s.Path()).String("key", "") != "old" {
		t.Fatal("old snapshot acknowledged newer edits")
	}
	if err := s.Sync(); err != nil {
		t.Fatal(err)
	}
	if s.IsDirty() || New(s.Path()).String("key", "") != "new" {
		t.Fatal("explicit Sync did not persist pending generation")
	}
}

func TestSettingsWriteFailureIsCodedDirtyAndRetryable(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "prefs"))
	settingsSet(t, s, "old")
	if err := s.Sync(); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(s.Path())
	settingsSet(t, s, "new")
	s.writeSnapshot = func(string, *core.TDoc) error { return io.ErrClosedPipe }
	err := s.Sync()
	var detail *diagnostic.Error
	if diagnostic.CodeOf(err) != diagnostic.SettingsWrite || !errors.Is(err, io.ErrClosedPipe) || !errors.As(err, &detail) || detail.Location.File != s.Path() || detail.Hint == "" {
		t.Fatalf("missing diagnostic: %v", err)
	}
	if !s.IsDirty() || s.Status() == nil {
		t.Fatal("failed write acknowledged")
	}
	after, _ := os.ReadFile(s.Path())
	if string(after) != string(before) {
		t.Fatal("failed write changed old file")
	}
	s.writeSnapshot = nil
	if err = s.Sync(); err != nil || s.Status() != nil || s.IsDirty() {
		t.Fatalf("retry: %v status=%v dirty=%v", err, s.Status(), s.IsDirty())
	}
}

func TestSettingsCorruptOriginalIsBackedUpBeforeRecovery(t *testing.T) {
	for _, clear := range []bool{false, true} {
		path := filepath.Join(t.TempDir(), "prefs")
		original := []byte{0x1f, 0x8b, 8}
		if err := os.WriteFile(path, original, 0600); err != nil {
			t.Fatal(err)
		}
		s := New(path)
		if clear {
			s.Clear()
		} else {
			settingsSet(t, s, "new")
		}
		backup := s.RecoveryBackup()
		got, err := os.ReadFile(backup)
		if err != nil || !bytes.Equal(got, original) {
			t.Fatalf("missing exact backup: %q %v", backup, err)
		}
		got, _ = os.ReadFile(path)
		if !bytes.Equal(got, original) {
			t.Fatal("load mutated original")
		}
		if err = s.Sync(); err != nil {
			t.Fatal(err)
		}
		if !clear && New(path).String("key", "") != "new" {
			t.Fatal("recovered preferences not saved")
		}
		got, _ = os.ReadFile(backup)
		if !bytes.Equal(got, original) {
			t.Fatal("sync damaged backup")
		}
	}
}

func TestSettingsAtomicWriterPreservesOriginalAndCleansTemp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prefs")
	original := []byte("=\nkey = original\n")
	if err := os.WriteFile(path, original, 0640); err != nil {
		t.Fatal(err)
	}
	err := writeSettingsFile(path, func(w io.Writer) error { _, _ = w.Write([]byte("partial")); return io.ErrClosedPipe })
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(original) {
		t.Fatal("encoding error truncated original")
	}
	files, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".silk-settings-*"))
	if err != nil || len(files) != 0 {
		t.Fatalf("temporary leak: %v %v", files, err)
	}
	d := core.NewTDoc()
	_ = d.WriteAttr("key", "replacement")
	if err = writeSettingsSnapshot(path, d); err != nil {
		t.Fatal(err)
	}
	if New(path).String("key", "") != "replacement" {
		t.Fatal("replacement failed")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0640 {
		t.Fatalf("permissions changed: %v", info.Mode())
	}
	// Refuse a directory rather than removing it to make replacement succeed.
	dir := filepath.Join(filepath.Dir(path), "directory")
	if err = os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err = writeSettingsSnapshot(dir, d); err == nil {
		t.Fatal("directory replaced")
	}
}

func TestSettingsConcurrentSyncUsesOneWriterAndJoins(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "prefs"))
	settingsSet(t, s, "old")
	var active, maxActive atomic.Int32
	s.writeSnapshot = func(path string, d *core.TDoc) error {
		n := active.Add(1)
		for {
			old := maxActive.Load()
			if n <= old || maxActive.CompareAndSwap(old, n) {
				break
			}
		}
		defer active.Add(-1)
		return writeSettingsSnapshot(path, d)
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				_ = s.SetInt("value", int64(j))
				if err := s.Sync(); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
	if err := s.Sync(); err != nil {
		t.Fatal(err)
	}
	if maxActive.Load() != 1 || active.Load() != 0 || settingsFlight(s) != nil || s.IsDirty() {
		t.Fatalf("unjoined/parallel writer max=%d active=%d", maxActive.Load(), active.Load())
	}
}

func TestSettingsAtomicReadersNeverSeePartialDocument(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prefs")
	s := New(path)
	old := strings.Repeat("old", 5000)
	newValue := strings.Repeat("new", 5000)
	settingsSet(t, s, old)
	if err := s.Sync(); err != nil {
		t.Fatal(err)
	}
	stop, done := make(chan struct{}), make(chan struct{})
	errs := make(chan error, 1)
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
			}
			got, err := core.LoadTDocFile(path)
			if err == nil {
				var value string
				err = got.ReadAttr("key", &value)
				if err == nil && value != old && value != newValue {
					err = errors.New("partial snapshot visible")
				}
			}
			if err != nil {
				select {
				case errs <- err:
				default:
				}
				return
			}
		}
	}()
	for i := 0; i < 20; i++ {
		settingsSet(t, s, newValue)
		if err := s.Sync(); err != nil {
			t.Error(err)
		}
		settingsSet(t, s, old)
		if err := s.Sync(); err != nil {
			t.Error(err)
		}
	}
	close(stop)
	settingsWait(t, done)
	select {
	case err := <-errs:
		t.Fatal(err)
	default:
	}
}

func TestSettingsValidatesCompressedTrailerBeforeAcceptingFile(t *testing.T) {
	d := core.NewTDoc()
	_ = d.WriteAttr("key", "original")
	var compressed bytes.Buffer
	if err := d.Save1(&compressed, true); err != nil {
		t.Fatal(err)
	}
	good := compressed.Bytes()
	for _, kind := range []string{"valid", "truncated", "checksum"} {
		t.Run(kind, func(t *testing.T) {
			data := append([]byte(nil), good...)
			switch kind {
			case "truncated":
				data = data[:len(data)-4]
			case "checksum":
				data[len(data)-8] ^= 0xff
			}
			path := filepath.Join(t.TempDir(), "prefs")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			s := New(path)
			value := s.String("key", "")
			if kind == "valid" {
				if value != "original" || s.Status() != nil {
					t.Fatalf("valid gzip: %q %v", value, s.Status())
				}
				return
			}
			if value != "" || s.RecoveryBackup() == "" {
				t.Fatalf("corrupt gzip accepted: %q %v", value, s.Status())
			}
			settingsSet(t, s, "new")
			if err := s.Sync(); err != nil {
				t.Fatal(err)
			}
			got, _ := os.ReadFile(s.RecoveryBackup())
			if !bytes.Equal(got, data) {
				t.Fatal("corrupt file overwritten")
			}
		})
	}
}
