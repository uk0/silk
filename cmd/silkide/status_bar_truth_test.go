package main

import (
	"runtime"
	"strings"
	"testing"

	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
)

func TestStatusBarDoesNotInventWorkspaceState(t *testing.T) {
	zoom, build := statusBarZoomLabel, statusBarBuildLabel
	t.Cleanup(func() { statusBarZoomLabel, statusBarBuildLabel = zoom, build })
	sb := buildStatusBar(gui.NewFrame())
	foundRuntime := false
	for _, cell := range sb.PermanentWidgets() {
		label, ok := cell.(interface{ Text() string })
		if !ok {
			continue
		}
		text := label.Text()
		if text == "main" || text == "Go 1.25" || text == "Ln 1, Col 1" || strings.HasPrefix(text, "Ln ") {
			t.Fatalf("invented workspace state: %q", text)
		}
		foundRuntime = foundRuntime || text == "Runtime "+runtime.Version()
	}
	if !foundRuntime {
		t.Fatal("missing actual runtime version")
	}
}

func TestStatusBarBuildAndZoomRelayoutOnChange(t *testing.T) {
	zoom, build, output := statusBarZoomLabel, statusBarBuildLabel, globalBuildOutput
	t.Cleanup(func() { statusBarZoomLabel, statusBarBuildLabel, globalBuildOutput = zoom, build, output })
	sb := buildStatusBar(gui.NewFrame())
	sb.SetSize(1100, 28)
	sb.Layout()
	if w, _ := statusBarBuildLabel.Size(); w != 0 {
		t.Fatal("empty build cell reserves space")
	}
	globalBuildOutput = ged.NewBuildOutput()
	globalBuildOutput.SetOutput("build completed")
	setBuildStatus()
	if w, h := statusBarBuildLabel.Size(); w <= 0 || h <= 0 {
		t.Fatal("new build status remains collapsed")
	}
	setZoomLabel(123.45)
	if got, _ := statusBarZoomLabel.Size(); got < statusBarZoomLabel.SizeHints().Width {
		t.Fatal("zoom label width not updated")
	}
}
