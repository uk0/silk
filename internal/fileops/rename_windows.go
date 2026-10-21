package fileops

import "golang.org/x/sys/windows"

func RenameNoReplace(oldPath, newPath string) error {
	from, err := windows.UTF16PtrFromString(oldPath)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(newPath)
	if err != nil {
		return err
	}
	// No MOVEFILE_REPLACE_EXISTING: the kernel performs the destination check.
	return windows.MoveFileEx(from, to, 0)
}
