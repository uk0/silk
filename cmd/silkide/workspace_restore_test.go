package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

func TestWorkspaceRestoreReadySnapshotDuringSaveHasTerminalRetryState(t *testing.T) {
	w := testWorkbench(t)
	p := writeFile(t, t.TempDir(), "pending.txt", "old bytes")
	posts := make(chan func(), 16)
	r := newWorkspaceRestore(workspaceSession{Version: 1, Editors: []editorSession{{Path: p}}, ActiveFile: p, Mode: modeEdit}, w.tabs, nil, workspaceRestoreOptions{post: func(f func()) { posts <- f }})
	cleanupRestore(t, r, nil)
	var ready func()
	select {
	case ready = <-posts:
	case <-time.After(5 * time.Second):
		t.Fatal("no reader completion")
	}
	ticket, ok := reserveSavePaths(new(saveReservationOwner), []string{p})
	if !ok {
		t.Fatal("reserve")
	}
	t.Cleanup(func() { ticket.finish(true); ticket.finish(false) })
	if err := os.WriteFile(p, []byte("new bytes"), 0644); err != nil {
		t.Fatal(err)
	}
	ready()
	if r.Busy() || openEditors[p] != nil || w.tabs.Count() != 1 || r.pages[0].label.Text() != i18n.T("A file operation is in progress; reopen this tab after it finishes") {
		t.Fatal("stale restore accepted or falsely left loading")
	}
	ticket.finish(true)
	ticket.finish(false)
	if !openTestEditorReady(t, w.tabs, p) || openEditors[p].Text() != "new bytes" || w.tabs.Count() != 1 {
		t.Fatal("failed page not replaceable with fresh output")
	}
}

func installRestorePost(t *testing.T) chan func() {
	t.Helper()
	posts := make(chan func(), 128)
	old := workspaceRestorePost
	workspaceRestorePost = func(f func()) { posts <- f }
	t.Cleanup(func() { workspaceRestorePost = old })
	return posts
}
func pumpRestore(t *testing.T, posts <-chan func(), done func() bool) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for !done() {
		select {
		case f := <-posts:
			f()
		case <-deadline.C:
			t.Fatal("restore completion timeout")
		}
	}
}
func cleanupRestore(t *testing.T, r *workspaceRestore, release func()) {
	t.Helper()
	t.Cleanup(func() {
		r.Close()
		if release != nil {
			release()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := r.jobs.Wait(ctx); err != nil {
			t.Error(err)
		}
	})
}
func TestWorkspaceRestoreBlockedReadKeepsUIAndOrderWithoutStealingEdits(t *testing.T) {
	w := testWorkbench(t)
	root := t.TempDir()
	a := writeFile(t, root, "a.txt", "a\noriginal")
	b := writeFile(t, root, "b.txt", "b\nsecond")
	c := writeFile(t, root, "c.txt", "c")
	posts := make(chan func(), 128)
	entered, release := make(chan struct{}), make(chan struct{})
	released := false
	state := workspaceSession{Version: 1, Editors: []editorSession{{Path: a, Line: 1, Column: 4, ScrollY: 12}, {Path: b}, {Path: c}}, ActiveFile: a, Mode: modeEdit}
	r := newWorkspaceRestore(state, w.tabs, nil, workspaceRestoreOptions{post: func(f func()) { posts <- f }, read: func(ctx context.Context, p string, d bool) (restoredFile, error) {
		if p == a {
			close(entered)
			<-release
		}
		return readRestoredFile(ctx, p, d)
	}})
	cleanupRestore(t, r, func() {
		if !released {
			close(release)
		}
	})
	<-entered
	if w.tabs.Count() != 3 || activeTestEditorReady(t, w.tabs) != nil {
		t.Fatal("restore blocked setup or exposed editable empty file")
	}
	captured := captureWorkspaceSession(w.tabs, nil)
	if !reflect.DeepEqual(captured.Editors, state.Editors) || captured.ActiveFile != a {
		t.Fatal("pending source metadata lost", captured)
	}
	pumpRestore(t, posts, func() bool { return openEditors[b] != nil && openEditors[c] != nil })
	ed := openEditors[b]
	focusEditorTab(w.tabs, ed)
	ed.ReplaceAllText("operator dirty content")
	ed.ScrollToLineCol(0, 5)
	revision := ed.TextRevision()
	close(release)
	released = true
	pumpRestore(t, posts, func() bool { return !r.Busy() })
	if activeTestEditorReady(t, w.tabs) != ed || ed.Text() != "operator dirty content" || ed.TextRevision() != revision || !ed.CanUndo() {
		t.Fatal("late restore stole/replaced edited buffer")
	}
	if got := orderedEditorPaths(w.tabs); !reflect.DeepEqual(got, []string{a, b, c}) {
		t.Fatal("worker completion order reordered tabs", got)
	}
	if openEditors[a].CursorLine() != 1 || openEditors[a].CursorCol() != 4 || openEditors[a].ScrollY() != 12 {
		t.Fatal("saved location not restored")
	}
	if diskText(t, b) != "b\nsecond" {
		t.Fatal("restore saved dirty source")
	}
}
func TestWorkspaceRestoreCloseAndManualOpenDropLateResults(t *testing.T) {
	for _, manual := range []bool{false, true} {
		t.Run(map[bool]string{false: "close", true: "manual"}[manual], func(t *testing.T) {
			w := testWorkbench(t)
			p := writeFile(t, t.TempDir(), "x.txt", "disk")
			posts := make(chan func(), 32)
			entered, release := make(chan struct{}), make(chan struct{})
			r := newWorkspaceRestore(workspaceSession{Editors: []editorSession{{Path: p}}, Mode: modeEdit}, w.tabs, nil, workspaceRestoreOptions{post: func(f func()) { posts <- f }, read: func(context.Context, string, bool) (restoredFile, error) {
				close(entered)
				<-release
				return restoredFile{data: []byte("obsolete bytes")}, nil
			}})
			released := false
			cleanupRestore(t, r, func() {
				if !released {
					close(release)
				}
			})
			<-entered
			if manual {
				if !openTestEditorReady(t, w.tabs, p) {
					t.Fatal("manual open failed")
				}
				activeTestEditorReady(t, w.tabs).ReplaceAllText("newer unsaved")
			} else if !closeEditorTab(w.tabs, 0) {
				t.Fatal("close loading page failed")
			}
			close(release)
			released = true
			pumpRestore(t, posts, func() bool { return !r.Busy() })
			if manual {
				if len(openEditors) != 1 || openEditors[p].Text() != "newer unsaved" || w.tabs.Count() != 1 {
					t.Fatal("late read replaced/reopened manual source")
				}
			} else if w.tabs.Count() != 0 || len(openEditors) != 0 {
				t.Fatal("closed pending source reopened")
			}
		})
	}
}
func TestWorkspaceRestoreTimeoutRetainsPhysicalSlotsAndCancelMenus(t *testing.T) {
	w := testWorkbench(t)
	posts := make(chan func(), 128)
	release := make(chan struct{})
	var calls atomic.Int32
	paths := []editorSession{{Path: filepath.Join(t.TempDir(), "a")}, {Path: filepath.Join(t.TempDir(), "b")}, {Path: filepath.Join(t.TempDir(), "c")}}
	r := newWorkspaceRestore(workspaceSession{Editors: paths, Mode: modeEdit}, w.tabs, nil, workspaceRestoreOptions{post: func(f func()) { posts <- f }, timeout: 25 * time.Millisecond, read: func(context.Context, string, bool) (restoredFile, error) {
		calls.Add(1)
		<-release
		return restoredFile{data: []byte("late")}, nil
	}})
	released := false
	cleanupRestore(t, r, func() {
		if !released {
			close(release)
		}
	})
	pumpRestore(t, posts, func() bool {
		return strings.Contains(r.pages[0].label.Text(), "timed out") && strings.Contains(r.pages[1].label.Text(), "timed out")
	})
	if calls.Load() != 2 || r.active != 2 || len(r.queue) != 1 {
		t.Fatal("timeout spawned replacements for blocked OS workers")
	}
	menu := buildFileMenu(w.tabs, nil)
	defer releaseWorkbenchMenu(menu)
	button := menuCommandButton(menu, "file.cancelRestore")
	if button == nil || !button.IsEnabled() {
		t.Fatal("missing cancel menu")
	}
	button.Action().Trigger(nil)
	if r.Busy() || w.tabs.Count() != 0 || r.tabs != nil || r.owner != nil {
		t.Fatal("cancel retained live UI owner")
	}
	close(release)
	released = true
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := r.jobs.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	for len(posts) > 0 {
		(<-posts)()
	}
	if len(openEditors) != 0 || w.tabs.Count() != 0 {
		t.Fatal("cancelled worker changed UI")
	}
}
func TestWorkspaceRestoreCapturedDesignAndInterveningMutation(t *testing.T) {
	for _, edit := range []bool{false, true} {
		t.Run(map[bool]string{false: "restore", true: "edited"}[edit], func(t *testing.T) {
			w := testWorkbench(t)
			path := filepath.Join(t.TempDir(), "form.silkui")
			writeRecoveryDesign(t, path, "Restored", "from disk")
			posts := make(chan func(), 64)
			entered, release := make(chan struct{}), make(chan struct{})
			r := newWorkspaceRestore(workspaceSession{Design: path, Mode: modeDesign}, w.tabs, w.canvas, workspaceRestoreOptions{post: func(f func()) { posts <- f }, read: func(ctx context.Context, p string, d bool) (restoredFile, error) {
				value, err := readRestoredFile(ctx, p, d)
				close(entered)
				<-release
				return value, err
			}})
			released := false
			cleanupRestore(t, r, func() {
				if !released {
					close(release)
				}
			})
			<-entered
			captured := captureWorkspaceSession(w.tabs, w.canvas)
			if captured.Design != path {
				t.Fatal("pending design lost from session")
			}
			if edit {
				w.canvas.GedScene().PushCommand(recoveryTitleEdit{w.canvas.GedScene(), w.canvas.GedScene().FormTitle(), "Keep new edits"})
			}
			close(release)
			released = true
			pumpRestore(t, posts, func() bool { return !r.Busy() })
			if edit {
				if w.canvas.GedScene().FormTitle() != "Keep new edits" || w.canvas.GedScene().IsClean() {
					t.Fatal("late design replaced user changes")
				}
			} else if w.canvas.GedScene().Filename() != path || recoveryButtonText(t, w.canvas.GedScene()) != "from disk" || !w.canvas.GedScene().IsClean() {
				t.Fatal("captured design not applied")
			}
		})
	}
}
func TestWorkspaceRestoreReaderBoundsRegularFileCompressionAndCancellation(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	p := writeFile(t, root, "empty", "")
	if v, e := readRestoredFile(ctx, p, false); e != nil || len(v.data) != 0 {
		t.Fatal("empty file", e)
	}
	if _, e := readRestoredFile(ctx, root, false); !errors.Is(e, errRestoreNotRegular) {
		t.Fatal("accepted directory", e)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e := readRestoredFile(cancelled, p, false); !errors.Is(e, context.Canceled) {
		t.Fatal("ignored cancellation", e)
	}
	if e := os.Truncate(p, restoreFileLimit+1); e != nil {
		t.Fatal(e)
	}
	if _, e := readRestoredFile(ctx, p, false); e == nil {
		t.Fatal("accepted oversized file")
	}
	design := filepath.Join(root, "form.silkui")
	writeRecoveryDesign(t, design, "Captured", "ok")
	data, _ := os.ReadFile(design)
	var zipped bytes.Buffer
	z := gzip.NewWriter(&zipped)
	_, _ = z.Write(data)
	_ = z.Close()
	doc, e := decodeRestoreDesign(ctx, zipped.Bytes())
	if e != nil || doc == nil {
		t.Fatal("valid compressed design", e)
	}
	broken := append([]byte(nil), zipped.Bytes()...)
	broken[len(broken)-5] ^= 1
	if _, e := decodeRestoreDesign(ctx, broken); e == nil {
		t.Fatal("ignored gzip trailer")
	}
	var large bytes.Buffer
	z = gzip.NewWriter(&large)
	_, _ = ioCopyZeros(z, restoreFileLimit+1)
	_ = z.Close()
	if _, e := decodeRestoreDesign(ctx, large.Bytes()); e == nil {
		t.Fatal("accepted expanded design bomb")
	}
}
func ioCopyZeros(z *gzip.Writer, n int) (int, error) {
	p := make([]byte, 32768)
	total := 0
	for total < n {
		m, e := z.Write(p[:min(len(p), n-total)])
		total += m
		if e != nil {
			return total, e
		}
	}
	return total, nil
}

func TestWorkspaceRestoreRecoverySnapshotsAndNestedModalGuard(t *testing.T) {
	for _, mutate := range []bool{false, true} {
		t.Run(map[bool]string{false: "recover", true: "nested edit"}[mutate], func(t *testing.T) {
			w := testWorkbench(t)
			root := t.TempDir()
			path := filepath.Join(root, "saved.silkui")
			recovery := path + ".autosave"
			writeRecoveryDesign(t, path, "Original", "old")
			writeRecoveryDesign(t, recovery, "Recovery", "recover me")
			now := time.Now()
			if err := os.Chtimes(path, now.Add(-time.Hour), now.Add(-time.Hour)); err != nil {
				t.Fatal(err)
			}
			before, recoverBefore := diskText(t, path), diskText(t, recovery)
			posts := make(chan func(), 32)
			asked := false
			r := newWorkspaceRestore(workspaceSession{Design: path, Mode: modeDesign}, w.tabs, w.canvas, workspaceRestoreOptions{post: func(f func()) { posts <- f }, confirmRecovery: func(_ gui.IWidget, p, candidate string) bool {
				asked = true
				if p != path || candidate != recovery {
					t.Error("recovery identity drift")
				}
				if mutate {
					scene := w.canvas.GedScene()
					scene.PushCommand(recoveryTitleEdit{scene, scene.FormTitle(), "Nested user change"})
				}
				return true
			}})
			cleanupRestore(t, r, nil)
			pumpRestore(t, posts, func() bool { return !r.Busy() })
			scene := w.canvas.GedScene()
			if !asked {
				t.Fatal("recovery prompt skipped")
			}
			if mutate {
				if scene.FormTitle() != "Nested user change" {
					t.Fatal("modal edit overwritten")
				}
			} else if scene.Filename() != path || scene.FormTitle() != "Recovery" || scene.IsClean() || scene.UndoStack().Count() != 0 {
				t.Fatal("recovery lost original identity or dirty history contract")
			}
			if diskText(t, path) != before || diskText(t, recovery) != recoverBefore {
				t.Fatal("restore wrote a source/recovery file")
			}
		})
	}
}

func TestWorkspaceRestoreQuitPersistsPendingBeforeCancellation(t *testing.T) {
	w := testWorkbench(t)
	prefs := seedLastLaunch(t)
	posts := make(chan func(), 32)
	entered, release := make(chan struct{}), make(chan struct{})
	path := filepath.Join(t.TempDir(), "still-loading.txt")
	saved := editorSession{Path: path, Line: 12, Column: 4, ScrollY: 36}
	r := newWorkspaceRestore(workspaceSession{Editors: []editorSession{saved}, ActiveFile: path, Mode: modeEdit}, w.tabs, w.canvas, workspaceRestoreOptions{post: func(f func()) { posts <- f }, read: func(context.Context, string, bool) (restoredFile, error) {
		close(entered)
		<-release
		return restoredFile{data: []byte("late")}, nil
	}})
	released := false
	cleanupRestore(t, r, func() {
		if !released {
			close(release)
		}
	})
	<-entered
	installClosedPersist(w.frame, w.canvas, prefs)
	w.frame.Close()
	if !r.closed || r.owner != nil || r.tabs != nil {
		t.Fatal("frame close retained restore owner")
	}
	flushTestPreferences(t, prefs)
	state, ok := testPreferences(t).WorkspaceSession()
	if !ok || len(state.Editors) != 1 || state.Editors[0] != saved || state.ActiveFile != path || state.Mode != modeEdit {
		t.Fatal("quit erased loading source metadata", state)
	}
	close(release)
	released = true
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := r.jobs.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	for len(posts) > 0 {
		(<-posts)()
	}
	if len(openEditors) != 0 {
		t.Fatal("quit late result reopened document")
	}
}
func TestWorkspaceRestoreWelcomeAndManualCaptureStayAuthoritative(t *testing.T) {
	w := testWorkbench(t)
	posts := make(chan func(), 32)
	entered, release := make(chan struct{}), make(chan struct{})
	p := writeFile(t, t.TempDir(), "x.txt", "disk")
	r := newWorkspaceRestore(workspaceSession{Editors: []editorSession{{Path: p}}, Mode: modeWelcome}, w.tabs, w.canvas, workspaceRestoreOptions{post: func(f func()) { posts <- f }, read: func(context.Context, string, bool) (restoredFile, error) {
		close(entered)
		<-release
		return restoredFile{data: []byte("captured")}, nil
	}})
	released := false
	cleanupRestore(t, r, func() {
		if !released {
			close(release)
		}
	})
	<-entered
	if documentMode(w.tabs, w.canvas) != modeWelcome {
		t.Fatal("restore changed Welcome mode")
	}
	if !openTestEditorReady(t, w.tabs, p) {
		t.Fatal("manual open failed")
	}
	ed := activeTestEditorReady(t, w.tabs)
	ed.ReplaceAllText("dirty")
	ed.ScrollToLineCol(0, 3)
	state := captureWorkspaceSession(w.tabs, w.canvas)
	if len(state.Editors) != 1 || state.Editors[0].Column != 3 || state.ActiveFile != p {
		t.Fatal("pending placeholder duplicated/replaced manual identity", state)
	}
	w.activate(modeWelcome)
	close(release)
	released = true
	pumpRestore(t, posts, func() bool { return !r.Busy() })
	if documentMode(w.tabs, w.canvas) != modeWelcome || openEditors[p] != ed || ed.Text() != "dirty" {
		t.Fatal("late restore changed mode/source")
	}
}

func TestWorkspaceRestoreReplacementNotificationCanCloseNewSource(t *testing.T) {
	w := testWorkbench(t)
	path := writeFile(t, t.TempDir(), "a.txt", "original")
	posts := make(chan func(), 32)
	r := newWorkspaceRestore(workspaceSession{Editors: []editorSession{{Path: path}}, Mode: modeEdit}, w.tabs, nil, workspaceRestoreOptions{post: func(f func()) { posts <- f }})
	cleanupRestore(t, r, nil)
	w.tabs.SetCurrentChangedCallback(func(_ interface{}, idx int) {
		if activeTestEditorReady(t, w.tabs) != nil {
			closeEditorTab(w.tabs, idx)
		}
	})
	pumpRestore(t, posts, func() bool { return !r.Busy() })
	if len(openEditors) != 0 || w.tabs.Count() != 0 {
		t.Fatal("replacement event resurrected a source closed by its observer")
	}
}

func TestLegacyWorkspaceRestoreEmptyAndLastDocumentMode(t *testing.T) {
	for _, kind := range []string{"empty", "code-last", "design-last"} {
		t.Run(kind, func(t *testing.T) {
			w, prefs := recentOpenWorkbench(t)
			root := t.TempDir()
			code := writeFile(t, root, "a.txt", "one")
			design := filepath.Join(root, "f.silkui")
			writeRecoveryDesign(t, design, "Form", "ok")
			var paths []string
			want := modeWelcome
			if kind == "code-last" {
				paths = []string{design, code}
				want = modeEdit
			} else if kind == "design-last" {
				paths = []string{code, design}
				want = modeDesign
			}
			prefs.SetOpenSession(paths)
			posts := installRestorePost(t)
			restoreSession(w.tabs, w.canvas)
			cleanupRestore(t, w.restore, nil)
			pumpRestore(t, posts, func() bool { return !w.restore.Busy() })
			if got := documentMode(w.tabs, w.canvas); got != want {
				t.Fatal("legacy/empty session selected wrong initial workspace", got, want)
			}
		})
	}
}

func TestWorkspaceRecoveryDefersAfterInteractionAndExplicitReviewReadsFresh(t *testing.T) {
	for _, action := range []string{"mode", "editor"} {
		t.Run(action, func(t *testing.T) {
			w := testWorkbench(t)
			root := t.TempDir()
			path := filepath.Join(root, "f.silkui")
			autosave := path + ".autosave"
			writeRecoveryDesign(t, path, "Original", "old")
			writeRecoveryDesign(t, autosave, "First recovery", "first")
			now := time.Now()
			if e := os.Chtimes(path, now.Add(-time.Hour), now.Add(-time.Hour)); e != nil {
				t.Fatal(e)
			}
			entered, release := make(chan struct{}), make(chan struct{})
			posts := make(chan func(), 64)
			var reads atomic.Int32
			asked := 0
			r := newWorkspaceRestore(workspaceSession{Design: path, Mode: modeDesign}, w.tabs, w.canvas, workspaceRestoreOptions{post: func(f func()) { posts <- f }, read: func(ctx context.Context, p string, d bool) (restoredFile, error) {
				v, e := readRestoredFile(ctx, p, d)
				if reads.Add(1) == 1 {
					close(entered)
					<-release
				}
				return v, e
			}, confirmRecovery: func(gui.IWidget, string, string) bool { asked++; return true }})
			released := false
			cleanupRestore(t, r, func() {
				if !released {
					close(release)
				}
			})
			<-entered
			if action == "mode" {
				w.activate(modeWelcome)
			} else {
				p := writeFile(t, root, "notes.txt", "notes")
				openTestEditorReady(t, w.tabs, p)
				activeTestEditorReady(t, w.tabs).ReplaceAllText("Keep unsaved notes")
			}
			close(release)
			released = true
			pumpRestore(t, posts, func() bool { return !r.Busy() })
			if asked != 0 || !r.canReviewRecovery() {
				t.Fatal("late restore interrupted user or lost deferred recovery")
			}
			if state := captureWorkspaceSession(w.tabs, w.canvas); state.Design != path {
				t.Fatal("deferred recovery identity lost", state)
			}
			writeRecoveryDesign(t, autosave, "Fresh recovery", "second after deferral")
			menu := buildFileMenu(w.tabs, w.canvas)
			defer releaseWorkbenchMenu(menu)
			b := menuCommandButton(menu, "file.reviewRecovery")
			if b == nil || !b.IsEnabled() {
				t.Fatal("review action missing")
			}
			b.Action().Trigger(nil)
			pumpRestore(t, posts, func() bool { return !r.Busy() })
			if asked != 1 || reads.Load() != 2 || w.canvas.GedScene().FormTitle() != "Fresh recovery" || w.canvas.GedScene().IsClean() {
				t.Fatal("review reused old bytes or lost dirty state")
			}
			if r.canReviewRecovery() {
				t.Fatal("applied recovery still available")
			}
			if action == "editor" {
				for _, ed := range openEditors {
					if ed.Text() != "Keep unsaved notes" {
						t.Fatal("recovery overwrote operator notes")
					}
				}
			}
		})
	}
}

type restoreDraft struct {
	pending bool
	commits int
}

func (d *restoreDraft) HasPendingChanges() bool     { return d.pending }
func (d *restoreDraft) CommitPendingChanges() error { d.commits++; d.pending = false; return nil }
func TestWorkspaceRestoreDoesNotCommitOrReplacePendingPropertyDraft(t *testing.T) {
	w := testWorkbench(t)
	path := filepath.Join(t.TempDir(), "f.silkui")
	writeRecoveryDesign(t, path, "Captured", "disk")
	entered, release := make(chan struct{}), make(chan struct{})
	posts := make(chan func(), 32)
	r := newWorkspaceRestore(workspaceSession{Design: path, Mode: modeDesign}, w.tabs, w.canvas, workspaceRestoreOptions{post: func(f func()) { posts <- f }, read: func(ctx context.Context, p string, d bool) (restoredFile, error) {
		v, e := readRestoredFile(ctx, p, d)
		close(entered)
		<-release
		return v, e
	}})
	released := false
	cleanupRestore(t, r, func() {
		if !released {
			close(release)
		}
	})
	<-entered
	draft := &restoreDraft{pending: true}
	w.canvas.GedScene().AddPendingEditor(draft)
	before := w.canvas.GedScene().FormTitle()
	close(release)
	released = true
	pumpRestore(t, posts, func() bool { return !r.Busy() })
	if draft.commits != 0 || !draft.pending || w.canvas.GedScene().FormTitle() != before {
		t.Fatal("background restore committed/discarded live property draft")
	}
}
func TestRestoreDesignNodeBudgetRejectsTruncatedPrefix(t *testing.T) {
	data := []byte("form\n" + strings.Repeat("\tchild\n", restoreNodeLimit+1))
	if _, e := decodeRestoreDesign(context.Background(), data); e == nil {
		t.Fatal("accepted a node-limit truncated design as complete")
	}
}

func TestWorkspaceRestoreInitialFocusTransfersButNeverStealsOtherInput(t *testing.T) {
	for _, other := range []bool{false, true} {
		t.Run(map[bool]string{false: "initial", true: "other input"}[other], func(t *testing.T) {
			w := testWorkbench(t)
			p := writeFile(t, t.TempDir(), "a.txt", "restored source")
			posts := make(chan func(), 32)
			entered, release := make(chan struct{}), make(chan struct{})
			r := newWorkspaceRestore(workspaceSession{Editors: []editorSession{{Path: p}}, ActiveFile: p, Mode: modeEdit}, w.tabs, nil, workspaceRestoreOptions{post: func(f func()) { posts <- f }, read: func(ctx context.Context, p string, d bool) (restoredFile, error) {
				close(entered)
				<-release
				return readRestoredFile(ctx, p, d)
			}})
			released := false
			cleanupRestore(t, r, func() {
				if !released {
					close(release)
				}
			})
			<-entered
			if !r.pages[0].HasFocus() {
				t.Fatal("initial loading source did not own keyboard focus")
			}
			field := gui.NewEdit()
			field.SetParent(w.frame)
			if other {
				field.SetFocus()
			}
			close(release)
			released = true
			pumpRestore(t, posts, func() bool { return !r.Busy() })
			ed := openEditors[p]
			if ed == nil {
				t.Fatal("source not restored")
			}
			if other {
				if !field.HasFocus() || ed.HasFocus() {
					t.Fatal("late restore stole another input's focus")
				}
			} else if !ed.HasFocus() {
				t.Fatal("restored source did not inherit initial keyboard focus")
			}
		})
	}
}

func TestWorkspaceRestoreMissingActiveSourcePassesInitialFocusToNextPage(t *testing.T) {
	w := testWorkbench(t)
	root := t.TempDir()
	missing := filepath.Join(root, "gone.txt")
	path := writeFile(t, root, "a.txt", "source")
	posts := make(chan func(), 32)
	release := make(chan struct{})
	r := newWorkspaceRestore(workspaceSession{Editors: []editorSession{{Path: missing}, {Path: path}}, ActiveFile: missing, Mode: modeEdit}, w.tabs, nil, workspaceRestoreOptions{post: func(f func()) { posts <- f }, read: func(ctx context.Context, p string, d bool) (restoredFile, error) {
		if p == missing {
			return restoredFile{}, os.ErrNotExist
		}
		<-release
		return readRestoredFile(ctx, p, d)
	}})
	released := false
	cleanupRestore(t, r, func() {
		if !released {
			close(release)
		}
	})
	pumpRestore(t, posts, func() bool { return w.tabs.Count() == 1 })
	if !w.tabs.Stack().CurrentPage().HasFocus() {
		t.Fatal("missing active source stranded initial keyboard focus")
	}
	close(release)
	released = true
	pumpRestore(t, posts, func() bool { return !r.Busy() })
	if ed := openEditors[path]; ed == nil || !ed.HasFocus() {
		t.Fatal("remaining source did not inherit keyboard focus")
	}
}
