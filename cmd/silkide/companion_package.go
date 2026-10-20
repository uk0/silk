package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// A generated sibling must belong to the same package as the user's code.
// Ignore old companions, tests and Go-ignored files so an old default "ui"
// companion cannot perpetuate a package conflict in a main package.
func companionPackage(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	name := ""
	for _, entry := range entries {
		file := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(file, ".go") || strings.HasSuffix(file, "_test.go") || strings.HasSuffix(file, ".silk.go") || strings.HasPrefix(file, "_") || strings.HasPrefix(file, ".") {
			continue
		}
		ast, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, file), nil, parser.PackageClauseOnly)
		if err != nil {
			return "", fmt.Errorf("package of %s: %w", file, err)
		}
		if name != "" && name != ast.Name.Name {
			return "", fmt.Errorf("conflicting packages %q and %q in %s", name, ast.Name.Name, dir)
		}
		name = ast.Name.Name
	}
	if name == "" {
		name = "ui"
	}
	return name, nil
}
