package gui

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const menuTypeaheadTimeout = time.Second
const menuTypeaheadLimit = 64

// OnTextInput selects a menu label without executing it. Enter/Space retain
// their existing confirmation semantics. Committed Unicode text (including IME
// input) follows the same path; a timeout is checked only on the next event,
// never by polling, a timer, or an idle repaint.
func (m *Menu) OnTextInput(text string) { m.typeaheadInput(text, time.Now()) }

func (m *Menu) resetTypeahead() {
	m.typePrefix = ""
	m.typeAt = time.Time{}
}

func (m *Menu) typeaheadInput(text string, now time.Time) {
	if !m.IsPopup() || !m.IsVisible() || !m.IsAllAncentorsVisible() || !m.IsEnabled() {
		return
	}
	if now.Sub(m.typeAt) >= menuTypeaheadTimeout {
		m.resetTypeahead()
	}
	consumed := 0
	for _, r := range text {
		if consumed == menuTypeaheadLimit {
			break
		}
		consumed++
		// Space confirms through OnKeyDown; its subsequent text event must not
		// begin another search. Control/invalid input never arms an action.
		if r == utf8.RuneError || !unicode.IsPrint(r) || unicode.IsSpace(r) {
			continue
		}
		if utf8.RuneCountInString(m.typePrefix) >= menuTypeaheadLimit {
			break
		}
		next := string(r)
		prefix := m.typePrefix + next
		continuing := m.typePrefix != ""
		// A repeated single-letter query cycles through matching commands.
		if strings.EqualFold(m.typePrefix, next) {
			prefix, continuing = next, false
		}
		idx := m.prefixIndex(prefix, continuing)
		if idx < 0 && continuing {
			// A failed extension starts a new query at the most recent character.
			prefix, idx = next, m.prefixIndex(next, false)
		}
		if idx >= 0 {
			m.typePrefix, m.typeAt = prefix, now
			m.HideAllSubs()
			m.setHighlight(idx)
		} else {
			m.resetTypeahead()
		}
	}
}

// prefixIndex reads the live labels/enabled state, so dynamic menus never need
// a second cache to invalidate. Work is bounded by menu size and 64 runes.
func (m *Menu) prefixIndex(prefix string, includeCurrent bool) int {
	n := len(m.items)
	if n == 0 || prefix == "" {
		return -1
	}
	start := m.highlight
	if start < 0 || start >= n {
		start = -1
		includeCurrent = false
	}
	if !includeCurrent {
		start++
	}
	runes := utf8.RuneCountInString(prefix)
	for offset := 0; offset < n; offset++ {
		idx := (start + offset) % n
		b, ok := m.items[idx].(*Button)
		if !ok || !b.IsVisible() || !b.IsEnabled() {
			continue
		}
		label := strings.TrimLeftFunc(b.Text(), unicode.IsSpace)
		end, count := 0, 0
		for count < runes && end < len(label) {
			_, size := utf8.DecodeRuneInString(label[end:])
			end += size
			count++
		}
		if count == runes && strings.EqualFold(label[:end], prefix) {
			return idx
		}
	}
	return -1
}

func (m *Menu) backspaceTypeahead(now time.Time) {
	if now.Sub(m.typeAt) >= menuTypeaheadTimeout {
		m.resetTypeahead()
		return
	}
	if m.typePrefix == "" {
		return
	}
	_, size := utf8.DecodeLastRuneInString(m.typePrefix)
	m.typePrefix = m.typePrefix[:len(m.typePrefix)-size]
	m.typeAt = now
	if idx := m.prefixIndex(m.typePrefix, true); idx >= 0 {
		m.setHighlight(idx)
	}
}
