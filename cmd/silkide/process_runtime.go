package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/uk0/silk/async"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/hmi"
	"github.com/uk0/silk/hmiapp"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/paint"
)

// The frame owns this pane, including when a dock retains/removes its tab. Dock
// Close only stops; shutdown permanently closes its workers at frame teardown.
type processRuntimePane struct {
	gui.Widget
	host            *workbench
	dock            *gui.Dock
	view            *hmiapp.RuntimeView
	info            *gui.Label
	owner           *gui.CodeEditor
	path            string
	revision        uint64
	generation      uint64
	loading, closed bool
	cancel          context.CancelFunc
	decodeJobs      *async.Executor
	load            func(context.Context, string, func(hmi.Project, error)) error
	startRuntime    func()
}

func init() {
	i18n.AddMany("zh", map[string]string{
		"Run 2D Process": "运行二维组态", "Stop 2D Process": "停止二维组态", "Show Process Runtime": "显示组态运行面板",
		"Open a .silkhmi source document first": "请先打开 .silkhmi 源文件", "Stop the current process runtime first": "请先停止当前组态运行时", "Loading frozen source snapshot…": "正在读取源文件快照…", "Document changed; run the current buffer again": "文档已变化，请重新运行当前缓冲区",
	})
}

func newProcessRuntimePane(host *workbench, dock *gui.Dock) *processRuntimePane {
	p := new(processRuntimePane)
	p.Init(p)
	p.host = host
	p.dock = dock
	p.decodeJobs = async.NewExecutor(1)
	p.view = hmiapp.NewRuntimeView(hmiapp.RuntimeViewOptions{EnsureDatabaseDirectory: true})
	p.view.SetParent(p)
	p.info = gui.NewLabel(i18n.T("Open a .silkhmi source document first"))
	p.info.SetParent(p)
	p.view.SetStartGuard(func() bool { return !p.loading && p.current() })
	p.startRuntime = p.view.Start
	p.load = func(ctx context.Context, source string, done func(hmi.Project, error)) error {
		_, err := gui.RunAsync(ctx, p.decodeJobs, func(ctx context.Context) (hmi.Project, error) {
			if err := ctx.Err(); err != nil {
				return hmi.Project{}, err
			}
			project, err := hmi.Parse([]byte(source))
			if err == nil {
				err = ctx.Err()
			}
			return project, err
		}, done)
		return err
	}
	return p
}
func (p *processRuntimePane) Title() string    { return i18n.T("Process Runtime") }
func (p *processRuntimePane) Icon() paint.Icon { return gui.LoadIcon("preview") }
func (p *processRuntimePane) Draw(g paint.Painter) {
	g.SetBrush1(gui.Theme().FormColor)
	g.Rectangle(0, 0, p.Width(), p.Height())
	g.Fill()
}
func (p *processRuntimePane) OnResize() {
	w, h := p.Size()
	p.info.SetBounds(8, 2, max(0, w-16), 22)
	p.view.SetBounds(0, 26, w, max(0, h-26))
}
func (p *processRuntimePane) current() bool {
	return p.sourceAlive() && p.owner.TextRevision() == p.revision && !fileRenameAffects(p.path) && !workspaceSearchAffects(p.path) && !editorCloseAffects(p.path)
}
func (p *processRuntimePane) sourceAlive() bool {
	return !p.closed && p.owner != nil && p.host != nil && openEditors[p.path] == p.owner && p.owner.FilePath() == p.path
}
func (p *processRuntimePane) focusSource() {
	if p.sourceAlive() {
		p.host.activate(modeEdit)
		focusEditorTab(p.host.tabs, p.owner)
		p.owner.SetFocus()
	}
}
func (p *processRuntimePane) stop() bool {
	active := p.loading || p.view.Busy()
	p.generation++
	p.loading = false
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	p.view.Stop()
	return active
}
func (p *processRuntimePane) invalidate(ed *gui.CodeEditor) {
	if p.closed || p.owner != ed {
		return
	}
	p.stop()
	p.view.Invalidate()
	p.info.SetText(i18n.T("Document changed; run the current buffer again"))
}
func (p *processRuntimePane) retire(ed *gui.CodeEditor) {
	if p.owner != ed {
		return
	}
	p.invalidate(ed)
	p.owner = nil
	p.view.SetSourceAction(nil)
}
func (p *processRuntimePane) OnHide() { p.stop() }
func (p *processRuntimePane) OnTreeVisibilityChanged(visible bool) {
	if !visible && p.view != nil {
		p.stop()
	}
}
func (p *processRuntimePane) Close() { p.stop() }
func (p *processRuntimePane) shutdown() {
	if p.closed {
		return
	}
	p.stop()
	p.closed = true
	p.decodeJobs.Close()
	p.view.Close()
}
func (p *processRuntimePane) wait(ctx context.Context) error {
	if err := p.decodeJobs.Wait(ctx); err != nil {
		return err
	}
	return p.view.Wait(ctx)
}
func (p *processRuntimePane) reveal() bool {
	if p.closed || p.host == nil || p.host.frame == nil || p.dock == nil {
		return false
	}
	dock := p.dock
	if owner, ok := p.Parent().(*gui.Dock); ok && owner.Frame() == p.host.frame {
		dock = owner
	}
	if !dock.HasView(p) {
		dock.AddView(p)
	}
	if !revealToolView(p.host.tabs, p.host.canvas, dock, p, modeEdit) {
		return false
	}
	// Only the explicit first reveal may enlarge the original bottom dock. Do
	// not keep fighting user resizing or resize an independently moved tool dock.
	if dock == p.dock && dock.ParentBrick() != nil && p.host.frame.Height() > 680 && p.Height() < 300 {
		dock.ParentBrick().SetSplit(0.55)
		p.host.frame.Layout()
	}
	return true
}
func processRuntimeFor(tabs *gui.TabWidget, canvas *ged.GedView) *processRuntimePane {
	if w := globalWorkbench; w != nil && w.tabs == tabs && w.canvas == canvas {
		return w.processRuntime
	}
	return nil
}
func processRuntimeSource(ed *gui.CodeEditor) bool {
	return ed != nil && strings.EqualFold(filepath.Ext(ed.FilePath()), ".silkhmi") && !ed.Formatting()
}
func processRuntimeAvailable(tabs *gui.TabWidget, canvas *ged.GedView, ed *gui.CodeEditor) bool {
	p := processRuntimeFor(tabs, canvas)
	return p != nil && !p.closed && !p.loading && !p.view.Busy() && processRuntimeSource(ed)
}
func processRuntimeDatabase(path string) string {
	key := canonicalEditorPath(path)
	if runtime.GOOS == "windows" {
		key = strings.ToLower(key)
	}
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(core.LocalDataDir(), "industrial-runtime", fmt.Sprintf("%x.db", sum))
}
func bindProcessRuntime(tabs *gui.TabWidget, canvas *ged.GedView) paletteCommand {
	ed := visibleCaretEditor(tabs, canvas)
	return paletteCommand{Name: i18n.T("Run 2D Process"), Available: func() bool {
		return visibleCaretEditor(tabs, canvas) == ed && processRuntimeAvailable(tabs, canvas, ed)
	}, Fn: func() {
		if ed != nil {
			runProcessRuntime(tabs, canvas, ed)
		}
	}}
}
func runProcessRuntime(tabs *gui.TabWidget, canvas *ged.GedView, expected *gui.CodeEditor) {
	p := processRuntimeFor(tabs, canvas)
	ed := visibleCaretEditor(tabs, canvas)
	if p == nil || p.closed || !processRuntimeSource(ed) || expected != nil && expected != ed {
		return
	}
	if p.loading || p.view.Busy() {
		p.reveal()
		silkideToast(i18n.T("Stop the current process runtime first"), gui.ToastWarning)
		return
	}
	path, revision, source := ed.FilePath(), ed.TextRevision(), ed.Text()
	if openEditors[path] != ed || rejectBusyFile(path) {
		return
	}
	if len(source) > hmi.MaxDocumentBytes {
		silkideToast("hmi: document too large", gui.ToastWarning)
		return
	}
	p.stop()
	p.view.Invalidate()
	p.owner = ed
	p.path = path
	p.revision = revision
	p.view.SetSourceAction(p.focusSource)
	if !p.reveal() {
		return
	}
	p.loading = true
	p.generation++
	generation := p.generation
	p.info.SetText(filepath.Base(path) + " · " + i18n.T("Loading frozen source snapshot…"))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	p.cancel = cancel
	finish := func(project hmi.Project, err error) {
		if p.closed || !p.loading || p.generation != generation {
			return
		}
		cancel()
		p.cancel = nil
		p.loading = false
		if !p.current() || visibleCaretEditor(tabs, canvas) != ed {
			p.view.Invalidate()
			return
		}
		if err != nil {
			p.info.SetText(err.Error())
			silkideToast(i18n.T("Run 2D Process")+": "+err.Error(), gui.ToastWarning)
			return
		}
		p.view.SetSourceLabel(filepath.Base(path))
		p.info.SetText(path + fmt.Sprintf(" · revision %d", revision))
		if err = p.view.SetSnapshot(project, processRuntimeDatabase(path)); err != nil {
			silkideToast(err.Error(), gui.ToastWarning)
			return
		}
		p.startRuntime()
	}
	if err := p.load(ctx, source, finish); err != nil {
		finish(hmi.Project{}, err)
	}
}
func dispatchProcessRuntime(canvas *ged.GedView) bool {
	w := globalWorkbench
	if w == nil || w.canvas != canvas || documentMode(w.tabs, canvas) != modeEdit {
		return false
	}
	if pending, ok := w.tabs.Stack().CurrentPage().(*restorePage); ok && strings.EqualFold(filepath.Ext(pending.saved.Path), ".silkhmi") {
		// A loading/failed process source is not permission to run an unrelated
		// Go project. Only the restored tracked buffer can own its runtime.
		return true
	}
	ed := activeEditor(w.tabs)
	if ed == nil || !strings.EqualFold(filepath.Ext(ed.FilePath()), ".silkhmi") {
		return false
	}
	// An unavailable process document must not fall through to executing an
	// unrelated Go project (for example while JSON formatting is in flight).
	if !processRuntimeSource(ed) {
		return true
	}
	runProcessRuntime(w.tabs, canvas, ed)
	return true
}
func stopIDEProcessRuntime() bool {
	if w := globalWorkbench; w != nil && w.processRuntime != nil {
		return w.processRuntime.stop()
	}
	return false
}
func invalidateProcessRuntime(ed *gui.CodeEditor) {
	if w := globalWorkbench; w != nil && w.processRuntime != nil {
		w.processRuntime.invalidate(ed)
	}
}
func retireProcessRuntime(ed *gui.CodeEditor) {
	if w := globalWorkbench; w != nil && w.processRuntime != nil {
		w.processRuntime.retire(ed)
	}
}
