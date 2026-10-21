package settings

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/internal/fileops"
)

// Same-directory temporary file: an encoding, fsync, close or rename failure
// never truncates the previous settings. No remove-then-rename fallback.
// Atomic visibility depends on the local filesystem's rename guarantees;
// this does not promise survival of every filesystem/power-loss scenario.
func writeSettingsSnapshot(path string, doc *core.TDoc) error {
	return writeSettingsFile(path, doc.Save)
}

func writeSettingsFile(path string, encode func(io.Writer) error) (err error) {
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	mode := os.FileMode(0600)
	if info, e := os.Lstat(path); e == nil {
		if !info.Mode().IsRegular() {
			return errors.New("refusing to replace non-regular settings path")
		}
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(e) {
		return e
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".silk-settings-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	closed := false
	defer func() {
		if !closed {
			err = errors.Join(err, f.Close())
		}
		_ = os.Remove(tmp)
	}()
	if err = f.Chmod(mode); err != nil {
		return err
	}
	if err = encode(f); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	err = f.Close()
	closed = true
	if err != nil {
		return err
	}
	// Recheck the destination after I/O as it might have changed meanwhile.
	if info, e := os.Lstat(path); e == nil {
		if !info.Mode().IsRegular() {
			return errors.New("settings destination became non-regular")
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	return replaceSettingsFile(tmp, path)
}

// Validate the entire file before accepting any settings. The legacy TDoc
// parser can stop at a blank line and does not propagate all reader errors;
// checking the full gzip stream here also catches truncated/corrupt trailers.
func readSettingsDoc(path string) (*core.TDoc, error) {
	data, err := fileops.ReadSnapshot(path)
	if err != nil {
		return nil, err
	}
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		zr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		data, err = io.ReadAll(zr)
		err = errors.Join(err, zr.Close())
		if err != nil {
			return nil, err
		}
	}
	return core.LoadTDoc(bytes.NewReader(data))
}
