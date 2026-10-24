package main

import (
	"context"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

// UI-thread owned, single-flight and latest-wins: rapid F12 presses retain at
// most one worker, one pending job and one posted completion, even during a
// slow didChange flush. Workers never read widgets or openEditors.
type navigationJob struct {
	run     func(context.Context) ([]core.LSPLocation, error)
	deliver func([]core.LSPLocation, error)
}
type navigationQueue struct {
	pending    *navigationJob
	running    bool
	generation uint64
	cancel     context.CancelFunc
	post       func(func())
}

func (q *navigationQueue) submit(job navigationJob) {
	q.generation++
	q.pending = &job
	if q.cancel != nil {
		q.cancel()
	}
	q.start()
}

func (q *navigationQueue) stop() {
	q.generation++
	q.pending = nil
	if q.cancel != nil {
		q.cancel()
	}
}

func (q *navigationQueue) start() {
	if q.running || q.pending == nil {
		return
	}
	job, generation := q.pending, q.generation
	q.pending, q.running = nil, true
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	q.cancel = cancel
	post := q.post
	if post == nil {
		post = gui.Post
	}
	go func() {
		locations, err := job.run(ctx)
		cancel()
		post(func() {
			q.running, q.cancel = false, nil
			if generation == q.generation {
				job.deliver(locations, err)
			}
			q.start()
		})
	}()
}

var symbolNavigation navigationQueue

type navigationSnapshot struct {
	tabs         *gui.TabWidget
	editor       *gui.CodeEditor
	client       *core.LSPClient
	document     *lspDocument
	path         string
	revision     uint64
	line, column int
}

func (s navigationSnapshot) current() bool {
	return globalLSP == s.client && openEditors[s.path] == s.editor &&
		activeEditor(s.tabs) == s.editor && documentMode(s.tabs, nil) == modeEdit &&
		lspDocuments[s.path] == s.document && !fileRenameAffects(s.path) &&
		s.editor.TextRevision() == s.revision && s.editor.CursorLine() == s.line && s.editor.CursorCol() == s.column
}

func requestSymbolNavigation(tabs *gui.TabWidget, operation string) {
	if activeEditor(tabs) != nil && documentMode(tabs, nil) != modeEdit {
		return
	}
	if globalLSP == nil {
		reportLSPUnavailable()
		return
	}
	ed := activeEditor(tabs)
	path := activeEditorPath(tabs)
	if ed == nil || !isGoFile(path) || fileRenameAffects(path) || documentMode(tabs, nil) != modeEdit {
		return
	}
	s := navigationSnapshot{tabs: tabs, editor: ed, client: globalLSP, document: lspDocuments[path],
		path: path, revision: ed.TextRevision(), line: ed.CursorLine(), column: ed.CursorCol()}
	column16 := ed.CursorUTF16Col()
	uri := fileURIOf(path)
	panel := globalReferencesPanel
	if panel != nil {
		panel.SetLocations(nil)
		panel.SetFilter("")
		panel.SetQuery(i18n.T(operation))
		panel.SetLoading(true)
		panel.SigCancel(func() { symbolNavigation.stop(); panel.SetLoading(false) })
	}
	symbolNavigation.submit(navigationJob{
		run: func(ctx context.Context) ([]core.LSPLocation, error) {
			if s.document != nil {
				if _, err := s.document.sync(); err != nil {
					return nil, err
				}
			}
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			switch operation {
			case "Go to Type Definition":
				return s.client.TypeDefinitionContext(ctx, uri, s.line, column16)
			case "Go to Implementation":
				return s.client.ImplementationContext(ctx, uri, s.line, column16)
			case "Find References":
				return s.client.ReferencesContext(ctx, uri, s.line, column16, true)
			default:
				return s.client.DefinitionContext(ctx, uri, s.line, column16)
			}
		},
		deliver: func(locs []core.LSPLocation, err error) {
			if panel != globalReferencesPanel {
				return
			}
			if panel != nil {
				panel.SetLoading(false)
			}
			if !s.current() {
				return
			}
			if err != nil {
				reportLSPRequestError(i18n.T(operation), path, err)
				return
			}
			locs = navigableLocations(locs)
			if len(locs) == 0 {
				silkideToast(i18n.T("No navigation targets found"), gui.ToastInfo)
				return
			}
			if operation != "Find References" && len(locs) == 1 {
				openFileInEditorAtLSP(tabs, uriToPath(locs[0].URI), locs[0].Range.Start)
				return
			}
			if panel == nil {
				return
			}
			refs := make([]ged.ReferenceLoc, 0, len(locs))
			for _, loc := range locs {
				p := uriToPath(loc.URI)
				refs = append(refs, lspReferenceLocation(p, loc.Range.Start, referenceSourceLine(p, loc.Range.Start.Line)))
			}
			panel.SetLocations(refs)
			revealWorkbenchTool(globalBottomDock, panel)
		},
	})
}

// Preserve server order while suppressing repeated targets. Reject non-file
// URIs/negative ranges instead of accidentally opening an empty path at line 0.
func navigableLocations(locs []core.LSPLocation) []core.LSPLocation {
	type key struct {
		path      string
		line, col int
	}
	seen := map[key]bool{}
	out := make([]core.LSPLocation, 0, len(locs))
	for _, loc := range locs {
		p := uriToPath(loc.URI)
		start := loc.Range.Start
		k := key{canonicalEditorPath(p), start.Line, start.Character}
		if k.path == "" || k.line < 0 || k.col < 0 || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, loc)
	}
	return out
}
