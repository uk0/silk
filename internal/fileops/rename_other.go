//go:build !darwin && !linux && !windows

package fileops

import "errors"

func RenameNoReplace(oldPath, newPath string) error {
	return errors.New("atomic no-replace rename is unsupported on this platform")
}
