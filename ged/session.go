package ged

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/uk0/silk/core"
)

// SessionState captures the designer's persistent working state across runs,
// similar to Qt Creator's session files. It remembers which files are open,
// which file is active, the current mode, and the window geometry.
type SessionState struct {
	LastMode     int      `json:"last_mode"`     // 0 = design, 1 = edit/code
	OpenFiles    []string `json:"open_files"`    // file paths open in editor tabs
	ActiveFile   string   `json:"active_file"`   // currently active file path
	LastProject  string   `json:"last_project"`  // last opened .silk/.form project file
	WindowWidth  int      `json:"window_width"`  // restored on next launch
	WindowHeight int      `json:"window_height"` // restored on next launch
}

// sessionFilePath returns the absolute path to the session state JSON file.
func sessionFilePath() string {
	return filepath.Join(core.LocalDataDir(), "session.json")
}

// SaveSession persists the given state to disk as JSON.
// The target directory is created by core at init time, so the write itself
// is the only failure path worth surfacing.
func SaveSession(state SessionState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(sessionFilePath(), data, 0644)
}

// LoadSession reads the previously saved session state. If no file exists
// (first run) or the file is malformed, a zero SessionState is returned and
// the caller should treat it as "nothing to restore".
func LoadSession() SessionState {
	var state SessionState
	data, err := os.ReadFile(sessionFilePath())
	if err != nil {
		return state
	}
	// Ignore unmarshal errors -- a stale file should never crash startup.
	_ = json.Unmarshal(data, &state)
	return state
}
