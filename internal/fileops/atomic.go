package fileops

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ErrChanged means the destination changed during preparation. The caller must
// review/retry with a new snapshot, not unconditionally overwrite that change.
var ErrChanged = errors.New("fileops: destination changed during atomic write")

type atomicTarget struct {
	info os.FileInfo
	sum  [sha256.Size]byte
}

// Inspect through a replaceable reader and recheck the pathname/handle after
// hashing. This detects ordinary concurrent replacement and in-place edits,
// including equal-length edits whose mtime was restored. It is not a lock or
// protection against an adversary racing the final check/rename interval.
func inspectAtomicTarget(path string) (s atomicTarget, err error) {
	before, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if !before.Mode().IsRegular() {
		return s, fmt.Errorf("fileops: refusing non-regular destination: %s", path)
	}
	f, err := OpenSnapshot(path)
	if err != nil {
		return s, err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	held, err := f.Stat()
	if err != nil {
		return s, err
	}
	if !sameAtomicInfo(before, held) {
		return s, fmt.Errorf("%w: %s", ErrChanged, path)
	}
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return s, err
	}
	after, err := os.Lstat(path)
	if err != nil {
		return s, err
	}
	if !sameAtomicInfo(before, after) {
		return s, fmt.Errorf("%w: %s", ErrChanged, path)
	}
	s.info = after
	copy(s.sum[:], h.Sum(nil))
	return s, nil
}

func sameAtomicInfo(a, b os.FileInfo) bool {
	return a != nil && b != nil && b.Mode().IsRegular() && os.SameFile(a, b) &&
		a.Size() == b.Size() && a.Mode() == b.Mode() && a.ModTime().Equal(b.ModTime())
}

func sameAtomicTarget(a, b atomicTarget) bool {
	if a.info == nil || b.info == nil {
		return a.info == nil && b.info == nil
	}
	return sameAtomicInfo(a.info, b.info) && a.sum == b.sum
}

type atomicOutput interface {
	io.Writer
	Name() string
	Chmod(os.FileMode) error
	Sync() error
	Close() error
}

// Retain a short/failed write even if an encoder accidentally discards it.
type atomicEncodeWriter struct {
	w   io.Writer
	err error
}

func (w *atomicEncodeWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	n, err := w.w.Write(p)
	if n < 0 || n > len(p) {
		n, err = 0, io.ErrShortWrite
	} else if n != len(p) && err == nil {
		err = io.ErrShortWrite
	}
	w.err = err
	return n, err
}

// WriteAtomic stages a complete encoded regular file in the destination's
// directory, syncs/closes it, checks for conflicting changes, and replaces the
// pathname using the platform snapshot-replacement primitive. Existing file
// permissions are preserved; read-only files and leaf symlinks are rejected.
// The parent directory must already exist. New files use mode.Perm().
//
// Encoding/sync/close/replace failure never truncates or removes the old file.
// Replacement breaks hard-link aliases; extended ACLs/ownership are not copied.
// Atomic visibility depends on filesystem rename guarantees. This does not
// promise cross-process exclusion or durability against every power failure.
// Call on a worker when storage may block; this function is synchronous.
func WriteAtomic(path string, mode os.FileMode, encode func(io.Writer) error) error {
	return writeAtomic(path, mode, encode, func(dir, pattern string) (atomicOutput, error) {
		return os.CreateTemp(dir, pattern)
	}, ReplaceSnapshotFile)
}

// WriteFileAtomic is WriteAtomic for an already captured byte slice. The caller
// must not mutate data until this synchronous call returns.
func WriteFileAtomic(path string, data []byte, mode os.FileMode) error {
	return WriteAtomic(path, mode, func(w io.Writer) error {
		_, err := io.Copy(w, bytes.NewReader(data))
		return err
	})
}

func writeAtomic(path string, mode os.FileMode, encode func(io.Writer) error,
	create func(string, string) (atomicOutput, error), replace func(string, string) error) (err error) {
	if path == "" || encode == nil {
		return &os.PathError{Op: "atomic-write", Path: path, Err: os.ErrInvalid}
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return err
	}
	before, err := inspectAtomicTarget(path)
	if err != nil {
		return err
	}
	if before.info != nil {
		mode = before.info.Mode().Perm()
		// Do not turn a read-only document into a writable replacement merely
		// because the directory permits rename (or the process runs as root).
		if mode&0222 == 0 {
			return &os.PathError{Op: "atomic-write", Path: path, Err: os.ErrPermission}
		}
	}
	f, err := create(filepath.Dir(path), ".silk-write-*")
	if err != nil {
		return err
	}
	temp, closed := f.Name(), false
	defer func() {
		if !closed {
			err = errors.Join(err, f.Close())
		}
		if e := os.Remove(temp); e != nil && !os.IsNotExist(e) {
			err = errors.Join(err, e)
		}
	}()
	if err = f.Chmod(mode.Perm()); err != nil {
		return err
	}
	w := &atomicEncodeWriter{w: f}
	encodeErr := encode(w)
	if err = errors.Join(encodeErr, w.err); err != nil {
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
	after, err := inspectAtomicTarget(path)
	if err != nil {
		return err
	}
	if !sameAtomicTarget(before, after) {
		return fmt.Errorf("%w: %s", ErrChanged, path)
	}
	return replace(temp, path)
}
