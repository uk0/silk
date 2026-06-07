package ged

import (
	"testing"
	"time"
)

// TestLogPanelAppendOrderAndTime asserts that Append accumulates entries
// in arrival order, that Entries() returns a copy reflecting that order,
// and that each row has a non-zero Time stamped from now().
func TestLogPanelAppendOrderAndTime(t *testing.T) {
	p := NewLogPanel()
	p.Append(LogInfo, "first")
	p.Append(LogWarn, "second")
	p.Append(LogError, "third")

	got := p.Entries()
	if len(got) != 3 {
		t.Fatalf("len(Entries()) = %d, want 3", len(got))
	}
	wantMsg := []string{"first", "second", "third"}
	wantLvl := []LogLevel{LogInfo, LogWarn, LogError}
	for i, e := range got {
		if e.Message != wantMsg[i] {
			t.Errorf("entry %d message = %q, want %q", i, e.Message, wantMsg[i])
		}
		if e.Level != wantLvl[i] {
			t.Errorf("entry %d level = %d, want %d", i, e.Level, wantLvl[i])
		}
		if e.Time.IsZero() {
			t.Errorf("entry %d Time is zero, want stamped", i)
		}
	}
}

// TestLogPanelEntriesIsCopy verifies Entries() returns a defensive copy
// — mutating the returned slice must not corrupt the panel state.
func TestLogPanelEntriesIsCopy(t *testing.T) {
	p := NewLogPanel()
	p.Append(LogInfo, "keep")

	got := p.Entries()
	got[0].Message = "tampered"

	again := p.Entries()
	if again[0].Message != "keep" {
		t.Fatalf("Entries() leaked its backing slice: %q", again[0].Message)
	}
}

// TestLogPanelMaxEntriesDropsFIFO verifies the FIFO cap: once the cap is
// hit, the oldest entries are dropped from the head while new entries
// are appended at the tail.
func TestLogPanelMaxEntriesDropsFIFO(t *testing.T) {
	p := NewLogPanel()
	p.SetMaxEntries(3)
	p.Append(LogInfo, "a")
	p.Append(LogInfo, "b")
	p.Append(LogInfo, "c")
	p.Append(LogInfo, "d") // pushes "a" out
	p.Append(LogInfo, "e") // pushes "b" out

	got := p.Entries()
	if len(got) != 3 {
		t.Fatalf("len(Entries()) = %d, want 3", len(got))
	}
	want := []string{"c", "d", "e"}
	for i, e := range got {
		if e.Message != want[i] {
			t.Errorf("entry %d message = %q, want %q", i, e.Message, want[i])
		}
	}
}

// TestLogPanelSetMaxEntriesTrimsImmediately verifies that lowering the
// cap below the current entry count trims the oldest rows on the spot,
// so the invariant len(entries) <= maxEntries always holds.
func TestLogPanelSetMaxEntriesTrimsImmediately(t *testing.T) {
	p := NewLogPanel()
	for i := 0; i < 10; i++ {
		p.Append(LogInfo, "n")
	}
	p.SetMaxEntries(4)
	if got := len(p.Entries()); got != 4 {
		t.Fatalf("after SetMaxEntries(4), len(Entries()) = %d, want 4", got)
	}
}

// TestLogPanelClearEmpties verifies Clear empties the entries while
// leaving the panel reusable for fresh Appends.
func TestLogPanelClearEmpties(t *testing.T) {
	p := NewLogPanel()
	p.Append(LogInfo, "x")
	p.Append(LogWarn, "y")
	p.Clear()
	if got := p.Entries(); len(got) != 0 {
		t.Fatalf("after Clear, Entries() = %+v, want empty", got)
	}
	p.Append(LogError, "z")
	if got := p.Entries(); len(got) != 1 || got[0].Message != "z" {
		t.Fatalf("Append after Clear failed, Entries() = %+v", got)
	}
}

// TestLogPanelSetFilterKeepsEntries verifies SetFilter does not drop
// entries — it only narrows visibleEntries(). Lowering the filter must
// expose the previously-hidden rows again.
func TestLogPanelSetFilterKeepsEntries(t *testing.T) {
	p := NewLogPanel()
	p.Append(LogDebug, "d")
	p.Append(LogInfo, "i")
	p.Append(LogWarn, "w")
	p.Append(LogError, "e")

	p.SetFilter(LogWarn)
	if got := len(p.Entries()); got != 4 {
		t.Fatalf("SetFilter dropped entries from storage: len = %d, want 4", got)
	}
	if p.Filter() != LogWarn {
		t.Errorf("Filter() = %d, want %d", p.Filter(), LogWarn)
	}

	vis := p.visibleEntries()
	if len(vis) != 2 {
		t.Fatalf("visibleEntries() = %d rows, want 2 (warn+error): %+v", len(vis), vis)
	}
	if vis[0].Message != "w" || vis[1].Message != "e" {
		t.Errorf("visibleEntries() = [%q, %q], want [\"w\", \"e\"]", vis[0].Message, vis[1].Message)
	}

	// Drop the filter back to debug — every entry should be visible
	// again, in arrival order.
	p.SetFilter(LogDebug)
	vis = p.visibleEntries()
	if len(vis) != 4 {
		t.Fatalf("after SetFilter(LogDebug), visibleEntries() = %d rows, want 4", len(vis))
	}
	wantMsg := []string{"d", "i", "w", "e"}
	for i, e := range vis {
		if e.Message != wantMsg[i] {
			t.Errorf("vis[%d] = %q, want %q", i, e.Message, wantMsg[i])
		}
	}
}

// TestCountsByLevel verifies the pure tally helper.
func TestCountsByLevel(t *testing.T) {
	entries := []LogEntry{
		{Level: LogDebug, Message: "d1"},
		{Level: LogInfo, Message: "i1"},
		{Level: LogInfo, Message: "i2"},
		{Level: LogWarn, Message: "w1"},
		{Level: LogError, Message: "e1"},
		{Level: LogError, Message: "e2"},
		{Level: LogError, Message: "e3"},
	}
	d, i, w, e := countsByLevel(entries)
	if d != 1 || i != 2 || w != 1 || e != 3 {
		t.Fatalf("countsByLevel = (%d, %d, %d, %d), want (1, 2, 1, 3)", d, i, w, e)
	}

	// Empty slice is the (0, 0, 0, 0) identity.
	d, i, w, e = countsByLevel(nil)
	if d != 0 || i != 0 || w != 0 || e != 0 {
		t.Errorf("countsByLevel(nil) = (%d, %d, %d, %d), want zeros", d, i, w, e)
	}
}

// TestLogPanelOnLeftDownFires verifies clicking a row fires
// SigEntryClicked with that exact entry. Rows sit below the header at
// rowHeight each, in arrival order; we click into the middle of row 1
// (the second entry).
func TestLogPanelOnLeftDownFires(t *testing.T) {
	p := NewLogPanel()
	p.Append(LogInfo, "alpha")
	p.Append(LogWarn, "beta")
	p.Append(LogError, "gamma")

	var (
		got   LogEntry
		fired bool
	)
	p.SigEntryClicked(func(e LogEntry) {
		got = e
		fired = true
	})

	// Row 1 (beta) is at y in [logPanelHeaderH + rowHeight, logPanelHeaderH + 2*rowHeight).
	y := logPanelHeaderH + p.rowHeight + p.rowHeight/2
	p.OnLeftDown(5, y)

	if !fired {
		t.Fatal("OnLeftDown did not fire SigEntryClicked")
	}
	if got.Message != "beta" || got.Level != LogWarn {
		t.Errorf("clicked entry = %+v, want {beta, LogWarn}", got)
	}
}

// TestLogPanelOnLeftDownHeaderNoop verifies a click on the header band
// (y < headerH) is inert.
func TestLogPanelOnLeftDownHeaderNoop(t *testing.T) {
	p := NewLogPanel()
	p.Append(LogInfo, "alpha")

	fired := false
	p.SigEntryClicked(func(LogEntry) { fired = true })
	p.OnLeftDown(5, 5) // inside the 22px header
	if fired {
		t.Error("OnLeftDown on header fired SigEntryClicked")
	}
}

// TestLogPanelOnLeftDownRespectsFilter verifies that the filter narrows
// which entries are clickable — a hidden entry cannot be activated by a
// click on the now-shorter row list.
func TestLogPanelOnLeftDownRespectsFilter(t *testing.T) {
	p := NewLogPanel()
	p.Append(LogDebug, "skip-me")
	p.Append(LogError, "click-me")
	p.SetFilter(LogError)

	var got LogEntry
	p.SigEntryClicked(func(e LogEntry) { got = e })

	// With the filter, only "click-me" is visible. Row 0 lands at
	// y in [headerH, headerH+rowHeight). Click into its middle.
	y := logPanelHeaderH + p.rowHeight/2
	p.OnLeftDown(5, y)

	if got.Message != "click-me" {
		t.Errorf("clicked entry = %q, want %q (filter ignored?)", got.Message, "click-me")
	}
}

// TestShouldAutoScrollFollowing covers the auto-follow decision in the
// canonical follow-the-tail case: scroll is pinned to the bottom, so a
// new Append should drag the view along.
func TestShouldAutoScrollFollowing(t *testing.T) {
	// content = 600 px, view = 200 px, max scroll = 400 px.
	// Pinned to the bottom -> follow.
	if !shouldAutoScroll(400, 600, 200) {
		t.Fatal("shouldAutoScroll(400, 600, 200) = false, want true (at bottom)")
	}
	// Within the 1-px tolerance -> still following.
	if !shouldAutoScroll(399.5, 600, 200) {
		t.Fatal("shouldAutoScroll(399.5, 600, 200) = false, want true (within tolerance)")
	}
}

// TestShouldAutoScrollNotFollowing covers the don't-yank-the-user case:
// the user scrolled up to read older entries, the view is not at the
// bottom, and a new Append must NOT scroll the panel.
func TestShouldAutoScrollNotFollowing(t *testing.T) {
	// content = 600 px, view = 200 px, max = 400; user is at 100 px.
	if shouldAutoScroll(100, 600, 200) {
		t.Fatal("shouldAutoScroll(100, 600, 200) = true, want false (user scrolled up)")
	}
}

// TestShouldAutoScrollSmallContent verifies that when everything fits in
// the view there is nothing to scroll, but the next Append should still
// "stay visible" — the function reports following so the caller does
// not chase a non-existent scroll position.
func TestShouldAutoScrollSmallContent(t *testing.T) {
	if !shouldAutoScroll(0, 50, 200) {
		t.Fatal("shouldAutoScroll(0, 50, 200) = false, want true (content fits)")
	}
}

// TestShouldAutoScrollUnlaidPanel verifies the zero-viewHeight case:
// the panel has not been laid out yet, so the auto-follow decision
// defaults to "follow" — otherwise the very first Append on a
// freshly-created panel would never auto-scroll.
func TestShouldAutoScrollUnlaidPanel(t *testing.T) {
	if !shouldAutoScroll(0, 0, 0) {
		t.Fatal("shouldAutoScroll(0, 0, 0) = false, want true (panel not laid out)")
	}
}

// TestLogPanelDefaultMaxEntries pins the documented default — important
// because the cap is the panel's main memory-safety knob.
func TestLogPanelDefaultMaxEntries(t *testing.T) {
	p := NewLogPanel()
	if got := p.MaxEntries(); got != 1000 {
		t.Errorf("default MaxEntries = %d, want 1000", got)
	}
}

// TestLogPanelAppendUsesNow stamps a custom now and asserts the entry's
// Time matches it. This locks down the contract that Append uses
// time.Now (via the injectable hook) rather than the zero value.
func TestLogPanelAppendUsesNow(t *testing.T) {
	p := NewLogPanel()
	fixed := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	p.nowFn = func() time.Time { return fixed }
	p.Append(LogInfo, "stamped")

	got := p.Entries()
	if len(got) != 1 {
		t.Fatalf("Entries length = %d, want 1", len(got))
	}
	if !got[0].Time.Equal(fixed) {
		t.Errorf("Time = %v, want %v", got[0].Time, fixed)
	}
}
