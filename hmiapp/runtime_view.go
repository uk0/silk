package hmiapp

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/hmi"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/paint"
)

// RuntimeViewOptions are immutable host policy. Construction and SetSnapshot
// perform no I/O. EnsureDatabaseDirectory only runs on an explicit Start worker;
// DatabasePath is a filesystem path when that option is enabled, not a SQL URI.
type RuntimeViewOptions struct {
	RequestHeaders          func(string) http.Header
	EnsureDatabaseDirectory bool
	// UI-thread, borrowed lifetime; nil revokes access before shutdown. Do not wait.
	RuntimeChanged func(*hmi.Runtime)
}

// RuntimeView is an embeddable, opt-in runtime, not an independent native window.
// It owns acquisition/store lifetime and borrows no authored document. Hiding the
// outer view stops acquisition; changing Diagram/Points tabs does not. All methods
// except Wait are UI-thread-only. Close cancels; Wait belongs outside the UI loop.
type RuntimeView struct {
	gui.Widget
	Diagram                        *gui.ProcessDiagram
	Points                         *gui.ProcessPointMonitor
	options                        RuntimeViewOptions
	project                        hmi.Project
	database, source               string
	configured, closed, pointsMode bool
	startGuard                     func() bool
	session                        *session
	err                            error
	status                         *gui.Label
	buttons                        []*gui.Button
	sourceAction                   func()
	post                           func(func())
	postContext                    func(context.Context, func()) error
	workerMu                       sync.Mutex
	idle                           chan struct{}
}

func init() {
	i18n.AddMany("zh", map[string]string{
		"Process Runtime": "组态运行", "Run snapshot": "运行快照", "Stop runtime": "停止运行", "Live diagram": "实时组态", "Live points": "实时点位", "Go to source": "返回源文件", "Fit diagram": "适应画布",
		"No snapshot; use Run 2D Process from a source document": "没有运行快照，请从源文件运行二维组态",
		"Ready; no acquisition until Run":                        "就绪；点击运行后才开始采集", "Starting process runtime…": "正在启动组态运行时…", "Running frozen source snapshot": "正在运行源文件快照", "Stopping process runtime…": "正在停止组态运行时…", "Stopped; no acquisition": "已停止；没有采集任务", "Source changed; run the current buffer again": "源文件已变化，请重新运行当前缓冲区",
	})
}

func NewRuntimeView(options RuntimeViewOptions) *RuntimeView {
	v := new(RuntimeView)
	v.Widget.Init(v)
	v.options = options
	v.post = gui.Post
	v.postContext = gui.PostContext
	v.idle = make(chan struct{})
	close(v.idle)
	v.Diagram = gui.NewProcessDiagram()
	v.Diagram.SetEditing(false)
	v.Diagram.SetParent(v)
	v.Points = gui.NewProcessPointMonitor()
	v.Points.SetParent(v)
	v.pointsMode = true
	v.Diagram.SetVisible(false)
	add := func(text, icon string, fn func()) {
		b := gui.NewButton1(i18n.T(text), gui.LoadIcon(icon))
		b.SetParent(v)
		b.SetTextVisible(true)
		b.Action().BindFunc0(fn)
		v.buttons = append(v.buttons, b)
	}
	add("Run snapshot", "run", v.Start)
	add("Stop runtime", "stop", v.Stop)
	add("Live diagram", "preview", func() { v.SetPointsMode(false) })
	add("Live points", "tree-view", func() { v.SetPointsMode(true) })
	add("Fit diagram", "maximize", func() { v.Diagram.FitToView() })
	add("Go to source", "document", func() {
		if v.sourceAction != nil {
			v.sourceAction()
		}
	})
	v.status = gui.NewLabel("")
	v.status.SetParent(v)
	v.setStatus("No snapshot; use Run 2D Process from a source document")
	v.refreshControls()
	return v
}

func (v *RuntimeView) Title() string    { return i18n.T("Process Runtime") }
func (v *RuntimeView) Icon() paint.Icon { return gui.LoadIcon("preview") }

func (v *RuntimeView) SizeHints() gui.SizeHints {
	return gui.SizeHints{Width: 960, Height: 400, Policy: gui.GrowHorizontal | gui.GrowVertical}
}
func (v *RuntimeView) Draw(g paint.Painter) {
	g.SetBrush1(gui.Theme().FormColor)
	g.Rectangle(0, 0, v.Width(), v.Height())
	g.Fill()
}
func (v *RuntimeView) OnResize() {
	w, h := v.Size()
	cols := max(1, min(len(v.buttons), int(max(0, w-16)/145)))
	bw := max(0, w-16) / float64(cols)
	rows := (len(v.buttons) + cols - 1) / cols
	top := 8 + float64(rows)*34
	for i, b := range v.buttons {
		b.SetBounds(8+float64(i%cols)*bw, 4+float64(i/cols)*34, max(0, bw-4), 30)
	}
	height := max(0, h-top-26)
	v.Diagram.SetBounds(0, top, w, height)
	v.Points.SetBounds(0, top, w, height)
	v.status.SetBounds(8, max(0, h-24), max(0, w-16), 22)
	v.Diagram.FitToView()
}
func (v *RuntimeView) setStatus(text string) {
	label := i18n.T(text)
	if v.source != "" {
		label = v.source + " · " + label
	}
	v.status.SetText(label)
}

// SetSnapshot stages an independent validated project. It never opens its store.
func (v *RuntimeView) SetSnapshot(project hmi.Project, database string) error {
	if v.closed || v.Busy() {
		return fmt.Errorf("hmiapp: runtime view is busy or closed")
	}
	if database == "" {
		return fmt.Errorf("hmiapp: empty runtime database path")
	}
	if err := v.Diagram.SetProject(project); err != nil {
		return err
	}
	v.Diagram.SetEditing(false)
	v.project = v.Diagram.Project()
	v.Diagram.FitToView()
	v.database = database
	v.configured = true
	v.err = nil
	v.setStatus("Ready; no acquisition until Run")
	v.refreshControls()
	return nil
}
func (v *RuntimeView) SetSourceLabel(source string) { v.source = source }
func (v *RuntimeView) SetSourceAction(fn func())    { v.sourceAction = fn; v.refreshControls() }
func (v *RuntimeView) SetStartGuard(fn func() bool) { v.startGuard = fn; v.refreshControls() }
func (v *RuntimeView) Busy() bool                   { return v.session != nil }
func (v *RuntimeView) LastError() error             { return v.err }
func (v *RuntimeView) Runtime() *hmi.Runtime {
	if v.closed || v.session == nil || v.session.stopping || !v.session.available {
		return nil
	}
	return v.session.runtime
}
func (v *RuntimeView) CanStart() bool {
	if v.closed || v.Busy() || !v.configured || !gui.WidgetTreeVisible(v) {
		return false
	}
	if v.startGuard != nil && !v.startGuard() {
		return false
	}
	return !v.closed && !v.Busy() && v.configured && gui.WidgetTreeVisible(v)
}
func (v *RuntimeView) refreshControls() {
	if len(v.buttons) == 0 {
		return
	}
	v.buttons[0].SetEnabled(v.CanStart())
	v.buttons[1].SetEnabled(!v.closed && v.Busy() && !v.session.stopping)
	for _, i := range []int{2, 3} {
		v.buttons[i].SetEnabled(!v.closed && v.configured)
	}
	v.buttons[2].Action().SetChecked(!v.pointsMode)
	v.buttons[3].Action().SetChecked(v.pointsMode)
	v.buttons[4].SetEnabled(!v.closed && v.configured && !v.pointsMode)
	v.buttons[5].SetEnabled(!v.closed && v.sourceAction != nil)
}
func (v *RuntimeView) SetPointsMode(points bool) {
	if v.closed {
		return
	}
	v.pointsMode = points
	v.Points.SetVisible(points)
	v.Diagram.SetVisible(!points)
	v.bindVisible()
	v.refreshControls()
	v.Update()
}
func (v *RuntimeView) bindVisible() {
	if err := v.bindRuntime(v.Runtime()); err != nil {
		v.err = err
		v.Stop()
		v.status.SetText(err.Error())
	}
}
func (v *RuntimeView) bindRuntime(rt *hmi.Runtime) error {
	v.Points.SetRuntime(nil)
	v.Diagram.UnbindTags()
	if rt == nil {
		return nil
	}
	if v.pointsMode {
		v.Points.SetRuntime(rt)
		return nil
	}
	_, err := v.Diagram.BindTags(rt.Tags)
	return err
}

func (v *RuntimeView) notify(s *session, rt *hmi.Runtime) {
	if rt != nil {
		if v.closed || v.session != s || s.stopping || s.available {
			return
		}
		s.available = true
	} else {
		if !s.available {
			return
		}
		s.available = false
	}
	if v.options.RuntimeChanged != nil {
		v.options.RuntimeChanged(rt)
	}
}
func (v *RuntimeView) Start() {
	if !v.CanStart() {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &session{cancel: cancel, done: make(chan struct{})}
	v.session = s
	v.err = nil
	v.workerMu.Lock()
	v.idle = s.done
	v.workerMu.Unlock()
	v.setStatus("Starting process runtime…")
	v.refreshControls()
	project, database, options, post, postContext := v.project, v.database, v.options, v.post, v.postContext
	go func() {
		var err error
		defer func() {
			close(s.done)
			post(func() {
				if v.closed || v.session != s {
					return
				}
				v.Points.SetRuntime(nil)
				v.Diagram.UnbindTags()
				v.session = nil
				if err == nil {
					err = v.err
				}
				v.err = err
				v.refreshControls()
				if err != nil {
					v.status.SetText(err.Error())
				} else {
					v.setStatus("Stopped; no acquisition")
				}
				v.notify(s, nil)
			})
		}()
		if options.EnsureDatabaseDirectory {
			if err = os.MkdirAll(filepath.Dir(database), 0700); err != nil {
				return
			}
		}
		err = runStoredRuntime(ctx, project, database, options.RequestHeaders, postContext, func(rt *hmi.Runtime) {
			if v.closed || v.session != s || s.stopping || ctx.Err() != nil {
				return
			}
			s.runtime = rt
			if bindErr := v.bindRuntime(rt); bindErr != nil {
				v.err = bindErr
				v.Stop()
				v.status.SetText(bindErr.Error())
				return
			}
			if v.closed || s.stopping {
				return
			}
			v.setStatus("Running frozen source snapshot")
			v.refreshControls()
			v.notify(s, rt)
		})
	}()
}
func (v *RuntimeView) Stop() {
	s := v.session
	if s == nil || s.stopping {
		return
	}
	s.stopping = true
	v.Points.SetRuntime(nil)
	v.Diagram.UnbindTags()
	s.cancel()
	v.setStatus("Stopping process runtime…")
	v.refreshControls()
	v.notify(s, nil)
}

// Invalidate revokes restart permission as well as any pending/current session.
func (v *RuntimeView) Invalidate() {
	if v.closed {
		return
	}
	v.configured = false
	v.Stop()
	v.setStatus("Source changed; run the current buffer again")
	v.refreshControls()
}
func (v *RuntimeView) OnHide() { v.Stop() }
func (v *RuntimeView) OnTreeVisibilityChanged(visible bool) {
	if !visible {
		v.Stop()
	}
	v.refreshControls()
}
func (v *RuntimeView) Close() {
	if v.closed {
		return
	}
	v.Stop()
	v.closed = true
	v.Points.Close()
	v.Diagram.Close()
	v.refreshControls()
}
func (v *RuntimeView) Wait(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("hmiapp: nil wait context")
	}
	v.workerMu.Lock()
	done := v.idle
	v.workerMu.Unlock()
	select {
	case <-done:
	case <-ctx.Done():
		return ctx.Err()
	}
	return v.Points.Wait(ctx)
}
