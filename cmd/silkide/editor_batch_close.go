package main

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/internal/filetxn"
	"github.com/uk0/silk/internal/processutil"
)

type editorCloseMode int

const (
	closeOtherEditors editorCloseMode = iota
	closeRightEditors
	closeAllEditors
)
const editorCloseMemoryLimit = 64 << 20

type editorCloseTarget struct {
	editor     *gui.CodeEditor
	path, text string
	revision   uint64
}
type editorCloseDisk struct {
	before filetxn.Snapshot
	err    error
	dirty  bool
}

// Only the UI thread owns a job and its widget identities. Workers receive
// immutable paths/text and return values; they never inspect editor widgets.
type editorCloseJob struct {
	tabs      *gui.TabWidget
	owner     *workbench
	targets   []editorCloseTarget
	reserved  map[string]bool
	ctx       context.Context
	cancel    context.CancelFunc
	cancelled bool
	next      int
	post      func(func())
	launch    func(func())
}

var activeEditorClose *editorCloseJob
var editorClosePost = gui.Post
var editorCloseLaunch = func(f func()) { go f() }
var editorCloseRead = filetxn.ReadLimit
var editorCloseCommit = filetxn.Commit

func editorCloseAffects(path string) bool {
	return activeEditorClose != nil && activeEditorClose.reserved[workspaceSearchKey(path)]
}

func editorCloseTargets(tabs *gui.TabWidget, anchor *gui.CodeEditor, mode editorCloseMode) []editorCloseTarget {
	if tabs == nil {
		return nil
	}
	anchorIndex := -1
	if mode != closeAllEditors {
		var path string
		anchorIndex, path = resolveEditorTabTarget(tabs, anchor)
		if anchorIndex < 0 || path == "" {
			return nil
		}
	}
	var targets []editorCloseTarget
	for i := 0; i < tabs.Count(); i++ {
		if mode == closeOtherEditors && tabs.Stack().Page(i) == anchor || mode == closeRightEditors && i <= anchorIndex {
			continue
		}
		ed, ok := tabs.Stack().Page(i).(*gui.CodeEditor)
		if !ok || ed.FilePath() == "" || openEditors[ed.FilePath()] != ed {
			continue
		}
		targets = append(targets, editorCloseTarget{editor: ed, path: ed.FilePath()})
	}
	return targets
}

func startEditorBatchClose(tabs *gui.TabWidget, anchor *gui.CodeEditor, mode editorCloseMode) bool {
	if activeEditorClose != nil || activeFileRename != nil || workspaceSearchBusy() || macroBusy || dirtyPromptUp {
		silkideToast(i18n.T("Finish the current file operation before closing editors"), gui.ToastWarning)
		return false
	}
	targets := editorCloseTargets(tabs, anchor, mode)
	if len(targets) == 0 {
		return false
	}
	total := 0
	for i := range targets {
		targets[i].text = targets[i].editor.Text()
		targets[i].revision = targets[i].editor.TextRevision()
		total += len(targets[i].text)
		if total > editorCloseMemoryLimit || len(targets) > 4096 {
			silkideToast(i18n.T("Close fewer editors at once (64 MiB snapshot limit)"), gui.ToastWarning)
			return false
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	job := &editorCloseJob{tabs: tabs, owner: globalWorkbench, targets: targets, reserved: map[string]bool{}, ctx: ctx, cancel: cancel, post: editorClosePost, launch: editorCloseLaunch}
	for _, target := range targets {
		job.reserved[workspaceSearchKey(target.path)] = true
	}
	activeEditorClose = job
	silkideToast(i18n.Tf("Checking %d editors in the background...", len(targets)), gui.ToastInfo)
	read := editorCloseRead
	// At most one worker and one completion. Cancelling a blocked OS read does
	// not create another worker: reservation stays until that worker returns.
	job.launch(func() {
		results := make([]editorCloseDisk, len(targets))
		remaining := int64(editorCloseMemoryLimit)
		for i, target := range targets {
			if ctx.Err() != nil {
				break
			}
			before, err := read(target.path, max(1, remaining))
			data := before.Bytes()
			remaining -= int64(len(data))
			results[i] = editorCloseDisk{before: before, err: err, dirty: err != nil && target.text != "" || err == nil && string(data) != target.text}
		}
		job.post(func() { job.inspected(results) })
	})
	return true
}

func cancelEditorBatchClose() {
	if job := activeEditorClose; job != nil && !job.cancelled {
		job.cancelled = true
		job.cancel()
		silkideToast(i18n.T("Closing cancelled; an in-flight save may still finish"), gui.ToastInfo)
	}
}
func (j *editorCloseJob) validTarget(t editorCloseTarget) bool {
	if openEditors[t.path] != t.editor || t.editor.FilePath() != t.path || t.editor.TextRevision() != t.revision {
		return false
	}
	for i := 0; i < j.tabs.Count(); i++ {
		if j.tabs.Stack().Page(i) == t.editor {
			return true
		}
	}
	return false
}
func (j *editorCloseJob) live() bool {
	return activeEditorClose == j && !j.cancelled && j.owner == globalWorkbench
}
func (j *editorCloseJob) allCurrent() bool {
	if !j.live() {
		return false
	}
	pages := make(map[*gui.CodeEditor]bool, j.tabs.Count())
	for i := 0; i < j.tabs.Count(); i++ {
		if ed, ok := j.tabs.Stack().Page(i).(*gui.CodeEditor); ok {
			pages[ed] = true
		}
	}
	for _, target := range j.targets[j.next:] {
		if !pages[target.editor] || openEditors[target.path] != target.editor || target.editor.FilePath() != target.path || target.editor.TextRevision() != target.revision {
			return false
		}
	}
	return true
}
func (j *editorCloseJob) finish(message string, kind gui.ToastLevel) {
	j.cancel()
	if activeEditorClose != j {
		return
	}
	activeEditorClose = nil
	j.targets = nil
	j.reserved = nil
	if message != "" {
		silkideToast(message, kind)
	}
}
func (j *editorCloseJob) changed() {
	j.finish(i18n.T("Closing stopped: editor contents or identity changed; remaining editors were kept"), gui.ToastWarning)
}
func (j *editorCloseJob) inspected(disk []editorCloseDisk) {
	if !j.live() {
		j.finish("", gui.ToastInfo)
		return
	}
	if !j.allCurrent() {
		j.changed()
		return
	}
	var dirty []string
	for i, d := range disk {
		if d.dirty {
			dirty = append(dirty, j.targets[i].path)
		}
	}
	choice := gui.DialogOK
	if len(dirty) > 0 {
		choice = askDirty(func() gui.DialogResult { return askCloseEditors(j.tabs, dirty) })
	}
	if !j.live() {
		j.finish("", gui.ToastInfo)
		return
	}
	if choice != gui.DialogOK && choice != gui.DialogNo {
		j.finish("", gui.ToastInfo)
		return
	}
	if !j.allCurrent() {
		j.changed()
		return
	}
	if choice == gui.DialogNo {
		j.post(j.closeNext)
		return
	}
	// Save All and clean closes share conflict verification on the worker.
	// No-op entries validate disk snapshots without rewriting clean files.
	targets := append([]editorCloseTarget(nil), j.targets...)
	commit := editorCloseCommit
	j.launch(func() {
		var changes []filetxn.Change
		outputs := make(map[int]string)
		unformatted := false
		var failure error
		for i, d := range disk {
			if j.ctx.Err() != nil {
				failure = j.ctx.Err()
				break
			}
			if d.err != nil {
				if d.dirty {
					failure = fmt.Errorf("%s: %w", targets[i].path, d.err)
					break
				}
				continue
			}
			out := targets[i].text
			if d.dirty && strings.EqualFold(filepath.Ext(targets[i].path), ".go") {
				if formatted, err := formatClosingEditor(j.ctx, out); err == nil {
					out = formatted
				} else {
					unformatted = true
				}
			}
			if d.dirty {
				outputs[i] = out
			}
			if d.before.Exists() || d.dirty {
				changes = append(changes, filetxn.Change{Before: d.before, Data: []byte(out)})
			}
		}
		if failure == nil {
			failure = j.ctx.Err()
		}
		if failure == nil {
			failure = commit(changes...)
		}
		j.post(func() { j.saved(outputs, unformatted, failure) })
	})
}
func formatClosingEditor(ctx context.Context, text string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gofmt")
	processutil.NoConsoleWindow(cmd)
	cmd.Stdin = strings.NewReader(text)
	data, err := cmd.Output()
	return string(data), err
}
func (j *editorCloseJob) saved(outputs map[int]string, unformatted bool, err error) {
	if activeEditorClose != j {
		return
	}
	if j.owner != globalWorkbench {
		j.finish("", gui.ToastInfo)
		return
	}
	if err != nil {
		if j.cancelled {
			j.finish("", gui.ToastInfo)
		} else {
			j.finish(i18n.Tf("Editors kept open: save or disk check failed: %v", err), gui.ToastError)
		}
		return
	}
	closeAllowed := j.allCurrent()
	j.post(func() { j.publishSaved(outputs, unformatted, closeAllowed, 0) })
}
func (j *editorCloseJob) publishSaved(outputs map[int]string, unformatted, closeAllowed bool, index int) {
	if activeEditorClose != j {
		return
	}
	if j.owner != globalWorkbench {
		j.finish("", gui.ToastInfo)
		return
	}
	// A save may finish after cancellation/typing. Publish one editor per turn;
	// do not overwrite/mark saved a buffer newer than the captured revision.
	if index < len(j.targets) {
		target := &j.targets[index]
		if out, ok := outputs[index]; ok && j.validTarget(*target) {
			if target.text != out {
				target.editor.ReplaceAllText(out)
				target.text = out
				target.revision = target.editor.TextRevision()
			}
			target.editor.MarkSaved()
			target.editor.RefreshGitStatus()
			refreshEditorTab(j.tabs, target.editor, target.path)
		}
		j.post(func() { j.publishSaved(outputs, unformatted, closeAllowed, index+1) })
		return
	}
	if len(outputs) > 0 {
		refreshGitChanges(globalCanvas)
	}
	if !j.live() {
		j.finish("", gui.ToastInfo)
		return
	}
	if !closeAllowed || !j.allCurrent() {
		j.changed()
		return
	}
	if unformatted {
		silkideToast(i18n.T("gofmt failed; saved unformatted"), gui.ToastWarning)
	}
	j.post(j.closeNext)
}
func (j *editorCloseJob) closeNext() {
	if !j.live() {
		j.finish("", gui.ToastInfo)
		return
	}
	if j.next == len(j.targets) {
		count := j.next
		j.finish(i18n.Tf("Closed %d editors", count), gui.ToastSuccess)
		if ed := activeEditor(j.tabs); ed != nil {
			focusEditorTab(j.tabs, ed)
		}
		return
	}
	target := j.targets[j.next]
	if !j.validTarget(target) || !detachEditorTab(j.tabs, target.editor, target.editor, target.path) {
		j.changed()
		return
	}
	delete(j.reserved, workspaceSearchKey(target.path))
	j.next++
	// One removal per event-loop turn, never a nested synchronous close loop.
	j.post(j.closeNext)
}

var askCloseEditors = func(tabs *gui.TabWidget, paths []string) gui.DialogResult {
	dlg := gui.NewDialog(i18n.T("Close editors"), tabs)
	content := gui.NewVBox()
	content.SetSpacing(12)
	label := gui.NewLabel(i18n.Tf("%d editors have unsaved changes. Save before closing?", len(paths)))
	label.SetWrap(true)
	content.AddWidget(label)
	// Bound native dialog height even for thousands of open documents. Full
	// paths remain available on hover; the total remains explicit.
	names := paths
	if len(names) > 8 {
		names = names[:8]
	}
	for _, path := range names {
		label := gui.NewLabel(closeEditorPathLabel(path))
		gui.SetToolTip(label, path)
		defer gui.SetToolTip(label, "")
		label.SetWrap(true)
		content.AddWidget(label)
	}
	if len(paths) > len(names) {
		content.AddWidget(gui.NewLabel(i18n.Tf("... and %d more editors", len(paths)-len(names))))
	}
	dlg.SetContent(content)
	dlg.AddButton(i18n.T("Save All"), gui.DialogOK)
	dlg.AddButton(i18n.T("Discard All"), gui.DialogNo)
	dlg.AddButton(i18n.T("Cancel"), gui.DialogCancel)
	return dlg.ShowModal()
}

func closeEditorPathLabel(path string) string {
	runes := []rune(path)
	if len(runes) <= 72 {
		return path
	}
	return string(runes[:12]) + "…" + string(runes[len(runes)-59:])
}
