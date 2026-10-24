package ged

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/uk0/silk/filesearch"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/internal/filetxn"
)

var ErrPartialSearch = errors.New("search is incomplete; narrow filters before replacing")

type globalSearchRequest struct {
	root, query, replacement string
	options                  filesearch.ContextOptions
	generation               uint64
	preview                  bool
}
type globalSearchResult struct {
	request      globalSearchRequest
	report       filesearch.SearchResult
	matches      []SearchMatch
	replacements []FileReplacement
	err          error
}
type GlobalSearchSnapshot struct {
	Query, Root, Status                              string
	Pending, Ready, Complete, PreviewReady, Applying bool
	Matches, Files                                   int
	Err                                              error
}

func (p *GlobalSearchPanel) Snapshot() GlobalSearchSnapshot {
	return GlobalSearchSnapshot{Query: p.query, Root: p.rootDir, Status: p.statusText(), Pending: p.searching, Ready: p.searchReady, Complete: p.searchReady && p.report.Complete(), PreviewReady: p.preview != nil, Applying: p.applying, Matches: len(p.results), Files: len(p.fileOrder), Err: p.searchErr}
}
func computeGlobalSearch(ctx context.Context, r globalSearchRequest) (out globalSearchResult) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out.request = r
	out.report, out.err = filesearch.SearchContext(ctx, r.root, r.query, r.options)
	for _, m := range out.report.Matches {
		out.matches = append(out.matches, SearchMatch{FilePath: m.Path, Line: m.Line, Column: m.Col - 1, LineText: m.Text, MatchLen: m.Length})
	}
	out.report.Matches = nil
	sort.Slice(out.matches, func(i, j int) bool {
		a, b := out.matches[i], out.matches[j]
		if a.FilePath != b.FilePath {
			return a.FilePath < b.FilePath
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Column < b.Column
	})
	if ctx.Err() != nil {
		out.err = ctx.Err()
		out.matches = nil
	}
	if !r.preview || out.err != nil {
		return
	}
	if !out.report.Complete() {
		out.err = ErrPartialSearch
		return
	}
	total := 0
	last := ""
	for _, m := range out.matches {
		if m.FilePath == last {
			continue
		}
		last = m.FilePath
		if out.err = ctx.Err(); out.err != nil {
			return
		}
		snap, err := filetxn.ReadLimit(m.FilePath, 8<<20)
		if err != nil {
			out.err = err
			return
		}
		if !snap.Exists() {
			out.err = fmt.Errorf("file disappeared: %s", m.FilePath)
			return
		}
		before := string(snap.Bytes())
		total += len(before)
		if total >= 32<<20 {
			out.err = filesearch.ErrReplacementLimit
			return
		}
		after, n, err := filesearch.ReplaceTextContext(ctx, before, r.query, r.replacement, r.options, (32<<20)-total)
		if err != nil {
			out.err = err
			return
		}
		total += len(after)
		if n > 0 && before != after {
			out.replacements = append(out.replacements, FileReplacement{Path: m.FilePath, Before: before, After: after, Count: n})
		}
	}
	return
}
func (p *GlobalSearchPanel) requestOptions() filesearch.ContextOptions {
	return filesearch.ContextOptions{Options: filesearch.Options{Regex: p.optRegex, IgnoreCase: !p.optCaseSense, Include: append([]string(nil), p.include...), SkipBinary: true}, WholeWord: p.optWholeWord, Exclude: append([]string(nil), p.exclude...)}
}
func (p *GlobalSearchPanel) invalidateSearch() {
	p.searchGeneration++
	if p.worker != nil {
		p.worker.cancel()
	}
	p.searching, p.searchReady = false, false
	p.searchErr = nil
	p.notice = ""
	p.preview = nil
	p.results = nil
	p.grouped = make(map[string][]SearchMatch)
	p.fileOrder = nil
	p.flatRows = nil
	p.report = filesearch.SearchResult{}
	p.selectedIdx, p.hoverIdx = -1, -1
	p.scrollY = 0
	p.syncControls()
	p.Update()
}
func (p *GlobalSearchPanel) startSearch(preview bool) {
	if p.applying || p.closed {
		return
	}
	p.invalidateSearch()
	if p.query == "" || p.rootDir == "" {
		return
	}
	p.searching = true
	p.previewing = preview
	r := globalSearchRequest{root: p.rootDir, query: p.query, replacement: string(p.replaceRunes), options: p.requestOptions(), generation: p.searchGeneration, preview: preview}
	if p.worker == nil {
		post := p.post
		if post == nil {
			post = gui.Post
		}
		p.worker = newGlobalSearchWorker(post, p.acceptSearchResult)
	}
	p.worker.request(r)
	p.syncControls()
	p.Update()
}
func (p *GlobalSearchPanel) acceptSearchResult(r globalSearchResult) {
	if p.closed || r.request.generation != p.searchGeneration || r.request.root != p.rootDir || r.request.query != p.query || !p.searching {
		return
	}
	p.searching = false
	p.searchErr = r.err
	p.report = r.report
	p.searchReady = r.err == nil || errors.Is(r.err, filesearch.ErrSearchLimit)
	if p.searchReady {
		p.applyResults(r.matches)
		p.rebuildFlatRows()
	}
	if r.request.preview && r.err == nil {
		p.preview = r.replacements
		if len(p.preview) == 0 {
			p.notice = "No changes to apply"
		} else {
			p.notice = "Preview ready: click a file to inspect its diff; Apply commits to disk"
		}
	}
	p.syncControls()
	p.Layout()
	p.Update()
}

// CancelSearch cancels compute work, not a disk transaction that already started.
func (p *GlobalSearchPanel) CancelSearch() {
	if p.applying {
		return
	}
	p.invalidateSearch()
	p.notice = "Search cancelled"
	p.syncControls()
	p.Update()
}

// CloseSearch must run before owner teardown. Hosts must defer closing while
// IsApplying is true; a started transaction is allowed to finish and report back.
func (p *GlobalSearchPanel) CloseSearch() {
	if p.applying {
		return
	}
	p.CancelSearch()
	p.closed = true
	if p.worker != nil {
		p.worker.close()
	}
	if p.controls != nil {
		p.controls.tooltips(false)
	}
}
func (p *GlobalSearchPanel) IsApplying() bool { return p.applying }
func (p *GlobalSearchPanel) OnHide() {
	if p.searching {
		p.CancelSearch()
	}
	if p.controls != nil {
		p.controls.tooltips(false)
	}
}
func (p *GlobalSearchPanel) OnShow() { p.syncControls() }

// FocusSearch focuses the real child input, including standard selection/paste.
func (p *GlobalSearchPanel) FocusSearch() {
	p.ensureControls()
	p.controls.query.SetFocus()
	p.controls.query.SelectAll()
}

// SigOpenMatch includes the exact byte column and original line for a host to
// validate against its current (possibly unsaved) buffer before navigation.
func (p *GlobalSearchPanel) SigOpenMatch(fn func(SearchMatch)) { p.cbOpenMatch = fn }

// SigPreviewReplacement opens a read-only diff; it must not apply the proposal.
func (p *GlobalSearchPanel) SigPreviewReplacement(fn func(FileReplacement)) { p.cbPreview = fn }

// SetReplacementGuard reserves host buffers on the UI thread before dispatch.
// The returned release callback always runs on the UI thread after commit,
// including failure. The host should protect save/close/rename while reserved.
func (p *GlobalSearchPanel) SetReplacementGuard(fn func([]FileReplacement) (func(error), error)) {
	p.prepareReplace = fn
}
func (p *GlobalSearchPanel) PreviewReplacements() {
	if p.searchReady && p.report.Complete() && !p.applying {
		p.startSearch(true)
	}
}

var globalSearchWritePermit = make(chan struct{}, 1)

func (p *GlobalSearchPanel) ApplyPreview() {
	if p.applying || len(p.preview) == 0 || !p.searchReady || !p.report.Complete() {
		return
	}
	reps := append([]FileReplacement(nil), p.preview...)
	var release func(error)
	if p.prepareReplace != nil {
		var err error
		release, err = p.prepareReplace(reps)
		if err != nil {
			p.searchErr = err
			p.syncControls()
			p.Update()
			return
		}
	}
	p.applying = true
	p.syncControls()
	p.Update()
	post := p.post
	if post == nil {
		post = gui.Post
	}
	go func() {
		globalSearchWritePermit <- struct{}{}
		err := commitSearchReplacements(reps)
		<-globalSearchWritePermit
		post(func() {
			if release != nil {
				release(err)
			}
			p.applying = false
			p.preview = nil
			if err == nil {
				if p.cbReplaced != nil {
					for _, r := range reps {
						p.cbReplaced(r.Path, r.After)
					}
				}
				p.startSearch(false)
			} else {
				p.searchErr = err
				p.notice = "Replacement failed; check error before retrying"
				p.syncControls()
				p.Update()
			}
		})
	}()
}
func (p *GlobalSearchPanel) statusText() string {
	if p.applying {
		return "Writing replacement transaction..."
	}
	if p.searching {
		return "Searching disk..."
	}
	if p.searchErr != nil && !errors.Is(p.searchErr, filesearch.ErrSearchLimit) {
		return p.searchErr.Error()
	}
	if p.notice != "" {
		return p.notice
	}
	if !p.searchReady {
		return "Disk search: Enter to search"
	}
	status := fmt.Sprintf("%d matches in %d files", len(p.results), len(p.fileOrder))
	if !p.report.Complete() {
		status += fmt.Sprintf(" (partial: %d large files, %d long lines, %d read errors)", p.report.SkippedLargeFiles, p.report.SkippedLongLines, p.report.ReadErrors)
		if p.report.Truncated {
			status += "; result/resource limit"
		}
	}
	return status
}
func (p *GlobalSearchPanel) activateRow(index int) {
	if index < 0 || index >= len(p.flatRows) {
		return
	}
	p.selectedIdx = index
	row := p.flatRows[index]
	if p.preview != nil && p.cbPreview != nil {
		for _, r := range p.preview {
			if r.Path == row.filePath {
				p.cbPreview(r)
				return
			}
		}
	}
	if row.kind == searchRowFile {
		p.collapsed[row.filePath] = !p.collapsed[row.filePath]
		p.rebuildFlatRows()
	} else if row.match != nil {
		if p.cbOpenMatch != nil {
			p.cbOpenMatch(*row.match)
		} else if p.cbOpen != nil {
			p.cbOpen(row.match.FilePath, row.match.Line)
		}
	}
	p.Update()
}
func (p *GlobalSearchPanel) moveRow(step int) {
	if len(p.flatRows) == 0 {
		return
	}
	p.selectedIdx = max(0, min(len(p.flatRows)-1, p.selectedIdx+step))
	top := float64(p.selectedIdx) * p.rowHeight
	view := max(0, p.Height()-p.listTop())
	if top < p.scrollY {
		p.scrollY = top
	}
	if top+p.rowHeight > p.scrollY+view {
		p.scrollY = max(0, top+p.rowHeight-view)
	}
	p.Update()
}
