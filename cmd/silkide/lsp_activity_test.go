package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

func TestLSPActivityUICoalescingRetainsFinalUpdate(t *testing.T) {
	var queue []func()
	applied := 0
	latest := 0
	seen := 0
	var notify func()
	notify = coalescedLSPActivity(func(f func()) { queue = append(queue, f) }, func() {
		applied++
		seen = latest
		if applied == 1 {
			latest = 10001
			notify()
		}
	})
	for latest = 1; latest <= 10000; latest++ {
		notify()
	}
	if len(queue) != 1 {
		t.Fatalf("burst queued %d UI tasks", len(queue))
	}
	f := queue[0]
	queue = queue[1:]
	f()
	if len(queue) != 1 {
		t.Fatal("update during apply lost")
	}
	queue[0]()
	if applied != 2 || seen != 10001 {
		t.Fatal(applied, seen)
	}
}

func TestLSPActivityPresentationIsBoundedAndTruthful(t *testing.T) {
	p := uint32(35)
	label, tip := lspActivityPresentation("connected", core.LSPWorkProgressSnapshot{Active: []core.LSPWorkProgress{{Title: strings.Repeat("Title\x00\u202e", 1000), Message: strings.Repeat("detail\n", 1000), Percentage: &p}}})
	if !strings.Contains(label, "35%") || len([]rune(tip)) > 400 || strings.ContainsAny(tip, "\x00\u202e") {
		t.Fatal(label, tip)
	}
	label, tip = lspActivityPresentation("connected", core.LSPWorkProgressSnapshot{})
	if label != i18n.T("gopls: connected") || !strings.Contains(tip, i18n.T("No reported work. Background caches may still be running.")) {
		t.Fatal(label, tip)
	}
	label, _ = lspActivityPresentation("connected", core.LSPWorkProgressSnapshot{Limited: true})
	if label != i18n.T("gopls: activity limited") {
		t.Fatal(label)
	}
	label, _ = lspActivityPresentation("connected", core.LSPWorkProgressSnapshot{Closed: true})
	if label != i18n.T("gopls: offline") {
		t.Fatal(label)
	}
}

func TestLSPActivityStatusCellAndStaleSession(t *testing.T) {
	oldButton, oldPhase, oldClient, oldEpoch := statusBarLSPButton, lspActivityPhase, globalLSP, lspSessionGeneration
	oldZoom, oldBuild := statusBarZoomLabel, statusBarBuildLabel
	t.Cleanup(func() {
		gui.SetToolTip(statusBarLSPButton, "")
		statusBarLSPButton, lspActivityPhase, globalLSP, lspSessionGeneration = oldButton, oldPhase, oldClient, oldEpoch
		statusBarZoomLabel, statusBarBuildLabel = oldZoom, oldBuild
	})
	globalLSP = &core.LSPClient{}
	lspSessionGeneration = 52
	lspActivityPhase = "starting"
	bar := buildStatusBar(gui.NewFrame())
	bar.SetSize(1000, 28)
	bar.Layout()
	if statusBarLSPButton.Icon() == nil || statusBarLSPButton.FocusPolicy() != gui.TabFocus || !statusBarLSPButton.IsTextVisible() {
		t.Fatal("status action lost icon, visible label, or keyboard access")
	}
	if statusBarLSPButton.Text() != i18n.T("gopls: starting") {
		t.Fatal(statusBarLSPButton.Text())
	}
	lspActivityPhase = "connected"
	refreshLSPActivityForSession(&core.LSPClient{}, 52)
	refreshLSPActivityForSession(globalLSP, 51)
	if statusBarLSPButton.Text() != i18n.T("gopls: starting") {
		t.Fatal("stale progress overwrote current session")
	}
	refreshLSPActivityForSession(globalLSP, 52)
	if statusBarLSPButton.Text() != i18n.T("gopls: connected") {
		t.Fatal("current progress not shown")
	}
	if !strings.Contains(lspActivityReport(), "No reported work is not proof of zero server CPU") {
		t.Fatal("report claims idle from silence")
	}
	for _, width := range []float64{1000, 400, 40, 0} {
		bar.SetSize(width, 28)
		bar.Layout()
		for _, w := range bar.PermanentWidgets() {
			x, y, cw, ch := w.Bounds()
			if x < 0 || y < 0 || x+cw > width+.01 || y+ch > 28.01 {
				t.Fatalf("status overlaps bar: %v at %v", []float64{x, y, cw, ch}, width)
			}
		}
	}
}

func TestLSPActivityCapabilityAndPlaintextIsolation(t *testing.T) {
	var c map[string]any
	if err := json.Unmarshal(ideLSPCapabilities(), &c); err != nil {
		t.Fatal(err)
	}
	if c["window"].(map[string]any)["workDoneProgress"] != true {
		t.Fatal("IDE progress capability absent")
	}
	var sdk map[string]any
	_ = json.Unmarshal(core.DefaultClientCapabilities(), &sdk)
	if w, ok := sdk["window"].(map[string]any); ok && w["workDoneProgress"] == true {
		t.Fatal("SDK forces progress on unaware hosts")
	}
	oldClient, oldDocs, oldVersions := globalLSP, lspDocuments, lspVersions
	defer func() { globalLSP, lspDocuments, lspVersions = oldClient, oldDocs, oldVersions }()
	globalLSP = &core.LSPClient{}
	lspDocuments = map[string]*lspDocument{}
	lspVersions = map[string]int{}
	ed := gui.NewCodeEditor()
	ed.SetText("plain text")
	for _, name := range []string{"notes.txt", "config.json", "README.md"} {
		p := filepath.Join(t.TempDir(), name)
		openLSPDocument(p, ed.Text())
		updateLSPDocument(p, ed, ed.Text())
	}
	if len(lspDocuments) != 0 || len(lspVersions) != 0 {
		t.Fatal("non-Go edits reached gopls document workers")
	}
}
