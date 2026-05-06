package settings

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// fresh returns a memory-backed Settings so tests don't touch disk.
func fresh() *Settings { return NewMemory() }

// TestSetValueGetValueRoundTrip pins the most basic contract: Value
// returns whatever SetValue stored. Strings round-trip as the
// canonical "raw" representation per the Value documentation.
func TestSetValueGetValueRoundTrip(t *testing.T) {
	s := fresh()
	if err := s.SetValue("name", "Alice"); err != nil {
		t.Fatalf("SetValue: %v", err)
	}
	if got := s.Value("name"); got != "Alice" {
		t.Errorf("Value(name) = %v, want Alice", got)
	}
}

// TestValueDefaultMissing returns the supplied default when the key
// isn't present. Without a default, Value returns nil.
func TestValueDefaultMissing(t *testing.T) {
	s := fresh()
	if got := s.Value("missing", "fallback"); got != "fallback" {
		t.Errorf("Value(missing, fallback) = %v, want fallback", got)
	}
	if got := s.Value("missing"); got != nil {
		t.Errorf("Value(missing) = %v, want nil", got)
	}
}

// TestBoolRoundTrip exercises the typed Bool getter / setter,
// including a few of the documented "yes/no/1/0" lenient forms.
func TestBoolRoundTrip(t *testing.T) {
	s := fresh()
	s.SetBool("enabled", true)
	if !s.Bool("enabled", false) {
		t.Errorf("Bool(enabled) = false, want true")
	}
	s.SetValue("legacyYes", "yes")
	if !s.Bool("legacyYes", false) {
		t.Errorf("Bool(legacyYes) should accept 'yes'")
	}
	if s.Bool("missing", false) {
		t.Errorf("Bool(missing, false) returned the wrong default")
	}
	if !s.Bool("missing", true) {
		t.Errorf("Bool(missing, true) returned the wrong default")
	}
}

// TestIntRoundTrip pins the int64 path: SetInt then Int returns the
// same value, and a non-numeric raw value falls back to default.
func TestIntRoundTrip(t *testing.T) {
	s := fresh()
	s.SetInt("count", 42)
	if got := s.Int("count", -1); got != 42 {
		t.Errorf("Int(count) = %d, want 42", got)
	}
	s.SetValue("garbage", "not-a-number")
	if got := s.Int("garbage", 99); got != 99 {
		t.Errorf("Int(garbage, 99) = %d, want 99 (parse fail → default)", got)
	}
}

func TestFloat64RoundTrip(t *testing.T) {
	s := fresh()
	s.SetFloat64("ratio", 0.5)
	if got := s.Float64("ratio", 0); got != 0.5 {
		t.Errorf("Float64(ratio) = %v, want 0.5", got)
	}
}

func TestStringRoundTrip(t *testing.T) {
	s := fresh()
	s.SetString("name", "Bob")
	if got := s.String("name", ""); got != "Bob" {
		t.Errorf("String(name) = %q, want Bob", got)
	}
}

// TestStringListRoundTrip covers the comma-separated encoding and the
// double-comma literal-comma escape.
func TestStringListRoundTrip(t *testing.T) {
	s := fresh()
	want := []string{"alpha", "beta", "ga,mma"} // gamma has a literal comma
	s.SetStringList("items", want)
	if got := s.StringList("items", nil); !reflect.DeepEqual(got, want) {
		t.Errorf("StringList = %#v, want %#v", got, want)
	}
}

// TestStringListEmpty is the boundary case: SetStringList(nil) +
// StringList must round-trip as an empty (non-nil) slice.
func TestStringListEmpty(t *testing.T) {
	s := fresh()
	s.SetStringList("items", nil)
	got := s.StringList("items", nil)
	if got == nil {
		t.Errorf("StringList(empty) = nil, want non-nil empty slice")
	}
	if len(got) != 0 {
		t.Errorf("StringList(empty) len = %d, want 0", len(got))
	}
}

// TestBeginGroupNests prefixes every key with the active group name.
// Multiple BeginGroup calls compose hierarchically.
func TestBeginGroupNests(t *testing.T) {
	s := fresh()
	s.BeginGroup("editor")
	s.SetValue("fontSize", "14")
	s.BeginGroup("syntax")
	s.SetValue("highlight", "true")
	s.EndGroup()
	s.EndGroup()

	if got := s.Value("editor/fontSize"); got != "14" {
		t.Errorf("editor/fontSize = %v, want 14", got)
	}
	if got := s.Value("editor/syntax/highlight"); got != "true" {
		t.Errorf("editor/syntax/highlight = %v, want true", got)
	}
}

// TestBeginGroupAffectsValue: reads inside a group resolve relative.
func TestBeginGroupAffectsValue(t *testing.T) {
	s := fresh()
	s.SetValue("editor/fontSize", "14")
	s.BeginGroup("editor")
	if got := s.Value("fontSize"); got != "14" {
		t.Errorf("inside group editor: fontSize = %v, want 14", got)
	}
}

// TestEndGroupNoActiveDoesNotPanic.
func TestEndGroupNoActiveDoesNotPanic(t *testing.T) {
	s := fresh()
	s.EndGroup() // no active group
	s.EndGroup() // still no panic
	if g := s.Group(); g != "" {
		t.Errorf("Group() after empty stack = %q, want empty", g)
	}
}

// TestBeginGroupEmptyName is a no-op.
func TestBeginGroupEmptyName(t *testing.T) {
	s := fresh()
	s.BeginGroup("")
	if g := s.Group(); g != "" {
		t.Errorf("BeginGroup(\"\") should be no-op, got Group=%q", g)
	}
}

// TestContains reports presence accurately, before and after writes.
func TestContains(t *testing.T) {
	s := fresh()
	if s.Contains("k") {
		t.Errorf("Contains on empty: true, want false")
	}
	s.SetValue("k", "v")
	if !s.Contains("k") {
		t.Errorf("Contains after SetValue: false, want true")
	}
}

// TestRemoveDeletes a single key.
func TestRemoveDeletes(t *testing.T) {
	s := fresh()
	s.SetValue("k", "v")
	s.Remove("k")
	if s.Contains("k") {
		t.Errorf("Contains after Remove: true, want false")
	}
}

// TestClearWipesAllKeys.
func TestClearWipesAllKeys(t *testing.T) {
	s := fresh()
	s.SetValue("a", "1")
	s.SetValue("b", "2")
	s.Clear()
	if got := s.AllKeys(); len(got) != 0 {
		t.Errorf("AllKeys after Clear = %v, want empty", got)
	}
}

// TestAllKeysReturnsLeafPaths includes nested keys with / separator.
func TestAllKeysReturnsLeafPaths(t *testing.T) {
	s := fresh()
	s.SetValue("a", "1")
	s.SetValue("b/c", "2")
	s.SetValue("b/d", "3")

	got := s.AllKeys()
	want := []string{"a", "b/c", "b/d"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("AllKeys = %v, want %v", got, want)
	}
}

// TestAllKeysFilteredByActiveGroup.
func TestAllKeysFilteredByActiveGroup(t *testing.T) {
	s := fresh()
	s.SetValue("a", "1")
	s.SetValue("b/c", "2")
	s.SetValue("b/d", "3")

	s.BeginGroup("b")
	got := s.AllKeys()
	want := []string{"c", "d"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("AllKeys inside group b = %v, want %v", got, want)
	}
}

// TestIsDirtyTracksSetValue.
func TestIsDirtyTracksSetValue(t *testing.T) {
	s := fresh()
	if s.IsDirty() {
		t.Errorf("fresh: IsDirty = true, want false")
	}
	s.SetValue("k", "v")
	if !s.IsDirty() {
		t.Errorf("after SetValue: IsDirty = false, want true")
	}
}

// TestSyncWritesAndReloadsRecovers covers the core persistence flow:
// write through one Settings instance, drop it, reload from disk via
// a fresh instance, see the same values come back.
func TestSyncWritesAndReloadsRecovers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.silkui")

	src := New(path)
	src.SetValue("name", "Alice")
	src.SetInt("count", 42)
	src.BeginGroup("editor")
	src.SetValue("font", "Mono")
	src.EndGroup()
	if err := src.Sync(); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	dst := New(path)
	if got := dst.Value("name"); got != "Alice" {
		t.Errorf("reloaded name = %v, want Alice", got)
	}
	if got := dst.Int("count", 0); got != 42 {
		t.Errorf("reloaded count = %d, want 42", got)
	}
	if got := dst.Value("editor/font"); got != "Mono" {
		t.Errorf("reloaded editor/font = %v, want Mono", got)
	}
}

// TestSyncNoDirtySkipsWrite verifies the dirty-tracking optimisation:
// a fresh Sync after no changes is a no-op, no IO needed.
func TestSyncNoDirtySkipsWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "noop.silkui")
	s := New(path)
	if err := s.Sync(); err != nil {
		t.Fatalf("Sync(empty fresh): %v", err)
	}
	// No SetValue → still not dirty → second Sync also no-op.
	if s.IsDirty() {
		t.Errorf("after empty Sync: IsDirty true")
	}
}

// TestSyncCreatesParentDir verifies the convenience: a path under a
// non-existent directory tree is created on first Sync.
func TestSyncCreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deep", "tree", "settings.silkui")
	s := New(path)
	s.SetValue("k", "v")
	if err := s.Sync(); err != nil {
		t.Fatalf("Sync should create parent dirs: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file at %s after Sync, got: %v", path, err)
	}
}

// TestStatusReturnsLastError.
func TestStatusReturnsLastError(t *testing.T) {
	// Try to write to an unwritable path. We use /dev/null/foo which
	// should fail mkdir on most systems.
	s := New("/dev/null/badpath/x.silkui")
	s.SetValue("k", "v")
	err := s.Sync()
	if err == nil {
		t.Skip("system allowed write to /dev/null subpath; skipping error path test")
	}
	if s.Status() == nil {
		t.Errorf("Status() should mirror last Sync error")
	}
}

// TestNewMemorySyncIsNoOp confirms in-memory mode never writes.
func TestNewMemorySyncIsNoOp(t *testing.T) {
	s := NewMemory()
	s.SetValue("k", "v")
	if err := s.Sync(); err != nil {
		t.Errorf("memory Sync should never error: %v", err)
	}
	if s.Path() != "" {
		t.Errorf("memory Settings Path() = %q, want empty", s.Path())
	}
}

// TestDefaultPathPlatformSpecific spot-checks that DefaultPath produces
// a sensible path for the current OS — the exact form varies, but it
// must contain the org and app names.
func TestDefaultPathPlatformSpecific(t *testing.T) {
	got := DefaultPath("MyOrg", "MyApp")
	if got == "" {
		t.Fatalf("DefaultPath returned empty")
	}
	if !strings.Contains(got, "MyOrg") || !strings.Contains(got, "MyApp") {
		t.Errorf("DefaultPath = %q, want path containing MyOrg and MyApp", got)
	}

	// Spot-check the platform-specific structure where reasonable.
	switch runtime.GOOS {
	case "darwin":
		if !strings.Contains(got, "Library/Application Support") {
			t.Errorf("macOS path = %q, want Library/Application Support segment", got)
		}
	case "windows":
		if !strings.Contains(got, "AppData") {
			t.Errorf("Windows path = %q, want AppData segment", got)
		}
	}
}

func TestDefaultPathEmptyArgsHasFallbacks(t *testing.T) {
	got := DefaultPath("", "")
	if !strings.Contains(got, "Silk") || !strings.Contains(got, "App") {
		t.Errorf("DefaultPath(\"\",\"\") = %q, want Silk/App fallback", got)
	}
}
