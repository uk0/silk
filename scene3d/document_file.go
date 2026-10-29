package scene3d

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// SaveDocumentFile explicitly saves through a sibling temporary file. Encoding,
// cancellation, write, sync and close failures before rename leave an existing
// destination untouched. Existing symlinks/directories are rejected; parent
// directories are not created. Callers must serialize writes and control the
// parent directory. Rename atomicity/durability depends on the OS/filesystem;
// this is not a compare-and-swap or a power-loss durability guarantee.
func SaveDocumentFile(ctx context.Context, path string, d *Document) error {
	if ctx == nil || path == "" {
		return ErrGeometry
	}
	var encoded bytes.Buffer
	if err := SaveDocument(ctx, &encoded, d); err != nil {
		return err
	}
	mode := os.FileMode(0600)
	info, err := os.Lstat(path)
	if err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%w: scene destination must be a regular file", ErrUnsupported)
		}
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".silk-scene-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	committed := false
	defer func() {
		if committed {
			return
		}
		// A failed Windows rename may leave our temporary file read-only.
		// Restore write permission on that temporary path before cleanup.
		_ = os.Chmod(tmp, 0600)
		_ = os.Remove(tmp)
	}()
	defer f.Close()
	if _, err = encoded.WriteTo(f); err != nil {
		return err
	}
	if err = f.Chmod(mode); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	err = os.Rename(tmp, path)
	committed = err == nil
	return err
}
