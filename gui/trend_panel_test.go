package gui

import (
	"testing"
	"time"

	"github.com/uk0/silk/core"
)

// TestTrendPanelConstructs checks NewTrendPanel builds a panel with a non-nil
// owned LineChart and the expected defaults: not playing, live mode, no range
// selected.
func TestTrendPanelConstructs(t *testing.T) {
	p := NewTrendPanel()
	if p.Chart() == nil {
		t.Fatal("NewTrendPanel().Chart() is nil, want an owned LineChart")
	}
	if p.IsPlaying() {
		t.Error("new panel IsPlaying() = true, want false")
	}
	if !p.IsLive() {
		t.Error("new panel IsLive() = false, want true (default live mode)")
	}
	if p.rangeSel != -1 {
		t.Errorf("new panel rangeSel = %d, want -1", p.rangeSel)
	}
}

// TestTrendPanelControlAt pins the pure hit-test: each control maps to its own
// region, a range label reports its index, and a y above/below the bar or an x
// past the last control maps to trendNone.
func TestTrendPanelControlAt(t *testing.T) {
	p := NewTrendPanel()
	midY := trendBarH * 0.5

	if ctl, _ := p.controlAt(trendPlayX()+trendBtnW*0.5, midY); ctl != trendPlay {
		t.Errorf("controlAt(play) = %d, want trendPlay", ctl)
	}
	if ctl, _ := p.controlAt(trendPauseX()+trendBtnW*0.5, midY); ctl != trendPause {
		t.Errorf("controlAt(pause) = %d, want trendPause", ctl)
	}
	if ctl, _ := p.controlAt(trendModeX()+trendModeW*0.5, midY); ctl != trendMode {
		t.Errorf("controlAt(mode) = %d, want trendMode", ctl)
	}
	if ctl, idx := p.controlAt(trendRangeX(1)+trendRangeW*0.5, midY); ctl != trendRange || idx != 1 {
		t.Errorf("controlAt(range 1) = (%d, %d), want (trendRange, 1)", ctl, idx)
	}

	// A click below the control bar belongs to the chart, not a control.
	if ctl, _ := p.controlAt(trendPlayX()+trendBtnW*0.5, trendBarH+1); ctl != trendNone {
		t.Errorf("controlAt(below bar) = %d, want trendNone", ctl)
	}
	// Past the last range label there is nothing.
	if ctl, _ := p.controlAt(trendRangeX(len(trendRanges)-1)+trendRangeW+50, midY); ctl != trendNone {
		t.Errorf("controlAt(past last range) = %d, want trendNone", ctl)
	}
}

// TestTrendPanelPlayClickFiresSig confirms clicking 播放 sets the playing flag
// and fires SigPlay.
func TestTrendPanelPlayClickFiresSig(t *testing.T) {
	p := NewTrendPanel()
	p.SetBounds(0, 0, 480, 240)

	fired := false
	p.SigPlay(func() { fired = true })

	p.OnLeftDown(trendPlayX()+trendBtnW*0.5, trendBarH*0.5)
	if !fired {
		t.Fatal("play click did not fire SigPlay")
	}
	if !p.IsPlaying() {
		t.Error("after play click IsPlaying() = false, want true")
	}
}

// TestTrendPanelPauseClickFiresSig confirms clicking 暂停 clears the playing
// flag and fires SigPause.
func TestTrendPanelPauseClickFiresSig(t *testing.T) {
	p := NewTrendPanel()
	p.SetBounds(0, 0, 480, 240)
	p.playing = true // start from playing so pause has an effect

	fired := false
	p.SigPause(func() { fired = true })

	p.OnLeftDown(trendPauseX()+trendBtnW*0.5, trendBarH*0.5)
	if !fired {
		t.Fatal("pause click did not fire SigPause")
	}
	if p.IsPlaying() {
		t.Error("after pause click IsPlaying() = true, want false")
	}
}

// TestTrendPanelModeToggleFiresSig confirms the 实时/历史 toggle flips the live
// flag and fires SigModeChanged with the new state.
func TestTrendPanelModeToggleFiresSig(t *testing.T) {
	p := NewTrendPanel()
	p.SetBounds(0, 0, 480, 240)
	if !p.IsLive() {
		t.Fatal("precondition: new panel should be live")
	}

	var got bool
	fired := false
	p.SigModeChanged(func(live bool) { fired = true; got = live })

	// First toggle: live -> history.
	p.OnLeftDown(trendModeX()+trendModeW*0.5, trendBarH*0.5)
	if !fired {
		t.Fatal("mode click did not fire SigModeChanged")
	}
	if got != false || p.IsLive() != false {
		t.Errorf("after 1st toggle: cb=%v IsLive=%v, want both false", got, p.IsLive())
	}

	// Second toggle: history -> live.
	p.OnLeftDown(trendModeX()+trendModeW*0.5, trendBarH*0.5)
	if got != true || p.IsLive() != true {
		t.Errorf("after 2nd toggle: cb=%v IsLive=%v, want both true", got, p.IsLive())
	}
}

// TestTrendPanelRangeClickFiresSig confirms a range label fires SigRange with
// its duration and records the selection; different labels emit different
// durations.
func TestTrendPanelRangeClickFiresSig(t *testing.T) {
	p := NewTrendPanel()
	p.SetBounds(0, 0, 480, 240)

	var got time.Duration
	fired := false
	p.SigRange(func(d time.Duration) { fired = true; got = d })

	// Click the "10m" label (index 1).
	p.OnLeftDown(trendRangeX(1)+trendRangeW*0.5, trendBarH*0.5)
	if !fired {
		t.Fatal("range click did not fire SigRange")
	}
	if got != 10*time.Minute {
		t.Errorf("SigRange duration = %v, want 10m", got)
	}
	if p.rangeSel != 1 {
		t.Errorf("rangeSel = %d, want 1", p.rangeSel)
	}

	// Click the "1h" label (index 2) — a different duration.
	p.OnLeftDown(trendRangeX(2)+trendRangeW*0.5, trendBarH*0.5)
	if got != time.Hour {
		t.Errorf("SigRange duration = %v, want 1h", got)
	}
	if p.rangeSel != 2 {
		t.Errorf("rangeSel = %d, want 2", p.rangeSel)
	}
}

// TestTrendPanelFactoryRegistered checks the factory id resolves to a
// constructible *TrendPanel with its chart wired, so the designer can place it.
func TestTrendPanelFactoryRegistered(t *testing.T) {
	obj := core.New("gui.TrendPanel")
	p, ok := obj.(*TrendPanel)
	if !ok {
		t.Fatalf("factory gui.TrendPanel built %T, want *TrendPanel", obj)
	}
	if p.Chart() == nil {
		t.Fatal("factory-built TrendPanel has nil Chart()")
	}
}
