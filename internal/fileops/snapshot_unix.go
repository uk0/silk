//go:build !windows

package fileops

import "os"

// OpenSnapshot opens a read-only handle; replacing the pathname leaves an
// already-open reader attached to the previous inode. The caller must close it.
func OpenSnapshot(path string) (*os.File, error) { return os.Open(path) }
func ReplaceSnapshotFile(from, to string) error  { return os.Rename(from, to) }
