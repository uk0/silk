package main

import (
	"github.com/uk0/silk/ged"
	"testing"
)

type selectionSheetProbe struct {
	calls    int
	selected []interface{}
}

func (s *selectionSheetProbe) Clear(interface{}) {}
func (s *selectionSheetProbe) Bind(objs []interface{}, _ string, _ interface{}) {
	s.calls++
	s.selected = objs
}
func TestWorkbenchSelectionIdleReachesPropertySheet(t *testing.T) {
	w := testWorkbench(t)
	w.activate(modeDesign)
	p := &selectionSheetProbe{}
	w.canvas.SetPropertyView(p)
	item, err := ged.NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatal(err)
	}
	item.SetParent(w.canvas.GedScene())
	w.canvas.Selection().Add(item)
	w.frame.OnIdle()
	if p.calls != 1 || len(p.selected) != 1 {
		t.Fatalf("idle did not deliver selection: calls=%d objects=%d", p.calls, len(p.selected))
	}
}
