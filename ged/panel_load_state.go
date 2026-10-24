package ged

import (
	"strings"
	"unicode"

	"github.com/uk0/silk/gui"
)

// PanelLoadPhase separates unavailable/loading data from a successful empty
// query. Zero preserves legacy hosts that push rows without a loader.
type PanelLoadPhase uint8

const (
	PanelReady PanelLoadPhase = iota
	PanelLoading
	PanelFailed
	PanelEmpty
)

// PanelLoadState is transient UI-thread-owned feedback, never repository data.
// Titles/details are bounded and control characters sanitized for presentation.
// A loading/failed/empty view does not render stale rows or accept mutation actions.
type PanelLoadState struct {
	Phase                     PanelLoadPhase
	Title, Detail, RetryLabel string
}

type panelLoadFeedback struct {
	state   PanelLoadState
	label   *gui.Label
	retry   *gui.Button
	onRetry func()
}

func panelFeedbackText(s string, limit int) string {
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
	return b.String()
}
func (f *panelLoadFeedback) active() bool { return f.state.Phase != PanelReady }
func (f *panelLoadFeedback) set(parent gui.IWidget, s PanelLoadState) {
	s.Title = panelFeedbackText(s.Title, 128)
	s.Detail = panelFeedbackText(s.Detail, 512)
	s.RetryLabel = panelFeedbackText(s.RetryLabel, 32)
	if s.Phase == PanelReady {
		s = PanelLoadState{}
	}
	if s == f.state {
		return
	}
	f.state = s
	if f.label == nil {
		f.label = gui.NewLabel("")
		f.label.SetWrap(true)
		f.label.SetParent(parent)
		f.retry = gui.NewButton1("Retry", gui.LoadIcon("edit-redo"))
		f.retry.SetTextVisible(true)
		f.retry.SetFocusPolicy(gui.TabFocus)
		f.retry.SetParent(parent)
		f.retry.Action().BindFunc0(func() {
			if f.active() && f.state.Phase != PanelLoading && f.onRetry != nil {
				f.onRetry()
			}
		})
	}
	f.label.SetText(s.Title + "\n" + s.Detail)
	label := s.RetryLabel
	if label == "" {
		label = "Retry"
	}
	f.retry.SetText(label)
	f.label.SetVisible(f.active())
	f.retry.SetVisible(f.active())
	f.retry.SetEnabled(f.active() && s.Phase != PanelLoading)
	f.layout(parent)
	parent.Update()
}
func (f *panelLoadFeedback) layout(parent gui.IWidget) {
	if f.label == nil {
		return
	}
	w, h := parent.Size()
	w = max(0, w)
	h = max(0, h)
	pad := min(8, w/2, h/2)
	inner := max(0, w-2*pad)
	bh := min(28, max(0, h-2*pad))
	bw := min(132, inner)
	f.label.SetBounds(pad, pad, inner, max(0, h-2*pad-bh-6))
	f.retry.SetBounds(pad, max(pad, h-pad-bh), bw, bh)
	f.retry.SetTextVisible(bw >= 96)
}

func (p *GitChangesPanel) SetLoadState(s PanelLoadState) {
	p.load.set(p, s)
	if p.load.active() {
		p.focusCommit(false)
	}
}
func (p *GitChangesPanel) LoadState() PanelLoadState     { return p.load.state }
func (p *GitChangesPanel) SigRefreshRequested(fn func()) { p.load.onRetry = fn }
func (p *GitChangesPanel) Layout()                       { p.load.layout(p) }
func (p *GitLogPanel) SetLoadState(s PanelLoadState)     { p.load.set(p, s) }
func (p *GitLogPanel) LoadState() PanelLoadState         { return p.load.state }
func (p *GitLogPanel) SigRefreshRequested(fn func())     { p.load.onRetry = fn }
func (p *GitLogPanel) Layout()                           { p.load.layout(p) }
