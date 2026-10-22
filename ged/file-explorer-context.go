package ged

import (
	"errors"
	"fmt"
	"github.com/uk0/silk/diagnostic"
	"github.com/uk0/silk/internal/fileops"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/uk0/silk/gui"
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
	fail := func(message string) error { return diagnostic.New(diagnostic.FileName, message, nil) }
	if n == "" {
		return fail("名称不能为空")
	}
	if n == "." || n == ".." {
		return fail("无效的名称")
	}
	if strings.ContainsAny(n, `/\:*?"<>|`) {
		return fail("名称不能包含路径分隔符或保留字符")
	}
	for _, r := range n {
		if unicode.IsControl(r) {
			return fail("名称不能包含控制字符")
		}
	}
	if strings.HasSuffix(n, ".") {
		return fail("名称不能以句点结尾")
	}
	base := strings.ToUpper(strings.TrimSpace(strings.SplitN(n, ".", 2)[0]))
	reserved := base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || base == "CONIN$" || base == "CONOUT$"
	if strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT") {
		tail := strings.TrimPrefix(strings.TrimPrefix(base, "COM"), "LPT")
		reserved = reserved || (len([]rune(tail)) == 1 && strings.ContainsAny(tail, "123456789¹²³"))
	}
	if reserved {
		return fail("名称是 Windows 保留设备名")
	}
	return nil
}

func fileOperationError(path string, err error) error {
	if errors.Is(err, fileops.ErrRenameUnsupported) {
		d := diagnostic.New(diagnostic.FileWrite, "Filesystem cannot perform an atomic no-replace rename", err).At(diagnostic.Location{File: path})
		d.Hint = "Use compatibility rename only after stopping other applications writing to this directory. The destination check cannot be atomic on this volume."
		return d
	}
	code := diagnostic.FileWrite
	if errors.Is(err, os.ErrExist) {
		code = diagnostic.FileConflict
	}
	return diagnostic.New(code, "", err).At(diagnostic.Location{File: path})
}

// O_EXCL protects existing contents, including dangling symlinks.
func fsCreateFile(dir, name string) (string, error) {
	if strings.TrimSpace(dir) == "" {
		return "", diagnostic.New(diagnostic.FileName, "choose a project directory first", nil)
	}
	if err := validateFileName(name); err != nil {
		return "", err
	}
	p := filepath.Join(dir, strings.TrimSpace(name))
	f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return "", fileOperationError(p, err)
	}
	if err = f.Close(); err != nil {
		return "", fileOperationError(p, err)
	}
	return p, nil
}
func fsCreateFolder(dir, name string) (string, error) {
	if strings.TrimSpace(dir) == "" {
		return "", diagnostic.New(diagnostic.FileName, "choose a project directory first", nil)
	}
	if err := validateFileName(name); err != nil {
		return "", err
	}
	p := filepath.Join(dir, strings.TrimSpace(name))
	if err := os.Mkdir(p, 0755); err != nil {
		return "", fileOperationError(p, err)
	}
	return p, nil
}

// Unlike a Stat + Rename sequence, the kernel operation cannot overwrite a
// target created between the check and the move (including dangling symlinks).
func fsRename(oldPath, newName string) (string, error) {
	return fsRenameMode(oldPath, newName, false)
}

func fsRenameMode(oldPath, newName string, compatible bool) (string, error) {
	if err := validateFileName(newName); err != nil {
		return "", err
	}
	newName = strings.TrimSpace(newName)
	if newName == filepath.Base(oldPath) {
		if _, err := os.Lstat(oldPath); err != nil {
			return "", fileOperationError(oldPath, err)
		}
		return oldPath, nil
	}
	np := filepath.Join(filepath.Dir(oldPath), newName)
	rename := fileops.RenameNoReplace
	if compatible {
		rename = fileops.RenameCompatible
	}
	if err := rename(oldPath, np); err != nil {
		return "", fileOperationError(np, err)
	}
	return np, nil
}

// The caller confirms deletion and protects the explorer project root.
func fsDelete(path string) error {
	clean := filepath.Clean(path)
	if strings.TrimSpace(path) == "" || clean == "." || clean == ".." || filepath.Dir(clean) == clean {
		return diagnostic.New(diagnostic.FileName, "refusing empty, current or filesystem-root deletion", nil)
	}
	if _, err := os.Lstat(path); err != nil {
		return fileOperationError(path, err)
	}
	if err := os.RemoveAll(path); err != nil {
		return fileOperationError(path, err)
	}
	return nil
}

// --- GUI glue ---

// OnRightDown opens the file-tree context menu. The clicked entry (if
// any) becomes the menu's target; a click on empty space targets the
// explorer root so New File / New Folder still work there.
func (this *FileExplorer) OnRightDown(x, y float64) {
	if this.rootDir == "" {
		return
	}
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
	if this.renameBusy {
		gui.ShowMessageBox(this.Self(), nil, "文件操作进行中", diagnostic.New(diagnostic.FileOperationBusy, "", nil).Error(), []string{"确定"})
		return
	}
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
	if this.renameBusy {
		gui.ShowMessageBox(this.Self(), nil, "文件操作进行中", diagnostic.New(diagnostic.FileOperationBusy, "", nil).Error(), []string{"确定"})
		return
	}
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
	if this.renameBusy {
		gui.ShowMessageBox(this.Self(), nil, "文件操作进行中", diagnostic.New(diagnostic.FileOperationBusy, "", nil).Error(), []string{"确定"})
		return
	}
	if entry == nil {
		return
	}
	name, ok := gui.ShowInputBox(this.Self(), nil, "重命名", "新名称:", entry.name)
	if !ok {
		return
	}
	report := func(err error) {
		if err != nil {
			message := err.Error()
			var detail *diagnostic.Error
			if errors.As(err, &detail) {
				message += "\n" + detail.Hint
			}
			gui.ShowMessageBox(this.Self(), nil, "重命名失败", message, []string{"确定"})
		}
	}
	path := entry.path
	complete := func(err error) {
		if errors.Is(err, fileops.ErrRenameUnsupported) {
			message := "此磁盘不支持原子防覆盖重命名。兼容模式会检查目标是否存在，但其他程序同时写入时仍可能发生覆盖。请先停止其他程序对此目录的写入。"
			if gui.ShowMessageBox(this.Self(), nil, "兼容模式重命名", message, []string{"取消", "兼容重命名"}) == "兼容重命名" {
				if e := this.renamePath(path, name, report, true); e != nil {
					report(e)
				}
			}
			return
		}
		report(err)
	}
	if err := this.RenamePath(path, name, complete); err != nil {
		complete(err)
	}
}

// confirmDelete pops a confirmation and removes the entry on "删除".
func (this *FileExplorer) confirmDelete(entry *fileEntry) {
	if this.renameBusy {
		gui.ShowMessageBox(this.Self(), nil, "文件操作进行中", diagnostic.New(diagnostic.FileOperationBusy, "", nil).Error(), []string{"确定"})
		return
	}
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
	if this.expanded == nil {
		this.expanded = map[string]bool{}
	}
	this.expanded[path] = true
	var walk func(*fileEntry)
	walk = func(e *fileEntry) {
		if e == nil {
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

// Refresh retains expansion, selected path and scroll position. Results from
// a previous root/generation are rejected even if already queued on the UI.
func (this *FileExplorer) refreshKeepingExpansion() { this.requestScan() }

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
