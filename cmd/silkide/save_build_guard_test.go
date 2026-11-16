package main

import (
	"testing"

	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
)

func TestSaveReservationsBlockBuildRunAndRelatedLaunches(t *testing.T) {
	w := testWorkbench(t)
	oldOutput, oldTerminal, oldToast := globalBuildOutput, globalTerminal, silkideToast
	globalBuildOutput, globalTerminal = nil, nil
	notices := 0
	silkideToast = func(string, gui.ToastLevel) { notices++ }
	t.Cleanup(func() { globalBuildOutput, globalTerminal, silkideToast = oldOutput, oldTerminal, oldToast })
	ticket, ok := reserveSavePaths(new(saveReservationOwner), designSavePaths(w.canvas.GedScene().Filename()))
	if !ok {
		t.Fatal("reservation")
	}
	t.Cleanup(func() { ticket.finish(true); ticket.finish(false) })
	for _, run := range []func(*ged.GedView){buildProject, runProjectInTerminal, runProjectTests, runProjectVet, runProjectWithCoverage, runProjectInDebugger} {
		run(w.canvas)
	}
	if notices != 6 || globalBuildOutput != nil || globalTerminal != nil {
		t.Fatal("launch passed pending-save guard", notices)
	}
}
