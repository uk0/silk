// Package diagnostic provides stable, machine-readable Silk SDK errors.
// Codes are API: consumers must not parse localized error messages.
package diagnostic

import (
	"errors"
	"fmt"
	"sort"
)

type Code string

type Definition struct {
	Code    Code   `json:"code"`
	Summary string `json:"summary"`
	Hint    string `json:"hint"`
}

const (
	DocumentSyntax       Code = "SILK-DOC001"
	DocumentRead         Code = "SILK-DOC002"
	DocumentCompressed   Code = "SILK-DOC003"
	SettingsRead         Code = "SILK-SET001"
	SettingsWrite        Code = "SILK-SET002"
	SettingsWait         Code = "SILK-SET003"
	Internal             Code = "SILK-0001"
	GitUnavailable       Code = "SILK-GIT001"
	GitDiffFailed        Code = "SILK-GIT002"
	GitDiffLimit         Code = "SILK-GIT003"
	GUIModalPanic        Code = "SILK-GUI001"
	FileRead             Code = "SILK-FS001"
	FileWrite            Code = "SILK-FS002"
	FileName             Code = "SILK-FS003"
	FileConflict         Code = "SILK-FS004"
	FileScan             Code = "SILK-FS005"
	FileBusy             Code = "SILK-FS006"
	FileScanLimit        Code = "SILK-FS007"
	FileOperationBusy    Code = "SILK-FS008"
	FileOperationWait    Code = "SILK-FS009"
	MacroLimit           Code = "SILK-MAC001"
	MacroDirective       Code = "SILK-MAC002"
	MacroDeclaration     Code = "SILK-MAC003"
	MacroNesting         Code = "SILK-MAC004"
	MacroDuplicate       Code = "SILK-MAC005"
	MacroParameter       Code = "SILK-MAC006"
	MacroUnclosed        Code = "SILK-MAC007"
	MacroUnknown         Code = "SILK-MAC008"
	MacroArity           Code = "SILK-MAC009"
	MacroGeneratedSyntax Code = "SILK-MAC010"
	MacroOutputProtected Code = "SILK-MAC011"
	MacroOutputWrite     Code = "SILK-MAC012"
	MacroUnsaved         Code = "SILK-MAC013"
	MacroStale           Code = "SILK-MAC014"
	LSPUnavailable       Code = "SILK-LSP001"
	LSPRequest           Code = "SILK-LSP002"
	LSPTransport         Code = "SILK-LSP003"
)

var catalog = map[Code]Definition{
	DocumentSyntax:       {DocumentSyntax, "Invalid TDoc document", "Check the indicated decoded UTF-8 line and byte column, indentation and key escapes. Keep the original file; a failed load does not return a partial document."},
	DocumentRead:         {DocumentRead, "Cannot read TDoc document", "Check the source stream, file permissions and storage health. Include the original I/O error and path when reporting this failure."},
	DocumentCompressed:   {DocumentCompressed, "Cannot decode compressed TDoc document", "The gzip member may be truncated, corrupt or unreadable. Restore a complete copy and retry a full load; limited header reads do not validate unread content."},
	SettingsRead:         {SettingsRead, "Cannot load settings", "The original file is protected. Back it up and check its format and permissions, then reopen the application. Runtime defaults and edits remain in memory."},
	SettingsWrite:        {SettingsWrite, "Cannot save settings", "Check the settings path, directory permissions and disk space, then retry saving. Pending settings remain dirty; failed replacement does not delete the previous file."},
	SettingsWait:         {SettingsWait, "Settings save is still pending", "The wait ended, not the disk operation. Keep the process alive and retry Flush to join the actual writer; do not report this as a completed save."},
	FileOperationWait:    {FileOperationWait, "Background file work did not stop", "Wait for background Git tasks to finish, then retry the rename. No filesystem change was made and unsaved buffers are retained. Include the underlying timeout or cancellation when reporting a failure."},
	GitUnavailable:       {GitUnavailable, "Git is unavailable", "Install Git or add it to the application PATH, then refresh editor Git markers. Editing and saving remain available."},
	GitDiffFailed:        {GitDiffFailed, "Cannot refresh Git markers", "Check the repository and file permissions, then retry. Include the underlying Git error when reporting a failure."},
	GitDiffLimit:         {GitDiffLimit, "Git diff resource limit reached", "Reduce the diff size or split the generated file. Markers are cleared rather than presenting an incomplete diff as current."},
	FileOperationBusy:    {FileOperationBusy, "File operation is busy", "Wait for the pending file operation or stop the active task, then retry. Your editor buffer is retained."},
	FileScanLimit:        {FileScanLimit, "Project scan limit reached", "Choose a smaller project root or adjust scan limits. The displayed index is incomplete; refining the query cannot reveal files that were not indexed."},
	LSPTransport:         {LSPTransport, "Language-server transport interrupted", "Save current work and restart the language service. The pipe was closed to prevent partial messages or an unresponsive server from blocking further requests."},
	GUIModalPanic:        {GUIModalPanic, "Modal operation failed", "Save current work and reopen the dialog. Include this code and the logged stack trace when reporting the failure."},
	Internal:             {Internal, "Internal operation failed", "Keep the error code and underlying cause when reporting this failure."},
	FileRead:             {FileRead, "Cannot read file", "Check the file still exists and is readable."},
	FileWrite:            {FileWrite, "Cannot write file", "Check directory permissions and available disk space; keep the unsaved buffer."},
	FileName:             {FileName, "Invalid filename", "Use a single portable filename without separators or reserved device names."},
	FileConflict:         {FileConflict, "Destination already exists", "Choose another name; existing files are not overwritten."},
	FileScan:             {FileScan, "Cannot scan directory", "Check directory permissions and retry Refresh."},
	FileBusy:             {FileBusy, "File has unsaved edits", "Save or close the affected editors before changing the filesystem."},
	MacroLimit:           {MacroLimit, "Macro resource limit exceeded", "Split the source or reduce expansion count/output size."},
	MacroDirective:       {MacroDirective, "Invalid macro directive", "Use standalone //silk:macro, //silk:end or //silk:expand lines."},
	MacroDeclaration:     {MacroDeclaration, "Invalid macro declaration or invocation", "Use Name(arguments); parameters must be distinct ASCII identifiers."},
	MacroNesting:         {MacroNesting, "Nested or recursive macro", "Close the current definition; macro bodies cannot contain expansions."},
	MacroDuplicate:       {MacroDuplicate, "Duplicate macro definition", "Rename or remove the duplicate definition in this file."},
	MacroParameter:       {MacroParameter, "Unknown macro parameter", "Declare the ${Name} parameter in the macro signature."},
	MacroUnclosed:        {MacroUnclosed, "Unclosed macro definition", "Add a standalone //silk:end after the macro body."},
	MacroUnknown:         {MacroUnknown, "Unknown macro", "Define the macro in this file; cross-file imports are not supported."},
	MacroArity:           {MacroArity, "Wrong macro argument count", "Supply one argument for each declared parameter."},
	MacroGeneratedSyntax: {MacroGeneratedSyntax, "Expanded Go is invalid", "Check the macro body and arguments at the expansion site; inspect the underlying Go parser error."},
	MacroOutputProtected: {MacroOutputProtected, "Generated output is protected", "Choose a regular .go output carrying silkgen's exact generated marker; never overwrite handwritten or symlink output."},
	MacroOutputWrite:     {MacroOutputWrite, "Cannot install macro output", "Check output-directory permissions and disk space; previous output is preserved if installation fails."},
	MacroUnsaved:         {MacroUnsaved, "Macro document is unsaved", "Save the .silkgo source and any modified generated output before expanding."},
	MacroStale:           {MacroStale, "Macro source changed during generation", "Expand again using the current source."},
	LSPUnavailable:       {LSPUnavailable, "Language server unavailable", "Install/configure gopls and restart the language service."},
	LSPRequest:           {LSPRequest, "Language request failed", "Check the underlying server error and retry after the workspace finishes loading."},
}

// Lookup and Catalog return copies; callers cannot mutate the shared catalog.
func Lookup(code Code) (Definition, bool) { d, ok := catalog[code]; return d, ok }
func Catalog() []Definition {
	out := make([]Definition, 0, len(catalog))
	for _, d := range catalog {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

// Location uses 1-based line and UTF-8 byte column, like go/token.Position.
// Zero line/column means a file-level error. Offset is a zero-based byte offset.
type Location struct {
	File   string `json:"file,omitempty"`
	Line   int    `json:"line,omitempty"`
	Column int    `json:"column,omitempty"`
	Offset int    `json:"offset,omitempty"`
}

type Error struct {
	Code      Code     `json:"code"`
	Severity  string   `json:"severity"`
	Message   string   `json:"message"`
	Hint      string   `json:"hint"`
	Location  Location `json:"location"`
	CauseText string   `json:"cause,omitempty"`
	cause     error
}

func New(code Code, message string, cause error) *Error {
	d, ok := Lookup(code)
	if !ok {
		d = catalog[Internal]
		code = Internal
	}
	if message == "" {
		message = d.Summary
	}
	e := &Error{Code: code, Severity: "error", Message: message, Hint: d.Hint, cause: cause}
	if cause != nil {
		e.CauseText = cause.Error()
	}
	return e
}

// At returns a copy, allowing a shared error to be annotated without mutation.
func (e *Error) At(location Location) *Error { c := *e; c.Location = location; return &c }
func (e *Error) Error() string {
	s := fmt.Sprintf("[%s] %s", e.Code, e.Message)
	if e.CauseText != "" {
		s += ": " + e.CauseText
	}
	if e.Location.File != "" {
		where := e.Location.File
		if e.Location.Line > 0 {
			where += fmt.Sprintf(":%d", e.Location.Line)
			if e.Location.Column > 0 {
				where += fmt.Sprintf(":%d", e.Location.Column)
			}
		}
		s = where + ": " + s
	}
	return s
}
func (e *Error) Unwrap() error { return e.cause }
func CodeOf(err error) Code {
	var d *Error
	if errors.As(err, &d) {
		return d.Code
	}
	if err == nil {
		return ""
	}
	return Internal
}
