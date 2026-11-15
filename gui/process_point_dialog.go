package gui

import (
	"context"
	"fmt"
	"time"

	"github.com/uk0/silk/async"
	"github.com/uk0/silk/hmi"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/paint"
	"github.com/uk0/silk/point"
)

var showProcessPointDialog = (*Dialog).ShowModal

type processPointDialogBody struct {
	Widget
	summary *Label
	input   *Edit
	status  *Label
	table   *processConfigTable
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
		b.table.SetBounds(8, 66, max(0, w-16), max(0, h-108))
		b.status.SetBounds(8, max(66, h-36), max(0, w-16), 30)
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
	body.summary.SetText(name + " · newest observations first · at most 200 rows\nDatabase query starts only from this explicit action.")
	model := NewSimpleTableModel([]string{"Observed (UTC)", "Value", "Quality"})
	model.SetColumnWidth(0, 220)
	model.SetColumnWidth(1, 260)
	model.SetColumnWidth(2, 120)
	body.table = new(processConfigTable)
	body.table.Init(body.table)
	body.table.SetParent(body)
	body.table.SetModel(model)
	body.table.SetCellsEditable(false)
	body.status.SetText("Reading bounded recent history…")
	dlg := NewDialog(i18n.T("Recent history..."), v)
	dlg.SetContent(body)
	dlg.AddButton(i18n.T("Close"), DialogCancel)
	closed := false
	v.closeDialog = func() { dlg.onBtnClick(dialogResultKey(DialogCancel)) }
	v.dialog = dlg
	ctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
	var future *async.Future[[]point.SampleRecord]
	defer func() {
		closed = true
		if v.dialog == dlg {
			v.dialog = nil
			v.closeDialog = nil
		}
		cancel()
		if future != nil {
			future.Cancel()
		}
	}()
	future, err := async.Submit(ctx, v.jobs, func(ctx context.Context) ([]point.SampleRecord, error) {
		return c.runtime.RecentHistory(ctx, name, 200)
	})
	if err != nil {
		body.status.SetText(err.Error())
	} else {
		_ = future.OnComplete(v.post, func(rows []point.SampleRecord, err error) {
			if closed || v.closed || v.connection != c {
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
			body.status.SetText(fmt.Sprintf("%d observations · live values remain independent", len(rows)))
		})
	}
	dlg.SetSize(740, 460)
	dlg.SetInitialFocus(body.table)
	_ = showProcessPointDialog(dlg)
}
