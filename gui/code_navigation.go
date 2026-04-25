package gui

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// isActionModifier returns true if the platform's primary action modifier is held.
// macOS: Command key (Super/LWin)
// Windows/Linux: Ctrl key
func isActionModifier() bool {
	if runtime.GOOS == "darwin" {
		return IsKeyDown(KeyLWin) || IsKeyDown(KeyRWin)
	}
	return IsKeyDown(KeyCtrl)
}

// NavigationTarget represents a found definition location.
type NavigationTarget struct {
	FilePath string
	Line     int
	Column   int
	Name     string
	Kind     string // "func", "type", "var", "method"
}

// NavPosition records a cursor position for back/forward navigation.
type NavPosition struct {
	FilePath string
	Line     int
	Column   int
	ScrollY  float64
}

// NavigationStack provides back/forward navigation history.
type NavigationStack struct {
	stack   []NavPosition
	current int // points to the current position (-1 = empty)
}

// Push adds a new position to the stack, discarding any forward history.
func (ns *NavigationStack) Push(pos NavPosition) {
	if ns.current < len(ns.stack)-1 {
		ns.stack = ns.stack[:ns.current+1]
	}
	ns.stack = append(ns.stack, pos)
	ns.current = len(ns.stack) - 1
}

// CanGoBack returns true if there is a previous position in history.
func (ns *NavigationStack) CanGoBack() bool {
	return ns.current > 0
}

// GoBack moves to the previous position and returns it.
func (ns *NavigationStack) GoBack() (NavPosition, bool) {
	if ns.current <= 0 {
		return NavPosition{}, false
	}
	ns.current--
	return ns.stack[ns.current], true
}

// CanGoForward returns true if there is a next position in history.
func (ns *NavigationStack) CanGoForward() bool {
	return ns.current < len(ns.stack)-1
}

// GoForward moves to the next position and returns it.
func (ns *NavigationStack) GoForward() (NavPosition, bool) {
	if ns.current >= len(ns.stack)-1 {
		return NavPosition{}, false
	}
	ns.current++
	return ns.stack[ns.current], true
}

// FindDefinition searches for the definition of an identifier.
// First searches the current file content, then other .go files in the same
// directory, then walks up to find a gui/ package directory.
func FindDefinition(word string, currentFile string, currentContent string) *NavigationTarget {
	if word == "" {
		return nil
	}

	// 1. Search current file content first
	target := findInContent(word, currentFile, currentContent)
	if target != nil {
		return target
	}

	// 2. Search other .go files in the same directory
	dir := filepath.Dir(currentFile)
	if dir != "" && dir != "." {
		entries, err := os.ReadDir(dir)
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
					continue
				}
				path := filepath.Join(dir, entry.Name())
				if path == currentFile {
					continue
				}
				content, err := os.ReadFile(path)
				if err != nil {
					continue
				}
				target := findInContent(word, path, string(content))
				if target != nil {
					return target
				}
			}
		}
	}

	// 3. Search gui/ package directory (for framework API navigation)
	guiDir := findGuiPackageDir(currentFile)
	if guiDir != "" && guiDir != dir {
		entries, err := os.ReadDir(guiDir)
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
					continue
				}
				path := filepath.Join(guiDir, entry.Name())
				content, err := os.ReadFile(path)
				if err != nil {
					continue
				}
				target := findInContent(word, path, string(content))
				if target != nil {
					return target
				}
			}
		}
	}

	return nil
}

// findInContent searches for a definition of word within the given file content.
func findInContent(word, filePath, content string) *NavigationTarget {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// func FuncName( ... or func (receiver) MethodName( ...
		if strings.HasPrefix(trimmed, "func ") {
			rest := trimmed[5:]
			kind := "func"
			// Check for method: func (r *Type) Name(
			if strings.HasPrefix(rest, "(") {
				closeP := strings.Index(rest, ")")
				if closeP >= 0 {
					rest = strings.TrimSpace(rest[closeP+1:])
					kind = "method"
				}
			}
			// rest now starts with the function/method name
			nameEnd := strings.IndexAny(rest, "( \t")
			if nameEnd > 0 {
				name := rest[:nameEnd]
				if name == word {
					return &NavigationTarget{FilePath: filePath, Line: i, Name: name, Kind: kind}
				}
			}
		}

		// type TypeName struct/interface/...
		if strings.HasPrefix(trimmed, "type ") {
			parts := strings.Fields(trimmed)
			if len(parts) >= 2 && parts[1] == word {
				return &NavigationTarget{FilePath: filePath, Line: i, Name: word, Kind: "type"}
			}
		}

		// var VarName ... or const ConstName ...
		if strings.HasPrefix(trimmed, "var ") || strings.HasPrefix(trimmed, "const ") {
			parts := strings.Fields(trimmed)
			if len(parts) >= 2 {
				// Strip trailing type annotation or '='
				name := strings.TrimRight(parts[1], "=")
				if name == word {
					return &NavigationTarget{FilePath: filePath, Line: i, Name: word, Kind: "var"}
				}
			}
		}
	}
	return nil
}

// findGuiPackageDir walks up from the current file's directory looking for
// a gui/ sibling directory (the framework's GUI package).
func findGuiPackageDir(currentFile string) string {
	dir := filepath.Dir(currentFile)
	for i := 0; i < 5; i++ {
		guiDir := filepath.Join(dir, "gui")
		if info, err := os.Stat(guiDir); err == nil && info.IsDir() {
			return guiDir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}
