package ged

import (
	"fmt"
	"sort"
)

// This file holds the three "code insight" presentation models the editor
// overlay consumes: semantic tokens, inlay hints and code lenses. All three
// are pure data + query logic — no widget, no GL, no LSP transport. The host
// (silkide) drives gopls, converts the wire payloads into the shapes below and
// pushes them in; the overlay only reads them back per visible line. Same
// split as ReferencesPanel/TodoPanel keeping off core.LSPLocation, except here
// there is not even a widget: CodeInsightsLayer (code-insights-layer.go)
// aggregates the three for one document and a later commit draws them.

// --- (a) Semantic tokens ---

// semanticTokenStride is the number of uint32 entries per token in the LSP
// semantic-tokens stream: deltaLine, deltaStartChar, length, tokenType,
// tokenModifiers.
const semanticTokenStride = 5

// SemanticLegend is the server-declared legend that gives meaning to the
// numeric tokenType index and the tokenModifiers bitmask in the token stream.
// It arrives once, in the initialize result's semanticTokensProvider.legend,
// and stays fixed for the session. Indices are positions in Types; modifier
// bit i (LSB first) names Modifiers[i].
type SemanticLegend struct {
	Types     []string // tokenTypes, e.g. ["namespace","type","function",...]
	Modifiers []string // tokenModifiers, e.g. ["declaration","definition",...]
}

// Token is one decoded semantic token with ABSOLUTE coordinates and
// legend-resolved names — the shape the overlay paints. Lines and characters
// stay 0-based, exactly as LSP reports them (core.LSPPosition convention), so
// no base juggling happens in the decoder; the drawer adds the editor's own
// offset once.
//
// Per spec a token never spans lines and tokens never overlap, so Line plus
// [StartChar, StartChar+Length) fully places it.
type Token struct {
	Line      int      // 0-based line
	StartChar int      // 0-based character offset within the line
	Length    int      // token length in characters
	Type      string   // legend-resolved type; "" when the index is outside the legend
	Mods      []string // legend-resolved modifiers in bit order; nil when the mask is 0
}

// SemanticTokenEdit is one splice into the PREVIOUS flat uint32 stream (LSP
// SemanticTokensEdit): drop DeleteCount entries starting at index Start, then
// insert Data there. Start/DeleteCount index the raw uint32 stream, NOT
// tokens, and a well-formed server emits them ascending and non-overlapping
// against the previous stream — see SemanticTokens.ApplyEdits.
type SemanticTokenEdit struct {
	Start       int      // offset into the previous data array
	DeleteCount int      // entries to remove at Start
	Data        []uint32 // entries to insert (may be empty for a pure delete)
}

// SemanticTokens holds one document's semantic highlighting: the raw
// delta-encoded uint32 stream as the server sent it, plus the decoded
// absolute tokens and a line index for the overlay's per-line lookups.
//
// The raw stream is retained on purpose: textDocument/semanticTokens/full/delta
// replies describe edits against it, so a delta update can only be applied by
// splicing the previous stream and re-decoding (ApplyEdits).
type SemanticTokens struct {
	legend   SemanticLegend
	resultID string
	data     []uint32 // last accepted raw stream, kept so deltas can splice it
	tokens   []Token
	byLine   map[int][]Token
}

// NewSemanticTokens creates an empty token set bound to a legend. The legend
// may be empty at construction and filled in later with SetLegend.
func NewSemanticTokens(legend SemanticLegend) *SemanticTokens {
	t := new(SemanticTokens)
	t.SetLegend(legend)
	return t
}

// SetLegend replaces the legend and re-decodes the retained raw stream, so a
// legend that arrives after the first tokens still resolves them. Copies the
// legend slices: the caller keeps ownership of what it handed in.
func (this *SemanticTokens) SetLegend(legend SemanticLegend) {
	this.legend = SemanticLegend{
		Types:     insightsCopyStrings(legend.Types),
		Modifiers: insightsCopyStrings(legend.Modifiers),
	}
	this.decode()
}

// Legend returns a defensive copy of the current legend.
func (this *SemanticTokens) Legend() SemanticLegend {
	return SemanticLegend{
		Types:     insightsCopyStrings(this.legend.Types),
		Modifiers: insightsCopyStrings(this.legend.Modifiers),
	}
}

// SetData accepts a full semanticTokens/full result: resultId plus the whole
// delta-encoded stream. The stream must be a whole number of 5-entry tokens;
// a malformed length is rejected and the previous state is kept untouched, so
// a bad reply never blanks working highlighting.
func (this *SemanticTokens) SetData(resultID string, data []uint32) error {
	if len(data)%semanticTokenStride != 0 {
		return fmt.Errorf("semantic tokens: data length %d is not a multiple of %d", len(data), semanticTokenStride)
	}
	this.resultID = resultID
	this.data = insightsCopyUint32(data)
	this.decode()
	return nil
}

// ApplyEdits accepts a semanticTokens/full/delta result: the edits splice the
// previous raw stream into the new one, which is then re-decoded.
//
// Edits index the previous stream and are applied as one simultaneous splice
// (all offsets relative to the pre-edit array), which is what the reference
// clients do. Therefore they must arrive ascending and non-overlapping; an
// out-of-order or out-of-range edit, or a result that is not a whole number of
// tokens, is rejected and the previous state is kept. An empty edit list means
// "unchanged" and only refreshes the result id, matching servers that answer a
// delta request with no edits.
func (this *SemanticTokens) ApplyEdits(resultID string, edits []SemanticTokenEdit) error {
	prev := this.data
	out := make([]uint32, 0, len(prev))
	cursor := 0 // how far into prev we have consumed
	for i, e := range edits {
		if e.Start < cursor {
			return fmt.Errorf("semantic tokens: edit %d starts at %d, behind cursor %d (edits must be ascending and non-overlapping)", i, e.Start, cursor)
		}
		if e.DeleteCount < 0 {
			return fmt.Errorf("semantic tokens: edit %d has negative deleteCount %d", i, e.DeleteCount)
		}
		if e.Start+e.DeleteCount > len(prev) {
			return fmt.Errorf("semantic tokens: edit %d range [%d,%d) exceeds previous data length %d", i, e.Start, e.Start+e.DeleteCount, len(prev))
		}
		out = append(out, prev[cursor:e.Start]...)
		out = append(out, e.Data...)
		cursor = e.Start + e.DeleteCount
	}
	out = append(out, prev[cursor:]...)

	if len(out)%semanticTokenStride != 0 {
		return fmt.Errorf("semantic tokens: spliced length %d is not a multiple of %d", len(out), semanticTokenStride)
	}
	this.resultID = resultID
	this.data = out
	this.decode()
	return nil
}

// ResultID is the id of the last accepted result, the value to send back as
// previousResultId when asking for a delta.
func (this *SemanticTokens) ResultID() string { return this.resultID }

// Data returns a defensive copy of the retained raw delta-encoded stream.
func (this *SemanticTokens) Data() []uint32 { return insightsCopyUint32(this.data) }

// Tokens returns a defensive copy of the decoded tokens in stream order,
// which per spec is document order (lines non-decreasing, characters
// increasing within a line).
func (this *SemanticTokens) Tokens() []Token {
	out := make([]Token, len(this.tokens))
	copy(out, this.tokens)
	return out
}

// TokensOnLine returns the tokens on one 0-based line, in character order —
// the overlay's per-visible-line query, served off a prebuilt index rather
// than a scan of the whole document. Returns nil for a line without tokens.
func (this *SemanticTokens) TokensOnLine(line int) []Token {
	src := this.byLine[line]
	if len(src) == 0 {
		return nil
	}
	out := make([]Token, len(src))
	copy(out, src)
	return out
}

// Clear drops the tokens, the raw stream and the result id, keeping the
// legend (it belongs to the session, not to the document).
func (this *SemanticTokens) Clear() {
	this.resultID = ""
	this.data = nil
	this.decode()
}

// decode rebuilds tokens + the line index from the retained raw stream. The
// stream is delta-encoded: deltaLine is relative to the previous token's line,
// and deltaStartChar is relative to the previous token's start character when
// deltaLine is 0, absolute otherwise. Callers guarantee len(data)%5 == 0.
func (this *SemanticTokens) decode() {
	this.tokens = nil
	this.byLine = make(map[int][]Token)
	line, char := 0, 0
	for i := 0; i+semanticTokenStride <= len(this.data); i += semanticTokenStride {
		deltaLine := int(this.data[i])
		deltaChar := int(this.data[i+1])
		if deltaLine == 0 {
			char += deltaChar
		} else {
			line += deltaLine
			char = deltaChar
		}
		tok := Token{
			Line:      line,
			StartChar: char,
			Length:    int(this.data[i+2]),
			Type:      semanticLegendName(this.legend.Types, int(this.data[i+3])),
			Mods:      expandTokenModifiers(this.legend.Modifiers, this.data[i+4]),
		}
		this.tokens = append(this.tokens, tok)
		this.byLine[line] = append(this.byLine[line], tok)
	}
}

// semanticLegendName resolves a legend index, tolerating an out-of-range index (a
// server that highlights with a type it never declared) by returning "" — the
// token still shows up geometrically, just unstyled.
func semanticLegendName(names []string, idx int) string {
	if idx < 0 || idx >= len(names) {
		return ""
	}
	return names[idx]
}

// expandTokenModifiers turns the modifier bitmask into the legend names it
// selects, in bit order (LSB first). Bits with no legend entry are ignored,
// and a zero mask yields nil so the common unmodified token allocates nothing.
func expandTokenModifiers(names []string, mask uint32) []string {
	if mask == 0 {
		return nil
	}
	var out []string
	for i := 0; i < len(names); i++ {
		if mask&(1<<uint(i)) != 0 {
			out = append(out, names[i])
		}
	}
	return out
}

// --- (b) Inlay hints ---

// Hint kind values, mirroring LSP InlayHintKind (1 = Type, 2 = Parameter) as
// the strings the presentation layer switches on. An empty Kind means the
// server did not say, and the overlay styles it neutrally.
const (
	HintKindType      = "type"
	HintKindParameter = "parameter"
)

// Hint is one inlay hint: a short label the editor paints INSIDE the line
// without it being part of the buffer — an inferred type after `x :=`, or a
// parameter name before an argument. Line/Col are 0-based, like Token.
//
// PaddingLeft/PaddingRight are the server's request for a space on either
// side of the label so it does not collide with the surrounding code; Text
// applies them.
type Hint struct {
	Line         int    // 0-based line the hint sits on
	Col          int    // 0-based character offset the hint is anchored before
	Label        string // the text to paint
	Kind         string // HintKindType | HintKindParameter | "" when unspecified
	PaddingLeft  bool   // render a leading space
	PaddingRight bool   // render a trailing space
}

// Text is the label with the padding flags applied — what actually gets
// painted. Kept a method so the padding fields have exactly one presentation.
func (h Hint) Text() string {
	s := h.Label
	if h.PaddingLeft {
		s = " " + s
	}
	if h.PaddingRight {
		s = s + " "
	}
	return s
}

// InlayHints stores one document's hints together with the document revision
// they were computed against.
//
// Hints are position-sensitive in a way tokens are not: a hint at line 12 col
// 30 is meaningless the moment the user types on line 12, and gopls answers
// asynchronously, so a reply can land after the buffer has already moved on.
// The revision is the guard — the host bumps SetRevision on every edit, and a
// stored set whose revision no longer matches is stale and yields no hints
// (HintsFor / HintsOnLine return nothing) instead of painting labels at wrong
// columns. A late reply for an older revision is refused by Set for the same
// reason.
type InlayHints struct {
	revision int // revision the stored hints belong to
	hints    []Hint
}

// NewInlayHints creates an empty hint set at revision 0.
func NewInlayHints() *InlayHints {
	return new(InlayHints)
}

// Set stores the hints computed for revision, replacing any previous set, and
// reports whether they were accepted. A reply for a revision older than the
// current one is dropped (returns false): it describes a buffer state that no
// longer exists.
func (this *InlayHints) Set(revision int, hints []Hint) bool {
	if revision < this.revision {
		return false
	}
	this.revision = revision
	this.hints = make([]Hint, len(hints))
	copy(this.hints, hints)
	return true
}

// SetRevision advances the document revision, which drops the stored hints:
// after an edit their columns no longer describe the buffer. Moving to the
// same or an older revision is a no-op, so redundant notifications are
// harmless.
func (this *InlayHints) SetRevision(revision int) {
	if revision <= this.revision {
		return
	}
	this.revision = revision
	this.hints = nil
}

// Revision is the revision the stored hints belong to.
func (this *InlayHints) Revision() int { return this.revision }

// Stale reports whether the stored hints do not describe the given document
// revision — the check the overlay makes before painting anything.
func (this *InlayHints) Stale(revision int) bool { return this.revision != revision }

// Hints returns a defensive copy of the stored hints regardless of revision.
// Use HintsFor when painting.
func (this *InlayHints) Hints() []Hint {
	out := make([]Hint, len(this.hints))
	copy(out, this.hints)
	return out
}

// HintsFor returns the hints only when they match the given document
// revision, and nil when they are stale — the revision-gated read.
func (this *InlayHints) HintsFor(revision int) []Hint {
	if this.Stale(revision) {
		return nil
	}
	return this.Hints()
}

// HintsOnLine returns the stored hints on one 0-based line in column order,
// gated on the revision the same way HintsFor is.
func (this *InlayHints) HintsOnLine(revision, line int) []Hint {
	if this.Stale(revision) {
		return nil
	}
	var out []Hint
	for _, h := range this.hints {
		if h.Line == line {
			out = append(out, h)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Col < out[j].Col })
	return out
}

// Clear drops the stored hints, keeping the revision.
func (this *InlayHints) Clear() { this.hints = nil }

// --- (c) Code lens ---

// Lens is one code lens: a clickable annotation the editor paints on its own
// row above Line ("3 references", "run test"). Command is the id the host
// executes on click and Args its arguments, already flattened to strings by
// the host — the presentation layer never interprets them, it just hands the
// whole Lens back through the execute callback.
type Lens struct {
	Line    int      // 0-based line the lens annotates
	Title   string   // the text to paint
	Command string   // command id to run on activation
	Args    []string // command arguments, host-stringified
}

// CodeLens stores one document's lenses and dispatches activation. Hit-testing
// is by line: the overlay reserves a row above each decorated line, and a
// click resolves to (line, index-within-line) which ExecuteAt turns into the
// OnExecute callback.
type CodeLens struct {
	lenses []Lens
	cbExec func(lens Lens)
}

// NewCodeLens creates an empty lens set.
func NewCodeLens() *CodeLens {
	return new(CodeLens)
}

// SetLenses replaces the lenses with a defensive copy.
func (this *CodeLens) SetLenses(lenses []Lens) {
	this.lenses = make([]Lens, len(lenses))
	copy(this.lenses, lenses)
}

// Lenses returns a defensive copy of all lenses in insertion order.
func (this *CodeLens) Lenses() []Lens {
	out := make([]Lens, len(this.lenses))
	copy(out, this.lenses)
	return out
}

// LensesOnLine returns the lenses annotating one 0-based line, in insertion
// order — the hit-test. nil when the line carries none, which is also how the
// drawer knows not to reserve a lens row for it.
func (this *CodeLens) LensesOnLine(line int) []Lens {
	var out []Lens
	for _, l := range this.lenses {
		if l.Line == line {
			out = append(out, l)
		}
	}
	return out
}

// OnExecute registers the callback fired when a lens is activated. The host
// runs lens.Command with lens.Args (workspace/executeCommand, or its own
// action for a synthetic lens).
func (this *CodeLens) OnExecute(fn func(lens Lens)) { this.cbExec = fn }

// ExecuteAt activates the index-th lens on a line and reports whether one was
// found. Index is the position within LensesOnLine(line), which is what the
// drawer's x hit-test yields; index 0 activates the only lens on a line. A
// miss, or no registered callback, is inert.
func (this *CodeLens) ExecuteAt(line, index int) bool {
	on := this.LensesOnLine(line)
	if index < 0 || index >= len(on) {
		return false
	}
	if this.cbExec != nil {
		this.cbExec(on[index])
	}
	return true
}

// Clear drops the lenses, keeping the callback registration.
func (this *CodeLens) Clear() { this.lenses = nil }

// --- shared copy helpers ---

// insightsCopyStrings copies a string slice, mapping empty to nil so an absent legend
// stays absent instead of becoming a zero-length slice.
func insightsCopyStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

// insightsCopyUint32 copies a uint32 slice, mapping empty to nil.
func insightsCopyUint32(in []uint32) []uint32 {
	if len(in) == 0 {
		return nil
	}
	out := make([]uint32, len(in))
	copy(out, in)
	return out
}
