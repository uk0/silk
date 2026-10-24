package gui

import "testing"

func TestFindNoOpReplacementRefreshesCounter(t *testing.T) {
	e := NewCodeEditor()
	t.Cleanup(e.CancelFind)
	e.SetText("foo foo")
	e.SetFindQuery("foo")
	e.replaceText = "foo"
	e.ShowFind(true)
	e.replaceCurrent()
	if e.findCurrentIdx != 1 || e.findBar.status.Text() != "2 / 2" {
		t.Fatal("counter did not follow navigation", e.findCurrentIdx, e.findBar.status.Text())
	}
	if len(e.undoStack) != 0 {
		t.Fatal("no-op replacement created undo")
	}
}
func TestFindHiddenEditorDoesNotRestartScanOnExternalEdit(t *testing.T) {
	e, posts := asyncFindEditor(t)
	e.SetFindQuery("foo")
	waitCompletionPost(t, posts)()
	e.Hide()
	e.ReplaceAllText(e.Text() + "\nexternal")
	if e.findState.worker != nil || e.findState.pending {
		t.Fatal("hidden editor restarted scanning")
	}
}
