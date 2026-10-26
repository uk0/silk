package ged

import (
	"github.com/uk0/silk/gui"
	"strings"
	"testing"
)

type historyPropertyProbe struct{ refreshes int }

func (*historyPropertyProbe) Clear(interface{})                       {}
func (*historyPropertyProbe) Bind([]interface{}, string, interface{}) {}
func (p *historyPropertyProbe) RefreshValues()                        { p.refreshes++ }
func historyCanvasAction(t *testing.T, v *GedView, name string) gui.IAction {
	t.Helper()
	for _, a := range v.Actions() {
		if a.ObjName() == name {
			return a
		}
	}
	t.Fatal("missing canvas action", name)
	return nil
}
func TestSelectionStatusActualCanvasHistoryActions(t *testing.T) {
	_, v, item, sb := selectionStatusFixture(t)
	s := v.GedScene()
	props := new(historyPropertyProbe)
	v.SetPropertyView(props)
	panel := NewCodePanel()
	panel.BindGedView(v)
	defer panel.BindGedView(nil)
	s.PushCommand(&statusSizeCommand{item, 85, 60.5})
	if props.refreshes != 1 {
		t.Fatal("command did not refresh properties", props.refreshes)
	}
	panel.RegenerateNow()
	historyCanvasAction(t, v, "edit-undo").Trigger(v)
	if item.Width() != 85 || s.UndoStack().Current() != 0 {
		t.Fatal("undo ran incorrectly")
	}
	if !strings.Contains(sb.Message(), "85×12") || props.refreshes != 2 || !panel.genPending {
		t.Fatalf("canvas undo bypassed notifications: status=%q properties=%d codePending=%v", sb.Message(), props.refreshes, panel.genPending)
	}
	panel.RegenerateNow()
	historyCanvasAction(t, v, "edit-redo").Trigger(v)
	if item.Width() != 60.5 || s.UndoStack().Current() != 1 || props.refreshes != 3 || !panel.genPending || !strings.Contains(sb.Message(), "60.5×12") {
		t.Fatal("canvas redo bypassed design lifecycle", sb.Message(), props.refreshes)
	}
	id := s.AddPendingEditor(&failingDraft{true})
	defer s.RemovePendingEditor(id)
	historyCanvasAction(t, v, "edit-undo").Trigger(v)
	if item.Width() != 60.5 || s.UndoStack().Current() != 1 {
		t.Fatal("canvas action bypassed failed pending-editor guard")
	}
}
