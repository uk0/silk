package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"silk/core"
)

// TestCoverageForPathExactMatch locks in the exact-match branch of the
// path resolver. `go test -coverprofile` records exact paths the way the
// toolchain saw them, so when the editor's tracked path happens to be
// the same string, the lookup should short-circuit on the map hit
// rather than fall through to the suffix walk.
func TestCoverageForPathExactMatch(t *testing.T) {
	fc := map[string]*core.FileCoverage{
		"silk/foo/bar.go": {
			File:    "silk/foo/bar.go",
			Covered: map[int]bool{1: true, 2: false},
		},
		"silk/baz/qux.go": {
			File:    "silk/baz/qux.go",
			Covered: map[int]bool{1: true},
		},
	}
	got, ok := coverageForPath(fc, "silk/foo/bar.go")
	if !ok {
		t.Fatalf("coverageForPath exact: ok = false, want true")
	}
	if got.File != "silk/foo/bar.go" {
		t.Errorf("got.File = %q, want %q", got.File, "silk/foo/bar.go")
	}
}

// TestCoverageForPathSuffixMatch is the common case: the editor opened
// a file by its absolute path (`/Users/.../dc/silk/foo/bar.go`) while
// the cover profile holds the module-relative form (`silk/foo/bar.go`).
// Suffix-match has to rescue the lookup on a directory boundary —
// otherwise the gutter stripes never appear on real coverage runs.
func TestCoverageForPathSuffixMatch(t *testing.T) {
	fc := map[string]*core.FileCoverage{
		"silk/foo/bar.go": {
			File:    "silk/foo/bar.go",
			Covered: map[int]bool{10: true, 11: false, 12: true},
		},
	}
	editorPath := filepath.Join(string(filepath.Separator)+"Users", "alice",
		"src", "dc", "silk", "foo", "bar.go")
	got, ok := coverageForPath(fc, editorPath)
	if !ok {
		t.Fatalf("coverageForPath suffix: ok = false, want true (editorPath=%q)", editorPath)
	}
	if got.File != "silk/foo/bar.go" {
		t.Errorf("got.File = %q, want %q", got.File, "silk/foo/bar.go")
	}
	if !got.Covered[10] || got.Covered[11] || !got.Covered[12] {
		t.Errorf("got.Covered = %v, want {10:true,11:false,12:true}", got.Covered)
	}
}

// TestCoverageForPathSuffixMatchUsesBoundary guards against the
// "wfoo.go" trap — a naive HasSuffix("foo.go") would match an editor
// path that ends in "wfoo.go", a false positive that would paint the
// wrong file's gutter. The matcher prepends a path separator before
// the key, so only directory-aligned tails hit.
func TestCoverageForPathSuffixMatchUsesBoundary(t *testing.T) {
	fc := map[string]*core.FileCoverage{
		"foo.go": {File: "foo.go", Covered: map[int]bool{1: true}},
	}
	if _, ok := coverageForPath(fc, "/tmp/wfoo.go"); ok {
		t.Errorf("coverageForPath('/tmp/wfoo.go') matched 'foo.go' (no path-boundary check)")
	}
	if _, ok := coverageForPath(fc, "/tmp/foo.go"); !ok {
		t.Errorf("coverageForPath('/tmp/foo.go') failed to match 'foo.go'")
	}
}

// TestCoverageForPathNoMatch covers the negative case — neither exact
// nor suffix matches anything in the profile. The lookup must return
// (nil, false) so the editor's gutter stays clean (vs. erroneously
// painting some other file's coverage stripes).
func TestCoverageForPathNoMatch(t *testing.T) {
	fc := map[string]*core.FileCoverage{
		"silk/foo/bar.go": {File: "silk/foo/bar.go", Covered: map[int]bool{1: true}},
	}
	cov, ok := coverageForPath(fc, "/elsewhere/other.go")
	if ok || cov != nil {
		t.Errorf("coverageForPath no match: got (%v, %v), want (nil, false)", cov, ok)
	}
}

// TestCoverageForPathEmptyPath: an empty editor path means there is no
// file backing the tab yet — buildEditorTabs seeds sample tabs that
// don't live in openEditors, so the walk should drop straight through
// without attempting either match.
func TestCoverageForPathEmptyPath(t *testing.T) {
	fc := map[string]*core.FileCoverage{
		"silk/foo/bar.go": {File: "silk/foo/bar.go", Covered: map[int]bool{1: true}},
	}
	if cov, ok := coverageForPath(fc, ""); ok || cov != nil {
		t.Errorf("coverageForPath('') = (%v, %v), want (nil, false)", cov, ok)
	}
}

// TestCoverageForPathEmptyMap: nothing in the profile (e.g. `go test`
// didn't write any coverage rows because no _test.go files exist). The
// lookup must return cleanly rather than panic on the nil-or-empty map.
func TestCoverageForPathEmptyMap(t *testing.T) {
	if cov, ok := coverageForPath(nil, "/whatever.go"); ok || cov != nil {
		t.Errorf("coverageForPath(nil, ...) = (%v, %v), want (nil, false)", cov, ok)
	}
	if cov, ok := coverageForPath(map[string]*core.FileCoverage{}, "/whatever.go"); ok || cov != nil {
		t.Errorf("coverageForPath(empty, ...) = (%v, %v), want (nil, false)", cov, ok)
	}
}

// TestCoverageForPathParseToFileCoverageRoundTrip wires the real
// core.ParseCoverage + core.BuildFileCoverage pipeline through
// coverageForPath. Locks in the contract that the helper consumes the
// shape these public APIs hand back — if either evolves a future
// refactor will fail this test before the GUI behaviour drifts.
func TestCoverageForPathParseToFileCoverageRoundTrip(t *testing.T) {
	profile := `mode: set
silk/foo/bar.go:10.13,15.2 3 1
silk/foo/bar.go:17.2,17.10 1 0
`
	_, blocks, err := core.ParseCoverage(profile)
	if err != nil {
		t.Fatalf("ParseCoverage: %v", err)
	}
	fc := core.BuildFileCoverage(blocks)
	got, ok := coverageForPath(fc, "/anywhere/silk/foo/bar.go")
	if !ok {
		t.Fatalf("coverageForPath round-trip: ok = false")
	}
	// Lines 10..15 are covered (count=1); line 17 is not (count=0).
	if !got.Covered[10] || !got.Covered[15] {
		t.Errorf("expected lines 10 and 15 covered, got %v", got.Covered)
	}
	if covered, exists := got.Covered[17]; !exists || covered {
		t.Errorf("expected line 17 present-but-uncovered, got covered=%v exists=%v",
			covered, exists)
	}
}

// TestApplyCoverageToOpenEditorsHandlesEmpty: when openEditors is empty
// the walk must complete without panic or work. Covers the common cold-
// start case before any file is opened.
func TestApplyCoverageToOpenEditorsHandlesEmpty(t *testing.T) {
	// Defensive snapshot — keep this test from bleeding into others if
	// silkide's startup code happens to seed openEditors during package
	// init in a future refactor.
	saved := openEditors
	openEditors = nil
	defer func() { openEditors = saved }()
	applyCoverageToOpenEditors(map[string]*core.FileCoverage{
		"silk/foo/bar.go": {File: "silk/foo/bar.go", Covered: map[int]bool{1: true}},
	})
}

// TestCoverageTempFileCleanupHonorsMissing checks the invariant the
// runProjectWithCoverage tear-down relies on: deleting a previously-
// recorded temp file that the OS has already swept must not be a fatal
// error. `os.Remove` returns *PathError wrapping ErrNotExist; the
// runProjectWithCoverage path discards it.
func TestCoverageTempFileCleanupHonorsMissing(t *testing.T) {
	missing := filepath.Join(os.TempDir(), "silkide-cover-does-not-exist.out")
	_ = os.Remove(missing)
	err := os.Remove(missing)
	if err == nil {
		t.Errorf("os.Remove on nonexistent path returned nil error; cleanup contract relies on this returning ErrNotExist")
	}
	if !os.IsNotExist(err) {
		t.Errorf("os.Remove on nonexistent path returned %v, want IsNotExist=true", err)
	}
}

// TestFilePathSeparatorMatchesGOOS sanity-checks the platform separator
// the suffix-match builder uses — Linux and macOS get '/', Windows gets
// '\\'. If runtime.GOOS ever returns something with an unexpected
// separator the matcher's "/" hardcoded fallback would still rescue
// most cases, but we'd rather catch the mismatch in CI.
func TestFilePathSeparatorMatchesGOOS(t *testing.T) {
	want := byte('/')
	if runtime.GOOS == "windows" {
		want = '\\'
	}
	if filepath.Separator != rune(want) {
		t.Errorf("filepath.Separator = %q on %s, want %q",
			filepath.Separator, runtime.GOOS, want)
	}
}
