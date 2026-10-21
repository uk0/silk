package fileops

import (
	"errors"
	"io"
)

// ReadSnapshot reads a replaceable document using a handle that does not lock
// out atomic replacement on Windows. The returned bytes are owned by the caller.
// Concurrent writes into the same inode are not isolated; writers must replace.
func ReadSnapshot(path string) ([]byte, error) {
	f, err := OpenSnapshot(path)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(f)
	return data, errors.Join(err, f.Close())
}
