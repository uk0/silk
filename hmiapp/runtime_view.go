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

type runtimeViewMode uint8

const (
	runtimePoints runtimeViewMode = iota
	runtimeDiagram
	runtimeAlarms
)

const (
	runtimeRunButton = iota
	runtimeStopButton
	runtimeDiagramButton
	runtimePointsButton
	runtimeFitButton
	runtimeSourceButton
	runtimeAlarmsButton
)

// RuntimeView is an embeddable, opt-in runtime, not an independent native window.
// It owns acquisition/store lifetime and borrows no authored document. Hiding the
// outer view stops acquisition; changing its presentation does not. All methods
// except Wait are UI-thread-only. Close cancels; Wait belongs outside the UI loop.
type RuntimeView struct {
	gui.Widget
	Diagram            *gui.ProcessDiagram
	Points             *gui.ProcessPointMonitor
	Alarms             *gui.ProcessAlarmMonitor
	options            RuntimeViewOptions
	project            hmi.Project
	database, source   string
	configured, closed bool
	mode               runtimeViewMode
	startGuard         func() bool
	session            *session
	err                error
	status             *gui.Label
	buttons            []*gui.Button
	sourceRunAction    func()
	sourceRunReady     bool
	sourceAction       func()
	sourceTargetAction func(hmi.SourceTarget)
	post               func(func())
	postContext        func(context.Context, func()) error
	workerMu           sync.Mutex
	idle               chan struct{}
}

func init() {
	i18n.AddMany("zh", map[string]string{
		"Process Runtime": "组态运行", "Run snapshot": "运行快照", "Stop runtime": "停止运行", "Live diagram": "实时组态", "Live points": "实时点位", "Go to source": "返回源文件", "Fit diagram": "适应画布",
		"Open the selected item in source": "在源文件中定位选中项",
		"Run latest":                       "运行最新源码", "Run the latest contents of the bound document": "运行已绑定文档的最新内容",
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
	v.Alarms = gui.NewProcessAlarmMonitor()
	v.Alarms.SetParent(v)
	v.showMode(runtimePoints)
	add := func(text, icon string, fn func()) {
		b := gui.NewButton1(i18n.T(text), gui.LoadIcon(icon))
		b.SetParent(v)
		b.SetTextVisible(true)
		b.Action().BindFunc0(fn)
		v.buttons = append(v.buttons, b)
	}
	add("Run snapshot", "run", v.runRequested)
	add("Stop runtime", "stop", v.Stop)
	add("Live diagram", "preview", func() { v.SetPointsMode(false) })
	add("Live points", "tree-view", func() { v.SetPointsMode(true) })
	add("Fit diagram", "maximize", func() { v.Diagram.FitToView() })
	add("Go to source", "document", v.goToSource)
	add("Live alarms", "notifications", v.SetAlarmMode)
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
	count := runtimeAlarmsButton
	if v.hasAlarmRules() {
		count++
	}
	cols := max(1, min(count, int(max(0, w-16)/128)))
	bw := max(0, w-16) / float64(cols)
	rows := (count + cols - 1) / cols
	top := 8 + float64(rows)*34
	for i, b := range v.buttons {
		if i >= count {
			b.SetBounds(0, 0, 0, 0)
			continue
		}
		b.SetBounds(8+float64(i%cols)*bw, 4+float64(i/cols)*34, max(0, bw-4), 30)
	}
	height := max(0, h-top-26)
	v.Diagram.SetBounds(0, top, w, height)
	v.Points.SetBounds(0, top, w, height)
	v.Alarms.SetBounds(0, top, w, height)
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
	if v.mode == runtimeAlarms && !v.hasAlarmRules() {
		v.showMode(runtimePoints)
	}
	v.Diagram.SetInspectionEnabled(v.sourceTargetAction != nil)
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

// SetSourceRunAction opts into reloading the host's bound source from the
// toolbar when no valid snapshot is staged. The host owns readiness and
// validation; invoking it never runs an old snapshot or starts implicitly.
func (v *RuntimeView) SetSourceRunAction(fn func(), ready bool) {
	if v.closed {
		return
	}
	ready = ready && fn != nil
	changed := (fn == nil) != (v.sourceRunAction == nil) || ready != v.sourceRunReady
	v.sourceRunAction, v.sourceRunReady = fn, ready
	if changed && !v.configured {
		v.refreshRunControl()
	}
}

func (v *RuntimeView) canRequestSourceRun() bool {
	return !v.closed && !v.configured && !v.Busy() && v.sourceRunAction != nil && v.sourceRunReady && gui.WidgetTreeVisible(v)
}

func (v *RuntimeView) runRequested() {
	if v.configured {
		v.Start()
		return
	}
	if v.canRequestSourceRun() {
		v.sourceRunAction()
	}
}

// SetSourceTargetAction opts into item navigation. The host must verify its
// source snapshot before applying a coordinate. With no selected entity, the
// original document-level SetSourceAction is used instead.
func (v *RuntimeView) SetSourceTargetAction(fn func(hmi.SourceTarget)) {
	if v.closed {
		return
	}
	v.sourceTargetAction = fn
	v.Diagram.SetInspectionEnabled(fn != nil && v.configured)
	tip := ""
	if fn != nil {
		tip = i18n.T("Open the selected item in source")
	}
	gui.SetToolTip(v.buttons[runtimeSourceButton], tip)
	v.refreshControls()
}

func (v *RuntimeView) goToSource() {
	if v.closed || !gui.WidgetTreeVisible(v) {
		return
	}
	if v.configured && v.sourceTargetAction != nil {
		var target hmi.SourceTarget
		switch v.mode {
		case runtimePoints:
			if name := v.Points.SelectedPoint(); name != "" {
				target = hmi.SourceTarget{Kind: hmi.SourcePoint, ID: name}
			}
		case runtimeAlarms:
			if id := v.Alarms.SelectedAlarm(); id != "" {
				target = hmi.SourceTarget{Kind: hmi.SourceAlarm, ID: id}
			}
		case runtimeDiagram:
			target = v.Diagram.InspectionTarget()
		}
		if target.ID != "" {
			v.sourceTargetAction(target)
			return
		}
	}
	if v.sourceAction != nil {
		v.sourceAction()
	}
}
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
func (v *RuntimeView) refreshRunControl() {
	if len(v.buttons) == 0 {
		return
	}
	run := v.buttons[runtimeRunButton]
	label := "Run snapshot"
	tip := ""
	if !v.configured && v.sourceRunAction != nil {
		label, tip = "Run latest", "Run the latest contents of the bound document"
	}
	if label = i18n.T(label); run.Text() != label {
		run.SetText(label)
		gui.SetToolTip(run, i18n.T(tip))
	}
	run.SetEnabled(v.CanStart() || v.canRequestSourceRun())
}

func (v *RuntimeView) refreshControls() {
	if len(v.buttons) == 0 {
		return
	}
	v.refreshRunControl()
	v.buttons[runtimeStopButton].SetEnabled(!v.closed && v.Busy() && !v.session.stopping)
	for _, i := range []int{runtimeDiagramButton, runtimePointsButton} {
		v.buttons[i].SetEnabled(!v.closed && v.configured)
	}
	v.buttons[runtimeDiagramButton].Action().SetChecked(v.mode == runtimeDiagram)
	v.buttons[runtimePointsButton].Action().SetChecked(v.mode == runtimePoints)
	v.buttons[runtimeFitButton].SetEnabled(!v.closed && v.configured && v.mode == runtimeDiagram)
	v.buttons[runtimeSourceButton].SetEnabled(!v.closed && (v.sourceAction != nil || v.configured && v.sourceTargetAction != nil))
	alarms := v.buttons[runtimeAlarmsButton]
	alarms.SetEnabled(!v.closed && v.hasAlarmRules())
	alarms.Action().SetChecked(v.mode == runtimeAlarms)
	if visible := v.hasAlarmRules(); alarms.IsVisible() != visible {
		alarms.SetVisible(visible)
		v.OnResize()
	}
}

func (v *RuntimeView) hasAlarmRules() bool { return v.configured && len(v.project.Alarms) != 0 }

func (v *RuntimeView) SetPointsMode(points bool) {
	if points {
		v.setMode(runtimePoints)
		return
	}
	v.setMode(runtimeDiagram)
}

// SetAlarmMode displays the alarm monitor for a configured snapshot. It does
// not start acquisition or create a second runtime.
func (v *RuntimeView) SetAlarmMode() {
	if v.hasAlarmRules() {
		v.setMode(runtimeAlarms)
	}
}

func (v *RuntimeView) showMode(mode runtimeViewMode) {
	v.mode = mode
	v.Points.SetVisible(mode == runtimePoints)
	v.Diagram.SetVisible(mode == runtimeDiagram)
	v.Alarms.SetVisible(mode == runtimeAlarms)
}

func (v *RuntimeView) setMode(mode runtimeViewMode) {
	if v.closed || v.mode == mode {
		return
	}
	v.showMode(mode)
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
	v.Alarms.SetRuntime(nil)
	v.Diagram.UnbindTags()
	if rt == nil {
		return nil
	}
	switch v.mode {
	case runtimePoints:
		v.Points.SetRuntime(rt)
		return nil
	case runtimeAlarms:
		v.Alarms.SetRuntime(rt)
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
				_ = v.bindRuntime(nil)
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
	_ = v.bindRuntime(nil)
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
	v.Diagram.SetInspectionEnabled(false)
	v.Stop()
	if v.mode == runtimeAlarms {
		v.showMode(runtimePoints)
	}
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
	v.sourceRunAction, v.sourceRunReady = nil, false
	gui.SetToolTip(v.buttons[runtimeSourceButton], "")
	v.Points.Close()
	v.Alarms.Close()
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
