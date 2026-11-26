package main

import (
	"errors"
	"fmt"

	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/printing"
)

var showDesignPrintDialog = gui.ShowWidgetPrintDialog

func canPrintDesign(canvas *ged.GedView) bool {
	return canvas != nil && canvas.GedScene() != nil &&
		(globalWorkbench == nil || documentMode(globalWorkbench.tabs, canvas) == modeDesign)
}

// Render a detached runtime preview after confirmation, not the design
// surface's selection handles and not a screenshot of the desktop.
func performPrintDesign(canvas *ged.GedView) {
	if !canPrintDesign(canvas) {
		silkideToast(i18n.T("Switch to Design mode to print"), gui.ToastWarning)
		return
	}
	scene := canvas.GedScene()
	if len(scene.MissingWidgets()) != 0 {
		silkideToast(i18n.T("Cannot print a design with missing widgets"), gui.ToastError)
		return
	}
	showDesignPrintDialog(canvas, func() (gui.IWidget, error) {
		if !canPrintDesign(canvas) || canvas.GedScene() != scene {
			return nil, errors.New(i18n.T("The design changed. Reopen Print for the current design."))
		}
		if len(scene.MissingWidgets()) != 0 {
			return nil, errors.New(i18n.T("Cannot print a design with missing widgets"))
		}
		return scene.Generate().Form(), nil
	}, printing.Options{Title: scene.FormTitle(), Copies: 1}, func(job printing.Job, err error) {
		if errors.Is(err, printing.ErrCanceled) {
			return
		}
		if err != nil {
			reportBuildOutput("print: " + err.Error())
			revealWorkbenchTool(globalBottomDock, globalBuildOutput)
			return // ShowPrintDialog already displayed its actionable error.
		}
		if job.Dispatched {
			reportBuildOutput("print: PDF renderer completed for " + job.PrinterName + "; check the printer queue")
		} else {
			reportBuildOutput(fmt.Sprintf("print: queued %s on %s", job.ID, job.PrinterName))
		}
	})
}
