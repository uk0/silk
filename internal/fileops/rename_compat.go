package fileops

import (
	"errors"
	"os"
	"sync"
)

// ErrRenameUnsupported means the volume cannot promise atomic no-replacement.
var ErrRenameUnsupported = errors.New("filesystem does not support atomic no-replace rename")

var compatibilityRenameMu sync.Mutex

// RenameCompatible must only be used after explicit opt-in. It tries the safe
// native operation first. On unsupported volumes it serializes our callers and
// rejects every existing destination (including dangling links), but an external
// process can still create a destination between Lstat and Rename. This is NOT
// an atomic no-replace guarantee. Never downgrade permission or other errors.
func RenameCompatible(oldPath, newPath string) error {
	return renameCompatible(oldPath, newPath, RenameNoReplace)
}

func renameCompatible(oldPath, newPath string, native func(string, string) error) error {
	compatibilityRenameMu.Lock()
	defer compatibilityRenameMu.Unlock()
	err := native(oldPath, newPath)
	if !errors.Is(err, ErrRenameUnsupported) {
		return err
	}
	if _, err := os.Lstat(newPath); err == nil {
		return &os.LinkError{Op: "rename", Old: oldPath, New: newPath, Err: os.ErrExist}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(oldPath, newPath)
}
