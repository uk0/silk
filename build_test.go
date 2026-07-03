package main

import (
	"os"
	"os/exec"
	"testing"
)

func TestAllPackagesBuild(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping build test in short mode")
	}

	packages := []string{
		"./core/",
		"./geom/",
		"./paint/",
		"./gui/",
		"./graph/",
		"./prop/",
		"./ged/",
	}

	for _, pkg := range packages {
		t.Run(pkg, func(t *testing.T) {
			cmd := exec.Command("go", "build", "-v", pkg)
			cmd.Env = append(os.Environ(), "CGO_ENABLED=1")
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Errorf("build failed for %s:\n%s", pkg, output)
			}
		})
	}
}

func TestMainPackageImports(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping import test in short mode")
	}

	// Verify the main packages listed in go.mod resolve their imports
	packages := []string{
		"github.com/uk0/silk/core", "github.com/uk0/silk/geom", "github.com/uk0/silk/paint", "github.com/uk0/silk/gui",
		"github.com/uk0/silk/graph", "github.com/uk0/silk/prop", "github.com/uk0/silk/ged",
	}

	for _, pkg := range packages {
		t.Run(pkg, func(t *testing.T) {
			cmd := exec.Command("go", "list", "-e", pkg)
			cmd.Env = append(os.Environ(), "CGO_ENABLED=1")
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Errorf("go list failed for %s:\n%s", pkg, output)
			}
		})
	}
}
