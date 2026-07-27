package lspedit

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Workspace transactions. A rename, an organize-imports, or a quick fix that
// spans files must not be applied file by file: the first failure then leaves
// half a refactoring on disk with no way back. A Transaction instead separates
// the three phases:
//
//	Preflight() — validate every step (path exists and is a regular file, the
//	              document version still matches, ranges resolve, no edits of
//	              one file overlap) and compute the resulting content of every
//	              touched file in memory. Nothing is written.
//	Preview()   — the same plan rendered as per-file before/after (preview.go).
//	Apply()     — preflight again, then write. If any write or removal fails,
//	              every file already touched is restored from the content
//	              captured during preflight, and the error reports whether the
//	              rollback itself was clean.
//
// Steps are ordered and applied in order against a virtual filesystem, so a
// step may edit a file an earlier step created and a second edit list for the
// same file is interpreted against the text the first one produced — LSP
// documentChanges semantics.
//
// Paths are plain filesystem paths, normalized with filepath.Abs; a host
// holding file:// URIs converts them before building the transaction.

// defaultMode is the permission used for files the transaction creates.
const defaultMode fs.FileMode = 0o644

// FileEdit is the edit list for one file.
//
// Version guards against applying edits computed from a document the user has
// since changed: when it is positive, the transaction asks its version source
// (SetVersions) for the file's current version and refuses to apply unless the
// two match. Zero or negative means unversioned.
type FileEdit struct {
	Path    string
	Edits   []TextEdit
	Version int
}

// OpKind is the kind of a resource operation.
type OpKind int

// Resource operation kinds, mirroring the LSP create/rename/delete file
// operations.
const (
	OpCreate OpKind = iota + 1
	OpRename
	OpDelete
)

// String implements fmt.Stringer.
func (k OpKind) String() string {
	switch k {
	case OpCreate:
		return "create"
	case OpRename:
		return "rename"
	case OpDelete:
		return "delete"
	}
	return fmt.Sprintf("OpKind(%d)", int(k))
}

// ResourceOp creates, renames or deletes a file as part of the transaction.
//
// Path is the target for create and delete and the source for rename; NewPath
// is the rename destination. A rename is planned as a delete of Path plus a
// create of NewPath carrying its content, so it participates in the same
// all-or-nothing write as the text edits. Directories are not operated on: the
// parent directory of a created or renamed-to path must already exist.
type ResourceOp struct {
	Kind    OpKind
	Path    string
	NewPath string
	Content string // create: the initial content
	// Overwrite allows a create or rename to replace an existing destination.
	Overwrite bool
	// IgnoreIfExists skips a create whose destination is already there
	// (checked before Overwrite).
	IgnoreIfExists bool
	// IgnoreIfNotExists skips a rename or delete whose source is missing
	// instead of failing the transaction.
	IgnoreIfNotExists bool
}

// Transaction is an ordered set of file edits and resource operations applied
// as a unit. Build it with NewTransaction, add steps, then Preview or Apply.
// A Transaction is not safe for concurrent use.
type Transaction struct {
	// Label names the change for previews and error messages, e.g.
	// `rename "Foo" to "Bar"`.
	Label string

	steps     []step
	versionOf func(path string) (int, bool)

	// Injection points for the filesystem mutations, so tests can force a
	// mid-apply failure and check the rollback. Both default to the os
	// functions and are used for rollback writes too.
	writeFile  func(path string, data []byte, perm fs.FileMode) error
	removeFile func(path string) error
}

// step is one entry of the ordered step list: exactly one of edit / op is set.
type step struct {
	edit *FileEdit
	op   *ResourceOp
}

// NewTransaction returns an empty transaction labelled label.
func NewTransaction(label string) *Transaction {
	return &Transaction{
		Label:      label,
		writeFile:  os.WriteFile,
		removeFile: os.Remove,
	}
}

// SetVersions installs the document version source consulted for every
// FileEdit with a positive Version. fn receives the path exactly as it was
// given in the FileEdit and reports the current version plus whether the
// document is known at all. Without it, a versioned FileEdit fails preflight.
func (t *Transaction) SetVersions(fn func(path string) (version int, ok bool)) {
	t.versionOf = fn
}

// AddEdit appends an edit step.
func (t *Transaction) AddEdit(fe FileEdit) {
	t.steps = append(t.steps, step{edit: &fe})
}

// AddOp appends a resource operation step.
func (t *Transaction) AddOp(op ResourceOp) {
	t.steps = append(t.steps, step{op: &op})
}

// Steps is the number of steps added so far.
func (t *Transaction) Steps() int { return len(t.steps) }

// PlannedFile is one file's fully resolved before/after state.
type PlannedFile struct {
	Path    string // absolute, cleaned
	Before  string // content on disk before the transaction; empty when !Existed
	After   string // content to write; empty when Deleted
	Existed bool   // the path was a regular file on disk before the transaction
	Deleted bool   // the transaction removes the path
	Mode    fs.FileMode
}

// Created reports that the transaction brings this path into existence.
func (p PlannedFile) Created() bool { return !p.Existed && !p.Deleted }

// Plan is the result of a preflight: every file the transaction changes, in
// the order the steps first touched them. Files whose content ends up
// identical to what is on disk are left out — the plan holds real changes
// only. Holding a Plan means nothing has been written yet.
type Plan struct {
	Label string
	Files []PlannedFile
}

// Result reports what a successful Apply did.
type Result struct {
	Label   string
	Written []string
	Deleted []string
}

// ApplyError reports a failed Apply. Err is the underlying filesystem error
// and Path the file it happened on. RollbackErrs is empty when every already
// touched file was restored, so an ApplyError with no RollbackErrs means the
// working tree is exactly as it was before Apply.
type ApplyError struct {
	Path         string
	Err          error
	RollbackErrs []error
}

// Error implements error.
func (e *ApplyError) Error() string {
	if len(e.RollbackErrs) == 0 {
		return fmt.Sprintf("lspedit: apply %s: %v (rolled back, no file changed)", e.Path, e.Err)
	}
	msgs := make([]string, 0, len(e.RollbackErrs))
	for _, re := range e.RollbackErrs {
		msgs = append(msgs, re.Error())
	}
	return fmt.Sprintf("lspedit: apply %s: %v (ROLLBACK INCOMPLETE: %s)",
		e.Path, e.Err, strings.Join(msgs, "; "))
}

// Unwrap exposes the filesystem error to errors.Is / errors.As.
func (e *ApplyError) Unwrap() error { return e.Err }

// vfile is the transaction's in-memory view of one path.
type vfile struct {
	path    string
	before  string      // disk content at first touch
	content string      // content after the steps applied so far
	existed bool        // regular file on disk at first touch
	gone    bool        // absent right now (never existed, or deleted by a step)
	mode    fs.FileMode // existing permission, reused when rewriting
}

// Preflight validates every step and computes the resulting content of every
// file, without writing anything. The returned error wraps one of the package
// sentinels (ErrNotFound, ErrStaleVersion, ErrInvalidRange, ErrOverlap,
// ErrExists, ErrBadPath) and names the offending path.
func (t *Transaction) Preflight() (*Plan, error) {
	files := make(map[string]*vfile, len(t.steps))
	order := make([]string, 0, len(t.steps))

	// load reads a path once and caches it; a missing path is cached as gone
	// so a later create/rename can fill it in.
	load := func(raw string) (*vfile, error) {
		path, err := normalizePath(raw)
		if err != nil {
			return nil, err
		}
		if f, ok := files[path]; ok {
			return f, nil
		}
		f := &vfile{path: path, mode: defaultMode}
		info, err := os.Stat(path)
		switch {
		case err == nil && info.IsDir():
			return nil, fmt.Errorf("%w: %s is a directory", ErrBadPath, path)
		case err == nil && !info.Mode().IsRegular():
			return nil, fmt.Errorf("%w: %s is not a regular file", ErrBadPath, path)
		case err == nil:
			data, rerr := os.ReadFile(path)
			if rerr != nil {
				return nil, rerr
			}
			f.existed = true
			f.before = string(data)
			f.content = f.before
			f.mode = info.Mode().Perm()
		case os.IsNotExist(err):
			f.gone = true
		default:
			return nil, err
		}
		files[path] = f
		order = append(order, path)
		return f, nil
	}

	for i, s := range t.steps {
		var err error
		switch {
		case s.edit != nil:
			err = t.planEdit(load, *s.edit)
		case s.op != nil:
			err = t.planOp(load, *s.op)
		default:
			err = fmt.Errorf("lspedit: step %d is empty", i)
		}
		if err != nil {
			return nil, fmt.Errorf("lspedit: step %d: %w", i, err)
		}
	}

	plan := &Plan{Label: t.Label, Files: make([]PlannedFile, 0, len(order))}
	for _, path := range order {
		f := files[path]
		switch {
		case f.existed && f.gone: // removed
			plan.Files = append(plan.Files, PlannedFile{
				Path: path, Before: f.before, Existed: true, Deleted: true, Mode: f.mode,
			})
		case !f.existed && !f.gone: // created
			plan.Files = append(plan.Files, PlannedFile{
				Path: path, After: f.content, Mode: f.mode,
			})
		case f.existed && f.content != f.before: // rewritten
			plan.Files = append(plan.Files, PlannedFile{
				Path: path, Before: f.before, After: f.content, Existed: true, Mode: f.mode,
			})
		}
		// Anything else is a no-op: an unchanged file, or one created and
		// deleted again inside the transaction.
	}
	return plan, nil
}

// planEdit validates one FileEdit and folds it into the virtual file.
func (t *Transaction) planEdit(load func(string) (*vfile, error), fe FileEdit) error {
	f, err := load(fe.Path)
	if err != nil {
		return err
	}
	if f.gone {
		return fmt.Errorf("%w: %s", ErrNotFound, f.path)
	}
	if fe.Version > 0 {
		if t.versionOf == nil {
			return fmt.Errorf("%w: %s: edits expect version %d but the transaction has no version source",
				ErrStaleVersion, f.path, fe.Version)
		}
		cur, known := t.versionOf(fe.Path)
		if !known {
			return fmt.Errorf("%w: %s: document is not open, cannot confirm version %d",
				ErrStaleVersion, f.path, fe.Version)
		}
		if cur != fe.Version {
			return fmt.Errorf("%w: %s: edits computed against version %d, document is at %d",
				ErrStaleVersion, f.path, fe.Version, cur)
		}
	}
	next, err := ApplyEdits(f.content, fe.Edits)
	if err != nil {
		return fmt.Errorf("%s: %w", f.path, err)
	}
	f.content = next
	return nil
}

// planOp validates one resource operation and folds it into the virtual
// filesystem.
func (t *Transaction) planOp(load func(string) (*vfile, error), op ResourceOp) error {
	switch op.Kind {
	case OpCreate:
		f, err := load(op.Path)
		if err != nil {
			return err
		}
		if !f.gone {
			if op.IgnoreIfExists {
				return nil
			}
			if !op.Overwrite {
				return fmt.Errorf("%w: %s", ErrExists, f.path)
			}
		}
		if err := requireDir(f.path); err != nil {
			return err
		}
		f.gone = false
		f.content = op.Content
		return nil

	case OpRename:
		src, err := load(op.Path)
		if err != nil {
			return err
		}
		if src.gone {
			if op.IgnoreIfNotExists {
				return nil
			}
			return fmt.Errorf("%w: %s", ErrNotFound, src.path)
		}
		dst, err := load(op.NewPath)
		if err != nil {
			return err
		}
		if dst.path == src.path {
			return nil
		}
		if !dst.gone && !op.Overwrite {
			return fmt.Errorf("%w: %s", ErrExists, dst.path)
		}
		if err := requireDir(dst.path); err != nil {
			return err
		}
		dst.gone = false
		dst.content = src.content
		src.gone = true
		src.content = ""
		return nil

	case OpDelete:
		f, err := load(op.Path)
		if err != nil {
			return err
		}
		if f.gone {
			if op.IgnoreIfNotExists {
				return nil
			}
			return fmt.Errorf("%w: %s", ErrNotFound, f.path)
		}
		f.gone = true
		f.content = ""
		return nil
	}
	return fmt.Errorf("lspedit: unknown resource operation kind %d", int(op.Kind))
}

// Apply preflights the transaction and then writes it. Either every planned
// file ends up in its new state, or — when a write or removal fails — every
// file already touched is restored and an *ApplyError is returned. A preflight
// failure returns before any write, so the error is the preflight one.
func (t *Transaction) Apply() (Result, error) {
	plan, err := t.Preflight()
	if err != nil {
		return Result{}, err
	}
	return t.applyPlan(plan)
}

// undo is one journal entry: how to put a path back the way it was.
type undo struct {
	path    string
	existed bool
	data    []byte
	mode    fs.FileMode
}

// applyPlan writes a plan, journalling the previous state of every file before
// it is touched so a failure can be undone in reverse order.
func (t *Transaction) applyPlan(plan *Plan) (Result, error) {
	res := Result{Label: plan.Label}
	journal := make([]undo, 0, len(plan.Files))
	for _, f := range plan.Files {
		journal = append(journal, undo{
			path:    f.Path,
			existed: f.Existed,
			data:    []byte(f.Before),
			mode:    f.Mode,
		})
		var err error
		if f.Deleted {
			err = t.removeFile(f.Path)
		} else {
			mode := f.Mode
			if mode == 0 {
				mode = defaultMode
			}
			err = t.writeFile(f.Path, []byte(f.After), mode)
		}
		if err != nil {
			// The failing file is in the journal too: a write may have
			// truncated it before failing, so it needs restoring as well.
			return Result{}, &ApplyError{Path: f.Path, Err: err, RollbackErrs: t.rollback(journal)}
		}
		if f.Deleted {
			res.Deleted = append(res.Deleted, f.Path)
		} else {
			res.Written = append(res.Written, f.Path)
		}
	}
	return res, nil
}

// rollback replays the journal backwards, restoring previous content and
// removing the files the transaction created. It returns the failures it hit;
// an empty result means the tree is back to its pre-Apply state.
func (t *Transaction) rollback(journal []undo) []error {
	var errs []error
	for i := len(journal) - 1; i >= 0; i-- {
		u := journal[i]
		if u.existed {
			mode := u.mode
			if mode == 0 {
				mode = defaultMode
			}
			if err := t.writeFile(u.path, u.data, mode); err != nil {
				errs = append(errs, fmt.Errorf("restore %s: %w", u.path, err))
			}
			continue
		}
		if err := t.removeFile(u.path); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("remove %s: %w", u.path, err))
		}
	}
	return errs
}

// normalizePath turns a caller path into an absolute cleaned path. Symlinks
// are intentionally left alone: a path that does not exist yet cannot be
// resolved, and the host's own paths must stay comparable.
func normalizePath(raw string) (string, error) {
	p := strings.TrimSpace(raw)
	if p == "" {
		return "", fmt.Errorf("%w: empty path", ErrBadPath)
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("%w: %s: %v", ErrBadPath, p, err)
	}
	return abs, nil
}

// requireDir checks that the parent directory of path exists, so a created
// file needs no directory of its own and the rollback stays exact (it never
// has to remove a directory it made).
func requireDir(path string) error {
	dir := filepath.Dir(path)
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("%w: %s: parent directory %s does not exist", ErrBadPath, path, dir)
	}
	return nil
}
