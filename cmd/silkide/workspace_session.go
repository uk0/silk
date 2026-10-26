package main

import (
	"encoding/json"
	"math"

	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
)

const workspaceSessionVersion = 1

type editorSession struct {
	Path    string  `json:"path"`
	Line    int     `json:"line"`
	Column  int     `json:"column"`
	ScrollY float64 `json:"scroll_y"`
}

type workspaceSession struct {
	Version    int             `json:"version"`
	Design     string          `json:"design,omitempty"`
	Editors    []editorSession `json:"editors,omitempty"`
	ActiveFile string          `json:"active_file,omitempty"`
	Mode       workspaceMode   `json:"mode"`
}

func (s workspaceSession) paths() []string {
	var paths []string
	if s.Design != "" {
		paths = append(paths, s.Design)
	}
	for _, ed := range s.Editors {
		paths = append(paths, ed.Path)
	}
	return paths
}

func (p *preferences) WorkspaceSession() (workspaceSession, bool) {
	var state workspaceSession
	err := json.Unmarshal([]byte(p.store.String("session/workspace", "")), &state)
	return state, err == nil && state.Version == workspaceSessionVersion
}

func (p *preferences) SetWorkspaceSession(state workspaceSession) {
	encoded, err := json.Marshal(state)
	if err != nil {
		return
	}
	_ = p.store.SetString("session/workspace", string(encoded))
	// Keep the old list readable by earlier releases and fallback recovery.
	_ = p.store.SetStringList("session/open", state.paths())
	_ = p.requestSave()
}

func captureWorkspaceSession(tabs *gui.TabWidget, canvas *ged.GedView) workspaceSession {
	state := workspaceSession{Version: workspaceSessionVersion, Mode: documentMode(tabs, canvas), ActiveFile: activeEditorPath(tabs)}
	if canvas != nil && canvas.GedScene() != nil {
		state.Design = canvas.GedScene().Filename()
	}
	for _, path := range orderedEditorPaths(tabs) {
		ed := openEditors[path]
		if ed == nil {
			continue
		}
		scroll := ed.ScrollY()
		if math.IsNaN(scroll) || math.IsInf(scroll, 0) || scroll < 0 {
			scroll = 0
		}
		state.Editors = append(state.Editors, editorSession{path, ed.CursorLine(), ed.CursorCol(), scroll})
	}
	return state
}

func restoreWorkspaceSession(state workspaceSession, tabs *gui.TabWidget, canvas *ged.GedView) {
	if len(existingPaths([]string{state.Design})) != 0 {
		openDocument(state.Design, tabs, canvas, nil, false)
	}
	for _, saved := range state.Editors {
		if len(existingPaths([]string{saved.Path})) == 0 || !loadFileInEditor(tabs, saved.Path) {
			continue
		}
		ed := activeEditor(tabs)
		ed.ScrollToLineCol(saved.Line, saved.Column)
		if saved.ScrollY >= 0 && !math.IsNaN(saved.ScrollY) && !math.IsInf(saved.ScrollY, 0) {
			ed.SetScrollY(saved.ScrollY)
		}
	}
	if ed := openEditors[state.ActiveFile]; ed != nil {
		focusEditorTab(tabs, ed)
	}
	if w := globalWorkbench; w != nil && w.tabs == tabs {
		w.activate(state.Mode)
	}
}
