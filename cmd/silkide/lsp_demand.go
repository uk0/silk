package main

import (
	"os"
	"path/filepath"
)

const lspStandby = "standby"

// All demand and phase changes are serialized on the UI thread. Do not
// repeatedly respawn a missing or failed tool on each open; Restart LSP is
// the explicit recovery path. start() synchronously enters "starting" before
// any filesystem/process work moves to its worker.
func startStandbyLSP(root string, start func(string)) bool {
	if lspActivityPhase != lspStandby || globalLSP != nil || lspStartCancel != nil {
		return false
	}
	start(root)
	return true
}

func demandLSPForEditor(path string, start func(string)) bool {
	if !isGoFile(path) || openEditors[path] == nil {
		return false
	}
	root := globalProjectRoot
	if root == "" {
		root = filepath.Dir(path)
	}
	return startStandbyLSP(root, start)
}

// For commands that need Go symbols without an open source editor. The worker
// walks to the enclosing module; don't do that filesystem walk on the UI.
func lspWorkspaceHint() string {
	if globalProjectRoot != "" {
		return globalProjectRoot
	}
	if w := globalWorkbench; w != nil && w.canvas != nil {
		if s := w.canvas.GedScene(); s != nil && s.Filename() != "" {
			return filepath.Dir(s.Filename())
		}
	}
	cwd, _ := os.Getwd()
	return cwd
}
