package gui

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestCompletionLexicalWorkerOwnedResultsAndRetirement(t *testing.T) {
	posts := make(chan func(), 8)
	var got completionIndexResult
	w := newCompletionIndexWorker(func(f func()) { posts <- f }, func(r completionIndexResult) { got = r }, nil)
	defer w.close()
	text := completionLexicalFixture(1300)
	query := func(text, prefix, language string, revision uint64) {
		t.Helper()
		w.cancel(true)
		w.request(completionIndexRequest{text: text, prefix: prefix, language: language, revision: revision, generation: revision})
		waitCompletionPost(t, posts)()
		waitCompletionWorker(t, w)
		all, _ := scanCompletionItemsForLanguage(context.Background(), text, language)
		want, _ := rankCompletionTop(context.Background(), all, prefix, completionCandidateLimit+1)
		if !reflect.DeepEqual(got.items, want) || got.request.text != "" {
			t.Fatal("default worker changed ranking or retained source in delivery")
		}
	}
	query(text, "value", "go", 1)
	first := got.items
	first[0].Text = "POISON"
	query(text+"\nBrandNewCandidate\n", "BrandNew", "go", 2)
	if len(got.items) != 1 || got.items[0].Text != "BrandNewCandidate" || first[0].Text != "POISON" {
		t.Fatal("delivery aliases private scratch")
	}
	query(strings.Replace(text, "value00000", "RenamedValue", 1), "value", "json", 3)
	w.mu.Lock()
	valid, retained := w.index.valid, w.index.text
	w.mu.Unlock()
	if !valid || retained == "" {
		t.Fatal("default worker did not prime private index")
	}
	w.close()
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cache != nil || w.index.text != "" || w.index.items != nil || w.index.chunks != nil || w.index.seen != nil {
		t.Fatal("closed worker retained lexical payload")
	}
}

func TestCompletionLexicalEditorShrinkAndLanguageRetireWorker(t *testing.T) {
	for _, action := range []string{"shrink", "language", "close"} {
		t.Run(action, func(t *testing.T) {
			ed, posts := indexedCompletionEditor(t)
			ed.TriggerCompletion()
			waitCompletionPost(t, posts)()
			w := ed.completion.indexWorker
			waitCompletionWorker(t, w)
			switch action {
			case "shrink":
				ed.SetText("small")
			case "language":
				ed.SetFilePath("notes.json")
			case "close":
				ed.CancelCompletion()
			}
			waitCompletionWorker(t, w)
			if ed.completion.indexWorker != nil {
				t.Fatal("retired document retains worker")
			}
			w.mu.Lock()
			defer w.mu.Unlock()
			if !w.closed || w.index.valid || w.index.text != "" || w.index.chunks != nil || w.cache != nil {
				t.Fatal("retirement retained private state")
			}
		})
	}
}

func TestCompletionLexicalEditorUndoRedoMatchesCurrentSource(t *testing.T) {
	ed, posts := indexedCompletionEditor(t)
	original := ed.Text()
	edited := strings.Replace(original, "RareRemoteValue", "RareRemoteEdited", 1)
	check := func(wantText, wantWord string) {
		t.Helper()
		if ed.Text() != wantText {
			t.Fatal("history restored wrong text")
		}
		ed.ScrollToLineCol(0, 4)
		ed.TriggerCompletion()
		waitCompletionPost(t, posts)()
		w := ed.completion.indexWorker
		waitCompletionWorker(t, w)
		all, _ := scanCompletionItemsForLanguage(context.Background(), ed.Text(), ed.Language())
		want, _ := rankCompletionTop(context.Background(), all, "rare", completionCandidateLimit+1)
		if !reflect.DeepEqual(ed.completion.localItems, want) {
			t.Fatal("editor history reused stale lexical candidates")
		}
		found := false
		for _, item := range ed.completion.localItems {
			if item.Text == wantWord {
				found = true
			}
		}
		if !found {
			t.Fatal("distant current-source identifier missing")
		}
	}
	check(original, "RareRemoteValue")
	ed.ReplaceAllText(edited)
	check(edited, "RareRemoteEdited")
	ed.Undo()
	check(original, "RareRemoteValue")
	ed.Redo()
	check(edited, "RareRemoteEdited")
}
