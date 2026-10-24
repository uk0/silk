//go:build !windows

package gui

import (
	"context"
	"fmt"
	"runtime"
	"testing"

	"github.com/go-gl/glfw/v3.3/glfw"
)

func withEditActionModifiers(t *testing.T, mods glfw.ModifierKey) {
	t.Helper()
	previous := eventModifiers
	eventModifiers = &mods
	t.Cleanup(func() { eventModifiers = previous })
}

func editActionTestModifiers() []glfw.ModifierKey {
	mods := []glfw.ModifierKey{glfw.ModControl}
	if runtime.GOOS == "darwin" {
		mods = append(mods, glfw.ModSuper)
	}
	return mods
}

func TestEditActionSelectAllReplacesNumericPropertyAndSubmits(t *testing.T) {
	for _, mods := range editActionTestModifiers() {
		t.Run(fmt.Sprint(mods), func(t *testing.T) {
			withEditActionModifiers(t, mods)
			edit := NewEdit()
			edit.SetText("10")
			edit.SetSelection(2, 2)
			var submitted string
			edit.SigSubmit(func(_ interface{}, value string) { submitted = value })
			edit.OnKeyDown('A', false)
			if edit.SelectionText() != "10" {
				t.Fatal("platform Select All did not select the numeric field")
			}
			plain := glfw.ModifierKey(0)
			eventModifiers = &plain
			edit.OnTextInput("25")
			edit.OnKeyDown(KeyEnter, false)
			if edit.Text() != "25" || submitted != "25" {
				t.Fatalf("numeric property appended rather than replaced: text=%q submitted=%q", edit.Text(), submitted)
			}
		})
	}
}

func TestEditActionSelectAllCoversUnicodeAndReadOnlyText(t *testing.T) {
	for _, mods := range editActionTestModifiers() {
		t.Run(fmt.Sprint(mods), func(t *testing.T) {
			withEditActionModifiers(t, mods)
			edit := NewEdit()
			edit.SetText("颜色 🟦 25")
			edit.SetReadOnly(true)
			edit.OnKeyDown('A', false)
			if edit.SelectionText() != edit.Text() {
				t.Fatal("read-only/Unicode text was not fully selectable")
			}
		})
	}
}

func TestEditActionClipboardKeysAndReadOnlyProtection(t *testing.T) {
	// Supply an isolated process-local clipboard. These key-routing tests must
	// neither require a native window nor overwrite the user's OS clipboard.
	previousWindows := winMap
	winMap = nil
	clipboardMu.Lock()
	previousData := clipboardLocalData
	clipboardLocalData = make(map[string]interface{})
	clipboardMu.Unlock()
	t.Cleanup(func() {
		winMap = previousWindows
		clipboardMu.Lock()
		clipboardLocalData = previousData
		clipboardMu.Unlock()
	})
	for _, mods := range editActionTestModifiers() {
		t.Run(fmt.Sprint(mods), func(t *testing.T) {
			withEditActionModifiers(t, mods)
			edit := NewEdit()
			edit.SetText("10")
			edit.SelectAll()
			clipboardMu.Lock()
			clipboardLocalData["text/plain"] = "25"
			clipboardMu.Unlock()
			// Production macOS paste is asynchronous. Keep this a key-routing
			// test with an explicit completion, never access the OS pasteboard.
			var finish func(string, error)
			edit.clipboardReader = func(_ context.Context, done func(string, error)) error {
				finish = done
				return nil
			}
			edit.OnKeyDown('V', false)
			if finish == nil || edit.Text() != "10" || !edit.PastePending() {
				t.Fatal("paste did not defer delivery without changing the selection")
			}
			data, err := Clipboard.Data("text/plain")
			finish(data.(string), err)
			if edit.PastePending() {
				t.Fatal("paste completion left pending work")
			}
			if edit.Text() != "25" {
				t.Fatal("platform Paste did not replace selection")
			}
			edit.SelectAll()
			edit.OnKeyDown('C', false)
			clipboardMu.Lock()
			remaining := len(clipboardLocalData)
			clipboardMu.Unlock()
			if remaining != 0 || edit.Text() != "25" {
				t.Fatal("platform Copy was not routed to Copy or changed text")
			}
			edit.OnKeyDown('X', false)
			if edit.Text() != "" {
				t.Fatal("platform Cut did not remove selection")
			}
			edit.SetText("read only")
			edit.SetReadOnly(true)
			edit.SelectAll()
			for _, key := range []int{'X', 'V'} {
				edit.OnKeyDown(key, false)
				if edit.Text() != "read only" {
					t.Fatal("clipboard shortcut modified read-only text")
				}
			}
		})
	}
}

func TestEditActionKeysDoNotConsumePlainTextOrDisabledInput(t *testing.T) {
	withEditActionModifiers(t, 0)
	edit := NewEdit()
	edit.SetText("10")
	edit.SetSelection(2, 2)
	for _, key := range []int{'A', 'C', 'V', 'X'} {
		edit.OnKeyDown(key, false)
		if a, b := edit.Selection(); a != 2 || b != 2 || edit.Text() != "10" {
			t.Fatal("unmodified letter invoked an editing shortcut")
		}
	}
	edit.OnTextInput("acvx")
	if edit.Text() != "10acvx" {
		t.Fatal("plain text typing was consumed")
	}
	edit.SetEnabled(false)
	mods := glfw.ModControl
	eventModifiers = &mods
	edit.OnKeyDown('A', false)
	if edit.SelectionText() != "" {
		t.Fatal("disabled field accepted Select All")
	}
}

func TestEditSuperDoesNotBecomeControlOutsideMacOS(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("Super is the native action modifier on macOS")
	}
	withEditActionModifiers(t, glfw.ModSuper)
	edit := NewEdit()
	edit.SetText("10")
	edit.SetSelection(2, 2)
	edit.OnKeyDown('A', false)
	if edit.SelectionText() != "" {
		t.Fatal("non-macOS Super unexpectedly acted as Control")
	}
}

func TestSearchBoxActionSelectAllReplacesQuery(t *testing.T) {
	for _, mods := range editActionTestModifiers() {
		t.Run(fmt.Sprint(mods), func(t *testing.T) {
			withEditActionModifiers(t, mods)
			s := NewSearchBox()
			s.SetText("Button 中文 🟦")
			s.OnKeyDown('A', false)
			if s.SelectionText() != s.Text() {
				t.Fatal("search shortcut did not select all")
			}
			plain := glfw.ModifierKey(0)
			eventModifiers = &plain
			s.OnTextInput("GanttChart")
			if s.Text() != "GanttChart" || s.SelectionText() != "" {
				t.Fatal("new query was appended instead of replacing")
			}
		})
	}
}
