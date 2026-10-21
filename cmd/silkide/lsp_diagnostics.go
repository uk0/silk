package main

import "github.com/uk0/silk/ged"

func lspProblemSeverity(severity int) ged.Severity {
	switch severity {
	case 2:
		return ged.SeverityWarning
	case 3:
		return ged.SeverityInfo
	case 4:
		return ged.SeverityHint
	default:
		return ged.SeverityError
	}
}
