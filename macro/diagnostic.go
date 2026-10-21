package macro

import (
	"fmt"
	"github.com/uk0/silk/diagnostic"
	"go/token"
)

func macroError(code diagnostic.Code, pos token.Position, cause error, format string, args ...any) *diagnostic.Error {
	return diagnostic.New(code, fmt.Sprintf(format, args...), cause).At(diagnostic.Location{File: pos.Filename, Line: pos.Line, Column: pos.Column, Offset: pos.Offset})
}
