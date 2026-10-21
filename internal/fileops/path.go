package fileops

import (
	"path/filepath"
	"strings"
)

// RebasePath maps oldPath and its descendants onto newPath. This is a lexical
// path operation (no I/O, symlink resolution or inode comparisons). It respects
// component boundaries and the platform's filepath.Rel volume/case semantics.
// Empty paths never match; callers retain the original path on a non-match.
func RebasePath(path, oldPath, newPath string) (string, bool) {
	if path == "" || oldPath == "" || newPath == "" {
		return path, false
	}
	rel, err := filepath.Rel(filepath.Clean(oldPath), filepath.Clean(path))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return path, false
	}
	return filepath.Join(newPath, rel), true
}
