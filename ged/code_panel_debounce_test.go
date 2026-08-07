package ged

import (
	"strings"
	"testing"

	"github.com/uk0/silk/gui"
)

// TestDebouncedRegenerationActuallyRuns drives the path no test drove before.
//
// Every behaviour test of the 生成代码 view calls RegenerateNow() by hand, so the
// stretch from "a design change armed the timer" to "the view holds new text"
// was covered by nothing: a wrong delay, a Stop() in the wrong place, or a
// callback that never re-arms would all have stayed invisible until a user
// noticed a stale listing. gui.PumpTimersForTest is the seam that lets a test
// stand where the event loop stands.
func TestDebouncedRegenerationActuallyRuns(t *testing.T) {
	panel, view := boundCodePanel(t)
	panel.ShowTab(codeTabGen)

	before := panel.genView.Text()
	if before == "" {
		t.Fatal("the generated view started empty; nothing to compare against")
	}

	sceneWidget(t, view.GedScene(), "gui.Button", "btnLate", 10, 10, 30, 8)
	panel.ScheduleRegenerate()

	if panel.genView.Text() != before {
		t.Error("the view regenerated synchronously; the debounce is not debouncing")
	}

	gui.PumpTimersForTest()

	after := panel.genView.Text()
	if after == before {
		t.Error("the armed timer never produced a regeneration; the 生成代码 view stays stale forever")
	}
	if !strings.Contains(after, "BtnLate") {
		t.Errorf("the regenerated view does not mention the widget that triggered it:\n%s", after)
	}
	if panel.genPending {
		t.Error("genPending survived the regeneration; the next change may be swallowed as already-pending")
	}
}

// TestDebounceCollapsesABurst pins why the delay exists: dragging a widget
// fires a change per mouse move, and regenerating the whole design on each one
// is the cost the timer was added to avoid.
func TestDebounceCollapsesABurst(t *testing.T) {
	panel, view := boundCodePanel(t)
	panel.ShowTab(codeTabGen)

	runs := 0
	panel.onRegenForTest = func() { runs++ }
	t.Cleanup(func() { panel.onRegenForTest = nil })

	for i := 0; i < 5; i++ {
		sceneWidget(t, view.GedScene(), "gui.Button", "", 10, float64(10+i*10), 30, 8)
		panel.ScheduleRegenerate()
	}
	gui.PumpTimersForTest()

	if runs != 1 {
		t.Errorf("a burst of 5 changes produced %d regenerations, want 1", runs)
	}
}

// TestHiddenTabDefersRegeneration: walking the whole design behind a view
// nobody is looking at is the other half of the cost the debounce avoids, and
// coming forward is where it catches up.
func TestHiddenTabDefersRegeneration(t *testing.T) {
	panel, view := boundCodePanel(t)
	panel.ShowTab(codeTabHandler)

	runs := 0
	panel.onRegenForTest = func() { runs++ }
	t.Cleanup(func() { panel.onRegenForTest = nil })

	sceneWidget(t, view.GedScene(), "gui.Button", "btnHidden", 10, 10, 30, 8)
	panel.ScheduleRegenerate()
	gui.PumpTimersForTest()

	if runs != 0 {
		t.Errorf("regenerated %d times behind a hidden tab, want 0", runs)
	}
	if !panel.genPending {
		t.Fatal("the deferred change was forgotten; coming forward will show a stale listing")
	}

	panel.ShowTab(codeTabGen)
	if runs != 1 {
		t.Errorf("bringing the tab forward ran %d regenerations, want 1", runs)
	}
	if !strings.Contains(panel.genView.Text(), "BtnHidden") {
		t.Error("the catch-up did not include the change made while the tab was hidden")
	}
}

func boundCodePanel(t *testing.T) (*CodePanel, *GedView) {
	t.Helper()
	view := NewGedView()
	panel := NewCodePanel()
	panel.BindGedView(view)
	return panel, view
}
