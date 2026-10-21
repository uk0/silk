package ged

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/uk0/silk/diagnostic"
	"github.com/uk0/silk/internal/processutil"
)

const defaultExplorerEntryLimit = 50000
const explorerDepthLimit = 64

type explorerScanRequest struct {
	root, filter                  string
	expanded, collapsed, excluded map[string]bool
	hidden, git                   bool
	limit                         int
	generation                    uint64
}
type explorerScanResult struct {
	root       *fileEntry
	gitStatus  map[string]rune
	err        error
	generation uint64
	gitOnly    bool
}

// scanExplorer opens a rooted directory capability. Children are never stat'ed
// through symlinks; os.Root also refuses a concurrent replacement escaping the
// chosen root. ReadDir is batched, traversal/count/depth bounded, and cancellation
// is checked between entries. A blocked filesystem syscall itself is not
// preemptible; it never holds the GUI thread.
func scanExplorer(ctx context.Context, req explorerScanRequest) explorerScanResult {
	result := explorerScanResult{generation: req.generation}
	fail := func(path, message string, cause error) {
		if result.err == nil {
			result.err = diagnostic.New(diagnostic.FileScan, message, cause).At(diagnostic.Location{File: path})
		}
	}
	root, err := os.OpenRoot(req.root)
	if err != nil {
		fail(req.root, "cannot open project directory", err)
		return result
	}
	defer root.Close()
	limit := req.limit
	if limit <= 0 {
		limit = defaultExplorerEntryLimit
	}
	count := 0
	query := strings.ToLower(req.filter)
	var read func(string, string, int) *fileEntry
	read = func(rel, name string, depth int) *fileEntry {
		if ctx.Err() != nil {
			return nil
		}
		path := filepath.Join(req.root, rel)
		e := &fileEntry{name: name, path: path, isDir: true, depth: depth, expanded: req.expanded[path]}
		if query != "" {
			e.expanded = !req.collapsed[path]
		}
		if !e.expanded && query == "" {
			return e
		}
		if depth > explorerDepthLimit {
			fail(path, "directory depth limit reached", nil)
			return e
		}
		f, err := root.Open(rel)
		if err != nil {
			fail(path, "cannot read directory", err)
			return e
		}
		defer f.Close()
		defer func() {
			sort.Slice(e.children, func(i, j int) bool {
				a, b := e.children[i], e.children[j]
				if a.isDir != b.isDir {
					return a.isDir
				}
				lo, hi := strings.ToLower(a.name), strings.ToLower(b.name)
				if lo == hi {
					return a.name < b.name
				}
				return lo < hi
			})
		}()
		e.loaded = true
		for {
			if ctx.Err() != nil {
				return nil
			}
			batch, readErr := f.ReadDir(256)
			for _, entry := range batch {
				if ctx.Err() != nil {
					return nil
				}
				count++
				if count > limit {
					fail(req.root, "file list limit reached; narrow the root or increase SetEntryLimit", nil)
					return e
				}
				name := entry.Name()
				if (!req.hidden && strings.HasPrefix(name, ".")) || req.excluded[name] {
					continue
				}
				childRel := filepath.Join(rel, name)
				// DirEntry.IsDir does not follow symlinks or junctions.
				var child *fileEntry
				if entry.IsDir() && entry.Type()&os.ModeSymlink == 0 {
					child = read(childRel, name, depth+1)
					if query != "" && child != nil && !strings.Contains(strings.ToLower(childRel), query) && len(child.children) == 0 {
						continue
					}
				} else {
					if query != "" && !strings.Contains(strings.ToLower(childRel), query) {
						continue
					}
					child = &fileEntry{name: name, path: filepath.Join(req.root, childRel), depth: depth + 1, isSymlink: entry.Type()&os.ModeSymlink != 0, loaded: true}
				}
				if child != nil {
					e.children = append(e.children, child)
				}
			}
			if readErr != nil {
				if !errors.Is(readErr, io.EOF) {
					fail(path, "cannot list directory", readErr)
				}
				break
			}
		}

		return e
	}
	result.root = read(".", filepath.Base(req.root), 0)
	if ctx.Err() != nil {
		result.err = ctx.Err()
	}
	return result
}

// At most one worker per explorer, one pending request, and one queued UI
// delivery per scan phase. Idle workers exit instead of living until shutdown.
type explorerScanWorker struct {
	mu      sync.Mutex
	pending *explorerScanRequest
	cancel  context.CancelFunc
	running bool
	scan    func(context.Context, explorerScanRequest) explorerScanResult
	deliver func(explorerScanResult)
}

func (w *explorerScanWorker) submit(req explorerScanRequest) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pending = &req
	if w.cancel != nil {
		w.cancel()
	}
	if !w.running {
		w.running = true
		go w.run()
	}
}
func (w *explorerScanWorker) stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pending = nil
	if w.cancel != nil {
		w.cancel()
	}
}
func (w *explorerScanWorker) run() {
	for {
		w.mu.Lock()
		req := w.pending
		if req == nil {
			w.running = false
			w.cancel = nil
			w.mu.Unlock()
			return
		}
		w.pending = nil
		ctx, cancel := context.WithCancel(context.Background())
		w.cancel = cancel
		w.mu.Unlock()
		result := w.scan(ctx, *req)
		if ctx.Err() == nil {
			w.deliver(result)
		}
		// Publish the useful tree before waiting for optional Git status.
		if req.git && ctx.Err() == nil {
			badges := scanExplorerGit(ctx, req.root)
			if ctx.Err() == nil {
				w.deliver(explorerScanResult{generation: req.generation, gitStatus: badges, gitOnly: true})
			}
		}
		cancel()
	}
}

type explorerOutput struct {
	bytes.Buffer
	limit int
}

func (b *explorerOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.limit {
		return 0, io.ErrShortBuffer
	}
	return b.Buffer.Write(p)
}
func scanExplorerGit(parent context.Context, dir string) map[string]rune {
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	run := func(args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, "git", append([]string{"--no-optional-locks", "-C", dir}, args...)...)
		processutil.NoConsoleWindow(cmd)
		cmd.WaitDelay = 250 * time.Millisecond
		var out = explorerOutput{limit: 2 << 20}
		cmd.Stdout = &out
		err := cmd.Run()
		return out.Bytes(), err
	}
	prefix, err := run("rev-parse", "--show-prefix")
	if err != nil {
		return nil
	}
	data, err := run("status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return nil
	}
	return parseGitPorcelainZPrefix(data, dir, strings.TrimSuffix(strings.TrimSuffix(string(prefix), "\n"), "\r"))
}

// -z paths are raw UTF-8 bytes, not C-quoted text. Rename/copy entries carry the
// destination first and the source in the following NUL field.
func parseGitPorcelainZ(data []byte, root string) map[string]rune {
	return parseGitPorcelainZPrefix(data, root, "")
}

// Keep keys in the caller's path namespace, even when Git resolves /var or
// another project-root symlink to a different physical spelling. Porcelain
// paths are repo-relative; --show-prefix identifies the chosen subtree.
func parseGitPorcelainZPrefix(data []byte, root, prefix string) map[string]rune {
	out := map[string]rune{}
	parts := bytes.Split(data, []byte{0})
	for i := 0; i < len(parts); i++ {
		p := parts[i]
		if len(p) < 4 || p[2] != ' ' {
			continue
		}
		x, y := p[0], p[1]
		name := string(p[3:])
		if x == 'R' || y == 'R' || x == 'C' || y == 'C' {
			i++
		}
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		name = strings.TrimPrefix(name, prefix)
		if name == "" || filepath.IsAbs(name) {
			continue
		}
		path := filepath.Clean(filepath.Join(root, name))
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		var badge rune
		switch {
		case x == 'U' || y == 'U' || x == 'A' && y == 'A' || x == 'D' && y == 'D':
			badge = '!'
		case x == '?' && y == '?':
			badge = '?'
		case x == 'D' || y == 'D':
			badge = 'D'
		case x == 'R' || x == 'C' || x == 'A' || y == 'R' || y == 'C' || y == 'A':
			badge = 'A'
		case x == 'M' || y == 'M':
			badge = 'M'
		}
		if badge != 0 {
			out[path] = badge
		}
	}
	return out
}

// explorerDelivery coalesces completed snapshots while the UI thread is busy.
// At most one UI callback and the newest tree/Git phases are retained. The
// callback is the only place that touches widget state.
type explorerDelivery struct {
	mu         sync.Mutex
	tree, git  *explorerScanResult
	generation uint64
	queued     bool
	post       func(func())
	apply      func(explorerScanResult)
}

func (d *explorerDelivery) offer(r explorerScanResult) {
	d.mu.Lock()
	if r.generation < d.generation {
		d.mu.Unlock()
		return
	}
	if r.generation > d.generation {
		d.tree = nil
		d.git = nil
		d.generation = r.generation
	}
	if r.gitOnly {
		d.git = &r
	} else {
		d.tree = &r
	}
	queue := !d.queued
	d.queued = true
	d.mu.Unlock()
	if queue {
		d.post(d.drain)
	}
}
func (d *explorerDelivery) drain() {
	d.mu.Lock()
	tree, git := d.tree, d.git
	d.tree, d.git, d.queued = nil, nil, false
	d.mu.Unlock()
	if tree != nil {
		d.apply(*tree)
	}
	if git != nil {
		d.apply(*git)
	}
}
