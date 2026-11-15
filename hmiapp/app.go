// Package hmiapp assembles the opt-in HMI editor/runtime with optional SQLite.
// GUI rendering itself never imports this adapter package.
package hmiapp

import (
	"context"
	"encoding/json"
	"fmt"
	"go/format"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/hmi"
)

type Config struct {
	RequestHeaders func(string) http.Header
	// RuntimeChanged runs on the owning UI thread when a borrowed runtime is
	// ready, and once with nil when it becomes unavailable. Return promptly;
	// cancel driver work here, but never synchronously Join the workbench.
	RuntimeChanged func(*hmi.Runtime)
	ProjectJSON    string
	ProjectPath    string
	DatabasePath   string
}
type session struct {
	cancel              context.CancelFunc
	done                chan struct{}
	runtime             *hmi.Runtime
	available, stopping bool
}
type Workbench struct {
	gui.Form
	ioDone       chan struct{}
	Editor       *gui.ProcessEditor
	Points       *gui.ProcessPointMonitor
	showPoints   bool
	buttons      []*gui.Button
	status       *gui.Label
	cfg          Config
	ctx          context.Context
	cancel       context.CancelFunc
	session      *session
	closed, busy bool
	generation   uint64
}

func New(c Config) (*Workbench, error) {
	f := new(Workbench)
	f.Form.Init(f)
	f.cfg = c
	f.ctx, f.cancel = context.WithCancel(context.Background())
	f.SetTitle("Silk - Industrial Process Designer")
	f.Editor = gui.NewProcessEditor()
	f.Editor.SetParent(f)
	if c.ProjectJSON != "" {
		f.Editor.SetProjectJSON(c.ProjectJSON)
		if e := f.Editor.LastError(); e != nil {
			f.cancel()
			f.Editor.Close()
			return nil, e
		}
	}
	if f.cfg.ProjectPath == "" {
		f.cfg.ProjectPath = filepath.Join(core.LocalDataDir(), "process.silkhmi")
	}
	if f.cfg.DatabasePath == "" {
		f.cfg.DatabasePath = filepath.Join(core.LocalDataDir(), "process-points.db")
	}
	f.Points = gui.NewProcessPointMonitor()
	f.Points.SetParent(f)
	f.Points.SetVisible(false)
	add := func(text string, fn func()) {
		b := gui.NewButton1(text, nil)
		b.SetParent(f)
		b.Action().BindFunc0(fn)
		f.buttons = append(f.buttons, b)
	}
	add("Run", f.StartRuntime)
	add("Stop", f.StopRuntime)
	add("Save", f.Save)
	add("Load", f.Load)
	add("Generate Go", f.ExportGo)
	add("Fit", f.Fit)
	add("Snapshot", f.Snapshot)
	add("Live points", f.TogglePoints)
	f.status = gui.NewLabel("Editor · no I/O until Run / Save / Load · blue ports connect pipes")
	f.status.SetParent(f)
	f.refreshControls()
	return f, nil
}
func (f *Workbench) OnResize() {
	w, h := f.Size()
	columns := max(1, int(max(0, w-16)/150))
	bw := (w - 16) / float64(columns)
	rows := (len(f.buttons) + columns - 1) / columns
	for i, b := range f.buttons {
		b.SetBounds(8+float64(i%columns)*bw, 8+float64(i/columns)*36, max(0, bw-5), 30)
	}
	top := 16 + float64(rows)*36
	available := max(0, h-top-30)
	pointHeight := 0.0
	if f.showPoints {
		pointHeight = min(260, available*0.4)
	}
	f.Editor.SetBounds(0, top, w, max(0, available-pointHeight))
	f.Points.SetBounds(0, top+available-pointHeight, w, pointHeight)
	f.status.SetBounds(8, h-28, max(0, w-16), 24)
	f.Fit()
}
func (f *Workbench) Fit() { f.Editor.Diagram.FitToView() }

// TogglePoints only shows/hides a borrowed runtime view. It does not start I/O.
func (f *Workbench) TogglePoints() {
	if f.closed {
		return
	}
	f.showPoints = !f.showPoints
	f.Points.SetVisible(f.showPoints)
	if f.showPoints {
		f.Points.SetRuntime(f.Runtime())
	} else {
		f.Points.SetRuntime(nil)
	}
	f.OnResize()
}

func (f *Workbench) StartRuntime() {
	if f.closed || f.session != nil || f.busy {
		return
	}
	p := f.Editor.Diagram.Project()
	ctx, cancel := context.WithCancel(f.ctx)
	s := &session{cancel: cancel, done: make(chan struct{})}
	f.session = s
	f.showPoints = true
	f.Points.SetVisible(true)
	f.refreshControls()
	f.OnResize()
	f.status.SetText("Starting shared point store and source workers…")
	go func() {
		var finalErr error
		defer func() { close(s.done); f.finish(s, finalErr) }()
		finalErr = runStoredRuntime(ctx, p, f.cfg.DatabasePath, f.cfg.RequestHeaders, gui.PostContext, func(runtime *hmi.Runtime) {
			if !f.closed && f.session == s && !s.stopping {
				s.runtime = runtime
				if f.showPoints {
					f.Points.SetRuntime(runtime)
				}
				_, e := f.Editor.Diagram.BindTags(runtime.Tags)
				if e != nil {
					f.Points.SetRuntime(nil)
					f.status.SetText(e.Error())
					s.cancel()
				} else {
					f.status.SetText("Running · shared points / bounded HTTP / quality / staleness")
					f.notifyRuntime(s, runtime)
				}
			}
		})
	}()
}
func (f *Workbench) finish(s *session, e error) {
	gui.Post(func() {
		if f.closed || f.session != s {
			return
		}
		f.Editor.Diagram.UnbindTags()
		f.Points.SetRuntime(nil)
		f.session = nil
		f.refreshControls()
		if e != nil {
			f.status.SetText(e.Error())
		} else {
			f.status.SetText("Stopped; authoring unlocked")
		}
		f.notifyRuntime(s, nil)
	})
}
func (f *Workbench) StopRuntime() {
	if f.session != nil && !f.session.stopping {
		f.session.stopping = true
		f.Points.SetRuntime(nil)
		f.session.cancel()
		f.status.SetText("Stopping…")
		f.notifyRuntime(f.session, nil)
	}
}

// Runtime returns the currently ready borrowed runtime, or nil during startup,
// stopping or close. UI-thread only; use RuntimeChanged instead of polling.
func (f *Workbench) Runtime() *hmi.Runtime {
	if f.closed || f.session == nil || f.session.stopping || !f.session.available {
		return nil
	}
	return f.session.runtime
}
func (f *Workbench) notifyRuntime(s *session, rt *hmi.Runtime) {
	if rt != nil {
		if f.closed || f.session != s || s.stopping || s.available {
			return
		}
		s.available = true
	} else {
		if !s.available {
			return
		}
		s.available = false
	}
	if f.cfg.RuntimeChanged != nil {
		f.cfg.RuntimeChanged(rt)
	}
}
func (f *Workbench) ioTask(path string, fn func() error) {
	if f.closed || f.busy || f.session != nil {
		return
	}
	f.busy = true
	f.refreshControls()
	f.status.SetText("Saving / exporting document…")
	done := make(chan struct{})
	f.ioDone = done
	go func() {
		defer close(done)
		e := fn()
		gui.Post(func() {
			f.busy = false
			if !f.closed {
				f.refreshControls()
				if e != nil {
					f.status.SetText(e.Error())
				} else {
					f.status.SetText("Saved: " + path)
				}
			}
		})
	}()
}
func (f *Workbench) Save() {
	p := f.Editor.Diagram.Project()
	f.ioTask(f.cfg.ProjectPath, func() error { return hmi.Save(f.cfg.ProjectPath, p) })
}
func (f *Workbench) Load() {
	if f.closed || f.busy || f.session != nil {
		return
	}
	f.busy = true
	f.refreshControls()
	f.status.SetText("Loading document…")
	done := make(chan struct{})
	f.ioDone = done
	f.generation++
	gen := f.generation
	go func() {
		defer close(done)
		p, e := hmi.Load(f.cfg.ProjectPath)
		gui.Post(func() {
			f.busy = false
			if f.closed || f.generation != gen {
				return
			}
			f.refreshControls()
			if e != nil {
				f.status.SetText(e.Error())
				return
			}
			_ = f.Editor.Diagram.SetProject(p)
			f.Editor.Diagram.ClearHistory()
			f.status.SetText("Loaded: " + f.cfg.ProjectPath)
			f.Fit()
		})
	}()
}

// Generate emits plain Go that opens the same inert project workbench. Running
// the generated binary still requires an explicit Run action before any HTTP.
func Generate(p hmi.Project) ([]byte, error) {
	b, e := p.JSON()
	if e != nil {
		return nil, e
	}
	return format.Source([]byte("package main\nimport (\"github.com/uk0/silk/hmiapp\")\nfunc main(){if err:=hmiapp.Run(hmiapp.Config{ProjectJSON:" + strconv.Quote(string(b)) + "});err!=nil{panic(err)}}\n"))
}
func (f *Workbench) ExportGo() {
	p := f.Editor.Diagram.Project()
	path := f.cfg.ProjectPath + ".go"
	f.ioTask(path, func() error {
		b, e := Generate(p)
		if e != nil {
			return e
		}
		file, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return fmt.Errorf("export refuses overwrite: %w", e)
		}
		_, e = file.Write(b)
		closeErr := file.Close()
		if e != nil {
			return e
		}
		return closeErr
	})
}
func (f *Workbench) Snapshot() {
	p := f.Editor.Diagram.Project()
	mode := "editor"
	errors := map[string]string{}
	samples := map[string]core.Value{}
	if f.session != nil {
		mode = "starting"
		if f.session.runtime != nil {
			mode = "running"
			errors = f.session.runtime.Errors()
			for _, pt := range p.Points {
				tag, _ := f.session.runtime.Tags.Get(pt.Name)
				samples[pt.Name] = tag.Value()
			}
		}
	}
	b, _ := json.Marshal(map[string]any{"mode": mode, "nodes": len(p.Nodes), "links": len(p.Links), "points": len(p.Points), "selected": f.Editor.Diagram.SelectedNode(), "selected_nodes": f.Editor.Diagram.SelectedNodes(), "selected_pipe": f.Editor.Diagram.SelectedLink(), "errors": errors, "samples": samples})
	fmt.Println("HMI_STATE " + string(b))
	f.status.SetText(fmt.Sprintf("%s · %d devices · %d pipes · %d points · %d source errors", mode, len(p.Nodes), len(p.Links), len(p.Points), len(errors)))
}
func (f *Workbench) Close() {
	if f.closed {
		return
	}
	f.closed = true
	f.Points.Close()
	f.refreshControls()
	f.cancel()
	f.StopRuntime()
	f.Editor.Close()
}
func (f *Workbench) Join() {
	_ = f.Points.Wait(context.Background())
	if f.ioDone != nil {
		<-f.ioDone
	}
	if f.session != nil {
		<-f.session.done
	}
}
func Run(c Config) error {
	defer core.Close()
	f, e := New(c)
	if e != nil {
		return e
	}
	defer f.Join()
	defer f.Close()
	if e = f.TryAttachWindow(gui.WtForm); e != nil {
		return e
	}
	_, _, screenW, screenH := gui.DesktopArea()
	f.Window().SetSize(min(1220, max(640, screenW-80)), min(900, max(560, screenH-100)))
	f.Window().MoveToCenter()
	f.Show()
	core.EventLoop()
	return nil
}

// State is reflected in controls rather than silently ignoring enabled buttons.
func (f *Workbench) refreshControls() {
	if len(f.buttons) < 7 {
		return
	}
	idle := !f.closed && !f.busy && f.session == nil
	f.buttons[0].SetEnabled(idle)
	f.buttons[1].SetEnabled(!f.closed && f.session != nil)
	for _, i := range []int{2, 3, 4} {
		f.buttons[i].SetEnabled(idle)
	}
	for _, i := range []int{5, 6, 7} {
		f.buttons[i].SetEnabled(!f.closed)
	}
	f.Editor.SetRunning(f.session != nil)
	f.Editor.SetReadOnly(f.closed || f.busy)
}
