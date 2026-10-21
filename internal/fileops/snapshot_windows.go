package fileops

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func snapshotWindowsPath(path string) ([]uint16, error) {
	if path == "" {
		return nil, windows.ERROR_FILE_NOT_FOUND
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	// Match os.Open's long-path support for local and UNC paths. Do not apply
	// the extended prefix to ordinary short names (their normalization differs).
	if len(absolute) >= 248 && !strings.HasPrefix(absolute, `\\?\`) && !strings.HasPrefix(absolute, `\\.\`) {
		if strings.HasPrefix(absolute, `\\`) {
			absolute = `\\?\UNC\` + absolute[2:]
		} else {
			absolute = `\\?\` + absolute
		}
	}
	return windows.UTF16FromString(absolute)
}

// OpenSnapshot is read-only but also shares DELETE access. Ordinary os.Open on
// Windows does not share DELETE and would prevent document replacement. The
// handle stays attached to the old file after a POSIX-style rename.
func OpenSnapshot(path string) (*os.File, error) {
	name, err := snapshotWindowsPath(path)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	h, err := windows.CreateFile(&name[0], windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(h), path), nil
}

type snapshotRenameInfo struct {
	Flags          uint32
	RootDirectory  windows.Handle
	FileNameLength uint32
	FileName       [1]uint16
}

// ReplaceSnapshotFile installs a closed/synced same-volume temporary file.
// Unlike MoveFileEx, FileRenameInfoEx with POSIX semantics can replace a target
// while shared-delete readers retain the old contents. Incompatible exclusive
// handles still fail; never close someone else's handle or delete the target.
func ReplaceSnapshotFile(from, to string) error {
	err := replaceSnapshotWindows(from, to)
	if err != nil {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: err}
	}
	return nil
}
func replaceSnapshotWindows(from, to string) error {
	src, err := snapshotWindowsPath(from)
	if err != nil {
		return err
	}
	dst, err := snapshotWindowsPath(to)
	if err != nil {
		return err
	}
	h, err := windows.CreateFile(&src[0], windows.DELETE|windows.SYNCHRONIZE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	offset := int(unsafe.Offsetof(snapshotRenameInfo{}.FileName))
	buf := make([]byte, offset+2*len(dst))
	info := (*snapshotRenameInfo)(unsafe.Pointer(&buf[0]))
	// FILE_RENAME_REPLACE_IF_EXISTS | FILE_RENAME_POSIX_SEMANTICS; deliberately
	// omit IGNORE_READONLY_ATTRIBUTE so read-only targets stay protected.
	info.Flags = 0x1 | 0x2
	info.FileNameLength = uint32(2 * (len(dst) - 1))
	copy(unsafe.Slice((*uint16)(unsafe.Pointer(&buf[offset])), len(dst)), dst)
	err = windows.SetFileInformationByHandle(h, windows.FileRenameInfoEx, &buf[0], uint32(len(buf)))
	// Some old kernels/filesystems do not support the extended info class. This
	// compatibility path has weaker open-reader semantics, but never retries
	// permission/sharing failures or removes the previous file first.
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) || errors.Is(err, windows.ERROR_NOT_SUPPORTED) || errors.Is(err, windows.ERROR_INVALID_FUNCTION) {
		info.Flags = 0x1 // legacy union's ReplaceIfExists boolean
		return windows.SetFileInformationByHandle(h, windows.FileRenameInfo, &buf[0], uint32(len(buf)))
	}
	return err
}
