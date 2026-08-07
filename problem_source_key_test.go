package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/uk0/silk/ged"
)

// TestDocKeySeparatesTwoDocuments is the guard a reviewer proved was missing.
//
// The 问题 rows are filed under a per-document key so that opening B cannot
// erase A's still-true load warning. Every test of that behaviour drove one
// document, so replacing docKey's body with `return ""` — filing every document
// under one key and re-creating the defect exactly — left the whole suite green.
// This asserts the property the merge actually rests on.
func TestDocKeySeparatesTwoDocuments(t *testing.T) {
	dir := t.TempDir()
	a := newSceneAt(t, filepath.Join(dir, "A.silkui"))
	b := newSceneAt(t, filepath.Join(dir, "B.silkui"))

	ka, kb := docKey(a), docKey(b)
	if ka == "" || kb == "" {
		t.Fatalf("docKey returned empty for a saved design: %q / %q", ka, kb)
	}
	if ka == kb {
		t.Fatalf("two designs share the problem key %q; one document's rows will erase the other's", ka)
	}

	// The property that key exists for: B's rows must not displace A's.
	rows := ged.MergeProblems(nil, ged.LoadSource(ka), ged.LoadProblems([]string{"gui.GhostA"}))
	rows = ged.MergeProblems(rows, ged.LoadSource(kb), ged.LoadProblems([]string{"gui.GhostB"}))

	var sawA, sawB bool
	for _, r := range rows {
		if strings.Contains(r.File+r.Message, "GhostA") {
			sawA = true
		}
		if strings.Contains(r.File+r.Message, "GhostB") {
			sawB = true
		}
	}
	if !sawA {
		t.Error("opening B erased A's load warning while A is still open")
	}
	if !sawB {
		t.Error("B's own load warning was not filed")
	}
}

// TestDropDocProblemsTakesOnlyItsOwn — closing one document must not take the
// other's rows with it, which is the same key doing the other half of its job.
func TestDropDocProblemsTakesOnlyItsOwn(t *testing.T) {
	dir := t.TempDir()
	a := newSceneAt(t, filepath.Join(dir, "A.silkui"))
	b := newSceneAt(t, filepath.Join(dir, "B.silkui"))

	rows := ged.MergeProblems(nil, ged.LoadSource(docKey(a)), ged.LoadProblems([]string{"gui.GhostA"}))
	rows = ged.MergeProblems(rows, ged.LoadSource(docKey(b)), ged.LoadProblems([]string{"gui.GhostB"}))

	// Closing B replaces B's source with nothing.
	rows = ged.MergeProblems(rows, ged.LoadSource(docKey(b)), nil)

	var sawA, sawB bool
	for _, r := range rows {
		if strings.Contains(r.File+r.Message, "GhostA") {
			sawA = true
		}
		if strings.Contains(r.File+r.Message, "GhostB") {
			sawB = true
		}
	}
	if !sawA {
		t.Error("closing B dropped A's rows too")
	}
	if sawB {
		t.Error("closing B left its own rows behind")
	}
}

func newSceneAt(t *testing.T, path string) *ged.GedScene {
	t.Helper()
	scene := ged.NewGedScene()
	scene.SetFilename(path)
	return scene
}
