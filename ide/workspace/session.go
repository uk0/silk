// Package workspace models an IDE session: the versioned, on-disk record
// of what a window had open — the project, the editor tabs (with their
// cursor, scroll position and folded regions), the split/pane layout, dock
// visibility, and the text of buffers that were never written to disk.
//
// It exists to replace silkide's original session format, a bare list of
// paths persisted through preferences and rebuilt by ranging a Go map, so
// the restored tab order differed on every launch (cmd/silkide/main.go
// currentSessionPaths / restoreSession). Everything here is ordered: Tabs
// is a slice, ActiveTab indexes it, and Save normalizes the parts a host
// naturally builds from maps (fold lines, dock state, unsaved buffers) so
// saving the same session twice yields byte-identical files.
//
// Sessions are versioned and Load never rejects an older layout. A file
// with no version is v0 and is migrated forward, which covers both
// historical shapes: a bare JSON array of paths (what preferences held)
// and the flat object ged.SessionState wrote.
package workspace

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

// Version is the session layout this package writes. Save stamps it and
// Load migrates anything lower.
const Version = 1

// Pos is a caret position inside a file. Both fields are 0-based — the
// convention gui.CodeEditor.CursorLine / CursorCol report — so a host
// round-trips them without arithmetic.
type Pos struct {
	Line int `json:"line"`
	Col  int `json:"col"`
}

// Tab is one open editor tab.
//
// Scroll is the first visible line index, not a pixel offset, so a
// restored tab lands on the same code after a font-size or window-height
// change. Folds lists the start lines of the collapsed regions (the key
// gui.CodeEditor.IsFolded takes); the editor holds those in a map, so
// Save sorts and de-dupes the list on the way out.
type Tab struct {
	Path   string `json:"path"`
	Cursor Pos    `json:"cursor"`
	Scroll int    `json:"scroll"`
	Folds  []int  `json:"folds,omitempty"`
}

// Pane is one editor pane of a split layout. Tabs holds indices into
// Session.Tabs in that pane's tab-bar order, and Active is the index
// within Tabs of its focused tab (-1 when the pane is empty). An empty
// Session.Panes means "a single pane holding every tab", which is what a
// migrated v0 session gets.
type Pane struct {
	Tabs   []int `json:"tabs"`
	Active int   `json:"active"`
}

// Split records how the editor area is divided between Session.Panes.
// Orientation is "" for a single pane, "h" for side-by-side and "v" for
// stacked; Ratio is the fraction of the area given to the first pane.
type Split struct {
	Orientation string  `json:"orientation,omitempty"`
	Ratio       float64 `json:"ratio,omitempty"`
}

// Session is the restorable state of one IDE window.
type Session struct {
	// Version is the layout this session was written with. Save always
	// stamps the current Version; Load migrates older values forward.
	Version int `json:"version"`

	// Project is the project / .silkui file the window was working on,
	// "" when none.
	Project string `json:"project,omitempty"`

	// Tabs are the open editor tabs in tab-bar order. That order is the
	// point of this type — it is what the previous format lost.
	Tabs []Tab `json:"tabs,omitempty"`

	// ActiveTab indexes Tabs, or -1 when nothing is open.
	ActiveTab int `json:"active_tab"`

	// Panes and Split describe the editor split layout.
	Panes []Pane `json:"panes,omitempty"`
	Split Split  `json:"split"`

	// DockState maps a tool-view id (gui.ToolViewDef.Id) to whether its
	// dock was visible.
	DockState map[string]bool `json:"dock_state,omitempty"`

	// Unsaved carries the buffer text of tabs with unsaved edits, keyed
	// by path, so restoring a session does not silently drop work. Tabs
	// absent here are re-read from disk.
	Unsaved map[string]string `json:"unsaved,omitempty"`
}

// New returns an empty session stamped with the current Version and no
// active tab.
func New() *Session {
	return &Session{Version: Version, ActiveTab: -1}
}

// FromPaths builds a session from an ordered path list — the v0 shape,
// and what a caller migrating silkide's preferences-backed session
// already holds. The first path becomes the active tab and per-tab state
// starts zeroed. Empty entries and repeats are dropped, keeping the first
// occurrence: the v0 producer appended the design canvas' file and then
// every editor path, so duplicates are expected input.
func FromPaths(paths []string) *Session {
	s := New()
	seen := make(map[string]bool, len(paths))
	for _, p := range paths {
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		s.Tabs = append(s.Tabs, Tab{Path: p})
	}
	if len(s.Tabs) > 0 {
		s.ActiveTab = 0
	}
	return s
}

// Paths returns the tab paths in tab-bar order.
func (s *Session) Paths() []string {
	out := make([]string, 0, len(s.Tabs))
	for _, t := range s.Tabs {
		out = append(out, t.Path)
	}
	return out
}

// Save writes the session to path as JSON, creating the parent directory
// when needed. The bytes go to a sibling temp file that is renamed into
// place, so a crash mid-write cannot leave a truncated session behind.
//
// The output is normalized first: the current Version is stamped,
// ActiveTab is clamped into range and every tab's Folds is sorted and
// de-duped. Together with encoding/json emitting map keys in sorted
// order, that makes the file a pure function of the session — it changes
// only when the session does, never because a map iterated differently.
// The receiver itself is left untouched.
func (s *Session) Save(path string) error {
	data, err := json.MarshalIndent(s.normalized(), "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// Load reads a session file. A file written by an older silkide is
// migrated instead of rejected, so upgrading never costs the user their
// session:
//
//	v0  a bare JSON array of paths (preferences' OpenSession list), or a
//	    flat object with no "version" key (what ged.SaveSession wrote:
//	    open_files / active_file / last_project)
//	v1  this package's Session
//
// A missing file is returned as an error — check os.IsNotExist to tell
// "first run" from "unreadable session". An empty file yields an empty
// session. A version newer than Version loads best-effort: the fields
// this build knows are kept as they are, and the version is left alone so
// a newer build's file is not silently downgraded.
func Load(path string) (*Session, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return decode(data)
}

// decode parses session bytes, dispatching on the layout it finds.
func decode(data []byte) (*Session, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return New(), nil
	}
	// v0, array form: the file *is* the ordered path list.
	if data[0] == '[' {
		var paths []string
		if err := json.Unmarshal(data, &paths); err != nil {
			return nil, err
		}
		return FromPaths(paths), nil
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	if s.Version == 0 {
		// v0, object form: the same bytes carry legacy keys that v1
		// renamed. Re-read them off the raw document.
		migrateV0(&s, data)
		s.Version = Version
	}
	s.ActiveTab = clampActive(s.ActiveTab, len(s.Tabs))
	return &s, nil
}

// legacyV0 is the pre-versioning object shape (ged.SessionState): a flat
// list of open files with no per-tab state. Only the fields v1 still has
// a home for are read; last_mode and the window geometry are the host's
// business and live in preferences now.
type legacyV0 struct {
	OpenFiles   []string `json:"open_files"`
	ActiveFile  string   `json:"active_file"`
	LastProject string   `json:"last_project"`
}

// migrateV0 folds the legacy keys in data into s. A legacy key is only
// consulted when its v1 counterpart is empty, so a file that already
// carries v1 tabs keeps them.
func migrateV0(s *Session, data []byte) {
	var old legacyV0
	if err := json.Unmarshal(data, &old); err != nil {
		return
	}
	if s.Project == "" {
		s.Project = old.LastProject
	}
	if len(s.Tabs) > 0 || len(old.OpenFiles) == 0 {
		return
	}
	s.Tabs = FromPaths(old.OpenFiles).Tabs
	s.ActiveTab = 0
	for i := range s.Tabs {
		if s.Tabs[i].Path == old.ActiveFile {
			s.ActiveTab = i
			break
		}
	}
}

// normalized returns the copy of s that Save serializes: current Version,
// an in-range ActiveTab, and sorted fold lists.
func (s *Session) normalized() Session {
	out := *s
	out.Version = Version
	out.Tabs = make([]Tab, len(s.Tabs))
	copy(out.Tabs, s.Tabs)
	for i := range out.Tabs {
		out.Tabs[i].Folds = sortedFolds(out.Tabs[i].Folds)
	}
	out.ActiveTab = clampActive(out.ActiveTab, len(out.Tabs))
	return out
}

// clampActive keeps an active-tab index addressable: -1 when there are no
// tabs, otherwise inside [0, n). An out-of-range index falls back to the
// first tab rather than to "nothing focused", which is what a user
// reopening a trimmed session expects.
func clampActive(idx, n int) int {
	if n == 0 {
		return -1
	}
	if idx < 0 || idx >= n {
		return 0
	}
	return idx
}

// sortedFolds returns the fold start lines in ascending order with
// duplicates dropped; nil in, nil out. The editor keeps its folded lines
// in a map, so unsorted and repeated input is normal.
func sortedFolds(in []int) []int {
	if len(in) == 0 {
		return nil
	}
	tmp := make([]int, len(in))
	copy(tmp, in)
	sort.Ints(tmp)
	out := make([]int, 0, len(tmp))
	for i, v := range tmp {
		if i == 0 || v != tmp[i-1] {
			out = append(out, v)
		}
	}
	return out
}
