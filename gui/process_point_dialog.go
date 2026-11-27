package gui

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/uk0/silk/async"
	"github.com/uk0/silk/hmi"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/paint"
	"github.com/uk0/silk/point"
)

var showProcessPointDialog = (*Dialog).ShowModal

func init() {
	i18n.AddMany("zh", map[string]string{
		"Newest observations first. Select a row limit, then read history.": "按采样时间从新到旧。选择行数上限，再读取历史。",
		" rows":                            " 行",
		"Read history":                     "读取历史",
		"Enter a row limit from 1 to 200.": "请输入 1 至 200 的行数上限。",
		"Reading recent history…":          "正在读取最近历史…",
		"History exceeds 16 MiB. Lower the row limit and read again.": "历史数据超过 16 MiB。请减少行数后重新读取。",
		"%d observations · at most %d requested":                      "%d 条观测 · 本次最多读取 %d 条",
	})
}

type processPointDialogBody struct {
	Widget
	summary *Label
	input   *Edit
	status  *Label
	table   *processConfigTable
	limit   *SpinBox
	read    *Button
}

func newProcessPointDialogBody() *processPointDialogBody {
	b := new(processPointDialogBody)
	b.Widget.Init(b)
	b.summary = NewLabel("")
	b.summary.SetWrap(true)
	b.summary.SetParent(b)
	b.status = NewLabel("")
	b.status.SetWrap(true)
	b.status.SetParent(b)
	return b
}
func (b *processPointDialogBody) SizeHints() SizeHints {
	return SizeHints{Width: 680, Height: 240, Policy: GrowHorizontal | GrowVertical}
}
func (b *processPointDialogBody) Draw(g paint.Painter) {
	g.SetBrush1(Theme().FormColor)
	g.Rectangle(0, 0, b.Width(), b.Height())
	g.Fill()
}
func (b *processPointDialogBody) OnResize() {
	w, h := b.Size()
	b.summary.SetBounds(8, 8, max(0, w-16), 52)
	if b.input != nil {
		b.input.SetBounds(8, 68, max(0, w-16), 32)
		b.status.SetBounds(8, 108, max(0, w-16), max(0, h-116))
	} else if b.table != nil {
		width := max(0, w-16)
		y := 66.
		if b.limit != nil && b.read != nil {
			gap := min(8., width)
			inputWidth := min(120., (width-gap)/2)
			b.limit.SetBounds(8, y, inputWidth, 32)
			b.read.SetBounds(8+inputWidth+gap, y, min(160., width-inputWidth-gap), 32)
			y += 40
		}
		bottom := max(y, h-44)
		b.table.SetBounds(8, y, width, max(0, bottom-y-4))
		b.status.SetBounds(8, bottom, width, max(0, h-bottom-6))
	}
}

func (v *ProcessPointMonitor) ShowWrite() {
	if v.closed || v.connection == nil || v.closeDialog != nil {
		return
	}
	c := v.connection
	s, err := c.runtime.PointState(v.selected)
	if err != nil {
		v.setError(err)
		return
	}
	if !s.Writable {
		v.setError(hmi.ErrPointReadOnly)
		return
	}
	text, err := processPointInlineText(s)
	if err != nil {
		v.setError(err)
		return
	}
	v.model.update(s)
	v.model.resort()
	v.restoreSelection()
	v.err = nil
	body := newProcessPointDialogBody()
	body.input = NewEdit()
	body.input.SetParent(body)
	body.input.SetText(text)
	body.summary.SetText(fmt.Sprintf("%s · %s · %s · revision %d\nExplicit manual/driver write. HTTP-owned points cannot be written.", s.Definition.Name, s.Definition.Kind, processPointQuality(s.Value.Quality), s.Revision))
	body.status.SetText("Cancel makes no write. A changed sample rejects this baseline; reopen to review it.")
	dlg := NewDialog(i18n.T("Write value..."), v)
	dlg.SetContent(body)
	dlg.AddButton(i18n.T("Cancel"), DialogCancel)
	write := dlg.AddButton(i18n.T("Write value..."), DialogOK)
	closed, attempted := false, false
	var future *async.Future[hmi.PointState]
	v.closeDialog = func() { dlg.onBtnClick(dialogResultKey(DialogCancel)) }
	v.dialog = dlg
	defer func() {
		closed = true
		if future != nil {
			future.Cancel()
		}
		if v.dialog == dlg {
			v.dialog = nil
			v.closeDialog = nil
		}
	}()
	write.Action().BindFunc0(func() {
		if closed || attempted || v.connection != c || c.ctx.Err() != nil {
			return
		}
		input := body.input.Text()
		if len(input) > 8192 {
			body.status.SetText("Inline input exceeds 8 KiB; value was not written.")
			return
		}
		raw, err := point.Convert(input, s.Definition.Kind)
		if err != nil {
			body.status.SetText(err.Error())
			return
		}
		future, err = async.Submit(c.ctx, v.jobs, func(ctx context.Context) (hmi.PointState, error) {
			return c.runtime.WritePoint(ctx, s.Definition.Name, s.Token, raw)
		})
		if err != nil {
			body.status.SetText(err.Error())
			return
		}
		attempted = true
		write.SetEnabled(false)
		body.input.SetEnabled(false)
		body.status.SetText("Writing… no automatic retry.")
		_ = future.OnComplete(v.post, func(result hmi.PointState, err error) {
			if closed || v.closed || v.connection != c {
				return
			}
			if err != nil {
				body.status.SetText("Write failed or was cancelled. Verify value/history before retry.\n" + err.Error())
				v.setError(err)
				return
			}
			v.model.update(result)
			v.model.resort()
			v.restoreSelection()
			v.table.Update()
			v.Update()
			v.setError(nil)
			dlg.onBtnClick(dialogResultKey(DialogOK))
		})
	})
	dlg.SetSize(700, 320)
	dlg.SetInitialFocus(body.input)
	_ = showProcessPointDialog(dlg)
}

func (v *ProcessPointMonitor) ShowHistory() {
	if v.closed || v.connection == nil || v.closeDialog != nil {
		return
	}
	c := v.connection
	name := v.selected
	if _, err := c.runtime.PointState(name); err != nil {
		v.setError(err)
		return
	}
	if !c.runtime.SupportsHistory() {
		v.setError(hmi.ErrHistoryUnavailable)
		return
	}
	body := newProcessPointDialogBody()
	body.summary.SetText(name + "\n" + i18n.T("Newest observations first. Select a row limit, then read history."))
	body.table = new(processConfigTable)
	body.table.Init(body.table)
	body.table.SetParent(body)
	body.table.SetCellsEditable(false)
	body.table.SetClipCellText(true)
	newModel := func() *SimpleTableModel {
		m := NewSimpleTableModel([]string{i18n.T("Observed") + " (UTC)", i18n.T("Value"), i18n.T("Quality")})
		m.SetColumnWidth(0, 220)
		m.SetColumnWidth(1, 260)
		m.SetColumnWidth(2, 120)
		return m
	}
	body.table.SetModel(newModel())
	body.limit = NewSpinBox()
	body.limit.SetRange(1, 200)
	body.limit.SetValue(200)
	body.limit.SetSuffix(i18n.T(" rows"))
	body.limit.SetParent(body)
	body.read = NewButton1(i18n.T("Read history"), nil)
	body.read.SetParent(body)

	dlg := NewDialog(i18n.T("Recent history..."), v)
	dlg.SetContent(body)
	dlg.AddButton(i18n.T("Close"), DialogCancel)
	closed := false
	v.closeDialog = func() { dlg.onBtnClick(dialogResultKey(DialogCancel)) }
	v.dialog = dlg
	var future *async.Future[[]point.SampleRecord]
	var cancelRead context.CancelFunc
	defer func() {
		closed = true
		if v.dialog == dlg {
			v.dialog = nil
			v.closeDialog = nil
		}
		if cancelRead != nil {
			cancelRead()
		}
		if future != nil {
			future.Cancel()
		}
	}()

	setControls := func(enabled bool) {
		body.limit.SetEnabled(enabled)
		body.read.SetEnabled(enabled)
	}
	load := func() {
		if closed || v.closed || v.connection != c || future != nil || c.ctx.Err() != nil {
			return
		}
		if !body.limit.Commit() {
			body.status.SetText(i18n.T("Enter a row limit from 1 to 200."))
			return
		}
		limit := body.limit.Value()
		model := newModel()
		body.table.SetModel(model)
		ctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
		job, err := async.Submit(ctx, v.jobs, func(ctx context.Context) ([]point.SampleRecord, error) {
			return c.runtime.RecentHistory(ctx, name, limit)
		})
		if err != nil {
			cancel()
			body.status.SetText(err.Error())
			return
		}
		future, cancelRead = job, cancel
		setControls(false)
		body.status.SetText(i18n.T("Reading recent history…"))
		err = job.OnComplete(v.post, func(rows []point.SampleRecord, err error) {
			if closed || v.closed || v.connection != c || future != job {
				return
			}
			future, cancelRead = nil, nil
			cancel()
			setControls(true)
			if errors.Is(err, point.ErrHistoryPayloadLimit) {
				body.status.SetText(i18n.T("History exceeds 16 MiB. Lower the row limit and read again."))
				return
			}
			if err != nil {
				body.status.SetText(err.Error())
				return
			}
			for _, row := range rows {
				value := processPointDisplay(row.Value.Raw)
				model.AddRow(row.Value.Time.UTC().Format("2006-01-02 15:04:05.000Z"), value, processPointQuality(row.Value.Quality))
			}
			body.table.SetModel(model)
			body.table.Update()
			body.status.SetText(fmt.Sprintf(i18n.T("%d observations · at most %d requested"), len(rows), limit))
		})
		if err != nil {
			cancel()
			job.Cancel()
			future, cancelRead = nil, nil
			setControls(true)
			body.status.SetText(err.Error())
		}
	}
	body.read.Action().BindFunc0(load)
	dlg.SetSize(740, 480)
	dlg.SetInitialFocus(body.table)
	load()
	_ = showProcessPointDialog(dlg)
}
