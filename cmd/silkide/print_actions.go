package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/printing"
)

var showDesignPrintDialog = gui.ShowPrintDialog

func canPrintDesign(canvas *ged.GedView) bool {
	return canvas != nil && canvas.GedScene() != nil &&
		(globalWorkbench == nil || documentMode(globalWorkbench.tabs, canvas) == modeDesign)
}

// Render a detached preview tree, never the editor selection/handles or a
// screenshot of the desktop. ShowPrintDialog stages the PDF before it returns,
// so the host temporary file can be removed on every exit path.
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
	preview := scene.Generate().Form()
	file, err := os.CreateTemp("", "silk-design-print-*.pdf")
	if err != nil {
		reportPrintFailure(err)
		return
	}
	path := file.Name()
	defer os.Remove(path)
	err = gui.RenderWidgetPDF(file, preview)
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		reportPrintFailure(err)
		return
	}
	showDesignPrintDialog(canvas, path, printing.Options{Title: scene.FormTitle(), Copies: 1}, func(job printing.Job, err error) {
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

func reportPrintFailure(err error) {
	reportBuildOutput("print: " + err.Error())
	revealWorkbenchTool(globalBottomDock, globalBuildOutput)
	silkideToast(i18n.T("Printing could not complete")+": "+err.Error(), gui.ToastError)
}
