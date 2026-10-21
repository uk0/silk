package core

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/uk0/silk/diagnostic"
)

type ProjectFile struct{ Path, Relative string }
type ProjectScanOptions struct {
	// Zero uses 50,000 visited entries / 64 directory levels. Negative is invalid.
	MaxEntries, MaxDepth           int
	IncludeHidden                  bool
	ExcludeDirs, ExcludeExtensions []string
}
type ProjectFiles struct {
	Root       string
	Files      []ProjectFile
	Visited    int
	Incomplete bool
}

// ScanProjectFiles is a synchronous, cancellable SDK primitive: call it off the
// UI thread. It opens a rooted capability, enumerates in batches and never
// follows child symlinks or includes special files. The chosen root itself may
// be a symlink. Partial results accompany coded errors and Incomplete=true;
// callers must not present them as a complete project index. Cancellation is
// checked between entries; an OS filesystem syscall is not forcibly preempted.
func ScanProjectFiles(ctx context.Context, dir string, opts ProjectScanOptions) (result ProjectFiles, err error) {
	fail := func(code diagnostic.Code, path, message string, cause error) {
		result.Incomplete = true
		if err == nil {
			err = diagnostic.New(code, message, cause).At(diagnostic.Location{File: path})
		}
	}
	if e := ctx.Err(); e != nil {
		result.Incomplete = true
		return result, e
	}
	if dir == "" || opts.MaxEntries < 0 || opts.MaxDepth < 0 {
		fail(diagnostic.FileScan, dir, "invalid project scan root or limits", nil)
		return
	}
	abs, e := filepath.Abs(dir)
	if e != nil {
		fail(diagnostic.FileScan, dir, "cannot resolve project directory", e)
		return
	}
	result.Root = abs
	root, e := os.OpenRoot(abs)
	if e != nil {
		fail(diagnostic.FileScan, abs, "cannot open project directory", e)
		return
	}
	defer root.Close()
	maxEntries, maxDepth := opts.MaxEntries, opts.MaxDepth
	if maxEntries == 0 {
		maxEntries = 50000
	}
	if maxDepth == 0 {
		maxDepth = 64
	}
	dirs, exts := map[string]bool{}, map[string]bool{}
	for _, name := range opts.ExcludeDirs {
		dirs[name] = true
	}
	for _, ext := range opts.ExcludeExtensions {
		exts[strings.ToLower(ext)] = true
	}
	exhausted := false
	var walk func(string, int)
	walk = func(rel string, depth int) {
		if ctx.Err() != nil || exhausted {
			return
		}
		if depth > maxDepth {
			fail(diagnostic.FileScanLimit, filepath.Join(abs, rel), "project directory depth limit reached", nil)
			return
		}
		f, e := root.Open(rel)
		if e != nil {
			fail(diagnostic.FileScan, filepath.Join(abs, rel), "cannot read project directory", e)
			return
		}
		defer f.Close()
		for {
			if ctx.Err() != nil || exhausted {
				return
			}
			entries, readErr := f.ReadDir(256)
			for _, entry := range entries {
				if ctx.Err() != nil || exhausted {
					return
				}
				if result.Visited == maxEntries {
					exhausted = true
					fail(diagnostic.FileScanLimit, abs, "project entry limit reached", nil)
					return
				}
				result.Visited++
				name := entry.Name()
				if (!opts.IncludeHidden && strings.HasPrefix(name, ".")) || entry.Type()&os.ModeSymlink != 0 {
					continue
				}
				child := filepath.Join(rel, name)
				if entry.IsDir() {
					if !dirs[name] {
						walk(child, depth+1)
					}
					continue
				}
				if !entry.Type().IsRegular() || exts[strings.ToLower(filepath.Ext(name))] {
					continue
				}
				result.Files = append(result.Files, ProjectFile{Path: filepath.Join(abs, child), Relative: filepath.Clean(child)})
			}
			if readErr != nil {
				if !errors.Is(readErr, io.EOF) {
					fail(diagnostic.FileScan, filepath.Join(abs, rel), "cannot list project directory", readErr)
				}
				return
			}
		}
	}
	walk(".", 0)
	if e := ctx.Err(); e != nil {
		result.Incomplete = true
		return result, e
	}
	sort.Slice(result.Files, func(i, j int) bool { return result.Files[i].Relative < result.Files[j].Relative })
	return
}
