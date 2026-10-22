package fileops

import (
	"errors"
	"golang.org/x/sys/unix"
)

func RenameNoReplace(oldPath, newPath string) error {
	err := unix.Renameat2(unix.AT_FDCWD, oldPath, unix.AT_FDCWD, newPath, unix.RENAME_NOREPLACE)
	if errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EINVAL) || errors.Is(err, unix.ENOSYS) {
		return errors.Join(ErrRenameUnsupported, err)
	}
	return err
}
