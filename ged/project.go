package ged

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/uk0/silk/core"
)

// Project is one silk project: the directory holding go.mod, plus every
// .silkui design beneath it.
//
// There is deliberately no project file. go.mod already answers "where does
// this project begin" for every Go toolchain the designer shells out to, and a
// .silkproj carrying the same answer would be a second source of truth that
// can disagree with the first.
type Project struct {
	Root    string   // absolute path of the directory holding go.mod
	Module  string   // module path go.mod declares; "" when it could not be parsed
	Designs []string // absolute .silkui paths under Root, sorted
}

// IsDesignPath reports whether path names a SilkUI design. It is the one rule
// the project scan and the file tree's double-click routing share, so a file
// the project generates code for is exactly a file the tree opens on the
// canvas. Extension comparison is case-insensitive because the designer's own
// save path is the only thing that normalises the case, and files arrive here
// from the filesystem.
func IsDesignPath(path string) bool {
	return strings.EqualFold(filepath.Ext(path), DefaultDesignExt)
}

// DesignOutputs names the two Go files a design owns in its own directory.
//
// The split is the point: Generated carries the widget tree and is rewritten
// on every generate, Companion is hand-written (main(), handlers) and is never
// touched by the designer. Nothing writes Companion today — the generator
// still emits one self-contained half — but the mapping is what the split
// generator swaps into.
type DesignOutputs struct {
	Generated string // "<stem>.silk.go"
	Companion string // "<stem>.go"
}

// OutputPathsFor maps a design onto the Go files that belong beside it. Pure:
// the paths are derived from the design path alone, so callers can report
// where output will land without touching the disk.
func OutputPathsFor(designPath string) DesignOutputs {
	if designPath == "" {
		return DesignOutputs{}
	}
	dir := filepath.Dir(designPath)
	stem := strings.TrimSuffix(filepath.Base(designPath), filepath.Ext(designPath))
	return DesignOutputs{
		Generated: filepath.Join(dir, stem+".silk.go"),
		Companion: filepath.Join(dir, stem+".go"),
	}
}

// FindProjectRoot returns the directory of the nearest go.mod at or above
// startDir. An error means startDir is outside any Go module, which is the
// same as saying it is not in a project.
func FindProjectRoot(startDir string) (string, error) {
	path, ok := core.FindGoMod(startDir)
	if !ok {
		return "", fmt.Errorf("no go.mod at or above %s", startDir)
	}
	return filepath.Dir(path), nil
}

// ScanProject resolves the project containing startDir and lists its designs.
func ScanProject(startDir string) (*Project, error) {
	root, err := FindProjectRoot(startDir)
	if err != nil {
		return nil, err
	}
	p := &Project{Root: root}
	// A go.mod that parses only partially still yields the module line, which
	// is all the project needs; the settings panel is where parse errors are
	// the user's problem.
	if gm, err := core.LoadGoMod(root); err == nil && gm != nil {
		p.Module = gm.Module
	}
	p.Designs = scanDesigns(root)
	return p, nil
}

// scanDesigns walks root for .silkui files.
//
// A subdirectory holding its own go.mod is a different project and is skipped
// whole — silk itself ships mod/map that way, and without this the outer
// project would claim the inner module's designs and generate code carrying
// the wrong module path. Noise directories are the same set the file tree
// hides, so the project list and the tree never disagree about what the
// project contains.
func scanDesigns(root string) []string {
	var out []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// An unreadable subtree is skipped rather than failing the scan:
			// one permission-denied directory must not cost the user every
			// other design in the project.
			return nil
		}
		if d.IsDir() {
			if path == root {
				return nil
			}
			name := d.Name()
			if strings.HasPrefix(name, ".") || skipDirs[name] {
				return fs.SkipDir
			}
			if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
				return fs.SkipDir
			}
			return nil
		}
		if IsDesignPath(path) {
			out = append(out, path)
		}
		return nil
	})
	sort.Strings(out)
	return out
}

// GenerateResult is one design's outcome in a project-wide generate.
type GenerateResult struct {
	Design string // the .silkui the result belongs to
	Output string // the file written; "" when nothing was
	Err    error
}

// GenerateAll regenerates every design in the project, one result per design.
// A design that fails to load or write does not stop the others: the caller
// reports the whole batch at once instead of interrupting the user per file.
func (p *Project) GenerateAll() []GenerateResult {
	if p == nil {
		return nil
	}
	results := make([]GenerateResult, 0, len(p.Designs))
	for _, design := range p.Designs {
		out, err := GenerateDesign(design, p.Module)
		results = append(results, GenerateResult{Design: design, Output: out, Err: err})
	}
	return results
}

// GenerateDesign turns one design into Go beside itself and returns the file
// written. This is the single place a design becomes code on disk: when the
// split generator lands, only the GenerateCodeFile call below changes and both
// halves of DesignOutputs get written from here.
//
// module is the project's module path, already parsed by ScanProject, so this
// does not re-read go.mod per design.
func GenerateDesign(designPath, module string) (string, error) {
	if !IsDesignPath(designPath) {
		return "", fmt.Errorf("not a design file: %s", designPath)
	}
	scene := NewGedScene()
	if err := scene.OpenFile(designPath); err != nil {
		return "", err
	}
	out := OutputPathsFor(designPath)
	stem := strings.TrimSuffix(filepath.Base(designPath), filepath.Ext(designPath))
	opts := CodeGenOptions{
		PackageName: packageNameForDir(filepath.Dir(designPath)),
		// From the file stem, not the form title: two designs in one directory
		// share a package, and two forms may well share a title.
		TypeName:   sanitizeIdentifier(stem) + "UI",
		ModulePath: module,
	}
	if err := scene.GenerateCodeFile(out.Generated, opts); err != nil {
		return "", err
	}
	return out.Generated, nil
}

// goKeywords are the identifiers a package clause may not use. A directory
// named "map" is not hypothetical — silk's own mod/map is one.
var goKeywords = map[string]bool{
	"break": true, "case": true, "chan": true, "const": true, "continue": true,
	"default": true, "defer": true, "else": true, "fallthrough": true, "for": true,
	"func": true, "go": true, "goto": true, "if": true, "import": true,
	"interface": true, "map": true, "package": true, "range": true, "return": true,
	"select": true, "struct": true, "switch": true, "type": true, "var": true,
}

// packageNameForDir derives the package clause generated code must carry to
// join the directory it is written into. Go's convention is package ==
// directory name, and that is as much as the designer can know without reading
// the directory's existing sources — which codegen deliberately does not do.
// Anything that cannot be a package identifier falls back to "ui".
//
// Known limitation: a directory whose package does not follow the convention —
// most often a module root holding package main — gets a clause that disagrees
// with its siblings, and go build says so. It is a loud one-line fix, unlike
// the alternative: naming the package "main" makes GenerateCode emit a second
// func main() into a directory that already has one.
func packageNameForDir(dir string) string {
	var b strings.Builder
	for _, r := range filepath.Base(dir) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
		}
	}
	name := b.String()
	if name == "" || name[0] >= '0' && name[0] <= '9' || goKeywords[name] {
		return "ui"
	}
	return name
}

// FormatGenerateReport renders a whole batch into one dialog body: the counts
// first, then a line per design with paths relative to the project root. One
// dialog for the batch is the point — a modal per design makes regenerating a
// twenty-screen project unusable.
func FormatGenerateReport(root string, results []GenerateResult) string {
	if len(results) == 0 {
		return "项目中没有 .silkui 设计文件。"
	}
	var ok, failed int
	var lines strings.Builder
	for _, r := range results {
		if r.Err != nil {
			failed++
			fmt.Fprintf(&lines, "✗ %s: %v\n", projectRelPath(root, r.Design), r.Err)
			continue
		}
		ok++
		fmt.Fprintf(&lines, "✓ %s → %s\n",
			projectRelPath(root, r.Design), projectRelPath(root, r.Output))
	}
	return fmt.Sprintf("成功 %d, 失败 %d\n\n%s", ok, failed, lines.String())
}

// projectRelPath shortens path for display against the project root, falling
// back to the absolute path for anything the root does not contain.
func projectRelPath(root, path string) string {
	if root == "" || path == "" {
		return path
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return path
	}
	return rel
}
