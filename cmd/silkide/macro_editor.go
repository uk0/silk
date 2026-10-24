package main

import (
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/macro"
)

type macroSnapshot struct {
	text     string
	revision uint64
}

// One bounded worker per document: no GUI reads, latest-snapshot replacement,
// and no goroutine per keystroke. Close retires queued/in-flight delivery.
type macroWorker struct {
	mu               sync.Mutex
	latest           macroSnapshot
	closed           bool
	wake, stop, done chan struct{}
	analyze          func(string, []byte) macro.Analysis
	deliver          func(macroSnapshot, macro.Analysis)
	path             string
}

func newMacroWorker(path string, deliver func(macroSnapshot, macro.Analysis)) *macroWorker {
	w := &macroWorker{path: path, wake: make(chan struct{}, 1), stop: make(chan struct{}), done: make(chan struct{}), analyze: macro.Analyze, deliver: deliver}
	go w.run()
	return w
}
func (w *macroWorker) update(s macroSnapshot) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	w.latest = s
	select {
	case w.wake <- struct{}{}:
	default:
	}
}
func (w *macroWorker) close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.closed {
		w.closed = true
		w.latest = macroSnapshot{}
		close(w.stop)
	}
}
func (w *macroWorker) run() {
	defer close(w.done)
	for {
		select {
		case <-w.stop:
			return
		case <-w.wake:
		}
		timer := time.NewTimer(90 * time.Millisecond)
	quiet:
		for {
			select {
			case <-w.stop:
				timer.Stop()
				return
			case <-w.wake:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(90 * time.Millisecond)
			case <-timer.C:
				break quiet
			}
		}
		w.mu.Lock()
		s, closed := w.latest, w.closed
		w.mu.Unlock()
		if closed {
			return
		}
		result := w.analyze(w.path, []byte(s.text))
		w.mu.Lock()
		current := !w.closed && w.latest.revision == s.revision
		w.mu.Unlock()
		if current && w.deliver != nil {
			w.deliver(s, result)
		}
	}
}

type macroEditorState struct {
	worker   *macroWorker
	editor   *gui.CodeEditor
	revision uint64
	analysis macro.Analysis
}

var macroEditors = map[string]*macroEditorState{} // UI thread only
func isMacroFile(path string) bool                { return strings.EqualFold(filepath.Ext(path), ".silkgo") }
func updateMacroEditor(tabs *gui.TabWidget, path string, ed *gui.CodeEditor) {
	if !isMacroFile(path) || ed == nil || fileRenameAffects(path) {
		return
	}
	state := macroEditors[path]
	if state == nil || state.editor != ed {
		closeMacroEditor(path)
		state = &macroEditorState{editor: ed}
		macroEditors[path] = state
		state.worker = newMacroWorker(path, func(s macroSnapshot, a macro.Analysis) {
			gui.Post(func() {
				if macroEditors[path] != state || openEditors[path] != ed || ed.TextRevision() != s.revision {
					return
				}
				state.revision, state.analysis = s.revision, a
				applyMacroAnalysis(ed, s, a)
				if globalProblems != nil {
					globalProblems.SetProblems(ged.MergeProblems(globalProblems.Problems(), macroProblemSource(path), macroProblems(path, s.text, a)))
				}
				if activeEditor(tabs) == ed {
					syncOutlineToActiveEditor(tabs)
				}
			})
		})
	}
	// Clear stale markers immediately; do not leave an old error attached to a
	// newly inserted line while waiting for the debounced result.
	ed.SetErrors(nil)
	if globalProblems != nil {
		globalProblems.SetProblems(ged.MergeProblems(globalProblems.Problems(), macroProblemSource(path), nil))
	}
	state.worker.update(macroSnapshot{text: ed.Text(), revision: ed.TextRevision()})
}
func closeMacroEditor(path string) {
	if state := macroEditors[path]; state != nil {
		state.worker.close()
		delete(macroEditors, path)
	}
	if globalProblems != nil {
		globalProblems.SetProblems(ged.MergeProblems(globalProblems.Problems(), macroProblemSource(path), nil))
	}
}
func macroProblemSource(path string) ged.ProblemSource {
	return ged.ProblemSource{Kind: ged.ProblemMacro, Doc: path}
}

type sourcePositions struct {
	text   string
	starts []int
}

func positionsFor(text string) sourcePositions {
	p := sourcePositions{text: text, starts: []int{0}}
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			p.starts = append(p.starts, i+1)
		}
	}
	return p
}
func (p sourcePositions) at(offset int) (line, col int) {
	offset = max(0, min(offset, len(p.text)))
	line = sort.Search(len(p.starts), func(i int) bool { return p.starts[i] > offset }) - 1
	return line, utf8.RuneCountInString(p.text[p.starts[line]:offset])
}
func applyMacroAnalysis(ed *gui.CodeEditor, s macroSnapshot, a macro.Analysis) {
	p := positionsFor(s.text)
	var highlights []gui.SyntaxHighlight
	for _, h := range a.Highlights {
		line, col := p.at(h.Start)
		endLine, endCol := p.at(h.End)
		if line != endLine {
			continue
		}
		kind := gui.SyntaxKeyword
		switch h.Kind {
		case macro.HighlightMacro:
			kind = gui.SyntaxFunction
		case macro.HighlightParameter:
			kind = gui.SyntaxType
		}
		highlights = append(highlights, gui.SyntaxHighlight{Line: line, StartCol: col, EndCol: endCol, Kind: kind})
	}
	ed.SetSyntaxHighlights(s.revision, highlights)
	var symbols []gui.CodeSymbol
	for _, sym := range a.Symbols {
		if sym.Kind == macro.SymbolMacro {
			line, _ := p.at(sym.Selection.Start)
			symbols = append(symbols, gui.CodeSymbol{Name: sym.Name, Kind: gui.SymMacro, Line: line, Detail: sym.Detail})
		}
	}
	ed.SetAdditionalSymbols(s.revision, symbols)
	errors := map[int]string{}
	for _, d := range a.Diagnostics {
		line := max(0, d.Location.Line-1)
		errors[line] = d.Error() + "\n" + d.Hint
	}
	if s.revision == ed.TextRevision() {
		ed.SetErrors(errors)
	}
}
func macroProblems(path, text string, a macro.Analysis) []ged.Problem {
	var rows []ged.Problem
	p := positionsFor(text)
	for _, d := range a.Diagnostics {
		line, col := p.at(d.Location.Offset)
		// The IDE's shared location callback takes zero-based rune columns.
		rows = append(rows, ged.Problem{File: path, Line: line + 1, Col: col, Severity: ged.SeverityError, Message: string(d.Code) + ": " + d.Message + " — " + d.Hint})
	}
	return rows
}
func goToMacroDefinition(tabs *gui.TabWidget) bool {
	path := activeEditorPath(tabs)
	ed := activeEditor(tabs)
	if !isMacroFile(path) || ed == nil {
		return false
	}
	// Explicit navigation resolves the current buffer, never the old index.
	a, p, offset := macroAnalysisAtCursor(ed, path)
	if sym, ok := a.DefinitionAt(offset); ok {
		l, c := p.at(sym.Selection.Start)
		openFileInEditorAt(tabs, path, l+1, c)
	} else {
		silkideToast("No macro definition at cursor", gui.ToastInfo)
	}
	return true
}

func retireMacroEditors() {
	for path, state := range macroEditors {
		state.worker.close()
		delete(macroEditors, path)
	}
}
func macroAnalysisAtCursor(ed *gui.CodeEditor, path string) (macro.Analysis, sourcePositions, int) {
	text := ed.Text()
	a := macro.Analysis{}
	if state := macroEditors[path]; state != nil && state.revision == ed.TextRevision() {
		a = state.analysis
	} else {
		a = macro.Analyze(path, []byte(text))
	}
	p := positionsFor(text)
	line := ed.CursorLine()
	r := []rune(ed.Lines()[line])
	offset := p.starts[line] + len(string(r[:ed.CursorCol()]))
	return a, p, offset
}
func findMacroReferences(tabs *gui.TabWidget) bool {
	path := activeEditorPath(tabs)
	ed := activeEditor(tabs)
	if !isMacroFile(path) || ed == nil {
		return false
	}
	a, p, offset := macroAnalysisAtCursor(ed, path)
	sym, ok := a.DefinitionAt(offset)
	var refs []ged.ReferenceLoc
	if ok {
		add := func(span macro.Span, kind ged.RefKind) {
			line, col := p.at(span.Start)
			refs = append(refs, ged.ReferenceLoc{File: path, Line: line + 1, Col: col, Preview: strings.TrimSpace(ed.Lines()[line]), Kind: kind})
		}
		add(sym.Selection, ged.RefDeclaration)
		for _, r := range a.References {
			if r.Target >= 0 && r.Target < len(a.Symbols) && a.Symbols[r.Target].Selection == sym.Selection {
				add(r.Span, ged.RefRead)
			}
		}
	}
	if globalReferencesPanel != nil {
		globalReferencesPanel.SetQuery(sym.Name)
		globalReferencesPanel.SetFilter("")
		globalReferencesPanel.SetReferences(refs)
		revealWorkbenchTool(globalBottomDock, globalReferencesPanel)
	}
	return true
}

func requestMacroCompletion(path string, ed *gui.CodeEditor) bool {
	if !isMacroFile(path) || ed == nil {
		return false
	}
	line, col := ed.CursorLine(), ed.CursorCol()
	runes := []rune(ed.Lines()[line])
	prefix := string(runes[:col])
	command := "//silk:expand "
	trimmed := strings.TrimLeft(prefix, " \t")
	if !strings.HasPrefix(trimmed, command) || strings.ContainsAny(strings.TrimPrefix(trimmed, command), "( \t") {
		ed.ClearExternalCompletions()
		return true
	}
	a, p, offset := macroAnalysisAtCursor(ed, path)
	typed := strings.TrimPrefix(trimmed, command)
	start := offset - len(typed)
	text, revision := ed.Text(), ed.TextRevision()
	var items []gui.ExternalCompletion
	for _, s := range a.Symbols {
		if s.Kind != macro.SymbolMacro {
			continue
		}
		symbol := s
		items = append(items, gui.ExternalCompletion{Label: s.Name, Detail: s.Detail, Apply: func(target *gui.CodeEditor) {
			if target != ed || target.TextRevision() != revision || target.CursorLine() != line || target.CursorCol() != col {
				return
			}
			replacement := text[:start] + symbol.Name + text[offset:]
			l, c := p.at(start)
			target.ReplaceAllTextAt(replacement, l, c+utf8.RuneCountInString(symbol.Name))
		}})
	}
	ed.SetExternalCompletions(items)
	ed.TriggerCompletion()
	return true
}
