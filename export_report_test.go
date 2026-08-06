package main

import (
	"strings"
	"testing"

	"github.com/uk0/silk/ged"
)

// TestExportReportNamesTheImportCodegenMayNotAdd: an appended handler whose
// signature names a package lands in a file whose import block the generator is
// not allowed to touch, so this dialog is the only place the developer learns
// which import to add before the build tells them. A report that lists the
// method and swallows the import is what turns a one-line fix into a hunt.
func TestExportReportNamesTheImportCodegenMayNotAdd(t *testing.T) {
	msg := exportReport(ged.SplitResult{
		MachineFile:    "/p/app.silk.go",
		UserFile:       "/p/app.go",
		AddedStubs:     []string{"onColorChanged"},
		MissingImports: []string{"github.com/uk0/silk/paint"},
	})
	if !strings.Contains(msg, "onColorChanged") {
		t.Errorf("the report does not name the appended method:\n%s", msg)
	}
	if !strings.Contains(msg, "github.com/uk0/silk/paint") {
		t.Errorf("the report does not name the import the developer must add:\n%s", msg)
	}

	// With nothing to add there is nothing to say: the line is a real gap, not
	// boilerplate on every export.
	quiet := exportReport(ged.SplitResult{
		MachineFile: "/p/app.silk.go",
		UserFile:    "/p/app.go",
		AddedStubs:  []string{"onGo"},
	})
	if strings.Contains(quiet, "import") {
		t.Errorf("the report asks for an import with none missing:\n%s", quiet)
	}
}
