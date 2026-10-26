package gui

import (
	"errors"
	"github.com/uk0/silk/paint"
	"strings"
	"testing"
)

func TestRenameDialogSDKLocalUndo(t *testing.T) {
	e := NewCodeEditor()
	t.Cleanup(e.CancelCompletion)
	source := "package p\r\nfunc Old() {}\r\nfunc call() { Old() }\r\n"
	e.SetText(source)
	e.ScrollToLineCol(2, 15)
	old, count, err := e.RenameSymbolAtCursor("New")
	if err != nil || old != "Old" || count != 2 {
		t.Fatal(old, count, err)
	}
	if !e.CanUndo() {
		t.Fatal("local rename lost undo transaction")
	}
	e.Undo()
	if e.Text() != source {
		t.Fatal("rename undo did not restore original bytes")
	}
	e.Redo()
	if e.Text() != strings.ReplaceAll(source, "Old", "New") {
		t.Fatal("rename redo failed")
	}
}
func TestRenameDialogKeyboardUsesHost(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("oldName")
	calls := 0
	e.SetSymbolActions(CodeEditorSymbolActions{RenameSymbol: func(got *CodeEditor) {
		if got != e {
			t.Fatal("wrong owner")
		}
		calls++
	}})
	e.activateRename()
	if calls != 1 || e.renameActive {
		t.Fatal("keyboard bypassed semantic host and entered textual rename", calls)
	}
}
func TestRenameDialogInputValidationAPI(t *testing.T) {
	b := NewInputBox()
	v, ok := any(b).(interface {
		SetValidator(Validator)
		HasAcceptableInput() bool
		ValidationError() string
		Text() string
	})
	if !ok {
		t.Fatal("InputBox has no reusable validation API")
	}
	v.SetValidator(ValidatorFunc(func(s string) error {
		if s == "valid" {
			return nil
		}
		return errors.New("enter valid")
	}))
	b.SetMessage("bad")
	if v.HasAcceptableInput() || v.ValidationError() != "enter valid" || b.btnBox.btnMap["@ok"].IsEnabled() {
		t.Fatal("invalid submission is enabled")
	}
	b.edit.SelectAll()
	b.edit.OnTextInput("valid")
	if !v.HasAcceptableInput() || v.ValidationError() != "" || !b.btnBox.btnMap["@ok"].IsEnabled() || v.Text() != "valid" {
		t.Fatal("valid draft did not reenable submission")
	}
}

func TestRenameDialogIdentifierDrafts(t *testing.T) {
	v := GoIdentifierValidator()
	for _, s := range []string{"", "_", "for", "123", "name other", " name", "name\n", "a🙂", "a\u0301", "\xff"} {
		if v.Validate(s) != Intermediate {
			t.Errorf("invalid draft accepted: %q", s)
		}
		if v.(ErrorMessager).ErrorMessage(s) == "" {
			t.Errorf("missing reason: %q", s)
		}
	}
	for _, s := range []string{"变量", "foo2", "_private", "À"} {
		if v.Validate(s) != Acceptable {
			t.Errorf("valid identifier rejected: %q", s)
		}
	}
	b := NewInputBox()
	b.SetValidator(v)
	b.SetMessage("for")
	b.SetValidator(nil)
	if !b.HasAcceptableInput() || b.ValidationError() != "" || !b.btnBox.btnMap["@ok"].IsEnabled() {
		t.Fatal("nil validator did not restore free-form dialog")
	}
}

func TestRenameDialogWordLookupBytesAndNoMutation(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("\xff🙂 变量2(foo)\r\n")
	for _, tc := range []struct {
		col  int
		want string
	}{{0, ""}, {1, ""}, {3, "变量2"}, {4, "变量2"}, {6, "变量2"}, {7, "foo"}, {10, "foo"}, {11, ""}, {-1, ""}, {50, ""}} {
		e.cursorCol = tc.col
		if got := e.WordAtCursor(); got != tc.want {
			t.Errorf("col=%d got=%q want=%q", tc.col, got, tc.want)
		}
		if e.cursorCol != tc.col {
			t.Fatal("read accessor moved caret")
		}
	}
	e.SetText("oldName " + strings.Repeat("tail ", 200000))
	e.cursorCol = 3
	if n := testing.AllocsPerRun(100, func() {
		if e.WordAtCursor() != "oldName" {
			panic("bad word")
		}
	}); n != 0 {
		t.Fatal("word lookup copies long line", n)
	}
}

func TestRenameDialogLocalPromptGuardsOrigin(t *testing.T) {
	saved := showRenameSymbolInput
	t.Cleanup(func() { showRenameSymbolInput = saved })
	for _, kind := range []string{"ok", "cancel", "same", "edit_undo", "caret", "readonly", "disabled", "path", "parent"} {
		t.Run(kind, func(t *testing.T) {
			e := NewCodeEditor()
			source := "package p\nfunc Old() {}\nfunc call() { Old() }\n"
			e.SetText(source)
			e.ScrollToLineCol(2, 15)
			showRenameSymbolInput = func(parent IWidget, ico paint.Icon, title, label, def string, v Validator) (string, bool) {
				if parent != e || def != "Old" || v.Validate("for") == Acceptable {
					t.Fatal("prompt configuration")
				}
				switch kind {
				case "cancel":
					return "New", false
				case "same":
					return def, true
				case "edit_undo":
					e.ReplaceAllText(source + "\n")
					e.Undo()
				case "caret":
					e.ScrollToLineCol(1, 6)
				case "readonly":
					e.SetReadOnly(true)
				case "disabled":
					e.SetEnabled(false)
				case "path":
					e.SetFilePath("changed.go")
				case "parent":
					e.SetParent(NewForm())
				}
				return "New", true
			}
			e.promptRenameSymbol("Old")
			if kind == "ok" {
				if e.Text() != strings.ReplaceAll(source, "Old", "New") || !e.CanUndo() {
					t.Fatal("valid prompt did not rename")
				}
			} else if e.Text() != source {
				t.Fatal("stale/cancel prompt renamed source", kind)
			}
		})
	}
}

func TestRenameDialogLayoutContained(t *testing.T) {
	old := CurrentThemeMode()
	t.Cleanup(func() { SetThemeMode(old) })
	for _, mode := range []ThemeMode{ThemeLight, ThemeDark} {
		SetThemeMode(mode)
		b := NewInputBox()
		b.SetLabel(strings.Repeat("Long prompt ", 8))
		b.SetValidator(ValidatorFunc(func(string) error { return errors.New(strings.Repeat("invalid draft ", 6)) }))
		b.SetMessage("bad")
		for _, size := range [][2]float64{{380, 240}, {180, 170}, {40, 50}, {0, 0}, {640, 300}} {
			b.SetSize(size[0], size[1])
			b.Layout()
			footerY := b.btnBox.Y()
			for _, w := range []IWidget{b.prompt, b.edit, b.validation, b.btnBox} {
				x, y := w.Pos()
				width, height := w.Size()
				if x < 0 || y < 0 || width < 0 || height < 0 || x+width > size[0]+.001 || y+height > size[1]+.001 {
					t.Fatalf("outside %v: %T %v %v %v %v", size, w, x, y, width, height)
				}
			}
			if b.validation.Y()+b.validation.Height() > footerY+.001 || b.edit.Y()+b.edit.Height() > footerY+.001 {
				t.Fatal("input/status covers footer")
			}
		}
	}
}

func BenchmarkRenameDialogWordLookup(b *testing.B) {
	e := NewCodeEditor()
	e.SetText("oldName " + strings.Repeat("longline ", 200000))
	e.cursorCol = 3
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if e.wordAtCursor() != "oldName" {
			b.Fatal("word missing")
		}
	}
}
