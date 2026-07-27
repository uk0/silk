package ged

// CodeInsightsLayer is the per-document aggregate of the three code-insight
// models — semantic tokens, inlay hints and code lenses (code-insights-model.go)
// — plus the document revision that gates them. It is the single handle the
// editor overlay holds: one object per open file, asked per visible line for
// everything that has to be painted on top of the plain text.
//
// It stays a pure model, exactly like its three members: no widget, no
// painting, no LSP transport. The host (silkide) owns one layer per open
// document, feeds each member as gopls replies arrive, and calls SetRevision on
// every buffer edit; the drawing commit that follows only reads Line().
//
// Freshness differs per kind, which is why the revision lives here:
//   - Inlay hints are column-anchored labels squeezed between characters, so a
//     single keystroke misplaces them. They are dropped the moment the
//     revision moves (InlayHints.SetRevision).
//   - Semantic tokens and lenses are re-requested after an edit but keep
//     showing the previous result until the new one lands — the same "slightly
//     stale beats flickering" tradeoff other editors make. Highlighting that
//     blinks off on every keystroke is worse than highlighting that lags a
//     frame behind.
type CodeInsightsLayer struct {
	tokens   *SemanticTokens
	hints    *InlayHints
	lens     *CodeLens
	revision int
}

// LineInsights is everything the overlay needs to decorate ONE line: the
// semantic tokens covering it, the inlay hints anchored inside it, and the
// lenses annotating it (drawn on their own row above the line). Empty slices
// are nil, so a plain line costs no allocation.
type LineInsights struct {
	Line   int
	Tokens []Token
	Hints  []Hint
	Lenses []Lens
}

// Empty reports whether the line carries no decoration at all — the drawer's
// early-out before it computes any geometry.
func (li LineInsights) Empty() bool {
	return len(li.Tokens) == 0 && len(li.Hints) == 0 && len(li.Lenses) == 0
}

// NewCodeInsightsLayer creates an empty layer at revision 0 with the semantic
// legend the session negotiated. The legend may be empty here and supplied
// later via SemanticTokens().SetLegend.
func NewCodeInsightsLayer(legend SemanticLegend) *CodeInsightsLayer {
	l := new(CodeInsightsLayer)
	l.tokens = NewSemanticTokens(legend)
	l.hints = NewInlayHints()
	l.lens = NewCodeLens()
	return l
}

// SemanticTokens returns the token model, for the host to push
// semanticTokens/full (SetData) and /full/delta (ApplyEdits) results into.
func (this *CodeInsightsLayer) SemanticTokens() *SemanticTokens { return this.tokens }

// InlayHints returns the hint model. Push replies with Set(revision, hints)
// using the revision the request was issued for, so a late reply is refused.
func (this *CodeInsightsLayer) InlayHints() *InlayHints { return this.hints }

// CodeLens returns the lens model, for SetLenses plus the OnExecute callback.
func (this *CodeInsightsLayer) CodeLens() *CodeLens { return this.lens }

// Revision is the document revision the layer currently tracks.
func (this *CodeInsightsLayer) Revision() int { return this.revision }

// SetRevision advances the document revision after a buffer edit, dropping the
// inlay hints (their columns no longer describe the text) while leaving tokens
// and lenses in place until fresh results arrive. Moving to the same or an
// older revision is a no-op.
func (this *CodeInsightsLayer) SetRevision(revision int) {
	if revision <= this.revision {
		return
	}
	this.revision = revision
	this.hints.SetRevision(revision)
}

// Line gathers the decoration for one 0-based line. Hints are revision-gated
// against the layer's revision, so hints computed for an older buffer state
// never reach the drawer.
func (this *CodeInsightsLayer) Line(line int) LineInsights {
	return LineInsights{
		Line:   line,
		Tokens: this.tokens.TokensOnLine(line),
		Hints:  this.hints.HintsOnLine(this.revision, line),
		Lenses: this.lens.LensesOnLine(line),
	}
}

// Lines gathers the decoration for the inclusive 0-based line range
// [first, last] — the overlay's viewport query. Lines with nothing on them are
// skipped, so the result is usually far shorter than the range.
func (this *CodeInsightsLayer) Lines(first, last int) []LineInsights {
	if first < 0 {
		first = 0
	}
	var out []LineInsights
	for line := first; line <= last; line++ {
		li := this.Line(line)
		if li.Empty() {
			continue
		}
		out = append(out, li)
	}
	return out
}

// Clear drops every insight for the document (on close, or when the language
// server goes away) while keeping the revision and the semantic legend.
func (this *CodeInsightsLayer) Clear() {
	this.tokens.Clear()
	this.hints.Clear()
	this.lens.Clear()
}
