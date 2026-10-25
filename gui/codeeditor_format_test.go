package gui

import (
	"context"
	"errors"
	"go/format"
	"testing"
	"time"

	"github.com/uk0/silk/async"
	"github.com/uk0/silk/formatx"
)

func TestLegacyFormatCodeUsesLocalFormatterAndPreservesUndo(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	e := NewCodeEditor()
	e.filePath = "sample.go"
	original := "package p\nfunc f(){println(\"中🙂\")}\n"
	e.SetText(original)
	e.ReplaceAllText(original + "// edit\n")
	before := e.Text()
	want, err := format.Source([]byte(before))
	if err != nil {
		t.Fatal(err)
	}
	changes := 0
	e.SigChanged(func(string) { changes++ })
	e.FormatCode()
	if e.Text() != string(want) || changes != 1 {
		t.Fatal("formatter failed or emitted multiple changes", e.Text(), changes)
	}
	e.Undo()
	if e.Text() != before {
		t.Fatal("formatting lost its undo entry")
	}
	e.Undo()
	if e.Text() != original {
		t.Fatal("formatting erased earlier history")
	}
}

func formatEditor(t *testing.T, language, text string) (*CodeEditor, chan func()) {
	t.Helper()
	e := NewCodeEditor()
	e.filePath = "sample." + language
	e.SetText(text)
	posts := make(chan func(), 8)
	e.formatPost = func(fn func()) { posts <- fn }
	t.Cleanup(func() {
		e.CancelGitStatus()
		e.CancelSymbolIndex()
		if j := e.formatJob; j != nil {
			e.CancelFormat()
			select {
			case <-j.future.Done():
			case <-time.After(5 * time.Second):
				t.Error("format worker leaked")
			}
		}
	})
	return e, posts
}

func TestEditorFormatAsyncDispatchUndoAndIdempotence(t *testing.T) {
	for _, tc := range []struct {
		lang, text string
		compact    bool
	}{
		{"go", "package p\nfunc f( ){println(\"中🙂\")}", false},
		{"json", `{"name":"中🙂","n":9007199254740993,"n":1e+09}`, false},
		{"json", "{\n \"x\" : [1,2]\n}\n", true},
	} {
		t.Run(tc.lang+tc.text[:1], func(t *testing.T) {
			e, posts := formatEditor(t, tc.lang, tc.text)
			e.ReplaceAllText(tc.text + " ")
			before := e.Text()
			changes, calls := 0, 0
			e.SigChanged(func(string) { changes++ })
			options := EditorFormatOptions{Compact: tc.compact}
			if err := e.FormatDocumentAsync(options, func(err error) {
				calls++
				if err != nil {
					t.Error(err)
				}
			}); err != nil {
				t.Fatal(err)
			}
			apply := waitCompletionPost(t, posts)
			if e.Text() != before || changes != 0 || calls != 0 || !e.Formatting() {
				t.Fatal("worker touched UI")
			}
			apply()
			want, err := formatx.Format(context.Background(), tc.lang, before, options)
			if err != nil || e.Text() != want || changes != 1 || calls != 1 || e.Formatting() {
				t.Fatal(e.Text(), changes, calls, err)
			}
			if err := e.FormatDocumentAsync(options, nil); err != nil {
				t.Fatal(err)
			}
			waitCompletionPost(t, posts)()
			if changes != 1 {
				t.Fatal("unchanged format emitted changes")
			}
			e.Undo()
			if e.Text() != before {
				t.Fatal("format undo")
			}
			e.Undo()
			if e.Text() != tc.text {
				t.Fatal("earlier undo")
			}
			e.Redo()
			e.Redo()
			if e.Text() != want {
				t.Fatal("format redo")
			}
		})
	}
}

func TestEditorFormatQueuedResultCannotOverwriteNewerState(t *testing.T) {
	for _, action := range []string{"edit", "undo", "rename", "language", "hide", "cancel", "escape", "sync"} {
		t.Run(action, func(t *testing.T) {
			e, posts := formatEditor(t, "json", `{"x":1}`)
			called := false
			if err := e.FormatDocumentAsync(EditorFormatOptions{}, func(error) { called = true }); err != nil {
				t.Fatal(err)
			}
			apply := waitCompletionPost(t, posts)
			switch action {
			case "edit", "undo":
				e.ReplaceAllText(`{"x":2}`)
				if action == "undo" {
					e.Undo()
				}
			case "rename":
				e.SetFilePath("renamed.json")
			case "language":
				e.SetFilePath("plain.txt")
			case "hide":
				e.SetVisible(false)
			case "cancel":
				e.CancelFormat()
			case "escape":
				e.onKeyDownWithShift(KeyEsc, false, false)
			case "sync":
				if err := e.FormatDocument(EditorFormatOptions{Compact: true}); err != nil {
					t.Fatal(err)
				}
			}
			before, revision, undo := e.Text(), e.TextRevision(), len(e.undoStack)
			apply()
			if called || e.Text() != before || e.TextRevision() != revision || len(e.undoStack) != undo || e.formatJob != nil {
				t.Fatal("retired result applied", action)
			}
		})
	}
}

func TestEditorFormatCancellationDoesNotReleaseRunningWork(t *testing.T) {
	e, posts := formatEditor(t, "json", `{"x":1}`)
	started, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	e.formatWork = func(ctx context.Context, language, text string, o EditorFormatOptions) (string, error) {
		close(started)
		<-release
		return formatx.Format(ctx, language, text, o)
	}
	if err := e.FormatDocumentAsync(EditorFormatOptions{}, func(error) { t.Error("cancelled callback") }); err != nil {
		t.Fatal(err)
	}
	<-started
	job := e.formatJob
	e.CancelFormat()
	for range 100 {
		if err := e.FormatDocumentAsync(EditorFormatOptions{}, nil); !errors.Is(err, async.ErrBusy) {
			t.Fatal("restarted cancelled worker", err)
		}
	}
	e.ReplaceAllText(`{"typing":"continues"}`)
	if e.Formatting() || e.formatJob != job || len(posts) != 0 {
		t.Fatal("cancellation waited/released running worker")
	}
}

func TestEditorFormatGlobalCapacityAndNoBackgroundWidgetAccess(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	for range 2 {
		e, _ := formatEditor(t, "json", `{}`)
		started := make(chan struct{})
		e.formatWork = func(ctx context.Context, _, _ string, _ EditorFormatOptions) (string, error) {
			close(started)
			<-release
			return "{}", ctx.Err()
		}
		if err := e.FormatDocumentAsync(EditorFormatOptions{}, nil); err != nil {
			t.Fatal(err)
		}
		<-started
	}
	e, _ := formatEditor(t, "json", `{}`)
	if err := e.FormatDocumentAsync(EditorFormatOptions{}, nil); !errors.Is(err, async.ErrBusy) || e.formatJob != nil {
		t.Fatal("unbounded admission", err)
	}
}

func TestEditorFormatErrorsAreDeliveredWithoutTextMutation(t *testing.T) {
	e, posts := formatEditor(t, "json", `{"x":]}`)
	called := false
	if err := e.FormatDocumentAsync(EditorFormatOptions{}, func(err error) {
		called = true
		if err == nil {
			t.Error("missing syntax error")
		}
	}); err != nil {
		t.Fatal(err)
	}
	waitCompletionPost(t, posts)()
	if !called || e.Text() != `{"x":]}` || len(e.undoStack) != 0 || e.Formatting() {
		t.Fatal("failed format changed document")
	}
	e.filePath = "a.txt"
	if err := e.FormatDocumentAsync(EditorFormatOptions{}, nil); !errors.Is(err, formatx.ErrLanguage) || e.formatJob != nil {
		t.Fatal(err)
	}
}
