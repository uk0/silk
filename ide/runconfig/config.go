// Package runconfig models named run/debug configurations for a project —
// the equivalent of Qt Creator's "Run Settings", where one project owns
// several launch targets (run the app, debug it, run the package tests)
// and one of them is the active default.
//
// A Config is a pure value: it says what to launch (Target), how
// (Mode), with which toolchain (KitName), what to build first (PreRun)
// and in which environment (Args / WorkDir / Env). It never launches
// anything itself — the IDE host turns a Config into a process. Store
// owns the named collection and its two invariants: names are unique,
// and at most one Config carries the Default flag.
package runconfig

import (
	"errors"
	"sort"
	"strings"
)

// Launch modes. Mode selects which command shape the host builds for a
// configuration: run the target, run it under the debugger, run its
// tests, or run its benchmarks.
const (
	ModeRun   = "run"
	ModeDebug = "debug"
	ModeTest  = "test"
	ModeBench = "bench"
)

// Validation errors. Callers can match these with errors.Is.
var (
	ErrEmptyName     = errors.New("runconfig: configuration name is empty")
	ErrEmptyTarget   = errors.New("runconfig: configuration target is empty")
	ErrInvalidMode   = errors.New("runconfig: unknown launch mode")
	ErrDuplicateName = errors.New("runconfig: duplicate configuration name")
	ErrNotFound      = errors.New("runconfig: configuration not found")
)

// Config is one named launch configuration.
type Config struct {
	Name    string            `json:"name"`              // unique within a Store; shown in the configuration list
	Target  string            `json:"target"`            // package path ("." / "./cmd/app") or a single file
	Args    []string          `json:"args,omitempty"`    // command-line arguments passed to the target
	WorkDir string            `json:"workDir,omitempty"` // process working directory; empty means the project root
	Env     map[string]string `json:"env,omitempty"`     // extra environment variables
	Mode    string            `json:"mode,omitempty"`    // one of the Mode* constants; empty means ModeRun
	KitName string            `json:"kit,omitempty"`     // toolchain / kit to build and launch with; empty means the default kit
	PreRun  []string          `json:"preRun,omitempty"`  // task names to run before launching (e.g. a build task)
	Default bool              `json:"default,omitempty"` // the configuration launched when none is named
}

// Modes returns the launch modes in UI order. Used by editors that cycle
// or list the mode field.
func Modes() []string {
	return []string{ModeRun, ModeDebug, ModeTest, ModeBench}
}

// ValidMode reports whether mode is one of the four launch modes. The
// empty string is NOT a valid mode here — Validate accepts it separately
// as "unset, treat as ModeRun".
func ValidMode(mode string) bool {
	switch mode {
	case ModeRun, ModeDebug, ModeTest, ModeBench:
		return true
	}
	return false
}

// Validate reports whether the configuration is launchable: it needs a
// name (it is the Store key and the list label) and a target (there is
// nothing to launch without one). An empty Mode is accepted and means
// ModeRun; any other unknown mode is rejected.
func (c Config) Validate() error {
	if strings.TrimSpace(c.Name) == "" {
		return ErrEmptyName
	}
	if strings.TrimSpace(c.Target) == "" {
		return ErrEmptyTarget
	}
	if c.Mode != "" && !ValidMode(c.Mode) {
		return ErrInvalidMode
	}
	return nil
}

// EffectiveMode returns Mode, substituting ModeRun when it is unset.
func (c Config) EffectiveMode() string {
	if c.Mode == "" {
		return ModeRun
	}
	return c.Mode
}

// Clone deep-copies the configuration so the caller can mutate the
// result without touching the original's slices or map.
func (c Config) Clone() Config {
	out := c
	if c.Args != nil {
		out.Args = make([]string, len(c.Args))
		copy(out.Args, c.Args)
	}
	if c.PreRun != nil {
		out.PreRun = make([]string, len(c.PreRun))
		copy(out.PreRun, c.PreRun)
	}
	if c.Env != nil {
		out.Env = make(map[string]string, len(c.Env))
		for k, v := range c.Env {
			out.Env[k] = v
		}
	}
	return out
}

// EnvSlice returns Env as "KEY=value" entries — the form os/exec wants —
// sorted by key so callers get a stable order out of the map. Returns
// nil when Env is empty.
func (c Config) EnvSlice() []string {
	if len(c.Env) == 0 {
		return nil
	}
	keys := make([]string, 0, len(c.Env))
	for k := range c.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+c.Env[k])
	}
	return out
}

// ParseEnv is the inverse of EnvSlice: it turns "KEY=value" entries into
// a map. Blank entries are dropped, surrounding whitespace is trimmed,
// an entry with no '=' becomes a key with an empty value, and a repeated
// key keeps the last value. Returns nil when nothing survives.
func ParseEnv(lines []string) map[string]string {
	out := make(map[string]string, len(lines))
	for _, l := range lines {
		s := strings.TrimSpace(l)
		if s == "" {
			continue
		}
		if i := strings.Index(s, "="); i >= 0 {
			out[s[:i]] = s[i+1:]
			continue
		}
		out[s] = ""
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
