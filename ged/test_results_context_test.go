package ged

import (
	"strings"
	"testing"
)

// The right-click menu logic is split into buildContextMenu (pure: row
// index → []testResultsMenuItem) and OnRightDown (the gui.ShowContextMenu
// glue). Tests drive buildContextMenu directly so they can assert
// labels + invoke Action funcs without standing up a real menu, popup,
// or GLFW window. The clipboard side is captured via clipboardFn — the
// same indirection clipboardWrite consults before falling back to
// gui.Clipboard.

// labelsOf flattens a menu spec to just its labels (separators become
// "---") so individual entry presence is easy to assert against.
func labelsOf(items []testResultsMenuItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		if it.Separator {
			out[i] = "---"
		} else {
			out[i] = it.Label
		}
	}
	return out
}

// findItem returns the first non-separator entry whose label matches.
// Test helper; fails the test when no match is found.
func findItem(t *testing.T, items []testResultsMenuItem, label string) testResultsMenuItem {
	t.Helper()
	for _, it := range items {
		if !it.Separator && it.Label == label {
			return it
		}
	}
	t.Fatalf("menu entry %q not found in %v", label, labelsOf(items))
	return testResultsMenuItem{}
}

// TestResultsBuildContextMenuFailRowEntries checks the FAIL row exposes
// the three canonical entries (Run Only This Test / Copy Name / Copy
// Output) followed by a separator and a 跳转 entry, with every entry
// enabled because the FAIL has a non-empty Name, Output, and recoverable
// File:Line plus a SigResultActivated subscriber.
func TestResultsBuildContextMenuFailRowEntries(t *testing.T) {
	p := NewTestResultsPanel()
	p.SetOutput(sampleGoTestOutput)
	p.SigResultActivated(func(TestResult) {}) // enables 跳转

	// Row 1 is the FAIL (TestParseBar) per the sample fixture.
	items := p.buildContextMenu(1)
	got := labelsOf(items)
	want := []string{"运行此测试", "复制名称", "复制输出", "---", "跳转"}
	if len(got) != len(want) {
		t.Fatalf("labels = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("labels[%d] = %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
	for _, lbl := range []string{"运行此测试", "复制名称", "复制输出", "跳转"} {
		if !findItem(t, items, lbl).Enabled {
			t.Errorf("entry %q expected Enabled on FAIL row, was disabled", lbl)
		}
	}
}

// TestResultsBuildContextMenuRunFiresCallback confirms that invoking
// the "运行此测试" entry's Action fires SigRunTestRequested with the
// row's test name verbatim — this is the hook silkide will translate
// into `go test -run ^<name>$ ./...`.
func TestResultsBuildContextMenuRunFiresCallback(t *testing.T) {
	p := NewTestResultsPanel()
	p.SetOutput(sampleGoTestOutput)

	var got string
	fired := 0
	p.SigRunTestRequested(func(name string) {
		got = name
		fired++
	})

	items := p.buildContextMenu(1) // FAIL row
	findItem(t, items, "运行此测试").Action()

	if fired != 1 {
		t.Fatalf("SigRunTestRequested fired %d times, want 1", fired)
	}
	if got != "TestParseBar" {
		t.Errorf("test name = %q, want TestParseBar", got)
	}
}

// TestResultsBuildContextMenuRunWithNoCallbackIsInert verifies that
// when the host has not registered SigRunTestRequested, invoking the
// entry is a silent no-op (no panic). The entry stays Enabled because
// the row has a name; the dispatch just goes nowhere.
func TestResultsBuildContextMenuRunWithNoCallbackIsInert(t *testing.T) {
	p := NewTestResultsPanel()
	p.SetOutput(sampleGoTestOutput)

	items := p.buildContextMenu(1)
	run := findItem(t, items, "运行此测试")
	if !run.Enabled {
		t.Fatal("运行此测试 entry should be enabled on a row with a name")
	}
	// Must not panic.
	run.Action()
}

// TestResultsBuildContextMenuCopyName checks the "复制名称" entry routes
// the row's Name through clipboardWrite — captured here via clipboardFn.
func TestResultsBuildContextMenuCopyName(t *testing.T) {
	p := NewTestResultsPanel()
	p.SetOutput(sampleGoTestOutput)

	var captured []string
	p.clipboardFn = func(s string) { captured = append(captured, s) }

	items := p.buildContextMenu(1) // FAIL: TestParseBar
	findItem(t, items, "复制名称").Action()

	if len(captured) != 1 || captured[0] != "TestParseBar" {
		t.Fatalf("clipboard captures = %v, want [TestParseBar]", captured)
	}
}

// TestResultsBuildContextMenuCopyOutput checks the "复制输出" entry
// routes the row's Output (the full captured failure body) through
// clipboardWrite. The fixture's FAIL row carries the parse_test.go
// locator + a trailing detail line; both must reach the clipboard.
func TestResultsBuildContextMenuCopyOutput(t *testing.T) {
	p := NewTestResultsPanel()
	p.SetOutput(sampleGoTestOutput)

	var got string
	p.clipboardFn = func(s string) { got = s }

	items := p.buildContextMenu(1)
	findItem(t, items, "复制输出").Action()

	if !strings.Contains(got, "parse_test.go:42: want 7, got 11") ||
		!strings.Contains(got, "parse_test.go:43: trailing detail line") {
		t.Fatalf("clipboard output missing FAIL body, got %q", got)
	}
}

// TestResultsBuildContextMenuCopyOutputDisabledOnPass verifies that on
// a PASS row (Output is empty) the "复制输出" entry is present but
// Disabled — copy-empty-string would be a UX papercut. The other two
// canonical entries remain enabled.
func TestResultsBuildContextMenuCopyOutputDisabledOnPass(t *testing.T) {
	p := NewTestResultsPanel()
	p.SetOutput(sampleGoTestOutput)

	items := p.buildContextMenu(0) // Row 0 = TestParseFoo (PASS)
	copyOut := findItem(t, items, "复制输出")
	if copyOut.Enabled {
		t.Error("复制输出 should be disabled on a PASS row (no captured output)")
	}
	if !findItem(t, items, "复制名称").Enabled {
		t.Error("复制名称 should be enabled on a PASS row")
	}
	if !findItem(t, items, "运行此测试").Enabled {
		t.Error("运行此测试 should be enabled on a PASS row")
	}
	// PASS rows have no locator, so no separator + 跳转.
	for _, l := range labelsOf(items) {
		if l == "跳转" || l == "---" {
			t.Errorf("PASS row menu should omit %q, got labels %v", l, labelsOf(items))
		}
	}
}

// TestResultsBuildContextMenuJumpReusesActivated verifies the optional
// 跳转 entry forwards through cbActivate — reusing the same callback
// OnLeftDown drives, so the host only needs one jump-handler.
func TestResultsBuildContextMenuJumpReusesActivated(t *testing.T) {
	p := NewTestResultsPanel()
	p.SetOutput(sampleGoTestOutput)

	var got TestResult
	fired := false
	p.SigResultActivated(func(r TestResult) {
		got = r
		fired = true
	})

	items := p.buildContextMenu(1) // FAIL row
	findItem(t, items, "跳转").Action()

	if !fired {
		t.Fatal("跳转 entry did not fire SigResultActivated")
	}
	if got.Name != "TestParseBar" || got.File != "parse_test.go" || got.Line != 42 {
		t.Errorf("activated row = %+v, want TestParseBar @ parse_test.go:42", got)
	}
}

// TestResultsBuildContextMenuOutOfRange covers the defensive bounds:
// row < 0 or row >= len(results) yields no menu.
func TestResultsBuildContextMenuOutOfRange(t *testing.T) {
	p := NewTestResultsPanel()
	p.SetOutput(sampleGoTestOutput)

	if got := p.buildContextMenu(-1); got != nil {
		t.Errorf("buildContextMenu(-1) = %+v, want nil", got)
	}
	if got := p.buildContextMenu(len(p.Results())); got != nil {
		t.Errorf("buildContextMenu(len) = %+v, want nil", got)
	}
}

// TestResultsOnRightDownHeaderIgnored confirms a right-click in the
// header band does not crash and does not invoke any of the menu
// callbacks. We register both callbacks and observe nothing fires.
func TestResultsOnRightDownHeaderIgnored(t *testing.T) {
	p := NewTestResultsPanel()
	p.SetOutput(sampleGoTestOutput)

	ranFired := false
	p.SigRunTestRequested(func(string) { ranFired = true })

	// y inside the header band → rowAt returns -1 → OnRightDown bails
	// out before reaching ShowContextMenu (which would need a window).
	p.OnRightDown(5, testResultsHeaderH/2)

	if ranFired {
		t.Error("SigRunTestRequested fired on a header right-click")
	}
}
