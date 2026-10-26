package gui

import (
	"errors"
	"github.com/uk0/silk/i18n"
	gotoken "go/token"
)

// GoIdentifierValidator validates a rename draft without rejecting intermediate
// keystrokes. Blank identifiers, keywords and surrounding whitespace are not
// valid rename targets. It does not check scopes/collisions: use gopls for that.
func GoIdentifierValidator() Validator {
	return ValidatorFunc(func(s string) error {
		if s == "" {
			return errors.New(i18n.T("Enter a new identifier name."))
		}
		if s == "_" || !gotoken.IsIdentifier(s) {
			return errors.New(i18n.T("Use a Go identifier, not a keyword or blank identifier."))
		}
		return nil
	})
}

var showRenameSymbolInput = ShowValidatedInputBox
