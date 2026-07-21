package gui

import (
	"github.com/uk0/silk/snippet"
	"strings"
)

// snippetRange is a byte-offset span [Start,End) into the editor buffer that a
// tab stop occupies. Start==End marks a zero-width caret position (a bare $N or
// the final $0); otherwise the span covers the placeholder's default text.
type snippetRange struct {
	Start int
	End   int
}

// snippetSession tracks Tab navigation over a snippet.Expansion that was spliced
// into the editor buffer at a fixed byte offset. Its steps follow the
// Expansion's order (index ascending with $0 last); stops that share an Index
// (mirrors) are grouped into one step and selected together. cur is the current
// step, -1 before the first Next().
//
// Offsets are captured once at creation. This minimal integration does not
// re-track edits, so navigating stops is reliable as long as the caller Tabs
// through them; typing content into a placeholder shifts later offsets, which
// is why the editor ends the session once it advances past the final stop.
type snippetSession struct {
	steps [][]snippetRange
	cur   int
}

// newSnippetSession builds a session from exp, whose Stop offsets are relative
// to the inserted text, shifting every offset by insByte — the byte offset in
// the buffer where exp.Text was inserted. Consecutive stops sharing an Index
// (mirrors, kept adjacent by Expand's stable index sort) collapse into one step.
// Returns nil when exp carries no stops.
func newSnippetSession(exp snippet.Expansion, insByte int) *snippetSession {
	if len(exp.Stops) == 0 {
		return nil
	}
	var steps [][]snippetRange
	for i := 0; i < len(exp.Stops); {
		idx := exp.Stops[i].Index
		var group []snippetRange
		j := i
		for j < len(exp.Stops) && exp.Stops[j].Index == idx {
			s := exp.Stops[j]
			group = append(group, snippetRange{Start: s.Start + insByte, End: s.End + insByte})
			j++
		}
		steps = append(steps, group)
		i = j
	}
	return &snippetSession{steps: steps, cur: -1}
}

// Active reports whether a further Tab has a stop to advance to. It is true from
// creation until Next() has landed on the final step ($0 / the last stop), after
// which the editor falls back to its default Tab handling.
func (s *snippetSession) Active() bool {
	if s == nil || len(s.steps) == 0 {
		return false
	}
	return s.cur < len(s.steps)-1
}

// Next advances to the following tab stop and returns its range(s) — more than
// one for a mirrored stop — with ok=true. At or past the last stop it does not
// advance and returns (nil, false).
func (s *snippetSession) Next() ([]snippetRange, bool) {
	if s == nil || s.cur+1 >= len(s.steps) {
		return nil, false
	}
	s.cur++
	return s.steps[s.cur], true
}

// Prev moves back to the preceding tab stop and returns its range(s) with
// ok=true. At the first stop it does not move and returns (nil, false).
func (s *snippetSession) Prev() ([]snippetRange, bool) {
	if s == nil || s.cur <= 0 {
		return nil, false
	}
	s.cur--
	return s.steps[s.cur], true
}

// expandSnippetBody prefixes indent to every body line after the first (so the
// expansion lines up under the trigger column), runs snippet.Expand, and builds
// a session whose stops are shifted to insByte. It returns the expanded text to
// splice into the buffer and the session (nil when the body has no stops).
func expandSnippetBody(body, indent string, insByte int) (string, *snippetSession, error) {
	lines := strings.Split(body, "\n")
	for i := 1; i < len(lines); i++ {
		lines[i] = indent + lines[i]
	}
	exp, err := snippet.Expand(strings.Join(lines, "\n"))
	if err != nil {
		return "", nil, err
	}
	return exp.Text, newSnippetSession(exp, insByte), nil
}

// selectSnippetRanges moves the caret to a tab stop's primary (first) range,
// selecting its default text when the range is non-empty and collapsing to a
// bare caret when it is zero-width ($0 / bare $N). Extra mirrored ranges are
// left in place by this minimal integration; the session still tracks them.
func (this *CodeEditor) selectSnippetRanges(rs []snippetRange) {
	if len(rs) == 0 {
		return
	}
	r := rs[0]
	sl, sc := cursorForByteOffset(this.lines, r.Start)
	if r.End > r.Start {
		el, ec := cursorForByteOffset(this.lines, r.End)
		this.selStartLine, this.selStartCol = sl, sc
		this.selEndLine, this.selEndCol = el, ec
		this.hasSelection = true
		this.cursorLine, this.cursorCol = el, ec
	} else {
		this.clearSelection()
		this.cursorLine, this.cursorCol = sl, sc
	}
	this.ensureCursorVisible()
	this.Self().Update()
}
