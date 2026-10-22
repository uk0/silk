package fileops

import (
	"errors"
	"golang.org/x/sys/unix"
)

// RenameNoReplace atomically moves oldPath to newPath without replacing an
// existing file, directory or dangling symlink. Filesystem support is required;
// an unsupported operation fails closed instead of falling back to os.Rename.
func RenameNoReplace(oldPath, newPath string) error {
	err := unix.RenamexNp(oldPath, newPath, unix.RENAME_EXCL)
	if errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EINVAL) || errors.Is(err, unix.ENOSYS) {
		return errors.Join(ErrRenameUnsupported, err)
	}
	return err
}
