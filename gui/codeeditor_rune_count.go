package gui

import "unicode/utf8"

// trackLineRuneDelta records the change to a set of complete line bodies
// before writing it. Count complete bodies, not inserted/deleted substrings:
// a byte edit can split or join malformed UTF-8 at the splice boundary. LF
// contributes one rune between any two bodies, including blank rows.
func (e *CodeEditor) trackLineRuneDelta(before, after []string) {
	e.textDirty = true
	if e.runeDeltaUnknown {
		return
	}

	// Bound work for very long lines and bulk changes. Falling back to the
	// joined buffer avoids decoding a large replacement twice.
	remaining := 4 << 10
	for _, lines := range [2][]string{before, after} {
		for _, line := range lines {
			remaining -= len(line) + 1
			if remaining < 0 {
				e.runeDeltaUnknown = true
				return
			}
		}
	}
	e.pendingRuneDelta += len(after) - len(before)
	for _, line := range before {
		e.pendingRuneDelta -= utf8.RuneCountInString(line)
	}
	for _, line := range after {
		e.pendingRuneDelta += utf8.RuneCountInString(line)
	}
}
