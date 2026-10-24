package main

import (
	"fmt"
	"strings"
	"sync/atomic"
	"unicode"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

var statusBarLSPButton *gui.Button
var lspActivityPhase = "offline" // UI-thread owned; handshake is not indexing completion.

// One queued UI callback per client, irrespective of a progress-report burst.
// Clear pending before applying: an update arriving during apply must schedule
// one more drain, rather than silently losing the final work-done/end state.
func coalescedLSPActivity(post func(func()), apply func()) func() {
	var pending atomic.Bool
	return func() {
		if !pending.CompareAndSwap(false, true) {
			return
		}
		post(func() { pending.Store(false); apply() })
	}
}

func watchLSPActivity(client *core.LSPClient, epoch uint64, updates <-chan struct{}) {
	notify := coalescedLSPActivity(gui.Post, func() {
		refreshLSPActivityForSession(client, epoch)
	})
	go func() {
		for range updates {
			notify()
		}
	}()
}

func refreshLSPActivityForSession(client *core.LSPClient, epoch uint64) {
	if globalLSP == client && lspSessionGeneration == epoch {
		refreshLSPActivity()
	}
}

func newLSPStatusButton(parent gui.IWidget) *gui.Button {
	b := gui.NewButton1("", gui.LoadIcon("settings"))
	// Icon-bearing buttons hide their label by default; activity must remain
	// visible without hover, including the offline/recovery state.
	b.SetTextVisible(true)
	b.SetFocusPolicy(gui.TabFocus)
	b.Action().BindFunc0(func() { showToolchainStatus(parent) })
	return b
}

// Titles/messages are server data: keep controls/bidi formatting and enormous
// labels out of the status bar and tooltip; the source/token remains in core.
func lspActivityText(s string, limit int) string {
	var b strings.Builder
	n := 0
	for _, r := range s {
		if n >= limit {
			b.WriteRune('…')
			break
		}
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			r = ' '
		}
		b.WriteRune(r)
		n++
	}
	return strings.TrimSpace(b.String())
}

func lspActivityPresentation(phase string, s core.LSPWorkProgressSnapshot) (string, string) {
	label := i18n.T("gopls: offline")
	detail := i18n.T("Open toolchain status for paths and recovery actions.")
	if phase == "starting" {
		return i18n.T("gopls: starting"), i18n.T("Starting the language service; editor input remains available.")
	}
	if phase != "connected" || s.Closed {
		return label, detail
	}
	switch {
	case s.Limited:
		label = i18n.T("gopls: activity limited")
	case len(s.Active) > 0:
		label = fmt.Sprintf(i18n.T("gopls: working (%d)"), len(s.Active))
		if len(s.Active) == 1 && s.Active[0].Percentage != nil {
			label = fmt.Sprintf(i18n.T("gopls: working %d%%"), *s.Active[0].Percentage)
		}
	default:
		label = i18n.T("gopls: connected")
	}
	detail = i18n.T("No reported work. Background caches may still be running.")
	if len(s.Active) > 0 {
		e := s.Active[0]
		detail = lspActivityText(e.Title, 96)
		if e.Message != "" {
			detail += "\n" + lspActivityText(e.Message, 160)
		}
	}
	if s.Limited {
		detail = i18n.T("Progress display limit reached; activity may be incomplete.")
	}
	return label, detail + "\n" + i18n.T("Click for a language-service snapshot and toolchain status.")
}

func currentLSPActivity() core.LSPWorkProgressSnapshot {
	if globalLSP != nil {
		return globalLSP.WorkProgressSnapshot()
	}
	return core.LSPWorkProgressSnapshot{}
}
func refreshLSPActivity() {
	if statusBarLSPButton == nil {
		return
	}
	label, tip := lspActivityPresentation(lspActivityPhase, currentLSPActivity())
	if statusBarLSPButton.Text() != label {
		statusBarLSPButton.SetText(label)
		statusBarLSPButton.InvalidateParentLayout()
	}
	if gui.GetToolTip(statusBarLSPButton) != tip {
		gui.SetToolTip(statusBarLSPButton, tip)
	}
}

// Captured on the UI thread before the asynchronous tool/version probes start.
func lspActivityReport() string {
	s := currentLSPActivity()
	label, _ := lspActivityPresentation(lspActivityPhase, s)
	var b strings.Builder
	fmt.Fprintf(&b, "Language service activity snapshot: %s\nReported operations: %d\nProgress display limited: %t\n", label, len(s.Active), s.Limited)
	for _, e := range s.Active[:min(len(s.Active), 8)] {
		fmt.Fprintf(&b, "- %s", lspActivityText(e.Title, 128))
		if e.Percentage != nil {
			fmt.Fprintf(&b, " (%d%%)", *e.Percentage)
		}
		if e.Message != "" {
			fmt.Fprintf(&b, ": %s", lspActivityText(e.Message, 256))
		}
		b.WriteByte('\n')
	}
	if len(s.Active) > 8 {
		fmt.Fprintf(&b, "... %d more reported operations\n", len(s.Active)-8)
	}
	if s.LastCompleted != "" {
		fmt.Fprintf(&b, "Last completed operation: %s\n", lspActivityText(s.LastCompleted, 128))
	}
	b.WriteString("No reported work is not proof of zero server CPU or completed background cache refresh.\n\n")
	return b.String()
}
