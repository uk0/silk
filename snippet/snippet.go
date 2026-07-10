// Package snippet implements Qt Creator / TextMate / LSP style snippet
// expansion. A snippet body is a template string containing tab-stop
// placeholders; Expand turns it into the final text plus a list of stops
// recording where the editor caret should land.
//
// Placeholder syntax:
//
//	$N            a bare tab stop with index N and no default (empty)
//	${N:default}  a tab stop with index N and a default value
//	${N}          a tab stop with index N and an empty default
//	$0 / ${0:..}  the final caret position; always sorted last
//	\$            an escaped literal dollar sign (not a placeholder)
//	\\            an escaped literal backslash
//	\}            a literal brace, useful inside a ${N:default}
//
// Offsets in Stop are BYTE offsets into Expansion.Text. Byte offsets keep
// slicing (Text[Start:End]) correct for multibyte UTF-8 defaults without any
// rune/byte conversion at the call site.
package snippet

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Stop is a single tab stop landed in the expanded text. Start and End are
// byte offsets into Expansion.Text delimiting the region occupied by the
// placeholder's default (Start == End when the default is empty). A snippet
// may contain several stops with the same Index (mirrors); each is recorded
// separately.
type Stop struct {
	Index   int
	Start   int
	End     int
	Default string
}

// Expansion is the result of expanding a template: the final Text and the
// Stops sorted by Index ascending with every index-0 stop ($0, the final
// caret) placed last. Stops sharing an index keep their appearance order.
type Expansion struct {
	Text  string
	Stops []Stop
}

// Expand parses template, replacing each placeholder with its default (empty
// for a bare $N) and recording a Stop for every placeholder. It returns an
// error for a malformed ${...} group: one with no closing brace, a
// non-numeric index, an out-of-range index, or a stray character where ':'
// or '}' was expected. A bare '$' not followed by a digit or '{' is a literal
// dollar, as is an escaped \$.
func Expand(template string) (Expansion, error) {
	var b strings.Builder
	var stops []Stop
	i, n := 0, len(template)
	for i < n {
		c := template[i]
		switch {
		case c == '\\':
			// A backslash escapes $, \ or } into a literal; before any other
			// byte (or at end of input) the backslash itself is literal.
			if i+1 < n {
				if nx := template[i+1]; nx == '$' || nx == '\\' || nx == '}' {
					b.WriteByte(nx)
					i += 2
					continue
				}
			}
			b.WriteByte('\\')
			i++
		case c == '$' && i+1 < n && template[i+1] == '{':
			idx, def, consumed, err := parseBraced(template, i)
			if err != nil {
				return Expansion{}, err
			}
			start := b.Len()
			b.WriteString(def)
			stops = append(stops, Stop{Index: idx, Start: start, End: b.Len(), Default: def})
			i += consumed
		case c == '$':
			j := i + 1
			for j < n && template[j] >= '0' && template[j] <= '9' {
				j++
			}
			if j == i+1 {
				// Lone '$' (not a placeholder): emit it literally.
				b.WriteByte('$')
				i++
				continue
			}
			idx, err := strconv.Atoi(template[i+1 : j])
			if err != nil {
				return Expansion{}, fmt.Errorf("snippet: tab-stop index at offset %d: %w", i, err)
			}
			pos := b.Len()
			stops = append(stops, Stop{Index: idx, Start: pos, End: pos, Default: ""})
			i = j
		default:
			// Ordinary byte, including UTF-8 lead/continuation bytes (all
			// >= 0x80, so never mistaken for a metacharacter): copy verbatim.
			b.WriteByte(c)
			i++
		}
	}

	sort.SliceStable(stops, func(a, b int) bool { return stopLess(stops[a], stops[b]) })
	return Expansion{Text: b.String(), Stops: stops}, nil
}

// parseBraced parses a ${N} or ${N:default} group. s[i]=='$' and s[i+1]=='{'
// are guaranteed by the caller. It returns the index, decoded default, the
// number of bytes consumed (from i), and an error if the group is malformed.
func parseBraced(s string, i int) (index int, def string, consumed int, err error) {
	n := len(s)
	k := i + 2 // first byte after "${"
	ds := k
	for k < n && s[k] >= '0' && s[k] <= '9' {
		k++
	}
	if k == ds {
		return 0, "", 0, fmt.Errorf("snippet: malformed placeholder at offset %d: expected numeric index after %q", i, "${")
	}
	index, err = strconv.Atoi(s[ds:k])
	if err != nil {
		return 0, "", 0, fmt.Errorf("snippet: placeholder index at offset %d: %w", i, err)
	}
	if k >= n {
		return 0, "", 0, fmt.Errorf("snippet: unterminated placeholder at offset %d: missing %q", i, "}")
	}
	switch s[k] {
	case '}':
		return index, "", (k + 1) - i, nil
	case ':':
		k++ // consume ':'
	default:
		return 0, "", 0, fmt.Errorf("snippet: malformed placeholder at offset %d: expected %q or %q", i, ":", "}")
	}
	var d strings.Builder
	for k < n {
		c := s[k]
		if c == '\\' {
			if k+1 < n {
				if nx := s[k+1]; nx == '$' || nx == '\\' || nx == '}' {
					d.WriteByte(nx)
					k += 2
					continue
				}
			}
			d.WriteByte('\\')
			k++
			continue
		}
		if c == '}' {
			return index, d.String(), (k + 1) - i, nil
		}
		d.WriteByte(c)
		k++
	}
	return 0, "", 0, fmt.Errorf("snippet: unterminated placeholder at offset %d: missing %q", i, "}")
}

// stopLess orders stops by Index ascending, with every index-0 stop ($0, the
// final caret) after all others. Equal-key stops compare equal so a stable
// sort preserves their appearance order (mirrors, repeated $0).
func stopLess(a, b Stop) bool {
	if a.Index == 0 || b.Index == 0 {
		if a.Index == 0 && b.Index == 0 {
			return false
		}
		return b.Index == 0 // a precedes b only when b is the index-0 stop
	}
	return a.Index < b.Index
}

// Template is a named, trigger-keyed snippet: typing Trigger and expanding
// requests Body. Name is a human label for a picker UI.
type Template struct {
	Name    string
	Trigger string
	Body    string
}

// NewBook indexes templates by Trigger for lookup by a typed trigger word.
// When several templates share a trigger the last one wins.
func NewBook(ts []Template) map[string]Template {
	m := make(map[string]Template, len(ts))
	for _, t := range ts {
		m[t.Trigger] = t
	}
	return m
}
