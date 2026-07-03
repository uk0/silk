package ged

import (
	"encoding/json"
	"github.com/uk0/silk/core"
	"os"
	"path/filepath"
	"sync"
)

// KeyBinding describes a single command-to-shortcut mapping.
//
// Command is a stable identifier (for example "file.new" or "editor.format").
// Key is a human-readable chord string (for example "Ctrl+Shift+F"). Context
// restricts where the binding applies: "global" (anywhere in the IDE) or
// "editor" (only when the code editor has focus).
type KeyBinding struct {
	Command string `json:"command"`
	Key     string `json:"key"`
	Context string `json:"context"`
}

// KeyMap is the runtime registry of keyboard shortcuts. It is safe for use
// from multiple goroutines (the mutex guards reads as well as writes so
// callers can iterate Bindings() consistently during a live edit).
type KeyMap struct {
	mu       sync.Mutex
	bindings []KeyBinding
}

// defaultKeymap defines the factory-default shortcut set. Reset() reverts to
// a deep copy of this slice. Keep it in sync with the UI documentation in
// ged/shortcuts-panel.go.
var defaultKeymap = []KeyBinding{
	{Command: "file.new", Key: "Ctrl+N", Context: "global"},
	{Command: "file.open", Key: "Ctrl+O", Context: "global"},
	{Command: "file.save", Key: "Ctrl+S", Context: "global"},
	{Command: "file.saveAs", Key: "Ctrl+Shift+S", Context: "global"},
	{Command: "edit.undo", Key: "Ctrl+Z", Context: "global"},
	{Command: "edit.redo", Key: "Ctrl+Y", Context: "global"},
	{Command: "edit.cut", Key: "Ctrl+X", Context: "global"},
	{Command: "edit.copy", Key: "Ctrl+C", Context: "global"},
	{Command: "edit.paste", Key: "Ctrl+V", Context: "global"},
	{Command: "edit.selectAll", Key: "Ctrl+A", Context: "global"},
	{Command: "view.design", Key: "Ctrl+1", Context: "global"},
	{Command: "view.code", Key: "Ctrl+2", Context: "global"},
	{Command: "view.zoomIn", Key: "Ctrl+=", Context: "global"},
	{Command: "view.zoomOut", Key: "Ctrl+-", Context: "global"},
	{Command: "run.compile", Key: "F5", Context: "global"},
	{Command: "run.preview", Key: "Ctrl+R", Context: "global"},
	{Command: "editor.find", Key: "Ctrl+F", Context: "editor"},
	{Command: "editor.format", Key: "Ctrl+Shift+F", Context: "editor"},
	{Command: "editor.gotoSymbol", Key: "Ctrl+Shift+O", Context: "editor"},
	{Command: "editor.gotoLine", Key: "Ctrl+G", Context: "editor"},
	{Command: "editor.comment", Key: "Ctrl+/", Context: "editor"},
	{Command: "editor.duplicateLine", Key: "Ctrl+D", Context: "editor"},
	{Command: "editor.bookmark", Key: "Ctrl+B", Context: "editor"},
	{Command: "editor.nextBookmark", Key: "F2", Context: "editor"},
	{Command: "editor.prevBookmark", Key: "Shift+F2", Context: "editor"},
	{Command: "editor.rename", Key: "Ctrl+Shift+R", Context: "editor"},
	{Command: "editor.completion", Key: "Ctrl+Space", Context: "editor"},
	{Command: "editor.addCursorAbove", Key: "Ctrl+Alt+Up", Context: "editor"},
	{Command: "editor.addCursorBelow", Key: "Ctrl+Alt+Down", Context: "editor"},
	{Command: "editor.navBack", Key: "Alt+Left", Context: "editor"},
	{Command: "editor.navForward", Key: "Alt+Right", Context: "editor"},
	{Command: "design.nextWidget", Key: "Tab", Context: "global"},
	{Command: "design.prevWidget", Key: "Shift+Tab", Context: "global"},
	{Command: "design.alignLeft", Key: "Alt+L", Context: "global"},
	{Command: "design.alignRight", Key: "Alt+R", Context: "global"},
	{Command: "design.alignTop", Key: "Alt+T", Context: "global"},
	{Command: "design.alignBottom", Key: "Alt+B", Context: "global"},
	{Command: "design.distributeH", Key: "Alt+H", Context: "global"},
	{Command: "design.distributeV", Key: "Alt+V", Context: "global"},
	{Command: "debug.perfOverlay", Key: "F12", Context: "global"},
}

// globalKeymap is the process-wide singleton. It is lazily loaded from disk
// on first access via LoadKeymap().
var (
	globalKeymapMu sync.Mutex
	globalKeymap   *KeyMap
)

// KeymapFilePath returns the on-disk location of the user's keymap override.
// Writing to this file is how user customizations persist across sessions.
func KeymapFilePath() string {
	return filepath.Join(core.LocalDataDir(), "keymap.json")
}

// LoadKeymap returns the shared, process-wide keymap. If a persisted keymap
// file exists at KeymapFilePath() it is merged over the defaults, otherwise
// defaults are used verbatim. Subsequent calls return the same instance.
func LoadKeymap() *KeyMap {
	globalKeymapMu.Lock()
	defer globalKeymapMu.Unlock()
	if globalKeymap != nil {
		return globalKeymap
	}
	km := &KeyMap{}
	km.bindings = make([]KeyBinding, len(defaultKeymap))
	copy(km.bindings, defaultKeymap)

	data, err := os.ReadFile(KeymapFilePath())
	if err == nil && len(data) > 0 {
		var user []KeyBinding
		if json.Unmarshal(data, &user) == nil {
			// Merge: any user-defined binding overrides defaults by Command.
			idx := make(map[string]int, len(km.bindings))
			for i, b := range km.bindings {
				idx[b.Command] = i
			}
			for _, u := range user {
				if i, ok := idx[u.Command]; ok {
					km.bindings[i].Key = u.Key
					if u.Context != "" {
						km.bindings[i].Context = u.Context
					}
				} else {
					km.bindings = append(km.bindings, u)
					idx[u.Command] = len(km.bindings) - 1
				}
			}
		}
	}
	globalKeymap = km
	return km
}

// SaveKeymap persists the entire binding list to KeymapFilePath(). The file
// is written atomically-ish via a straightforward WriteFile; this is
// adequate for single-user desktop use.
func SaveKeymap(km *KeyMap) error {
	km.mu.Lock()
	defer km.mu.Unlock()
	data, err := json.MarshalIndent(km.bindings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(KeymapFilePath(), data, 0644)
}

// Get returns the key chord string bound to command, or "" if no such
// command is known. Thread-safe.
func (k *KeyMap) Get(command string) string {
	k.mu.Lock()
	defer k.mu.Unlock()
	for _, b := range k.bindings {
		if b.Command == command {
			return b.Key
		}
	}
	return ""
}

// Set updates the key chord for command. If command is not yet registered,
// a new binding is appended with Context="global". Thread-safe.
func (k *KeyMap) Set(command, key string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	for i := range k.bindings {
		if k.bindings[i].Command == command {
			k.bindings[i].Key = key
			return
		}
	}
	k.bindings = append(k.bindings, KeyBinding{Command: command, Key: key, Context: "global"})
}

// Reset restores the factory-default bindings, discarding any user overrides.
// The caller is responsible for calling SaveKeymap() afterwards to persist.
func (k *KeyMap) Reset() {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.bindings = make([]KeyBinding, len(defaultKeymap))
	copy(k.bindings, defaultKeymap)
}

// Bindings returns a snapshot of the binding list. The returned slice is
// safe for the caller to read; modifying it does not affect the KeyMap.
func (k *KeyMap) Bindings() []KeyBinding {
	k.mu.Lock()
	defer k.mu.Unlock()
	out := make([]KeyBinding, len(k.bindings))
	copy(out, k.bindings)
	return out
}

// Len returns the number of configured bindings. Thread-safe.
func (k *KeyMap) Len() int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return len(k.bindings)
}
