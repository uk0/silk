package main

import (
	"testing"

	"github.com/uk0/silk/ged"
)

func TestLSPDiagnosticSeverityPreservesHints(t *testing.T) {
	panel := ged.NewProblemsPanel()
	var problems []ged.Problem
	for i, want := range []ged.Severity{ged.SeverityError, ged.SeverityError, ged.SeverityWarning, ged.SeverityInfo, ged.SeverityHint} {
		got := lspProblemSeverity(i)
		if got != want {
			t.Fatalf("severity %d = %v, want %v", i, got, want)
		}
		problems = append(problems, ged.Problem{Severity: got})
	}
	panel.SetProblems(problems)
	if panel.ErrorCount() != 2 || panel.WarningCount() != 1 || panel.InfoCount() != 2 {
		t.Fatal("hints counted as errors or warnings")
	}
}
