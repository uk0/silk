package gui

import "testing"

func TestEditorInteractionsRetireHoverIdentity(t *testing.T) {
	for name, act := range map[string]func(*CodeEditor){
		"leave":      func(e *CodeEditor) { e.OnMouseLeave() },
		"escape":     func(e *CodeEditor) { e.OnKeyDown(KeyEsc, false) },
		"typing":     func(e *CodeEditor) { e.OnTextInput("x") },
		"click":      func(e *CodeEditor) { e.OnLeftDown(100, 50) },
		"scroll":     func(e *CodeEditor) { e.OnMouseWheel(100, 50, 1) },
		"replace":    func(e *CodeEditor) { e.ReplaceAllText("package changed\n") },
		"focus-loss": func(e *CodeEditor) { e.OnFocusChanged(NewEdit(), e) },
		"navigation": func(e *CodeEditor) { e.NavigateToLineCol(0, 1) },
	} {
		t.Run(name, func(t *testing.T) {
			e := NewCodeEditor()
			e.SetSize(400, 300)
			e.SetText("package p\n")
			e.hoverReqLine, e.hoverReqCol = 0, 8
			e.hoverErrorLine = 0
			token := e.LanguageHintRevision()
			act(e)
			if e.hoverReqLine != -1 || e.hoverReqCol != -1 || e.hoverErrorLine != -1 || token == e.LanguageHintRevision() {
				t.Fatal("interaction left the old hover identifier live")
			}
		})
	}
}
