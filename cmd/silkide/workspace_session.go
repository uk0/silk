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
	if w := globalWorkbench; w != nil && w.tabs == tabs && w.restore != nil {
		r := w.restore
		if !r.closed && r.designPending && !r.designChanged && r.canvas == canvas && r.design != nil && r.design.Filename() == r.designInitialFilename {
			state.Design = r.state.Design
		}
	}
	if tabs == nil {
		return state
	}
	for i := 0; i < tabs.Count(); i++ {
		page := tabs.Stack().Page(i)
		if pending, ok := page.(*restorePage); ok {
			if !pending.closed && openEditors[pending.saved.Path] == nil {
				state.Editors = append(state.Editors, pending.saved)
				if tabs.CurrentIndex() == i {
					state.ActiveFile = pending.saved.Path
				}
			}
			continue
		}
		ed, ok := page.(*gui.CodeEditor)
		if !ok || openEditors[ed.FilePath()] != ed {
			continue
		}
		path := ed.FilePath()
		scroll := ed.ScrollY()
		if math.IsNaN(scroll) || math.IsInf(scroll, 0) || scroll < 0 {
			scroll = 0
		}
		state.Editors = append(state.Editors, editorSession{path, ed.CursorLine(), ed.CursorCol(), scroll})
	}
	return state
}

func restoreWorkspaceSession(state workspaceSession, tabs *gui.TabWidget, canvas *ged.GedView) *workspaceRestore {
	return newWorkspaceRestore(state, tabs, canvas, workspaceRestoreOptions{})
}
