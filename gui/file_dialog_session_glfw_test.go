//go:build !windows

package gui

import "testing"

type testNativeFileDialogSession struct {
	path              string
	done              bool
	cancelled, closed int
	onClose           func()
}

func (s *testNativeFileDialogSession) result() (string, bool) { return s.path, s.done }
func (s *testNativeFileDialogSession) cancel()                { s.cancelled++ }
func (s *testNativeFileDialogSession) close() {
	s.closed++
	if s.onClose != nil {
		s.onClose()
	}
}

func TestNativeFileDialogPumpsAndRestoresOnlyItsOwner(t *testing.T) {
	for _, accepted := range []bool{false, true} {
		for _, originallyEnabled := range []bool{false, true} {
			func() {
				previousFrame, previousQuit := nativeDialogFrame, shouldQuit
				defer func() { nativeDialogFrame, shouldQuit = previousFrame, previousQuit }()
				shouldQuit = false
				owner := &Window{enabled: originallyEnabled}
				other := &Window{enabled: true}
				session := new(testNativeFileDialogSession)
				session.onClose = func() {
					if owner.IsEnabled() != originallyEnabled {
						t.Error("native teardown ran before owner state restored")
					}
				}
				frames := 0
				nativeDialogFrame = func() {
					frames++
					if owner.IsEnabled() || !other.IsEnabled() {
						t.Fatal("native sheet must suspend only its owner")
					}
					if frames == 2 {
						session.done = true
						if accepted {
							// Unlike shell stdout parsing, native results must preserve
							// valid leading/trailing spaces and Unicode in filenames.
							session.path = "/tmp/设计 document.go "
						}
					}
				}
				path := waitNativeFileDialog(owner, session, func() bool { return true })
				if path != session.path || frames != 2 || session.cancelled != 0 || session.closed != 1 {
					t.Fatalf("result=%q frames=%d cancel/close=%d/%d", path, frames, session.cancelled, session.closed)
				}
				if owner.IsEnabled() != originallyEnabled || !other.IsEnabled() {
					t.Fatal("completion changed original window enabled states")
				}
			}()
		}
	}
}

func TestNativeFileDialogCancelsForOwnerCloseOrQuit(t *testing.T) {
	for _, reason := range []string{"owner-close", "quit"} {
		t.Run(reason, func(t *testing.T) {
			previousFrame, previousQuit := nativeDialogFrame, shouldQuit
			defer func() { nativeDialogFrame, shouldQuit = previousFrame, previousQuit }()
			shouldQuit = false
			owner := &Window{enabled: true}
			session := new(testNativeFileDialogSession)
			alive := true
			nativeDialogFrame = func() {
				if reason == "quit" {
					shouldQuit = true
				} else {
					alive = false
				}
			}
			if path := waitNativeFileDialog(owner, session, func() bool { return alive }); path != "" {
				t.Fatal("closing owner accepted an incomplete panel result")
			}
			if session.cancelled != 1 || session.closed != 1 || !owner.IsEnabled() {
				t.Fatal("cancellation left a native session or disabled owner behind")
			}
		})
	}
}

func TestNativeFileDialogUnwindRestoresOwnerWithoutSwallowingPanic(t *testing.T) {
	previousFrame, previousQuit := nativeDialogFrame, shouldQuit
	defer func() { nativeDialogFrame, shouldQuit = previousFrame, previousQuit }()
	shouldQuit = false
	owner := &Window{enabled: true}
	session := new(testNativeFileDialogSession)
	nativeDialogFrame = func() { panic("UI callback failure") }
	defer func() {
		if value := recover(); value != "UI callback failure" {
			t.Fatalf("UI panic was swallowed/replaced: %v", value)
		}
		if !owner.IsEnabled() || session.closed != 1 {
			t.Fatal("unwind stranded a disabled owner or unreleased native panel")
		}
	}()
	waitNativeFileDialog(owner, session, func() bool { return true })
}

func TestFileDialogFocusReturnRejectsUnavailableOwners(t *testing.T) {
	for _, state := range []string{"ready", "nil", "detached", "replaced", "disabled", "hidden", "minimized", "quit"} {
		t.Run(state, func(t *testing.T) {
			isolateFocusReturns(t)
			owner, _ := focusReturnForm(t)
			switch state {
			case "nil":
				owner = nil
			case "detached":
				owner.widget = nil
			case "replaced":
				owner.widget = NewForm()
			case "disabled":
				owner.enabled = false
			case "hidden":
				owner.widget.SetVisible(false)
			case "minimized":
				owner.iconified = true
			case "quit":
				shouldQuit = true
			}
			if got := fileDialogOwnerCanRestoreFocus(owner); got != (state == "ready") {
				t.Fatalf("restore=%v state=%s", got, state)
			}
		})
	}
}
