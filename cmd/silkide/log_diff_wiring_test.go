package main

import (
	"errors"
	"reflect"
	"testing"

	"silk/core"
	"silk/ged"
)

// TestBuildEventLevel pins the classifier the "Build finished" log event
// uses: a non-nil exit error logs as a warning, a clean run as info.
func TestBuildEventLevel(t *testing.T) {
	if got := buildEventLevel(nil); got != ged.LogInfo {
		t.Errorf("buildEventLevel(nil) = %d, want LogInfo (%d)", got, ged.LogInfo)
	}
	if got := buildEventLevel(errors.New("exit status 2")); got != ged.LogWarn {
		t.Errorf("buildEventLevel(err) = %d, want LogWarn (%d)", got, ged.LogWarn)
	}
}

// TestDiffOldNewFromHunks verifies the old/new reconstruction that feeds
// gui.DiffView: context lines land in both sides, removed lines only in
// old, added lines only in new, and the NoNewline marker contributes to
// neither side.
func TestDiffOldNewFromHunks(t *testing.T) {
	t.Run("context added removed mix", func(t *testing.T) {
		hunks := []core.DiffHunk{{
			Lines: []core.DiffLine{
				{Kind: core.DiffLineContext, Text: "ctx1"},
				{Kind: core.DiffLineRemoved, Text: "gone"},
				{Kind: core.DiffLineAdded, Text: "new1"},
				{Kind: core.DiffLineAdded, Text: "new2"},
				{Kind: core.DiffLineContext, Text: "ctx2"},
			},
		}}
		oldLines, newLines := diffOldNewFromHunks(hunks)
		wantOld := []string{"ctx1", "gone", "ctx2"}
		wantNew := []string{"ctx1", "new1", "new2", "ctx2"}
		if !reflect.DeepEqual(oldLines, wantOld) {
			t.Errorf("old = %#v, want %#v", oldLines, wantOld)
		}
		if !reflect.DeepEqual(newLines, wantNew) {
			t.Errorf("new = %#v, want %#v", newLines, wantNew)
		}
	})

	t.Run("noNewline marker contributes nothing", func(t *testing.T) {
		hunks := []core.DiffHunk{{
			Lines: []core.DiffLine{
				{Kind: core.DiffLineContext, Text: "a"},
				{Kind: core.DiffLineNoNewline},
			},
		}}
		oldLines, newLines := diffOldNewFromHunks(hunks)
		want := []string{"a"}
		if !reflect.DeepEqual(oldLines, want) {
			t.Errorf("old = %#v, want %#v", oldLines, want)
		}
		if !reflect.DeepEqual(newLines, want) {
			t.Errorf("new = %#v, want %#v", newLines, want)
		}
	})

	t.Run("multiple hunks concatenate in order", func(t *testing.T) {
		hunks := []core.DiffHunk{
			{Lines: []core.DiffLine{{Kind: core.DiffLineRemoved, Text: "x"}}},
			{Lines: []core.DiffLine{{Kind: core.DiffLineAdded, Text: "y"}}},
		}
		oldLines, newLines := diffOldNewFromHunks(hunks)
		if !reflect.DeepEqual(oldLines, []string{"x"}) {
			t.Errorf("old = %#v, want [x]", oldLines)
		}
		if !reflect.DeepEqual(newLines, []string{"y"}) {
			t.Errorf("new = %#v, want [y]", newLines)
		}
	})

	t.Run("empty hunks yield nil slices", func(t *testing.T) {
		oldLines, newLines := diffOldNewFromHunks(nil)
		if oldLines != nil || newLines != nil {
			t.Errorf("diffOldNewFromHunks(nil) = (%#v, %#v), want (nil, nil)", oldLines, newLines)
		}
	})
}
