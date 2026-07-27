package lspedit

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// renameEdit is the edit a symbol rename produces on a one-declaration file:
// replace the identifier on line 2, column 4.
func renameEdit(newName string, oldLen int) TextEdit {
	return TextEdit{
		Range:   Range{Start: Position{2, 4}, End: Position{2, 4 + oldLen}},
		NewText: newName,
	}
}

func seed(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("seed %s: %v", name, err)
	}
	return path
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func mustNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("%s exists (stat err %v), want absent", path, err)
	}
}

func TestApplyMultiFileEdits(t *testing.T) {
	dir := t.TempDir()
	a := seed(t, dir, "a.go", "package p\n\nvar A = 1\n")
	b := seed(t, dir, "b.go", "package p\n\nvar B = A\n")

	tx := NewTransaction(`rename "A" to "Alpha"`)
	tx.AddEdit(FileEdit{Path: a, Edits: []TextEdit{renameEdit("Alpha", 1)}})
	tx.AddEdit(FileEdit{Path: b, Edits: []TextEdit{{
		Range:   Range{Start: Position{2, 8}, End: Position{2, 9}},
		NewText: "Alpha",
	}}})

	res, err := tx.Apply()
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(res.Written) != 2 || len(res.Deleted) != 0 {
		t.Fatalf("result = %+v, want two writes and no deletes", res)
	}
	if got, want := mustRead(t, a), "package p\n\nvar Alpha = 1\n"; got != want {
		t.Fatalf("a.go = %q, want %q", got, want)
	}
	if got, want := mustRead(t, b), "package p\n\nvar B = Alpha\n"; got != want {
		t.Fatalf("b.go = %q, want %q", got, want)
	}
}

func TestApplySkipsUnchangedFiles(t *testing.T) {
	dir := t.TempDir()
	a := seed(t, dir, "a.go", "package p\n\nvar A = 1\n")
	tx := NewTransaction("no-op rename")
	tx.AddEdit(FileEdit{Path: a, Edits: []TextEdit{renameEdit("A", 1)}})

	plan, err := tx.Preflight()
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	if len(plan.Files) != 0 {
		t.Fatalf("plan = %+v, want no changed files", plan.Files)
	}
	res, err := tx.Apply()
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(res.Written) != 0 {
		t.Fatalf("Written = %v, want nothing written", res.Written)
	}
}

func TestPreflightRejectsStaleVersion(t *testing.T) {
	dir := t.TempDir()
	a := seed(t, dir, "a.go", "package p\n\nvar A = 1\n")
	original := mustRead(t, a)

	newTx := func() *Transaction {
		tx := NewTransaction("rename")
		tx.AddEdit(FileEdit{Path: a, Edits: []TextEdit{renameEdit("Alpha", 1)}, Version: 4})
		return tx
	}

	// The document moved on while the rename RPC was in flight.
	tx := newTx()
	tx.SetVersions(func(string) (int, bool) { return 5, true })
	if _, err := tx.Apply(); !errors.Is(err, ErrStaleVersion) {
		t.Fatalf("stale version: err = %v, want ErrStaleVersion", err)
	}
	if mustRead(t, a) != original {
		t.Fatal("a.go was modified by a transaction that failed preflight")
	}

	// A versioned edit with no version source cannot be confirmed.
	if _, err := newTx().Apply(); !errors.Is(err, ErrStaleVersion) {
		t.Fatalf("missing version source: err = %v, want ErrStaleVersion", err)
	}

	// Unknown document.
	tx = newTx()
	tx.SetVersions(func(string) (int, bool) { return 0, false })
	if _, err := tx.Apply(); !errors.Is(err, ErrStaleVersion) {
		t.Fatalf("unknown document: err = %v, want ErrStaleVersion", err)
	}

	// Matching version applies.
	tx = newTx()
	tx.SetVersions(func(path string) (int, bool) {
		if path != a {
			t.Errorf("version source got path %q, want %q", path, a)
		}
		return 4, true
	})
	if _, err := tx.Apply(); err != nil {
		t.Fatalf("matching version: %v", err)
	}
	if got, want := mustRead(t, a), "package p\n\nvar Alpha = 1\n"; got != want {
		t.Fatalf("a.go = %q, want %q", got, want)
	}
}

func TestPreflightRejectsOverlappingEdits(t *testing.T) {
	dir := t.TempDir()
	a := seed(t, dir, "a.go", "package p\n\nvar A = 1\n")
	original := mustRead(t, a)

	tx := NewTransaction("bad server response")
	tx.AddEdit(FileEdit{Path: a, Edits: []TextEdit{
		{Range: Range{Position{2, 0}, Position{2, 5}}, NewText: "const"},
		{Range: Range{Position{2, 3}, Position{2, 7}}, NewText: "X"},
	}})

	if _, err := tx.Preflight(); !errors.Is(err, ErrOverlap) {
		t.Fatalf("Preflight: err = %v, want ErrOverlap", err)
	}
	if _, err := tx.Apply(); !errors.Is(err, ErrOverlap) {
		t.Fatalf("Apply: err = %v, want ErrOverlap", err)
	}
	if mustRead(t, a) != original {
		t.Fatal("a.go was modified despite overlapping edits")
	}
}

func TestPreflightRejectsMissingFile(t *testing.T) {
	dir := t.TempDir()
	a := seed(t, dir, "a.go", "package p\n\nvar A = 1\n")
	missing := filepath.Join(dir, "gone.go")

	tx := NewTransaction("rename")
	tx.AddEdit(FileEdit{Path: a, Edits: []TextEdit{renameEdit("Alpha", 1)}})
	tx.AddEdit(FileEdit{Path: missing, Edits: []TextEdit{renameEdit("Alpha", 1)}})

	if _, err := tx.Apply(); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	// The first file must not have been written: the missing one is only
	// discovered in preflight, which runs before any write.
	if got, want := mustRead(t, a), "package p\n\nvar A = 1\n"; got != want {
		t.Fatalf("a.go = %q, want the original %q", got, want)
	}
	mustNotExist(t, missing)

	// Empty and directory paths are rejected too.
	tx = NewTransaction("bad paths")
	tx.AddEdit(FileEdit{Path: "  "})
	if _, err := tx.Preflight(); !errors.Is(err, ErrBadPath) {
		t.Fatalf("empty path: err = %v, want ErrBadPath", err)
	}
	tx = NewTransaction("bad paths")
	tx.AddEdit(FileEdit{Path: dir})
	if _, err := tx.Preflight(); !errors.Is(err, ErrBadPath) {
		t.Fatalf("directory path: err = %v, want ErrBadPath", err)
	}
}

func TestApplyRollsBackWhenLaterWriteFails(t *testing.T) {
	dir := t.TempDir()
	a := seed(t, dir, "a.go", "package p\n\nvar A = 1\n")
	b := seed(t, dir, "b.go", "package p\n\nvar B = A\n")
	c := filepath.Join(dir, "c.go")
	aBefore, bBefore := mustRead(t, a), mustRead(t, b)

	tx := NewTransaction(`rename "A" to "Alpha"`)
	tx.AddEdit(FileEdit{Path: a, Edits: []TextEdit{renameEdit("Alpha", 1)}})
	tx.AddOp(ResourceOp{Kind: OpCreate, Path: c, Content: "package p\n"})
	tx.AddEdit(FileEdit{Path: b, Edits: []TextEdit{{
		Range:   Range{Start: Position{2, 8}, End: Position{2, 9}},
		NewText: "Alpha",
	}}})

	// Fail the write of b.go once (a transient disk error), which is only
	// reached after a.go and c.go are already on disk.
	boom := errors.New("disk on fire")
	failed := false
	tx.writeFile = func(path string, data []byte, perm fs.FileMode) error {
		if path == b && !failed {
			failed = true
			return boom
		}
		return os.WriteFile(path, data, perm)
	}

	res, err := tx.Apply()
	if err == nil {
		t.Fatalf("Apply succeeded, want failure; result %+v", res)
	}
	if !failed {
		t.Fatal("the injected failure never fired; the plan order changed")
	}
	var ae *ApplyError
	if !errors.As(err, &ae) {
		t.Fatalf("err = %v (%T), want *ApplyError", err, err)
	}
	if ae.Path != b {
		t.Fatalf("ApplyError.Path = %q, want %q", ae.Path, b)
	}
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want it to wrap the write error", err)
	}
	if len(ae.RollbackErrs) != 0 {
		t.Fatalf("RollbackErrs = %v, want a clean rollback", ae.RollbackErrs)
	}
	if len(res.Written) != 0 || len(res.Deleted) != 0 {
		t.Fatalf("result = %+v, want the zero Result on failure", res)
	}

	// Everything the transaction had already written is back the way it was.
	if got := mustRead(t, a); got != aBefore {
		t.Fatalf("a.go = %q, want it restored to %q", got, aBefore)
	}
	if got := mustRead(t, b); got != bBefore {
		t.Fatalf("b.go = %q, want it untouched at %q", got, bBefore)
	}
	mustNotExist(t, c)
}

func TestApplyReportsIncompleteRollback(t *testing.T) {
	dir := t.TempDir()
	a := seed(t, dir, "a.go", "package p\n\nvar A = 1\n")
	b := seed(t, dir, "b.go", "package p\n\nvar B = A\n")

	tx := NewTransaction("rename")
	tx.AddEdit(FileEdit{Path: a, Edits: []TextEdit{renameEdit("Alpha", 1)}})
	tx.AddEdit(FileEdit{Path: b, Edits: []TextEdit{{
		Range:   Range{Start: Position{2, 8}, End: Position{2, 9}},
		NewText: "Alpha",
	}}})

	// Every write to b.go fails, so restoring it fails as well — the caller has
	// to learn that the tree is not clean.
	boom := errors.New("read-only filesystem")
	tx.writeFile = func(path string, data []byte, perm fs.FileMode) error {
		if path == b {
			return boom
		}
		return os.WriteFile(path, data, perm)
	}

	_, err := tx.Apply()
	var ae *ApplyError
	if !errors.As(err, &ae) {
		t.Fatalf("err = %v (%T), want *ApplyError", err, err)
	}
	if len(ae.RollbackErrs) == 0 {
		t.Fatal("RollbackErrs is empty, want the failed restore reported")
	}
	// a.go still rolls back even though b.go could not.
	if got, want := mustRead(t, a), "package p\n\nvar A = 1\n"; got != want {
		t.Fatalf("a.go = %q, want it restored to %q", got, want)
	}
}

// resourceTx seeds a directory and builds the same mixed transaction for the
// preview and apply tests: one edit, one create, one delete, one rename.
func resourceTx(t *testing.T, dir string) (tx *Transaction, a, create, del, src, dst string) {
	t.Helper()
	a = seed(t, dir, "a.go", "package p\n\nvar A = 1\n")
	del = seed(t, dir, "dead.go", "package p\n\nvar Dead = 0\n")
	src = seed(t, dir, "old.go", "package p\n\nvar Moved = 1\n")
	create = filepath.Join(dir, "new-file.go")
	dst = filepath.Join(dir, "new.go")

	tx = NewTransaction("refactor")
	tx.AddEdit(FileEdit{Path: a, Edits: []TextEdit{renameEdit("Alpha", 1)}})
	tx.AddOp(ResourceOp{Kind: OpCreate, Path: create, Content: "package p\n"})
	tx.AddOp(ResourceOp{Kind: OpDelete, Path: del})
	tx.AddOp(ResourceOp{Kind: OpRename, Path: src, NewPath: dst})
	return tx, a, create, del, src, dst
}

func TestPreviewWritesNothing(t *testing.T) {
	dir := t.TempDir()
	tx, a, create, del, src, dst := resourceTx(t, dir)

	pv, err := tx.Preview()
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if pv.Label != "refactor" {
		t.Fatalf("Label = %q", pv.Label)
	}
	want := []struct {
		path   string
		action Action
		before string
		after  string
	}{
		{a, ActionModify, "package p\n\nvar A = 1\n", "package p\n\nvar Alpha = 1\n"},
		{create, ActionCreate, "", "package p\n"},
		{del, ActionDelete, "package p\n\nvar Dead = 0\n", ""},
		{src, ActionDelete, "package p\n\nvar Moved = 1\n", ""},
		{dst, ActionCreate, "", "package p\n\nvar Moved = 1\n"},
	}
	if len(pv.Files) != len(want) {
		t.Fatalf("preview has %d files (%v), want %d", len(pv.Files), pv.Paths(), len(want))
	}
	for i, w := range want {
		got := pv.Files[i]
		if got.Path != w.path || got.Action != w.action || got.Before != w.before || got.After != w.after {
			t.Fatalf("file %d = %+v, want {%s %s %q %q}", i, got, w.path, w.action, w.before, w.after)
		}
	}

	if pv.Empty() {
		t.Fatal("Empty() = true, want the four steps reported")
	}
	if got, want := pv.String(), "refactor\nM "+a+"\nA "+create+"\nD "+del+"\nD "+src+"\nA "+dst; got != want {
		t.Fatalf("String() =\n%s\nwant\n%s", got, want)
	}

	// Not one byte on disk moved.
	if got, want := mustRead(t, a), "package p\n\nvar A = 1\n"; got != want {
		t.Fatalf("a.go = %q, want the original %q", got, want)
	}
	if got, want := mustRead(t, del), "package p\n\nvar Dead = 0\n"; got != want {
		t.Fatalf("dead.go = %q, want the original %q", got, want)
	}
	if got, want := mustRead(t, src), "package p\n\nvar Moved = 1\n"; got != want {
		t.Fatalf("old.go = %q, want the original %q", got, want)
	}
	mustNotExist(t, create)
	mustNotExist(t, dst)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("directory holds %d entries, want the 3 seeded files", len(entries))
	}
}

func TestApplyResourceOperations(t *testing.T) {
	dir := t.TempDir()
	tx, a, create, del, src, dst := resourceTx(t, dir)

	res, err := tx.Apply()
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(res.Written) != 3 || len(res.Deleted) != 2 {
		t.Fatalf("result = %+v, want 3 writes (a, create, dst) and 2 deletes (del, src)", res)
	}
	if got, want := mustRead(t, a), "package p\n\nvar Alpha = 1\n"; got != want {
		t.Fatalf("a.go = %q, want %q", got, want)
	}
	if got, want := mustRead(t, create), "package p\n"; got != want {
		t.Fatalf("new-file.go = %q, want %q", got, want)
	}
	if got, want := mustRead(t, dst), "package p\n\nvar Moved = 1\n"; got != want {
		t.Fatalf("new.go = %q, want the moved content %q", got, want)
	}
	mustNotExist(t, del)
	mustNotExist(t, src)
}

func TestPreflightResourceOpGuards(t *testing.T) {
	dir := t.TempDir()
	existing := seed(t, dir, "there.go", "package p\n")
	missing := filepath.Join(dir, "nowhere.go")

	// Create over an existing file needs Overwrite.
	tx := NewTransaction("create")
	tx.AddOp(ResourceOp{Kind: OpCreate, Path: existing, Content: "x"})
	if _, err := tx.Preflight(); !errors.Is(err, ErrExists) {
		t.Fatalf("create over existing: err = %v, want ErrExists", err)
	}

	// IgnoreIfExists turns it into a no-op.
	tx = NewTransaction("create")
	tx.AddOp(ResourceOp{Kind: OpCreate, Path: existing, Content: "x", IgnoreIfExists: true})
	plan, err := tx.Preflight()
	if err != nil {
		t.Fatalf("IgnoreIfExists: %v", err)
	}
	if len(plan.Files) != 0 {
		t.Fatalf("plan = %+v, want no changes", plan.Files)
	}

	// Overwrite is honoured.
	tx = NewTransaction("create")
	tx.AddOp(ResourceOp{Kind: OpCreate, Path: existing, Content: "package q\n", Overwrite: true})
	if _, err := tx.Apply(); err != nil {
		t.Fatalf("Overwrite: %v", err)
	}
	if got, want := mustRead(t, existing), "package q\n"; got != want {
		t.Fatalf("there.go = %q, want %q", got, want)
	}

	// A create needs its parent directory to exist.
	tx = NewTransaction("create")
	tx.AddOp(ResourceOp{Kind: OpCreate, Path: filepath.Join(dir, "sub", "x.go")})
	if _, err := tx.Preflight(); !errors.Is(err, ErrBadPath) {
		t.Fatalf("missing parent: err = %v, want ErrBadPath", err)
	}

	// Rename and delete of a missing source.
	tx = NewTransaction("rename file")
	tx.AddOp(ResourceOp{Kind: OpRename, Path: missing, NewPath: filepath.Join(dir, "x.go")})
	if _, err := tx.Preflight(); !errors.Is(err, ErrNotFound) {
		t.Fatalf("rename missing: err = %v, want ErrNotFound", err)
	}
	tx = NewTransaction("delete file")
	tx.AddOp(ResourceOp{Kind: OpDelete, Path: missing})
	if _, err := tx.Preflight(); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete missing: err = %v, want ErrNotFound", err)
	}
	tx = NewTransaction("delete file")
	tx.AddOp(ResourceOp{Kind: OpDelete, Path: missing, IgnoreIfNotExists: true})
	if plan, err := tx.Preflight(); err != nil || len(plan.Files) != 0 {
		t.Fatalf("IgnoreIfNotExists: plan = %v, err = %v", plan, err)
	}

	// Rename onto an existing destination needs Overwrite.
	other := seed(t, dir, "other.go", "package p\n")
	tx = NewTransaction("rename file")
	tx.AddOp(ResourceOp{Kind: OpRename, Path: other, NewPath: existing})
	if _, err := tx.Preflight(); !errors.Is(err, ErrExists) {
		t.Fatalf("rename onto existing: err = %v, want ErrExists", err)
	}
}

func TestPreflightEditsFileCreatedEarlier(t *testing.T) {
	// Steps are ordered against a virtual filesystem: an edit may target a file
	// an earlier step created, and a second edit list for the same file sees the
	// text the first one produced.
	dir := t.TempDir()
	created := filepath.Join(dir, "gen.go")

	tx := NewTransaction("generate and patch")
	tx.AddOp(ResourceOp{Kind: OpCreate, Path: created, Content: "package p\n\nvar A = 1\n"})
	tx.AddEdit(FileEdit{Path: created, Edits: []TextEdit{renameEdit("Alpha", 1)}})
	tx.AddEdit(FileEdit{Path: created, Edits: []TextEdit{{
		Range:   Range{Start: Position{2, 4}, End: Position{2, 9}},
		NewText: "Beta",
	}}})

	if _, err := tx.Apply(); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got, want := mustRead(t, created), "package p\n\nvar Beta = 1\n"; got != want {
		t.Fatalf("gen.go = %q, want %q", got, want)
	}
}

func TestPreviewOfCompletionApplication(t *testing.T) {
	// The two halves of the package meet here: a completion item with an
	// auto-import becomes one previewable file transaction.
	dir := t.TempDir()
	path := seed(t, dir, "main.go", bufSrc)

	app, err := ResolveCompletion(bufSrc, caret, CompletionItem{
		Label: "Println",
		TextEdit: &TextEdit{
			Range:   Range{Start: Position{3, 5}, End: Position{3, 7}},
			NewText: "Println",
		},
		AdditionalTextEdits: []TextEdit{{
			Range:   Range{Start: Position{1, 0}, End: Position{1, 0}},
			NewText: "import \"fmt\"\n",
		}},
	})
	if err != nil {
		t.Fatalf("ResolveCompletion: %v", err)
	}

	tx := NewTransaction("accept completion")
	tx.AddEdit(FileEdit{Path: path, Edits: app.Edits()})
	pv, err := tx.Preview()
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	want := "package main\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println\n}\n"
	if len(pv.Files) != 1 || pv.Files[0].After != want {
		t.Fatalf("preview = %+v, want one file with %q", pv.Files, want)
	}
	if got := pv.Files[0].LineDelta(); got != 1 {
		t.Fatalf("LineDelta = %d, want 1 added line", got)
	}
	if mustRead(t, path) != bufSrc {
		t.Fatal("main.go changed during Preview")
	}

	if _, err := tx.Apply(); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got := mustRead(t, path); got != want {
		t.Fatalf("main.go = %q, want %q", got, want)
	}
}
