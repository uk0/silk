package gui

import (
	"fmt"
	"strconv"

	"github.com/uk0/silk/async"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/hmi"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/paint"
)

// ProcessAlarmMonitor is a UI-thread-owned, inert view of a borrowed HMI
// runtime. SetRuntime never starts acquisition. Closing detaches callbacks
// without joining the runtime or persisting acknowledgment to the point store.
type ProcessAlarmMonitor struct {
	Widget
	runtime      *hmi.Runtime
	cancel       core.CancelFunc
	latest       *async.Latest[struct{}]
	table        *Table
	model        *SimpleTableModel
	ack          *Button
	toggle       *Button
	status       *Label
	states       map[string]hmi.AlarmState
	history      bool
	shownHistory bool
	closed       bool
}

func init() {
	i18n.AddMany("zh", map[string]string{
		"Live alarms": "实时报警", "Alarm history": "报警历史", "Live rules": "实时规则",
		"Acknowledge alarm": "确认报警", "No configured alarms": "没有配置报警规则",
		"No runtime; use Run or SetRuntime explicitly": "未连接运行时，请显式运行或绑定",
		"Alarm state": "报警状态", "Normal": "正常", "Active alarm": "活动报警",
		"Active / acknowledged": "活动 / 已确认", "Returned / awaiting acknowledgment": "已复归 / 待确认",
		"Returned / awaiting confirmed recovery": "已复归 / 等待确认恢复",
		"Event":                                  "事件", "At (UTC)": "时间 (UTC)", "raised": "触发", "returned": "复归",
		"acknowledged": "确认", "cleared": "清除", "info": "提示", "warning": "警告", "critical": "严重",
	})
}

func NewProcessAlarmMonitor() *ProcessAlarmMonitor {
	return newProcessAlarmMonitor(Post)
}

func newProcessAlarmMonitor(post func(func())) *ProcessAlarmMonitor {
	v := new(ProcessAlarmMonitor)
	v.Widget.Init(v)
	v.latest, _ = async.NewLatest(post, func(struct{}) { v.refresh() })
	v.table = NewTable()
	v.table.SetParent(v)
	v.table.SetCellsEditable(false)
	v.table.SetClipCellText(true)
	v.table.SetSelectionChangedCallback(func(interface{}, int) { v.syncAck() })
	v.table.SigSortChanged(func(int, bool) {
		// The table's selected index stays fixed across reorders. Never
		// transfer that operator selection to the alarm newly at that index.
		v.table.SetSelectedRow(-1)
		v.syncAck()
	})
	v.ack = NewButton1(i18n.T("Acknowledge alarm"), nil)
	v.ack.SetParent(v)
	v.ack.Action().BindFunc0(v.acknowledge)
	v.toggle = NewButton1(i18n.T("Alarm history"), nil)
	v.toggle.SetParent(v)
	v.toggle.Action().BindFunc0(func() {
		if v.runtime != nil && !v.closed {
			v.history = !v.history
			v.refresh()
		}
	})
	v.status = NewLabel("")
	v.status.SetParent(v)
	v.refresh()
	return v
}

func (v *ProcessAlarmMonitor) SizeHints() SizeHints {
	return SizeHints{Width: 720, Height: 220, Policy: GrowHorizontal | GrowVertical}
}

func (v *ProcessAlarmMonitor) Draw(g paint.Painter) {
	g.SetBrush1(Theme().FormColor)
	g.Rectangle(0, 0, v.Width(), v.Height())
	g.Fill()
}

func (v *ProcessAlarmMonitor) OnResize() {
	w, h := v.Size()
	bw := min(190, max(0, (w-20)/2))
	v.ack.SetBounds(8, 6, bw, 30)
	v.toggle.SetBounds(12+bw, 6, bw, 30)
	v.table.SetBounds(8, 42, max(0, w-16), max(0, h-70))
	v.status.SetBounds(8, max(42, h-26), max(0, w-16), 22)
}

// SetRuntime subscribes to lifecycle/quality changes; analog-only changes do
// not add redraws. Older callbacks can only request a refresh of the current
// owner, never deliver stale state or a stale acknowledgment.
func (v *ProcessAlarmMonitor) SetRuntime(rt *hmi.Runtime) {
	if v.closed || rt == v.runtime {
		return
	}
	v.detach()
	v.history = false
	v.table.SetSelectedRow(-1) // a replacement runtime never inherits an ACK target
	v.runtime = rt
	if rt != nil {
		latest := v.latest
		v.cancel = rt.SubscribeAlarmChanges(func() { latest.Publish(struct{}{}) })
	}
	v.refresh()
}

func (v *ProcessAlarmMonitor) detach() {
	if fn := v.cancel; fn != nil {
		v.cancel = nil
		fn()
	}
	v.runtime = nil
}

func (v *ProcessAlarmMonitor) Close() {
	if v.closed {
		return
	}
	v.closed = true
	v.detach()
	v.latest.Close()
	v.refresh()
}

func processAlarmState(s hmi.AlarmState) string {
	switch {
	case !s.Active:
		return i18n.T("Normal")
	case !s.ConditionActive && !s.Acknowledged:
		return i18n.T("Returned / awaiting acknowledgment")
	case !s.ConditionActive:
		return i18n.T("Returned / awaiting confirmed recovery")
	case s.Acknowledged:
		return i18n.T("Active / acknowledged")
	default:
		return i18n.T("Active alarm")
	}
}

func (v *ProcessAlarmMonitor) selectedID() string {
	if v.model != nil {
		return v.model.CellText(v.table.SelectedRow(), 0)
	}
	return ""
}

// History rows carry the runtime's sequence after the visible columns so
// the identity travels with the cells when SimpleTableModel sorts them.
func historyAlarmKey(m *SimpleTableModel, row int) string {
	if m == nil {
		return ""
	}
	return m.CellText(row, m.ColumnCount())
}

// SelectedAlarm returns the authored rule ID of the selected live or history
// row. A stopped, detached, or closed monitor has no navigable selection.
func (v *ProcessAlarmMonitor) SelectedAlarm() string {
	if id := v.selectedID(); v.runtime != nil && !v.closed {
		if _, ok := v.states[id]; ok {
			return id
		}
	}
	return ""
}

// SelectAlarm selects a rule in the current live list without acknowledging it.
func (v *ProcessAlarmMonitor) SelectAlarm(id string) bool {
	if v.runtime == nil || v.closed || v.history || id == "" {
		return false
	}
	for i := 0; i < v.model.RowCount(); i++ {
		if v.model.CellText(i, 0) == id {
			v.table.SetSelectedRow(i)
			return true
		}
	}
	return false
}

func (v *ProcessAlarmMonitor) refresh() {
	var selected, selectedEvent string
	if v.history && v.shownHistory {
		selectedEvent = historyAlarmKey(v.model, v.table.SelectedRow())
	} else if !v.history && !v.shownHistory {
		selected = v.selectedID()
	}
	selectedToken := v.states[selected].Token
	v.states = nil
	cols := []string{"Alarm ID", "Point", "Alarm priority", "Alarm state", "Quality", "Value"}
	if v.history {
		cols = []string{"Alarm ID", "Point", "Event", "Alarm priority", "Quality", "Value", "At (UTC)"}
	}
	for i, c := range cols {
		cols[i] = i18n.T(c)
	}
	model := NewSimpleTableModel(cols)
	for i, width := range []float64{130, 110, 120, 290, 100, 165, 190} {
		if i < len(cols) {
			model.SetColumnWidth(i, width)
		}
	}
	status := i18n.T("No runtime; use Run or SetRuntime explicitly")
	if rt := v.runtime; rt != nil {
		states := rt.AlarmStates()
		v.states = make(map[string]hmi.AlarmState, len(states))
		active := 0
		for _, s := range states {
			if s.Active {
				active++
			}
			v.states[s.ID] = s
			if !v.history {
				model.AddRow(s.ID, s.Point, i18n.T(string(s.Priority)), processAlarmState(s), processPointQuality(s.Quality), processPointDisplay(s.Value))
			}
		}
		status = i18n.T("No configured alarms")
		if len(states) != 0 {
			status = fmt.Sprintf("%s: %d / %d", i18n.T("Live alarms"), active, len(states))
		}
		if v.history {
			events := rt.AlarmEvents(0)
			for i := len(events) - 1; i >= 0; i-- {
				e := events[i]
				s := e.State
				model.rows = append(model.rows, []string{
					s.ID, s.Point, i18n.T(string(e.Kind)), i18n.T(string(s.Priority)),
					processPointQuality(s.Quality), processPointDisplay(s.Value),
					e.At.UTC().Format("2006-01-02 15:04:05.000"), strconv.FormatUint(e.Sequence, 10),
				})
			}
			status = fmt.Sprintf("%s: %d / %d", i18n.T("Alarm history"), len(events), hmi.AlarmHistoryLimit)
		}
	}
	if v.model == nil || v.shownHistory != v.history {
		v.shownHistory = v.history
		v.model = model
		v.table.SetModel(model)
	} else {
		// Keep user column sizes and sorting across burst-coalesced updates.
		v.model.rows, v.model.original = model.rows, nil
		if v.table.sortColumn >= 0 {
			v.model.SortByColumn(v.table.sortColumn, v.table.sortAscending)
		}
		v.table.updateScroll()
	}
	row := -1
	if v.history && selectedEvent != "" {
		for i := 0; i < v.model.RowCount(); i++ {
			if historyAlarmKey(v.model, i) == selectedEvent {
				row = i
				break
			}
		}
	} else if !v.history && selected != "" && v.states[selected].Token == selectedToken {
		for i := 0; i < v.model.RowCount(); i++ {
			if v.model.CellText(i, 0) == selected {
				row = i
				break
			}
		}
	}
	v.table.SetSelectedRow(row)
	v.toggle.SetEnabled(v.runtime != nil && !v.closed)
	toggle := "Alarm history"
	if v.history {
		toggle = "Live rules"
	}
	v.toggle.SetText(i18n.T(toggle))
	v.status.SetText(status)
	v.syncAck()
}

func (v *ProcessAlarmMonitor) syncAck() {
	s := v.states[v.selectedID()]
	v.ack.SetEnabled(!v.closed && v.runtime != nil && !v.history && s.Active && !s.Acknowledged)
}

func (v *ProcessAlarmMonitor) acknowledge() {
	if v.closed || v.runtime == nil || v.history {
		return
	}
	s := v.states[v.selectedID()]
	if !s.Active || s.Acknowledged {
		return
	}
	err := v.runtime.AcknowledgeAlarm(s.ID, s.Token)
	v.refresh()
	if err != nil {
		v.status.SetText(err.Error())
	}
}
