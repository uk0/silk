package settings

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/uk0/silk/core"
)

// Settings holds an in-memory TDoc-backed key/value store. Reads and
// writes are concurrent-safe via an internal RWMutex; the TDoc itself
// is not goroutine-safe, so all access goes through the lock.
//
// The persistence model is "lazy load, explicit flush": opening a file
// that doesn't exist yet does not error — it produces an empty Settings
// whose first Sync writes a fresh file. SetValue marks the instance
// dirty; Sync writes only if dirty so frequent ticking syncs are cheap
// no-ops.
type Settings struct {
	mu sync.RWMutex

	// path is the on-disk file location. Empty for in-memory-only
	// instances (NewMemory).
	path string

	// doc is the in-memory TDoc carrying the entire settings tree.
	// Allocated lazily so Default(...) followed by Status() doesn't
	// touch the disk.
	doc *core.TDoc

	// groupStack records the current BeginGroup chain. SetValue/Value
	// prepend join(groupStack, "/") to the user's key. EndGroup pops.
	groupStack []string

	// arrayCtx records the active beginWriteArray / beginReadArray
	// position. Nil when no array is being written.
	arrayCtx *arrayContext

	// dirty flags whether SetValue / Remove / Clear has been called
	// since the last Sync. Sync writes only when dirty.
	dirty bool

	// lastErr captures the most recent IO failure for Status() to
	// surface. Cleared on a subsequent successful Sync.
	lastErr error

	// loaded tracks whether the file has been read from disk. We
	// defer the read to first access so callers that always SetValue
	// without reading don't pay the IO cost.
	loaded bool
}

// arrayContext tracks the cursor inside a beginWriteArray / beginReadArray
// region. Index is 1-based per Qt convention.
type arrayContext struct {
	prefix string // group prefix at array start
	index  int    // current array index (1-based)
	read   bool   // true when read-only
}

// New constructs a Settings rooted at path. The file is not loaded
// until the first read or write. A path of "" yields an in-memory
// instance (Sync is then a no-op + sets Status to errInMemoryOnly).
//
// For platform-default paths use Default(org, app) — that path
// resolution is OS-aware (macOS Application Support, XDG on Linux,
// %APPDATA% on Windows).
func New(path string) *Settings {
	return &Settings{path: path}
}

// NewMemory returns a Settings with no backing file. All operations
// happen in memory; useful for tests and ephemeral runs. Sync is a
// no-op and returns nil.
func NewMemory() *Settings {
	return &Settings{}
}

// Default opens (or prepares to open) the user's per-app settings
// file. The path is platform-dependent — see DefaultPath. Errors
// from path resolution are deferred to Sync; this constructor never
// fails.
func Default(org, app string) *Settings {
	path := DefaultPath(org, app)
	return New(path)
}

// Path returns the backing file path. Empty for in-memory instances.
func (s *Settings) Path() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.path
}

// loadOnce reads the backing file into doc. Caller must hold mu.Lock.
// Missing files are not an error — we treat them as an empty doc and
// defer creation to Sync.
func (s *Settings) loadOnce() {
	if s.loaded {
		return
	}
	s.loaded = true
	if s.path == "" {
		s.doc = core.NewTDoc()
		return
	}
	d, err := core.LoadTDocFile(s.path)
	if err != nil {
		// Missing file: start empty. Other read errors get surfaced via
		// Status() but don't prevent runtime use — a fresh Sync will
		// overwrite if writable.
		if !os.IsNotExist(err) {
			s.lastErr = err
		}
		s.doc = core.NewTDoc()
		return
	}
	s.doc = d
}

// keyPath joins the group stack with the user's key, separated by /.
// "editor" + "fontSize" → "editor/fontSize"; deeper groups stack:
// ["editor","syntax"] + "color" → "editor/syntax/color".
//
// Caller may pass a key that itself contains / (multi-level shortcut);
// it concatenates without further splitting.
func (s *Settings) keyPath(key string) string {
	key = strings.TrimSpace(key)
	if len(s.groupStack) == 0 {
		return key
	}
	prefix := strings.Join(s.groupStack, "/")
	if key == "" {
		return prefix
	}
	return prefix + "/" + key
}

// --- Group hierarchy --------------------------------------------------

// BeginGroup pushes name onto the group stack. Subsequent SetValue /
// Value / Contains / Remove calls treat their keys as relative to the
// current group. Must be matched by an EndGroup at the same nesting
// depth — unbalanced calls are diagnosable via Group() returning the
// current full prefix.
//
// Empty names are ignored to keep API tolerant against e.g. dynamic
// group construction that produces a blank string.
func (s *Settings) BeginGroup(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.groupStack = append(s.groupStack, name)
}

// EndGroup pops the most recent BeginGroup. No-op when no group is
// active so well-meaning callers don't crash on early-out paths.
func (s *Settings) EndGroup() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n := len(s.groupStack); n > 0 {
		s.groupStack = s.groupStack[:n-1]
	}
}

// Group returns the current group prefix joined by "/". Empty string
// when no BeginGroup is active.
func (s *Settings) Group() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return strings.Join(s.groupStack, "/")
}

// --- Read / Write -----------------------------------------------------

// SetValue writes value at key, creating any intermediate groups as
// needed. value must be a type the persist layer accepts (string,
// numeric, bool, []string). Type mismatch returns an error rather
// than panicking on the persist boundary.
//
// Marks the instance dirty; Sync flushes it.
func (s *Settings) SetValue(key string, value interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loadOnce()
	full := s.keyPath(key)
	if full == "" {
		return errors.New("settings: empty key")
	}
	if err := s.doc.WriteAttr(full, value); err != nil {
		return err
	}
	s.dirty = true
	return nil
}

// Value reads key into a Go value. The first variadic argument, if
// present, is returned when the key is missing — typed default. Without
// a default, missing keys return nil.
//
// Decoding strategy: TDoc stores everything as a persisted string. We
// return the raw string here and let the caller pick a type via the
// typed accessors (Bool, Int, Float64, String, StringList) which
// re-decode through PersistSscan.
//
// Lock policy: takes the write lock so loadOnce can run safely on the
// first call. The cost is negligible for read-heavy workloads because
// settings access is rare relative to UI updates.
func (s *Settings) Value(key string, def ...interface{}) interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loadOnce()
	full := s.keyPath(key)
	var raw string
	err := s.doc.ReadAttr(full, &raw)
	if err != nil {
		if len(def) > 0 {
			return def[0]
		}
		return nil
	}
	return raw
}

// Contains reports whether key is present in the active group.
func (s *Settings) Contains(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loadOnce()
	full := s.keyPath(key)
	if full == "" {
		return false
	}
	var raw string
	return s.doc.ReadAttr(full, &raw) == nil
}

// Remove deletes the entry at key. Removing a group prefix removes
// every nested key under it (Qt's recursive-remove semantics).
//
// Mark dirty regardless of whether the key existed — Qt's remove also
// dirties even on a missing key.
//
// Implementation note: the underlying TDoc.AddChild does not set the
// parent pointer on the inserted node, so calling Detach() on the
// removed node is a no-op (the parent.subs slice still references it).
// We instead Clear() the node — wiping its value and any nested
// children — and let AllKeys / Contains skip value-less leaves. This
// matches the observable Qt behaviour without modifying core/tdoc.
func (s *Settings) Remove(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loadOnce()
	full := s.keyPath(key)
	if full == "" {
		return
	}
	node := s.doc.InnerNodeByKeyPath(full, false)
	if node != nil {
		node.Clear()
	}
	s.dirty = true
}

// Clear empties the entire settings store, including all groups.
// Equivalent to deleting the backing file but in-memory.
func (s *Settings) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loaded = true
	s.doc = core.NewTDoc()
	s.dirty = true
}

// AllKeys returns every leaf key (no group prefixes), filtered to the
// current group when one is active. Result is sorted for determinism —
// callers iterating in tests don't have to depend on map order.
//
// "Leaf key" = a TDoc node carrying a value. Nodes that are pure
// containers (e.g. groups with no own value) are not returned.
func (s *Settings) AllKeys() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loadOnce()
	prefix := strings.Join(s.groupStack, "/")
	var node *core.TDoc
	if prefix == "" {
		node = s.doc
	} else {
		node = s.doc.InnerNodeByKeyPath(prefix, false)
		if node == nil {
			return nil
		}
	}
	var keys []string
	collectLeafKeys(node, "", &keys)
	// Sort for stable iteration — settings rarely have huge key counts
	// (hundreds at most) so the sort cost is invisible.
	sortStrings(keys)
	return keys
}

func collectLeafKeys(node *core.TDoc, prefix string, out *[]string) {
	if node == nil {
		return
	}
	if node.HasValue() && prefix != "" {
		*out = append(*out, prefix)
	}
	for _, sub := range node.Childdren() {
		k := sub.Key()
		if k == "" {
			continue
		}
		next := k
		if prefix != "" {
			next = prefix + "/" + k
		}
		collectLeafKeys(sub, next, out)
	}
}

// --- Sync + status ---------------------------------------------------

// Sync flushes pending writes to disk. No-op when nothing has changed
// since the last Sync, or when this is an in-memory instance. Errors
// are saved into Status() and returned from this call.
//
// Creates parent directories as needed — apps invoked from a fresh
// install on a host with no ~/.config/MyOrg can still persist on first
// run.
func (s *Settings) Sync() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.path == "" {
		// In-memory mode: no-op success. Marking !dirty so subsequent
		// calls don't block on the path-empty check repeatedly.
		s.dirty = false
		return nil
	}
	if !s.loaded {
		s.loadOnce()
	}
	if !s.dirty {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		s.lastErr = fmt.Errorf("settings: mkdir %s: %w", filepath.Dir(s.path), err)
		return s.lastErr
	}
	if err := s.doc.SaveFile(s.path); err != nil {
		s.lastErr = fmt.Errorf("settings: write %s: %w", s.path, err)
		return s.lastErr
	}
	s.dirty = false
	s.lastErr = nil
	return nil
}

// Status returns the most recent IO error, or nil. Cleared on the next
// successful Sync. Useful for hosts that want to surface a "settings
// corrupted, falling back to defaults" state to the user.
func (s *Settings) Status() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastErr
}

// IsDirty reports whether there are pending writes that Sync would
// flush. Cheap inspection used by tooling that wants to gate "Save"
// menu items.
func (s *Settings) IsDirty() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.dirty
}

// sortStrings is a small ascending sort. We avoid pulling in "sort"
// for one call site; settings tables are small and the body of this
// function dominates the cost of the entire AllKeys path anyway.
func sortStrings(a []string) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j-1] > a[j]; j-- {
			a[j-1], a[j] = a[j], a[j-1]
		}
	}
}
