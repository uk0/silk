package fileops

import "golang.org/x/sys/unix"

// RenameNoReplace atomically moves oldPath to newPath without replacing an
// existing file, directory or dangling symlink. Filesystem support is required;
// an unsupported operation fails closed instead of falling back to os.Rename.
func RenameNoReplace(oldPath, newPath string) error {
	return unix.RenamexNp(oldPath, newPath, unix.RENAME_EXCL)
}
