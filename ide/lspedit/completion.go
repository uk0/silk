// Package lspedit turns language-server edit descriptions into text and
// filesystem changes that either fully apply or not at all.
//
// Three layers, bottom to top:
//
//   - Text layer (this file): Position / Range / TextEdit plus ApplyEdits,
//     which resolves LSP UTF-16 coordinates to byte offsets, rejects invalid
//     or overlapping ranges, and applies every edit against the ORIGINAL
//     text so a list of server edits cannot drift.
//   - Completion layer (this file): ResolveCompletion turns one completion
//     item into the edits that accepting it performs — the server's textEdit
//     when present, otherwise a plain insertion over the identifier prefix at
//     the caret — carrying additionalTextEdits (the auto-import lines) along
//     instead of dropping them, and handing snippet bodies back as a template
//     for a snippet engine rather than inserting "${1:x}" literally.
//   - Transaction layer (workspace.go, preview.go): a Transaction collects
//     per-file edits and resource operations, preflights all of them, computes
//     the resulting content in memory, and only then writes — restoring the
//     already-written files when a later write fails.
//
// The package deliberately defines its own wire-free structs and imports no
// other silk package: the host adapts its LSP client types at the boundary,
// and everything here stays unit-testable without a language server.
//
// Coordinates follow LSP: zero-based lines, and Character is a UTF-16 code
// unit offset within the line (not a byte and not a rune index). Unlike a
// lenient applier, a position outside the document is an error instead of
// being clamped to the nearest valid offset — a transaction that cannot be
// resolved exactly must not be applied at all.
package lspedit

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Sentinel errors. Every error this package returns wraps one of these (or an
// *ApplyError / os error), so callers can classify with errors.Is instead of
// matching strings.
var (
	// ErrInvalidRange is a position or range that does not resolve against the
	// document: line out of bounds, character past end of line, character
	// splitting a surrogate pair, or an inverted range.
	ErrInvalidRange = errors.New("lspedit: invalid range")
	// ErrOverlap is two edits of the same document whose byte spans intersect.
	// LSP forbids it; applying them anyway silently corrupts the file.
	ErrOverlap = errors.New("lspedit: overlapping edits")
	// ErrStaleVersion is an edit computed against a document version that is
	// no longer current.
	ErrStaleVersion = errors.New("lspedit: stale document version")
	// ErrNotFound is an edit or resource operation on a path that does not
	// exist.
	ErrNotFound = errors.New("lspedit: file not found")
	// ErrExists is a create/rename whose destination is already there and that
	// did not ask to overwrite.
	ErrExists = errors.New("lspedit: file already exists")
	// ErrBadPath is an empty path, a non-regular file, or a missing parent
	// directory.
	ErrBadPath = errors.New("lspedit: invalid path")
	// ErrEmptyItem is a completion item carrying neither label, insertText nor
	// textEdit — there is nothing to insert.
	ErrEmptyItem = errors.New("lspedit: completion item has no text")
)

// Position is a zero-based LSP text position. Character counts UTF-16 code
// units from the start of the line, so an emoji before it advances Character
// by 2 while advancing the byte offset by 4.
type Position struct {
	Line      int
	Character int
}

// Range is the half-open LSP interval [Start, End). Start == End is an
// insertion point.
type Range struct {
	Start Position
	End   Position
}

// TextEdit replaces Range with NewText. An empty Range inserts; an empty
// NewText deletes.
type TextEdit struct {
	Range   Range
	NewText string
}

// ApplyEdits applies edits to text and returns the result.
//
// All coordinates are interpreted against the ORIGINAL text — the edits are
// resolved to byte spans first and applied in one pass, so their order in the
// slice does not matter and earlier edits cannot shift later ones. Overlapping
// spans, inverted ranges and out-of-document positions are errors: no partial
// text is ever returned. edits is not modified.
func ApplyEdits(text string, edits []TextEdit) (string, error) {
	if len(edits) == 0 {
		return text, nil
	}
	spans, err := resolveSpans(text, edits)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.Grow(len(text))
	prev := 0
	for _, s := range spans {
		b.WriteString(text[prev:s.start])
		b.WriteString(s.newText)
		prev = s.end
	}
	b.WriteString(text[prev:])
	return b.String(), nil
}

// ValidateEdits reports whether edits resolve cleanly against text, without
// building the result. Used by the transaction preflight to fail before
// anything is computed or written.
func ValidateEdits(text string, edits []TextEdit) error {
	_, err := resolveSpans(text, edits)
	return err
}

// span is one edit resolved to an absolute byte interval of the document.
type span struct {
	start   int
	end     int
	newText string
	order   int // original index, for a stable tie-break at equal start
}

// resolveSpans converts every edit to a byte span, sorts them ascending and
// rejects overlaps. Half-open spans that merely touch (prev.end == next.start)
// are allowed — two insertions at the same offset keep their slice order.
func resolveSpans(text string, edits []TextEdit) ([]span, error) {
	starts := lineStarts(text)
	out := make([]span, 0, len(edits))
	for i, e := range edits {
		start, err := offsetOf(text, starts, e.Range.Start)
		if err != nil {
			return nil, fmt.Errorf("edit %d start: %w", i, err)
		}
		end, err := offsetOf(text, starts, e.Range.End)
		if err != nil {
			return nil, fmt.Errorf("edit %d end: %w", i, err)
		}
		if start > end {
			return nil, fmt.Errorf("edit %d: %w: start byte %d after end byte %d", i, ErrInvalidRange, start, end)
		}
		out = append(out, span{start: start, end: end, newText: e.NewText, order: i})
	}
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].start != out[b].start {
			return out[a].start < out[b].start
		}
		return out[a].order < out[b].order
	})
	for i := 1; i < len(out); i++ {
		if out[i].start < out[i-1].end {
			return nil, fmt.Errorf("%w: [%d,%d) and [%d,%d)",
				ErrOverlap, out[i-1].start, out[i-1].end, out[i].start, out[i].end)
		}
	}
	return out, nil
}

// lineStarts returns the byte offset of every line start. Line 0 starts at 0;
// the byte after each "\n" starts the next line. A "\r" is not a line
// boundary — it stays inside the line and counts as one UTF-16 unit, which is
// how a server that sent CRLF text back computes its own columns.
func lineStarts(text string) []int {
	starts := make([]int, 1, 1+strings.Count(text, "\n"))
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

// offsetOf resolves an LSP position to an absolute byte offset. Out-of-range
// lines and columns are rejected rather than clamped.
func offsetOf(text string, starts []int, p Position) (int, error) {
	if p.Line < 0 || p.Line >= len(starts) {
		return 0, fmt.Errorf("%w: line %d outside document (%d lines)", ErrInvalidRange, p.Line, len(starts))
	}
	if p.Character < 0 {
		return 0, fmt.Errorf("%w: negative character %d", ErrInvalidRange, p.Character)
	}
	begin := starts[p.Line]
	end := len(text)
	if p.Line+1 < len(starts) {
		end = starts[p.Line+1] - 1 // drop the "\n" that ends the line
	}
	col, ok := utf16ColumnOffset(text[begin:end], p.Character)
	if !ok {
		return 0, fmt.Errorf("%w: character %d outside line %d", ErrInvalidRange, p.Character, p.Line)
	}
	return begin + col, nil
}

// utf16ColumnOffset converts a UTF-16 column inside a single line to a byte
// offset within that line. ok is false when the column runs past the end of
// the line or lands inside a surrogate pair — either way the caller must not
// guess a byte offset.
func utf16ColumnOffset(line string, col int) (int, bool) {
	if col == 0 {
		return 0, true
	}
	units := 0
	for i, r := range line {
		if units == col {
			return i, true
		}
		if units > col {
			return 0, false // col split a surrogate pair
		}
		units += utf16Len(r)
	}
	if units == col {
		return len(line), true // end of line is a valid position
	}
	return 0, false
}

// utf16Len is the number of UTF-16 code units a rune occupies: 2 for anything
// outside the BMP (a surrogate pair), 1 otherwise.
func utf16Len(r rune) int {
	if r >= 0x10000 {
		return 2
	}
	return 1
}

// utf16LenOf is utf16Len summed over a string.
func utf16LenOf(s string) int {
	n := 0
	for _, r := range s {
		n += utf16Len(r)
	}
	return n
}

// CompletionItem is the provider-agnostic subset of an LSP CompletionItem
// that decides what accepting the item does to the buffer.
//
// The fields that matter and that a label/insert-only conversion throws away:
// TextEdit (the server's own replace range, which knows how much of the typed
// prefix to consume), AdditionalTextEdits (the import line gopls adds
// elsewhere in the file so the completed symbol resolves), and IsSnippet
// (whether NewText is a template with tab stops).
type CompletionItem struct {
	Label               string
	InsertText          string
	TextEdit            *TextEdit  // wins over InsertText/Label when non-nil
	AdditionalTextEdits []TextEdit // e.g. the auto-import edit; same original coordinates
	IsSnippet           bool       // NewText is a snippet template, not literal text
}

// Application is the resolved effect of accepting a CompletionItem: every
// edit to make, all resolved against the same original buffer.
type Application struct {
	// Primary is the main edit: the range the completion replaces and the text
	// replacing it.
	//
	// When Snippet is true, Primary.NewText is the raw snippet template with
	// its tab stops intact (the same string as SnippetBody). A caller with a
	// snippet engine expands SnippetBody into Primary.Range; a caller without
	// one may insert Primary.NewText verbatim.
	Primary TextEdit
	// Additional carries the item's additionalTextEdits unchanged. Their
	// coordinates refer to the original buffer, which is why they must be
	// applied together with Primary and never one after the other.
	Additional []TextEdit
	// Snippet reports that Primary.NewText is a template, not literal text.
	Snippet bool
	// SnippetBody is the template handed to the snippet engine. Empty unless
	// Snippet.
	SnippetBody string
}

// Edits returns the primary edit followed by the additional ones, ready for
// ApplyEdits or for a FileEdit in a Transaction. The result is a fresh slice.
func (a Application) Edits() []TextEdit {
	out := make([]TextEdit, 0, 1+len(a.Additional))
	out = append(out, a.Primary)
	out = append(out, a.Additional...)
	return out
}

// Apply returns text with every edit of the application applied at once.
//
// For a snippet item this inserts the template verbatim; hosts that support
// tab stops expand SnippetBody instead of calling this.
func (a Application) Apply(text string) (string, error) {
	return ApplyEdits(text, a.Edits())
}

// ResolveCompletion computes what accepting item does to text with the caret
// at cursor.
//
// Precedence, in order:
//
//  1. item.TextEdit — the server said exactly which range to replace, so it
//     wins unconditionally and InsertText/Label are ignored (LSP requires
//     this, and gopls relies on it for prefix-aware replacements).
//  2. item.InsertText — inserted over the identifier prefix immediately left
//     of the caret, so typing "fmt.Pr" and accepting "Println" yields
//     "fmt.Println" rather than "fmt.PrPrintln".
//  3. item.Label — used when InsertText is empty.
//
// item.AdditionalTextEdits are carried into the result and validated together
// with the primary edit; an additional edit that overlaps the primary one is
// rejected instead of silently corrupting the buffer. Every range is resolved
// against text as given — nothing is written and text is not modified.
func ResolveCompletion(text string, cursor Position, item CompletionItem) (Application, error) {
	var app Application
	if item.TextEdit != nil {
		app.Primary = *item.TextEdit
	} else {
		body := item.InsertText
		if body == "" {
			body = item.Label
		}
		if body == "" {
			return Application{}, ErrEmptyItem
		}
		starts := lineStarts(text)
		caret, err := offsetOf(text, starts, cursor)
		if err != nil {
			return Application{}, fmt.Errorf("cursor: %w", err)
		}
		lineStart := starts[cursor.Line]
		start := identPrefixStart(text, lineStart, caret)
		app.Primary = TextEdit{
			Range: Range{
				// The prefix cannot cross a line break, so the start shares the
				// caret's line and only the column moves back.
				Start: Position{Line: cursor.Line, Character: utf16LenOf(text[lineStart:start])},
				End:   cursor,
			},
			NewText: body,
		}
	}
	if len(item.AdditionalTextEdits) > 0 {
		app.Additional = append([]TextEdit(nil), item.AdditionalTextEdits...)
	}
	if item.IsSnippet {
		app.Snippet = true
		app.SnippetBody = app.Primary.NewText
	}
	// Validate the whole set up front: the caller gets one error before any
	// buffer or file is touched, instead of a half-applied completion.
	if err := ValidateEdits(text, app.Edits()); err != nil {
		return Application{}, err
	}
	return app, nil
}

// identPrefixStart walks back from the caret over identifier runes and returns
// the byte offset where the typed prefix begins, never crossing lineStart.
func identPrefixStart(text string, lineStart, caret int) int {
	for caret > lineStart {
		r, size := utf8.DecodeLastRuneInString(text[lineStart:caret])
		if size == 0 || !isIdentRune(r) {
			break
		}
		caret -= size
	}
	return caret
}

// isIdentRune matches the characters a Go identifier is made of, which is what
// a completion prefix consists of.
func isIdentRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}
