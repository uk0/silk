package gui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/uk0/silk/paint"
)

// ─── QSS-lite Stylesheet ───
//
// A small, well-defined subset of Qt's QSS / CSS used to describe widget
// appearance as declarative rules. The parser turns a source string into a
// StyleSheet; theme/widget code can later consult it via Lookup. Wiring the
// result into Theme and widget drawing is a deliberate follow-up — this file
// only delivers the parser, model, lookup and typed accessors.
//
// Supported grammar (everything else is a parse error for that rule):
//
//	stylesheet := { comment | rule }
//	comment    := "/*" ... "*/"            (may appear between rules; nesting not supported)
//	rule       := selector "{" { decl } "}"
//	selector   := type [ "#" id ] [ ":" state ]
//	type       := ident | "*"               ("*" is the universal type, matches any widget)
//	id         := ident
//	state      := ident                     (e.g. hover, pressed, focus, disabled)
//	decl       := prop ":" value ";"        (trailing ";" optional on the last decl)
//	prop       := ident
//	value      := any text up to ";" or "}" (trimmed; kept verbatim as a string)
//	ident      := [A-Za-z_][A-Za-z0-9_-]*
//
// Notes:
//   - Selectors are case-sensitive; properties are stored verbatim (case-sensitive).
//   - Unknown properties are kept as raw strings (forward-compatible); typed
//     accessors interpret them on demand.
//   - Malformed rules are skipped and collected; ParseStyleSheet returns the
//     successfully-parsed rules together with a non-nil error describing the
//     bad ones. The parser never panics.

// Selector identifies which widgets a Rule applies to.
type Selector struct {
	Type  string // widget type name, or "*" for the universal selector
	ID    string // optional widget id (without the leading '#'); "" if none
	State string // optional pseudo-class state (without the leading ':'); "" if none
}

// String renders the selector back into its QSS-lite source form.
func (s Selector) String() string {
	out := s.Type
	if s.ID != "" {
		out += "#" + s.ID
	}
	if s.State != "" {
		out += ":" + s.State
	}
	return out
}

// Rule is one parsed `selector { ... }` block.
type Rule struct {
	Selector     Selector
	Declarations map[string]string // property name -> raw value string
}

// StyleSheet is an ordered collection of parsed rules.
type StyleSheet struct {
	Rules []Rule
}

// ParseError aggregates the per-rule failures encountered while parsing.
// It implements error so callers can treat it as a normal error value while
// still inspecting the individual problems via Errors.
type ParseError struct {
	Errors []string
}

func (e *ParseError) Error() string {
	if len(e.Errors) == 1 {
		return "stylesheet: " + e.Errors[0]
	}
	return fmt.Sprintf("stylesheet: %d errors: %s", len(e.Errors), strings.Join(e.Errors, "; "))
}

// ParseStyleSheet parses QSS-lite source into a StyleSheet.
//
// Well-formed rules are always collected. If any rule is malformed, the bad
// rules are skipped and a *ParseError is returned alongside the partially
// populated sheet (the sheet is never nil). The parser never panics.
func ParseStyleSheet(src string) (*StyleSheet, error) {
	sheet := &StyleSheet{}
	perr := &ParseError{}

	s := stripComments(src)
	i := 0
	n := len(s)
	for i < n {
		// Skip leading whitespace between rules.
		for i < n && isSpace(s[i]) {
			i++
		}
		if i >= n {
			break
		}

		// A rule body is delimited by the next '{' ... '}'. Find them.
		open := strings.IndexByte(s[i:], '{')
		if open < 0 {
			// Trailing non-whitespace with no block: report and stop.
			rest := strings.TrimSpace(s[i:])
			if rest != "" {
				perr.Errors = append(perr.Errors, fmt.Sprintf("trailing tokens with no rule body: %q", rest))
			}
			break
		}
		open += i
		closeIdx := strings.IndexByte(s[open:], '}')
		if closeIdx < 0 {
			perr.Errors = append(perr.Errors, fmt.Sprintf("unterminated rule (missing '}'): %q", strings.TrimSpace(s[i:])))
			break
		}
		closeIdx += open

		selText := strings.TrimSpace(s[i:open])
		body := s[open+1 : closeIdx]
		i = closeIdx + 1

		sel, err := parseSelector(selText)
		if err != nil {
			perr.Errors = append(perr.Errors, err.Error())
			continue
		}
		decls, derrs := parseDeclarations(body)
		if len(derrs) > 0 {
			for _, de := range derrs {
				perr.Errors = append(perr.Errors, fmt.Sprintf("in rule %q: %s", selText, de))
			}
			// A rule with some bad declarations still contributes its good ones,
			// provided at least one declaration parsed; otherwise skip it.
			if len(decls) == 0 {
				continue
			}
		}
		sheet.Rules = append(sheet.Rules, Rule{Selector: sel, Declarations: decls})
	}

	if len(perr.Errors) > 0 {
		return sheet, perr
	}
	return sheet, nil
}

// stripComments removes /* ... */ spans, replacing each with a single space so
// surrounding tokens never accidentally fuse together.
func stripComments(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if i+1 < len(s) && s[i] == '/' && s[i+1] == '*' {
			end := strings.Index(s[i+2:], "*/")
			if end < 0 {
				// Unterminated comment: drop the remainder.
				break
			}
			b.WriteByte(' ')
			i += end + 4 // skip past "*/"
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// parseSelector parses `Type`, `Type#id`, `Type:state`, `#id`, `Type#id:state`.
// The '#id' and ':state' parts are optional and may appear in either combination,
// but id must precede state when both are present.
func parseSelector(text string) (Selector, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return Selector{}, fmt.Errorf("empty selector")
	}

	var sel Selector

	// Split off the state (last ':') first so a type-less "#id:state" still works.
	if idx := strings.IndexByte(text, ':'); idx >= 0 {
		sel.State = strings.TrimSpace(text[idx+1:])
		text = strings.TrimSpace(text[:idx])
		if !isIdent(sel.State) {
			return Selector{}, fmt.Errorf("invalid state in selector: %q", sel.State)
		}
	}

	// Then split off the id ('#').
	if idx := strings.IndexByte(text, '#'); idx >= 0 {
		sel.ID = strings.TrimSpace(text[idx+1:])
		text = strings.TrimSpace(text[:idx])
		if !isIdent(sel.ID) {
			return Selector{}, fmt.Errorf("invalid id in selector: %q", sel.ID)
		}
	}

	// Whatever remains is the type. Empty type is only allowed when an id was
	// given (a bare "#id" selector); otherwise it defaults to the universal "*".
	text = strings.TrimSpace(text)
	switch {
	case text == "":
		if sel.ID == "" {
			return Selector{}, fmt.Errorf("selector has no type or id")
		}
		sel.Type = "*"
	case text == "*":
		sel.Type = "*"
	case isIdent(text):
		sel.Type = text
	default:
		return Selector{}, fmt.Errorf("invalid type in selector: %q", text)
	}

	return sel, nil
}

// parseDeclarations parses a `prop: value; prop: value` body into a map.
// It returns the parsed declarations plus a slice of human-readable errors for
// any malformed entries (which are skipped).
func parseDeclarations(body string) (map[string]string, []string) {
	decls := make(map[string]string)
	var errs []string

	for _, part := range strings.Split(body, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		colon := strings.IndexByte(part, ':')
		if colon < 0 {
			errs = append(errs, fmt.Sprintf("declaration missing ':': %q", part))
			continue
		}
		prop := strings.TrimSpace(part[:colon])
		val := strings.TrimSpace(part[colon+1:])
		if !isIdent(prop) {
			errs = append(errs, fmt.Sprintf("invalid property name: %q", prop))
			continue
		}
		if val == "" {
			errs = append(errs, fmt.Sprintf("empty value for property %q", prop))
			continue
		}
		decls[prop] = val
	}
	return decls, errs
}

// ─── Lookup ───

// Lookup merges every rule matching (widgetType, id, state) into a single
// property map, ordered by ascending specificity so more specific rules win:
//
//  0. universal type, stateless    ("*")
//  1. concrete type, stateless     (Type)
//  2. universal type + state       ("*:state")
//  3. concrete type + state        (Type:state)
//  4. id, stateless                (#id / Type#id)
//  5. id + state                   (#id:state / Type#id:state)
//
// Within the same specificity tier, later rules in source order override
// earlier ones. Passing "" for id or state simply means those tiers do not
// match. The returned map is always non-nil and safe to mutate.
func (ss *StyleSheet) Lookup(widgetType, id, state string) map[string]string {
	type scored struct {
		spec int
		decl map[string]string
	}
	var matches []scored

	for _, r := range ss.Rules {
		sel := r.Selector

		// Type must match: either universal, or exactly the widget type.
		typeOK := sel.Type == "*" || sel.Type == widgetType
		if !typeOK {
			continue
		}
		// Id, if the selector specifies one, must match the queried id.
		if sel.ID != "" && sel.ID != id {
			continue
		}
		// State, if the selector specifies one, must match the queried state.
		if sel.State != "" && sel.State != state {
			continue
		}

		matches = append(matches, scored{spec: selectorSpecificity(sel), decl: r.Declarations})
	}

	// Apply tiers in ascending specificity; within a tier, source order is
	// preserved so later rules override earlier ones.
	out := make(map[string]string)
	for s := 0; s <= maxSpecificity; s++ {
		for _, m := range matches {
			if m.spec != s {
				continue
			}
			for k, v := range m.decl {
				out[k] = v
			}
		}
	}
	return out
}

const maxSpecificity = 5

// selectorSpecificity ranks a matching selector. Id presence dominates, then
// state presence, then a concrete (non-universal) type as a tiebreaker.
func selectorSpecificity(sel Selector) int {
	spec := 0
	if sel.ID != "" {
		spec += 4
	}
	if sel.State != "" {
		spec += 2
	}
	if sel.Type != "*" {
		spec++
	}
	return spec
}

// ─── Typed accessors ───
//
// These read a single declaration value (already resolved via Lookup or taken
// directly from a Rule) and interpret it as a common type. Each returns the
// parsed value and an ok flag; ok is false when the key is absent or the value
// cannot be interpreted as the requested type.

// Color reads decls[key] as a paint.Color. Hex literals (#RGB, #RGBA, #RRGGBB,
// #RRGGBBAA) and CSS/中文 named colors are supported, delegating to
// paint.ParseColor. ok is false for an absent key, a malformed hex literal, or
// an unrecognised name.
func Color(decls map[string]string, key string) (paint.Color, bool) {
	raw, ok := decls[key]
	if !ok {
		return paint.Color{}, false
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return paint.Color{}, false
	}

	if raw[0] == '#' {
		// paint.ParseColor only accepts these hex lengths; validate the digits
		// ourselves so a bad literal reports ok=false instead of silent black.
		switch len(raw) {
		case 4, 5, 7, 9:
			if !isHexDigits(raw[1:]) {
				return paint.Color{}, false
			}
			return paint.ParseColor(raw), true
		default:
			return paint.Color{}, false
		}
	}

	// Named color: paint.ParseColor returns opaque black both for "black" and
	// for unknown names, so treat the explicit black names as the only valid
	// way to obtain black and otherwise require a non-black resolution.
	c := paint.ParseColor(raw)
	if c == (paint.Color{R: 0, G: 0, B: 0, A: 255}) {
		switch strings.ToLower(raw) {
		case "black", "黑":
			return c, true
		default:
			return paint.Color{}, false
		}
	}
	return c, true
}

// Float reads decls[key] as a float64. A single optional trailing unit token
// such as "px" or "%" is tolerated (e.g. "12px" -> 12). ok is false for an
// absent key or an unparseable value.
func Float(decls map[string]string, key string) (float64, bool) {
	raw, ok := decls[key]
	if !ok {
		return 0, false
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	num := trimUnit(raw)
	f, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

// Int reads decls[key] as an int, tolerating the same optional trailing unit as
// Float. A fractional value (e.g. "1.5") is rejected. ok is false for an absent
// key or an unparseable value.
func Int(decls map[string]string, key string) (int, bool) {
	raw, ok := decls[key]
	if !ok {
		return 0, false
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	num := trimUnit(raw)
	v, err := strconv.Atoi(num)
	if err != nil {
		return 0, false
	}
	return v, true
}

// ─── helpers ───

// trimUnit strips a trailing alphabetic / '%' unit suffix from a numeric token,
// leaving the leading number. "12px" -> "12", "1.5em" -> "1.5", "50%" -> "50".
func trimUnit(s string) string {
	end := len(s)
	for end > 0 {
		c := s[end-1]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '%' {
			end--
			continue
		}
		break
	}
	return strings.TrimSpace(s[:end])
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v'
}

// isIdent reports whether s is a valid identifier: [A-Za-z_][A-Za-z0-9_-]*.
func isIdent(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '_':
			// always allowed
		case c >= '0' && c <= '9', c == '-':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func isHexDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}
