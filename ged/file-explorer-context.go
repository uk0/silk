package ged

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"silk/gui"
)

// This file adds the right-click context-menu workflow to FileExplorer:
// New File / New Folder / Rename / Delete / Refresh — the create/rename/
// delete operations every IDE file tree has and Qt Creator offers via
// its Projects pane context menu.
//
// The actual filesystem mutations live in small free functions
// (validateFileName, fsCreateFile, fsCreateFolder, fsRename, fsDelete,
// newItemDir) so they can be unit-tested in a t.TempDir without standing
// up a window, a menu, or a modal dialog. The FileExplorer methods are
// thin GUI glue: hit-test → build menu → prompt → call the free function
// → refresh the tree.

// newItemDir returns the directory a "New File" / "New Folder" action
// should create into for the given entry. A directory entry creates
// inside itself; a file entry creates alongside it (in its parent dir);
// a nil entry (right-click on empty space) targets the explorer root.
func newItemDir(entry *fileEntry, root string) string {
	if entry == nil {
		return root
	}
	if entry.isDir {
		return entry.path
	}
	return filepath.Dir(entry.path)
}

// validateFileName rejects names that would let a create/rename action
// escape the chosen directory or collide with the dot entries: empty
// (after trimming), "." / "..", or anything containing a path
// separator. Returns nil for an acceptable single path component.
func validateFileName(name string) error {
	n := strings.TrimSpace(name)
	if n == "" {
		return errors.New("名称不能为空")
	}
	if n == "." || n == ".." {
		return errors.New("无效的名称")
	}
	if strings.ContainsAny(n, `/\`) {
		return errors.New("名称不能包含路径分隔符")
	}
	return nil
}

// fsCreateFile creates an empty file `name` inside `dir`. It fails if
// the name is invalid or the target already exists (O_EXCL), so a
// "New File" never silently truncates an existing file. Returns the
// created path on success.
func fsCreateFile(dir, name string) (string, error) {
	if err := validateFileName(name); err != nil {
		return "", err
	}
	p := filepath.Join(dir, strings.TrimSpace(name))
	f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return "", fmt.Errorf("%s 已存在", name)
		}
		return "", err
	}
	f.Close()
	return p, nil
}

// fsCreateFolder makes a directory `name` inside `dir`. Fails on an
// invalid name or an existing target. Returns the created path.
func fsCreateFolder(dir, name string) (string, error) {
	if err := validateFileName(name); err != nil {
		return "", err
	}
	p := filepath.Join(dir, strings.TrimSpace(name))
	if _, err := os.Stat(p); err == nil {
		return "", fmt.Errorf("%s 已存在", name)
	}
	if err := os.Mkdir(p, 0o755); err != nil {
		return "", err
	}
	return p, nil
}

// fsRename renames the entry at oldPath to newName within the same
// parent directory. A no-op rename (newName equals the current base)
// succeeds silently. Fails on an invalid name or a colliding target.
// Returns the new path.
func fsRename(oldPath, newName string) (string, error) {
	if err := validateFileName(newName); err != nil {
		return "", err
	}
	newName = strings.TrimSpace(newName)
	if newName == filepath.Base(oldPath) {
		return oldPath, nil
	}
	np := filepath.Join(filepath.Dir(oldPath), newName)
	if _, err := os.Stat(np); err == nil {
		return "", fmt.Errorf("%s 已存在", newName)
	}
	if err := os.Rename(oldPath, np); err != nil {
		return "", err
	}
	return np, nil
}

// fsDelete removes the file or directory tree at path. Directories are
// removed recursively (os.RemoveAll) — the caller is expected to have
// confirmed with the user first.
func fsDelete(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("空路径")
	}
	return os.RemoveAll(path)
}

// --- GUI glue ---

// OnRightDown opens the file-tree context menu. The clicked entry (if
// any) becomes the menu's target; a click on empty space targets the
// explorer root so New File / New Folder still work there.
func (this *FileExplorer) OnRightDown(x, y float64) {
	this.SetFocus()
	idx := this.hitTest(y)
	var entry *fileEntry
	if idx >= 0 && idx < len(this.flatList) {
		entry = this.flatList[idx]
		this.selectedIdx = idx
		this.Self().Update()
	}

	gui.ShowContextMenu(this.Self(), x, y, func(m *gui.Menu) {
		m.AddButton1("新建文件", nil).Action().BindFunc0(func() { this.promptNewFile(entry) })
		m.AddButton1("新建文件夹", nil).Action().BindFunc0(func() { this.promptNewFolder(entry) })
		// Rename / Delete only make sense on a real entry, and never on
		// the root (renaming the project root from inside the tree would
		// orphan the explorer).
		if entry != nil && entry.path != this.rootDir {
			m.AddSeparator()
			m.AddButton1("重命名", nil).Action().BindFunc0(func() { this.promptRename(entry) })
			m.AddButton1("删除", nil).Action().BindFunc0(func() { this.confirmDelete(entry) })
		}
		m.AddSeparator()
		m.AddButton1("刷新", nil).Action().BindFunc0(func() { this.refreshKeepingExpansion() })
	})
}

// promptNewFile asks for a filename and creates it inside the target
// directory, then opens it in the editor so the user can start typing.
func (this *FileExplorer) promptNewFile(entry *fileEntry) {
	dir := newItemDir(entry, this.rootDir)
	name, ok := gui.ShowInputBox(this.Self(), nil, "新建文件", "文件名:", "")
	if !ok {
		return
	}
	path, err := fsCreateFile(dir, name)
	if err != nil {
		gui.ShowMessageBox(this.Self(), nil, "新建文件失败", err.Error(), []string{"确定"})
		return
	}
	this.expandDir(dir)
	this.refreshKeepingExpansion()
	if this.cbFileOpen != nil {
		this.cbFileOpen(path)
	}
}

// promptNewFolder asks for a folder name and creates it inside the
// target directory.
func (this *FileExplorer) promptNewFolder(entry *fileEntry) {
	dir := newItemDir(entry, this.rootDir)
	name, ok := gui.ShowInputBox(this.Self(), nil, "新建文件夹", "文件夹名:", "")
	if !ok {
		return
	}
	if _, err := fsCreateFolder(dir, name); err != nil {
		gui.ShowMessageBox(this.Self(), nil, "新建文件夹失败", err.Error(), []string{"确定"})
		return
	}
	this.expandDir(dir)
	this.refreshKeepingExpansion()
}

// promptRename asks for a new name for the entry and applies it.
func (this *FileExplorer) promptRename(entry *fileEntry) {
	if entry == nil {
		return
	}
	name, ok := gui.ShowInputBox(this.Self(), nil, "重命名", "新名称:", entry.name)
	if !ok {
		return
	}
	if _, err := fsRename(entry.path, name); err != nil {
		gui.ShowMessageBox(this.Self(), nil, "重命名失败", err.Error(), []string{"确定"})
		return
	}
	this.refreshKeepingExpansion()
}

// confirmDelete pops a confirmation and removes the entry on "删除".
func (this *FileExplorer) confirmDelete(entry *fileEntry) {
	if entry == nil {
		return
	}
	kind := "文件"
	if entry.isDir {
		kind = "文件夹"
	}
	msg := fmt.Sprintf("确定删除%s \"%s\" 吗？此操作不可撤销。", kind, entry.name)
	if btn := gui.ShowMessageBox(this.Self(), nil, "删除", msg, []string{"删除", "取消"}); btn != "删除" {
		return
	}
	if err := fsDelete(entry.path); err != nil {
		gui.ShowMessageBox(this.Self(), nil, "删除失败", err.Error(), []string{"确定"})
		return
	}
	this.refreshKeepingExpansion()
}

// expandDir marks the directory at `path` (and the root) expanded so a
// freshly-created child is visible after the refresh. No-op when the
// path isn't a directory node in the current tree.
func (this *FileExplorer) expandDir(path string) {
	var walk func(e *fileEntry)
	walk = func(e *fileEntry) {
		if e == nil || !e.isDir {
			return
		}
		if e.path == path {
			e.expanded = true
		}
		for _, c := range e.children {
			walk(c)
		}
	}
	walk(this.root)
}

// refreshKeepingExpansion rescans the root directory while preserving
// the user's expanded folders — unlike Refresh()/SetRootDir, which
// reset every folder to collapsed (bar the top level). Used after every
// create/rename/delete so the tree updates in place without yanking the
// user back to a fully-collapsed view.
func (this *FileExplorer) refreshKeepingExpansion() {
	if this.rootDir == "" {
		return
	}
	expanded := map[string]bool{}
	collectExpandedPaths(this.root, expanded)
	this.root = this.scanDir(this.rootDir, 0)
	applyExpandedPaths(this.root, expanded)
	this.rebuildFlatList()
	if this.selectedIdx >= len(this.flatList) {
		this.selectedIdx = len(this.flatList) - 1
	}
	this.Self().Update()
}

// collectExpandedPaths records the paths of every expanded directory in
// the tree into `set`.
func collectExpandedPaths(e *fileEntry, set map[string]bool) {
	if e == nil || !e.isDir {
		return
	}
	if e.expanded {
		set[e.path] = true
	}
	for _, c := range e.children {
		collectExpandedPaths(c, set)
	}
}

// applyExpandedPaths re-expands directories whose paths are in `set`.
func applyExpandedPaths(e *fileEntry, set map[string]bool) {
	if e == nil || !e.isDir {
		return
	}
	if set[e.path] {
		e.expanded = true
	}
	for _, c := range e.children {
		applyExpandedPaths(c, set)
	}
}
