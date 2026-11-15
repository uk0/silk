package gui

import (
	"errors"
	"testing"

	"github.com/uk0/silk/hmi"
	"github.com/uk0/silk/paint"
)

func TestProcessEditorCompositePaintsThemeBackground(t *testing.T) {
	old := CurrentThemeMode()
	t.Cleanup(func() { SetThemeMode(old) })
	for _, mode := range []ThemeMode{ThemeLight, ThemeDark} {
		SetThemeMode(mode)
		for _, modal := range []bool{false, true} {
			var editor IWidget
			if modal {
				e := new(processDocumentEditor)
				e.ProcessEditor.Init(e)
				defer e.Close()
				editor = e
			} else {
				e := NewProcessEditor()
				defer e.Close()
				editor = e
			}
			editor.SetSize(96, 80)
			pm := paint.NewPixmap(104, 88)
			g := pm.NewPainter()
			editor.Draw(g)
			paint.ClosePainter(g)
			im, err := pm.Image()
			if err != nil {
				t.Fatal(err)
			}
			want := Theme().FormColor
			for _, xy := range [][2]int{{0, 0}, {48, 40}, {95, 79}} {
				r, green, b, a := im.At(xy[0], xy[1]).RGBA()
				if uint8(r>>8) != want.R || uint8(green>>8) != want.G || uint8(b>>8) != want.B || uint8(a>>8) != want.A {
					t.Fatal("composite leaks placeholder/transparent background", mode, modal, xy, r, green, b, a, want)
				}
			}
			_, _, _, a := im.At(100, 84).RGBA()
			if a != 0 {
				t.Fatal("background escaped widget bounds")
			}
			paint.CloseSurface(pm)
		}
	}
}

func TestProcessDocumentModalIsStagedAndCommitsOnlyAuthoredChanges(t *testing.T) {
	old := showProcessDocumentDialog
	t.Cleanup(func() { showProcessDocumentDialog = old })
	p := hmi.Template()
	before, _ := p.JSON()
	commits := 0
	showProcessDocumentDialog = func(d *Dialog) DialogResult {
		e := d.Content().(*processDocumentEditor)
		e.SetSize(1060, 680)
		if e.Diagram.Height() <= 0 || e.Diagram.Zoom() >= 1 {
			t.Fatal("document not fitted to modal viewport")
		}
		e.Diagram.SelectNodes([]string{"reactor", "exchanger"})
		if err := e.Diagram.NudgeSelection(20, 10); err != nil {
			t.Fatal(err)
		}
		return DialogCancel
	}
	if err := ShowProcessDocumentEditor(nil, p, func(hmi.Project) error { commits++; return nil }); err != nil || commits != 0 {
		t.Fatal(err, commits)
	}
	after, _ := p.JSON()
	if string(after) != string(before) {
		t.Fatal("cancel modified input")
	}
	showProcessDocumentDialog = func(d *Dialog) DialogResult {
		e := d.Content().(*processDocumentEditor)
		e.Diagram.SelectNodes([]string{"reactor"})
		_ = e.Diagram.NudgeSelection(20, 10)
		d.buttons[1].Action().Trigger(nil)
		if d.closed || e.LastError() == nil {
			t.Fatal("failed commit closed modal or hid error")
		}
		return DialogCancel
	}
	if err := ShowProcessDocumentEditor(nil, p, func(hmi.Project) error { commits++; return errors.New("stale source") }); err != nil || commits != 1 {
		t.Fatal(err, commits)
	}
	showProcessDocumentDialog = func(d *Dialog) DialogResult {
		e := d.Content().(*processDocumentEditor)
		e.Diagram.SelectNodes([]string{"reactor", "exchanger"})
		_ = e.Diagram.NudgeSelection(20, 10)
		// Clicking Use changes during an unfinished gesture cancels its preview.
		e.Diagram.pointerDown(150, 200, processModifiers{})
		e.Diagram.pointerMove(190, 230, processModifiers{})
		d.buttons[1].Action().Trigger(nil)
		if !d.closed {
			t.Fatal("successful commit stayed open")
		}
		return DialogOK
	}
	if err := ShowProcessDocumentEditor(nil, p, func(next hmi.Project) error {
		commits++
		if next.Nodes[0].X != 110 || next.Nodes[1].X != 420 {
			t.Fatal("lost authored change or committed preview", next.Nodes)
		}
		return nil
	}); err != nil || commits != 2 {
		t.Fatal(err, commits)
	}
	showProcessDocumentDialog = func(d *Dialog) DialogResult {
		d.buttons[1].Action().Trigger(nil)
		return DialogOK
	}
	if err := ShowProcessDocumentEditor(nil, p, func(hmi.Project) error { t.Fatal("unchanged commit"); return nil }); err != nil {
		t.Fatal(err)
	}
	if err := ShowProcessDocumentEditor(nil, p, nil); err == nil {
		t.Fatal("nil host commit accepted")
	}
	p.Version = 99
	if err := ShowProcessDocumentEditor(nil, p, func(hmi.Project) error { t.Fatal("invalid commit"); return nil }); err == nil {
		t.Fatal("invalid source accepted")
	}
}
