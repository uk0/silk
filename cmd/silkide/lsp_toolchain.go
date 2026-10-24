package main

import (
	"fmt"
	"os/exec"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/internal/idetools"
)

// Called by the background startup worker, never by a UI layout/paint pass.
func launchIDELanguageServer() (*core.LSPClient, error) {
	tool, err := idetools.Resolve()
	if err != nil {
		return nil, err
	}
	if _, err = exec.LookPath("go"); err != nil {
		return nil, fmt.Errorf("gopls found at %s (%s), but Go SDK is unavailable; install Go and restart SilkIDE: %w", tool.Path, tool.Source, err)
	}
	core.Log("silkide: gopls ", tool.Version, " (", tool.Source, ") from ", tool.Path)
	return core.LaunchLSPClient(tool.Path)
}
