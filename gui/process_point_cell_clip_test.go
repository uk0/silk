package gui

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/uk0/silk/paint"
)

func assertMonitorValueCellClips(t *testing.T, table *Table) {
	t.Helper()
	const w, h = 374, 92
	const textRight = 160
	table.SetSize(w, h)
	qualityInk := func(value string) []byte {
		model := NewSimpleTableModel([]string{"Value", "Quality"})
		model.SetColumnWidth(0, textRight)
		model.SetColumnWidth(1, 180)
		model.AddRow(value, "Good")
		table.SetModel(model)
		pm := paint.NewPixmap(w, h)
		defer paint.CloseSurface(pm)
		p := pm.NewPainter()
		table.Draw(p)
		paint.ClosePainter(p)
		im, err := pm.Image()
		if err != nil {
			t.Fatal(err)
		}
		out := make([]byte, 0, 170*h*4)
		for y := 0; y < h; y++ {
			for x := textRight; x < 330; x++ {
				r, g, b, a := im.At(x, y).RGBA()
				out = append(out, byte(r>>8), byte(g>>8), byte(b>>8), byte(a>>8))
			}
		}
		return out
	}
	want := qualityInk("")
	got := qualityInk(strings.Repeat("Reactor feedback 1234567890 ", 24))
	if !bytes.Equal(want, got) {
		t.Error("long point value painted into Quality")
	}
}

func TestProcessPointTablesKeepLongValueOutOfQuality(t *testing.T) {
	t.Run("live monitor", func(t *testing.T) {
		v := NewProcessPointMonitor()
		defer func() { v.Close(); _ = v.Wait(context.Background()) }()
		assertMonitorValueCellClips(t, v.table)
	})
	t.Run("history dialog", func(t *testing.T) {
		orig := showProcessPointDialog
		t.Cleanup(func() { showProcessPointDialog = orig })
		v, _, posts := newMonitorTest(t, new(pointMonitorStore))
		drainPointPosts(posts)
		v.SelectPoint("BI11")
		showProcessPointDialog = func(d *Dialog) DialogResult {
			body := d.Content().(*processPointDialogBody)
			assertMonitorValueCellClips(t, &body.table.Table)
			return DialogCancel
		}
		v.ShowHistory()
	})
}
