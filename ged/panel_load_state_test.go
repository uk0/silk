package ged

import (
	"strings"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

func TestGitPanelLoadFeedbackStatesAndLayout(t *testing.T) {
	p := NewGitChangesPanel()
	p.SetSize(500, 180)
	p.SetEntries([]core.GitStatusEntry{{Path: "a.go", Unstaged: 'M'}})
	calls := 0
	p.SigFileActivated(func(core.GitStatusEntry) { calls++ })
	p.SigCommit(func(string, []string) { calls++ })
	p.SigRefreshRequested(func() { calls++ })
	p.staged = map[string]bool{"a.go": true}
	p.commitMsg = "test"
	p.SetLoadState(PanelLoadState{Phase: PanelLoading, Title: "Loading", Detail: "Please wait"})
	p.OnLeftDown(100, 30)
	p.commitFocused = true
	p.OnKeyDown(gui.KeyEnter, false)
	p.OnTextInput("ignored")
	p.load.retry.OnKeyDown(gui.KeyEnter, false)
	if calls != 0 || p.commitMsg != "test" || p.load.retry.IsEnabled() {
		t.Fatal("busy panel accepted action", calls)
	}
	p.SetLoadState(PanelLoadState{Phase: PanelFailed, Title: "Missing Git", Detail: strings.Repeat("x", 10000) + "\x00", RetryLabel: "Retry"})
	if len(p.LoadState().Detail) > 520 || strings.ContainsRune(p.LoadState().Detail, 0) {
		t.Fatal("unbounded feedback")
	}
	if p.load.retry.Icon() == nil || p.load.retry.FocusPolicy() != gui.TabFocus || !p.load.retry.IsTextVisible() {
		t.Fatal("missing icon/label/keyboard access")
	}
	p.load.retry.OnKeyDown(gui.KeyEnter, false)
	if calls != 1 {
		t.Fatal("retry not routed")
	}
	for _, size := range [][2]float64{{500, 180}, {120, 80}, {40, 20}, {0, 0}} {
		p.SetSize(size[0], size[1])
		p.Layout()
		for _, child := range []gui.IWidget{p.load.label, p.load.retry} {
			x, y, w, h := child.Bounds()
			if x < 0 || y < 0 || x+w > size[0]+.01 || y+h > size[1]+.01 {
				t.Fatal("child overflow", size, x, y, w, h)
			}
		}
	}
	p.SetEntries(nil)
	if gui.GetToolTip(p.load.label) != "" {
		t.Fatal("load feedback retained a global tooltip reference")
	}
	if p.LoadState().Phase != PanelReady || p.load.retry.IsVisible() || p.load.label.IsVisible() {
		t.Fatal("successful refresh retained failure")
	}
}
func TestGitLogLoadFeedbackSuppressesStaleActivation(t *testing.T) {
	p := NewGitLogPanel()
	p.SetSize(400, 150)
	p.SetCommits([]core.GitCommit{{Hash: "abc", Subject: "old"}})
	calls := 0
	p.SigCommitActivated(func(core.GitCommit) { calls++ })
	p.SetLoadState(PanelLoadState{Phase: PanelFailed, Title: "Unavailable"})
	p.OnLeftDown(100, 30)
	if calls != 0 {
		t.Fatal("stale commit activated")
	}
	p.SetCommits([]core.GitCommit{{Hash: "def"}})
	p.OnLeftDown(100, 30)
	if calls != 1 {
		t.Fatal("recovered commit not activated")
	}
}
