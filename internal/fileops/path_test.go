package fileops

import (
	"path/filepath"
	"testing"
)

func TestRebasePathBoundaries(t *testing.T) {
	root := t.TempDir()
	old, next := filepath.Join(root, "foo"), filepath.Join(root, "bar")
	for _, tc := range []struct {
		path, want string
		match      bool
	}{
		{old, next, true}, {filepath.Join(old, "中文.go"), filepath.Join(next, "中文.go"), true},
		{filepath.Join(old, "nested", "one.go"), filepath.Join(next, "nested", "one.go"), true},
		{old + "bar", old + "bar", false}, {root, root, false}, {"", "", false},
		{filepath.Join(old, "..", "other.go"), filepath.Join(root, "other.go"), false},
	} {
		if got, ok := RebasePath(tc.path, old, next); got != tc.want || ok != tc.match {
			t.Fatalf("%q => %q/%v, want %q/%v", tc.path, got, ok, tc.want, tc.match)
		}
	}
}
