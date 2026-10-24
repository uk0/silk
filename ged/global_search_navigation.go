package ged

// CanNavigateMatches reports whether F4-style source navigation is available.
// A replacement preview has different row actions and is deliberately excluded.
// UI-thread only, like other panel methods.
func (p *GlobalSearchPanel) CanNavigateMatches() bool {
	return !p.closed && !p.applying && !p.searching && p.searchReady && p.preview == nil && len(p.results) != 0
}

// NavigateMatch opens the next/previous source hit, wrapping at either end and
// expanding its file group if necessary. It does no I/O; the host validates the
// original line and byte column through SigOpenMatch. At most the bounded result
// set is traversed, not the source files. Returns false for inactive results.
func (p *GlobalSearchPanel) NavigateMatch(previous bool) bool {
	if !p.CanNavigateMatches() {
		return false
	}
	// Use file/line/byte-column identity: visible row indexes change on collapse.
	current := -1
	if p.selectedIdx >= 0 && p.selectedIdx < len(p.flatRows) {
		row := p.flatRows[p.selectedIdx]
		for i, m := range p.results {
			if row.kind == searchRowMatch && row.match != nil && m.FilePath == row.match.FilePath && m.Line == row.match.Line && m.Column == row.match.Column {
				current = i
				break
			}
			if row.kind == searchRowFile && m.FilePath == row.filePath {
				// Next enters the group; Previous moves to the preceding file's last hit.
				current = i
				if !previous {
					current--
				}
				break
			}
		}
	}
	target := current + 1
	if previous {
		target = current - 1
		if current == -1 {
			target = len(p.results) - 1
		}
	}
	target = (target + len(p.results)) % len(p.results)
	hit := p.results[target]
	if p.collapsed[hit.FilePath] {
		p.collapsed[hit.FilePath] = false
		p.rebuildFlatRows()
	}
	for i, row := range p.flatRows {
		if row.match != nil && row.match.FilePath == hit.FilePath && row.match.Line == hit.Line && row.match.Column == hit.Column {
			p.selectedIdx = i
			p.moveRow(0) // Scroll to the actual result, including off-screen groups.
			p.activateRow(i)
			return true
		}
	}
	return false
}
