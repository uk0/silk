package gui

import (
	"context"
	"fmt"
	"sync"

	"github.com/uk0/silk/async"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/hmi"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/paint"
)

// ProcessPointMonitor is an inert, reusable live-point view. SetRuntime binds
// memory/subscriptions only; it does not start acquisition or open a database.
// Explicit Write/History actions use bounded background jobs. All methods except
// Wait are UI-thread-only. Detach before stopping/replacing a runtime; Close
// cancels owned work without waiting. The host, not this widget, owns Runtime.
type ProcessPointMonitor struct {
	Widget
	table                                     *Table
	model                                     *processPointModel
	status                                    *Label
	refreshButton, writeButton, historyButton *Button
	selected                                  string
	restoring                                 bool
	connection                                *processPointConnection
	jobs                                      *async.Executor
	post                                      func(func())
	closeDialog                               func()
	dialog                                    *Dialog
	closed                                    bool
	err                                       error
}

type processPointConnection struct {
	runtime     *hmi.Runtime
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.Mutex
	closed      bool
	pending     map[string]hmi.PointState
	unsubscribe core.CancelFunc
	delivery    *async.Latest[struct{}]
}

func init() {
	core.RegisterFactory("gui.ProcessPointMonitor", core.TypeOf((*ProcessPointMonitor)(nil)))
	i18n.AddMany("zh", map[string]string{"Live points": "实时点位", "Refresh values": "刷新点值", "Write value...": "写入点值...", "Recent history...": "最近历史...", "Point": "点位", "Category": "类别", "Type": "类型", "Value": "数值", "Quality": "质量", "Observed": "采样时间", "Writer": "写入来源", "Unit": "单位", "Good": "良好", "Uncertain": "不确定", "Bad": "异常", "Manual / driver": "手动 / 驱动", "No runtime; use Run or SetRuntime explicitly": "未连接运行时，请显式运行或绑定", "Select a point to inspect": "选择点位查看详情"})
}
func NewProcessPointMonitor() *ProcessPointMonitor {
	v := new(ProcessPointMonitor)
	v.Init(v)
	return v
}
func (v *ProcessPointMonitor) Init(self IWidget) {
	v.Widget.Init(self)
	v.jobs = async.NewExecutor(2)
	v.post = Post
	v.model = newProcessPointModel(nil)
	v.table = NewTable()
	v.table.SetParent(self)
	v.table.SetModel(v.model)
	v.table.SetCellsEditable(false)
	v.table.SetClipCellText(true)
	v.table.SetSelectionChangedCallback(func(interface{}, int) { v.selectCurrent() })
	v.table.SigSortChanged(func(int, bool) { v.restoreSelection() })
	add := func(text string, fn func()) *Button {
		b := NewButton1(i18n.T(text), nil)
		b.SetParent(self)
		b.Action().BindFunc0(fn)
		return b
	}
	v.refreshButton = add("Refresh values", v.Refresh)
	v.writeButton = add("Write value...", v.ShowWrite)
	v.historyButton = add("Recent history...", v.ShowHistory)
	v.status = NewLabel("")
	v.status.SetParent(self)
	v.refreshActions()
}
func (v *ProcessPointMonitor) SizeHints() SizeHints {
	return SizeHints{Width: 960, Height: 260, Policy: GrowHorizontal | GrowVertical}
}
func (v *ProcessPointMonitor) OnResize() {
	w, h := v.Size()
	bw := max(0, min(168, (w-24)/3))
	for i, b := range []*Button{v.refreshButton, v.writeButton, v.historyButton} {
		b.SetBounds(8+float64(i)*(bw+4), 6, bw, 30)
	}
	v.table.SetBounds(8, 42, max(0, w-16), max(0, h-72))
	v.status.SetBounds(24, max(42, h-27), max(0, w-32), 22)
}
func (v *ProcessPointMonitor) Draw(g paint.Painter) {
	g.SetBrush1(Theme().FormColor)
	g.Rectangle(0, 0, v.Width(), v.Height())
	g.Fill()
	if v.Height() < 68 {
		return
	}
	c := paint.Color{R: 100, G: 116, B: 139, A: 255}
	if s, ok := v.selectedState(); ok && v.connection != nil {
		dark := CurrentThemeMode() == ThemeDark
		switch s.Value.Quality {
		case core.QualityGood:
			if dark {
				c = paint.Color{R: 74, G: 222, B: 128, A: 255}
			} else {
				c = paint.Color{R: 21, G: 128, B: 61, A: 255}
			}
		case core.QualityUncertain:
			if dark {
				c = paint.Color{R: 250, G: 204, B: 21, A: 255}
			} else {
				c = paint.Color{R: 180, G: 83, B: 9, A: 255}
			}
		case core.QualityBad:
			if dark {
				c = paint.Color{R: 248, G: 113, B: 113, A: 255}
			} else {
				c = paint.Color{R: 185, G: 28, B: 28, A: 255}
			}
		}
	}
	g.SetBrush1(c)
	g.Rectangle(9, v.Height()-19, 8, 8)
	g.Fill()
}

func (v *ProcessPointMonitor) SetRuntime(rt *hmi.Runtime) {
	if v.closed && rt != nil {
		return
	}
	if v.connection != nil && v.connection.runtime == rt {
		v.Refresh()
		return
	}
	old := v.connection
	v.connection = nil
	if closer := v.closeDialog; closer != nil {
		v.closeDialog = nil
		v.dialog = nil
		closer()
	}
	if old != nil {
		old.close()
	}
	v.selected = ""
	v.err = nil
	var states []hmi.PointState
	if rt != nil {
		states = rt.PointStates()
	}
	v.model = newProcessPointModel(states)
	v.table.SetModel(v.model)
	v.table.SetSelectedRow(-1)
	if rt != nil && !v.closed {
		ctx, cancel := context.WithCancel(context.Background())
		c := &processPointConnection{runtime: rt, ctx: ctx, cancel: cancel, pending: make(map[string]hmi.PointState)}
		c.delivery, _ = async.NewLatest(v.post, func(struct{}) { v.flush(c) })
		v.connection = c
		cancelSubscription := rt.SubscribePoints(c.publish)
		c.mu.Lock()
		alreadyClosed := c.closed
		if !alreadyClosed {
			c.unsubscribe = cancelSubscription
		}
		c.mu.Unlock()
		if alreadyClosed {
			cancelSubscription()
		}
		if len(states) > 0 {
			v.SelectPoint(states[0].Definition.Name)
		}
	}
	v.refreshActions()
	v.Update()
}
func (c *processPointConnection) publish(s hmi.PointState) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	if old, ok := c.pending[s.Definition.Name]; !ok || s.Revision >= old.Revision {
		c.pending[s.Definition.Name] = s
	}
	c.mu.Unlock()
	c.delivery.Publish(struct{}{})
}
func (c *processPointConnection) close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	c.pending = nil
	cancel := c.unsubscribe
	c.unsubscribe = nil
	c.mu.Unlock()
	c.cancel()
	if cancel != nil {
		cancel()
	}
	c.delivery.Close()
}
func (v *ProcessPointMonitor) flush(c *processPointConnection) {
	if v.closed || v.connection != c {
		return
	}
	c.mu.Lock()
	pending := c.pending
	c.pending = make(map[string]hmi.PointState)
	c.mu.Unlock()
	previous, hadSelection := v.selectedState()
	changed := false
	for _, s := range pending {
		changed = v.model.update(s) || changed
	}
	if changed {
		v.model.resort()
		v.restoreSelection()
		v.table.Update()
		if current, ok := v.selectedState(); ok && (!hadSelection || current.Value.Quality != previous.Value.Quality) {
			// The quality marker belongs to the composite, outside the table and
			// label dirty rectangles. Invalidate it when its quality changes.
			v.Update()
		}
	}
	v.refreshActions()
}

// Refresh reads the latest memory, including silent acquisition heartbeats.
func (v *ProcessPointMonitor) Refresh() {
	if v.closed {
		return
	}
	v.err = nil
	if c := v.connection; c != nil {
		for _, s := range c.runtime.PointStates() {
			v.model.update(s)
		}
		v.model.resort()
		v.restoreSelection()
		v.table.Update()
		v.Update()
	}
	v.refreshActions()
}
func (v *ProcessPointMonitor) SelectedPoint() string { return v.selected }
func (v *ProcessPointMonitor) SelectPoint(name string) bool {
	for i, row := range v.model.rows {
		if row.state.Definition.Name == name {
			v.table.SetSelectedRow(i)
			v.selectCurrent()
			return true
		}
	}
	return false
}

// PointStates returns an owned view in current display order, not mutable rows.
func (v *ProcessPointMonitor) PointStates() []hmi.PointState {
	out := make([]hmi.PointState, len(v.model.rows))
	for i, row := range v.model.rows {
		out[i] = row.state
	}
	return out
}
func (v *ProcessPointMonitor) LastError() error   { return v.err }
func (v *ProcessPointMonitor) setError(err error) { v.err = err; v.refreshActions() }
func (v *ProcessPointMonitor) selectedState() (hmi.PointState, bool) {
	row, ok := v.model.byName[v.selected]
	if !ok {
		return hmi.PointState{}, false
	}
	return row.state, true
}
func (v *ProcessPointMonitor) selectCurrent() {
	if !v.restoring {
		v.err = nil
	}
	v.selected = ""
	i := v.table.SelectedRow()
	if i >= 0 && i < len(v.model.rows) {
		v.selected = v.model.rows[i].state.Definition.Name
	}
	v.refreshActions()
	v.Update()
}
func (v *ProcessPointMonitor) restoreSelection() {
	v.restoring = true
	defer func() { v.restoring = false }()
	name := v.selected
	for i, row := range v.model.rows {
		if row.state.Definition.Name == name {
			v.table.SetSelectedRow(i)
			v.selected = name
			return
		}
	}
	v.table.SetSelectedRow(-1)
	v.selected = ""
}
func (v *ProcessPointMonitor) refreshActions() {
	if v.refreshButton == nil {
		return
	}
	c := v.connection
	active := !v.closed && c != nil
	s, selected := v.selectedState()
	v.refreshButton.SetEnabled(active)
	v.writeButton.SetEnabled(active && selected && s.Writable)
	v.historyButton.SetEnabled(active && selected && c.runtime.SupportsHistory())
	text := i18n.T("No runtime; use Run or SetRuntime explicitly")
	if active {
		if selected {
			text = fmt.Sprintf("%s · %s · %s · %s", s.Definition.Name, s.Definition.Kind, processPointQuality(s.Value.Quality), processPointWriter(s))
		} else {
			text = i18n.T("Select a point to inspect")
		}
	}
	if v.err != nil {
		text = v.err.Error()
	}
	v.status.SetText(text)
}
func (v *ProcessPointMonitor) Close() {
	if v.closed {
		return
	}
	v.closed = true
	v.SetRuntime(nil)
	v.jobs.Close()
	v.refreshActions()
}

// Wait belongs after the event loop or on a worker, never in a UI callback.
func (v *ProcessPointMonitor) Wait(ctx context.Context) error { return v.jobs.Wait(ctx) }
