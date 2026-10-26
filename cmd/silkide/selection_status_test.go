package main

import (
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
	"testing"
)

func TestItemBoundsLabelPreservesFractions(t *testing.T) {
	item, err := ged.NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatal(err)
	}
	item.SetBounds(15.25, -2.75, 60.5, 12.125)
	if got := itemBoundsLabel(item); got != "(15.25, -2.75) 60.5×12.125" {
		t.Fatalf("fractional bounds = %q", got)
	}
}

func TestSelectionStatusWorkbenchFormatter(t *testing.T) {
	w := testWorkbench(t)
	w.activate(modeDesign)
	sb := gui.NewStatusBar()
	w.frame.SetStatusBar(sb)
	wireSelectionStatus(w.canvas)
	item, err := ged.NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatal(err)
	}
	item.SetParent(w.canvas.GedScene())
	item.SetBounds(15.25, -2.75, 60.5, 12.125)
	w.canvas.Selection().Add(item)
	w.frame.OnIdle()
	want := i18n.Tf("Selected: %s", itemDisplayName(item)) + " (15.25, -2.75) 60.5×12.125 mm"
	if sb.Message() != want {
		t.Fatalf("host status overwritten: got %q want %q", sb.Message(), want)
	}
	item.SetSize(70.25, 12.125)
	w.canvas.GedScene().NotifyDesignChanged()
	want = i18n.Tf("Selected: %s", itemDisplayName(item)) + " (15.25, -2.75) 70.25×12.125 mm"
	if sb.Message() != want {
		t.Fatal("mutation left stale status", sb.Message())
	}
	w.activate(modeEdit)
	sb.ShowMessage("editor")
	item.SetSize(75.5, 12.125)
	w.canvas.GedScene().NotifyDesignChanged()
	if sb.Message() != "editor" {
		t.Fatal("inactive design overwrote editor status")
	}
	w.activate(modeDesign)
	want = i18n.Tf("Selected: %s", itemDisplayName(item)) + " (15.25, -2.75) 75.5×12.125 mm"
	if sb.Message() != want {
		t.Fatal("mode switch did not restore selection status", sb.Message())
	}
	w.canvas.Selection().Clear()
	w.frame.OnIdle()
	if sb.Message() != "" {
		t.Fatal("IDE empty selection should clear status", sb.Message())
	}
}
