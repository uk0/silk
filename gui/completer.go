package gui

import (
	"sort"
	"strings"
)

// MatchMode picks how Completer.Filter compares the user's typed prefix
// against each candidate. Mirrors QCompleter's three-way mode set:
// dominant for code-completion (StartsWith), dominant for fuzzy search
// (Contains), and a fallback that lets every candidate through with no
// substring constraint at all (Anywhere — useful for "show recent values
// regardless of typing").
type MatchMode int

const (
	// MatchStartsWith accepts candidates whose first characters are the
	// typed prefix. The default, matching QCompleter::PopupCompletion's
	// default and the dominant interactive UX.
	MatchStartsWith MatchMode = iota

	// MatchContains accepts candidates that contain the typed prefix
	// anywhere. Useful for path / symbol search where the user remembers
	// part of the middle of the name.
	MatchContains

	// MatchAnywhere accepts every candidate regardless of the typed
	// prefix. Best for "recent values" or "command history" where the
	// candidate set itself is the filter.
	MatchAnywhere
)

// String aids debug + property panels.
func (m MatchMode) String() string {
	switch m {
	case MatchStartsWith:
		return "StartsWith"
	case MatchContains:
		return "Contains"
	case MatchAnywhere:
		return "Anywhere"
	}
	return "<unknown>"
}

// CompletionSource is the optional dynamic-data interface for a
// Completer. When the candidate set is computed at filter time (e.g.
// filesystem paths, database queries, language symbols) the host
// implements CompletionSource and assigns it to the Completer; the
// pre-baked Candidates slice is consulted only when Source is nil.
//
// CandidateAt(prefix) returns the full candidate list given the
// current input. It is the implementation's responsibility to cache /
// debounce expensive lookups; the Completer does not memoise.
type CompletionSource interface {
	CandidateAt(prefix string) []string
}

// stringSliceSource adapts a static []string into a CompletionSource.
// Used internally when callers pass Candidates without overriding Source.
type stringSliceSource struct {
	items []string
}

func (s stringSliceSource) CandidateAt(prefix string) []string {
	return s.items
}

// Completer ranks candidates against a typed prefix and exposes the
// matches as Suggestions(). Mirrors QCompleter at the data layer; UI
// integration (popup display, keyboard navigation) is handled by the
// host widget — Edit reaches into Completer.Suggestions() to render
// its own completion popup.
//
// Default settings: MatchStartsWith with case-insensitive comparison.
// Override via SetMode / SetCaseSensitive when constructing.
type Completer struct {
	// Source produces the candidate list dynamically. When nil, the
	// pre-baked Candidates slice is used. Either may be set; if both
	// are set, Source wins.
	Source CompletionSource

	// Candidates is a pre-baked candidate list. Compatible with the
	// dominant case (a fixed dictionary, recent-values list, etc.).
	Candidates []string

	mode          MatchMode
	caseSensitive bool

	// suggestions holds the result of the most recent Filter() call.
	// Held on the Completer itself so the host doesn't need to thread
	// the slice through method returns when binding to a UI popup.
	suggestions []string

	// dedupe removes duplicate entries from the rendered suggestions
	// list. Defaults to true — duplicates in a candidate set are
	// usually a data-load mistake the user shouldn't have to see.
	dedupe bool

	// maxSuggestions caps the suggestion list length. 0 = unlimited.
	// Useful for huge candidate sets where the popup would otherwise
	// scroll forever.
	maxSuggestions int
}

// NewCompleter builds a Completer over a static candidate list with
// the default settings (StartsWith + case-insensitive + dedupe).
// Callers needing a dynamic source can ignore the candidates argument
// and assign Source after construction.
func NewCompleter(candidates ...string) *Completer {
	c := &Completer{
		Candidates:    candidates,
		mode:          MatchStartsWith,
		caseSensitive: false,
		dedupe:        true,
	}
	return c
}

// SetMode changes the matcher.
func (c *Completer) SetMode(m MatchMode) { c.mode = m }

// Mode returns the current match mode.
func (c *Completer) Mode() MatchMode { return c.mode }

// SetCaseSensitive toggles case sensitivity.
func (c *Completer) SetCaseSensitive(b bool) { c.caseSensitive = b }

// IsCaseSensitive reports the current case-sensitivity setting.
func (c *Completer) IsCaseSensitive() bool { return c.caseSensitive }

// SetMaxSuggestions caps the result list length. n=0 means no cap.
func (c *Completer) SetMaxSuggestions(n int) { c.maxSuggestions = n }

// SetDedupe toggles duplicate-removal in the suggestion list.
func (c *Completer) SetDedupe(b bool) { c.dedupe = b }

// Filter recomputes suggestions for the given typed prefix and stores
// the result on the Completer. Returns the same slice that Suggestions
// would return — convenient for callers that want the values inline.
//
// The input order in the result is: (1) candidates whose match starts
// at position 0, (2) candidates whose match is later in the string,
// then (3) alphabetical. This puts "best matches" first without a
// dedicated relevance score, which is fine for the dominant UI use
// case.
func (c *Completer) Filter(prefix string) []string {
	src := c.candidateSet()
	if len(src) == 0 {
		c.suggestions = nil
		return nil
	}

	cmpPrefix := prefix
	if !c.caseSensitive {
		cmpPrefix = strings.ToLower(prefix)
	}

	// Two-bucket pass: prefix matches (front) vs contains-only matches
	// (back). MatchAnywhere collapses both into "front" since position
	// is irrelevant. MatchStartsWith doesn't take the back bucket at all.
	var front, back []string
	for _, cand := range src {
		cmpCand := cand
		if !c.caseSensitive {
			cmpCand = strings.ToLower(cand)
		}
		switch c.mode {
		case MatchStartsWith:
			if cmpPrefix == "" || strings.HasPrefix(cmpCand, cmpPrefix) {
				front = append(front, cand)
			}
		case MatchContains:
			if cmpPrefix == "" {
				front = append(front, cand)
				continue
			}
			idx := strings.Index(cmpCand, cmpPrefix)
			if idx == 0 {
				front = append(front, cand)
			} else if idx > 0 {
				back = append(back, cand)
			}
		case MatchAnywhere:
			front = append(front, cand)
		}
	}

	// Sort each bucket alphabetically for stable presentation.
	sort.Strings(front)
	sort.Strings(back)

	out := append(front, back...)
	if c.dedupe {
		out = dedupeStrings(out)
	}
	if c.maxSuggestions > 0 && len(out) > c.maxSuggestions {
		out = out[:c.maxSuggestions]
	}
	c.suggestions = out
	return out
}

// Suggestions returns the most recent Filter result. Empty when no
// Filter has run, or when no candidate matched.
func (c *Completer) Suggestions() []string {
	return c.suggestions
}

// candidateSet picks between Source and Candidates. Source wins when
// both are set so callers can override a pre-baked list with a dynamic
// one without first clearing Candidates.
func (c *Completer) candidateSet() []string {
	if c.Source != nil {
		return c.Source.CandidateAt("")
	}
	return c.Candidates
}

// dedupeStrings removes consecutive duplicates from a sorted slice.
// We rely on the input being sorted by the bucket-then-sort logic in
// Filter so a single linear pass suffices.
func dedupeStrings(in []string) []string {
	if len(in) < 2 {
		return in
	}
	out := in[:1]
	for i := 1; i < len(in); i++ {
		if in[i] != in[i-1] {
			out = append(out, in[i])
		}
	}
	return out
}
