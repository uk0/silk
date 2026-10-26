package gui

import (
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/paint"
	"testing"
)

func TestDialogPolishInputBoxIconProperty(t *testing.T) {
	b := NewInputBox()
	icon := paint.LoadIcon("document")
	b.SetIcon(icon)
	if b.Icon() != icon {
		t.Fatal("InputBox SetIcon and inherited Icon disagree")
	}
	b.SetIcon(nil)
	if b.Icon() != nil {
		t.Fatal("nil did not restore icon inheritance")
	}
}
func TestDialogPolishWindowIconAccessor(t *testing.T) {
	if _, ok := any(&Window{}).(interface{ Icon() paint.Icon }); !ok {
		t.Fatal("native window does not expose its logical icon for owned forms")
	}
}
func TestDialogPolishLocalizedDraftAndStandardButtons(t *testing.T) {
	old := i18n.Locale()
	t.Cleanup(func() { i18n.SetLocale(old) })
	for _, locale := range []string{"en", "zh-CN"} {
		t.Run(locale, func(t *testing.T) {
			i18n.SetLocale(locale)
			b := NewInputBox()
			b.SetValidator(GoIdentifierValidator())
			b.SetMessage("for")
			keyword, empty, okText, cancel := "Use a Go identifier, not a keyword or blank identifier.", "Enter a new identifier name.", "OK", "Cancel"
			if locale == "zh-CN" {
				keyword, empty, okText, cancel = "请使用 Go 标识符，不能是关键字或 _。", "请输入新的标识符名称。", "确定", "取消"
			}
			if b.ValidationError() != keyword {
				t.Errorf("keyword reason = %q", b.ValidationError())
			}
			if b.btnBox.btnMap["@ok"].Text() != okText || b.btnBox.btnMap["@cancel"].Text() != cancel {
				t.Error("standard dialog buttons did not follow locale")
			}
			b.SetMessage("")
			if b.ValidationError() != empty {
				t.Errorf("empty reason = %q", b.ValidationError())
			}
			b.SetMessage("新名称")
			if !b.HasAcceptableInput() {
				t.Fatal("Chinese identifier rejected")
			}
			box := NewButtonBox()
			box.SetButtons([]string{"@ok", "@cancel", "Cancel", "custom", "@save"})
			if box.btnMap["Cancel"].Text() != "Cancel" || box.btnMap["custom"].Text() != "custom" {
				t.Fatal("custom captions were silently translated")
			}
			var result string
			box.SigSubmit(func(s string) { result = s })
			for _, key := range []string{"@ok", "@cancel", "Cancel", "custom", "@save"} {
				b := box.btnMap[key]
				box.onBtnClick(b.Action(), b)
				if result != key {
					t.Fatal("localized caption changed command payload", key, result)
				}
			}
		})
	}
}

func TestDialogPolishInitialIconPrecedence(t *testing.T) {
	old := appIcon
	t.Cleanup(func() { appIcon = old })
	app, parentIcon, explicit := paint.LoadIcon("document"), paint.LoadIcon("folder"), paint.LoadIcon("save")
	SetAppIcon(app)
	parent := &Window{icon: parentIcon}
	constructors := []struct {
		name string
		make func() IWidget
	}{
		{"form", func() IWidget { return NewForm() }},
		{"input", func() IWidget { return NewInputBox() }},
		{"message", func() IWidget { return NewMessageBox() }},
		{"dialog", func() IWidget { return NewDialog("title", nil) }},
	}
	for _, tc := range constructors {
		t.Run(tc.name, func(t *testing.T) {
			w := tc.make()
			setter := w.(interface{ SetIcon(paint.Icon) })
			if initialWindowIcon(w, nil) != app || initialWindowIcon(w, parent) != parentIcon {
				t.Fatal("application/owner fallback failed")
			}
			setter.SetIcon(explicit)
			if initialWindowIcon(w, parent) != explicit {
				t.Fatal("explicit icon lost to owner")
			}
			setter.SetIcon(nil)
			if initialWindowIcon(w, parent) != parentIcon {
				t.Fatal("nil did not restore owner")
			}
			setter.SetIcon(paint.AirIcon())
			if initialWindowIcon(w, parent) != parentIcon {
				t.Fatal("empty icon hid owner")
			}
			if w.Window() != nil {
				t.Fatal("reading or setting icon eagerly created a native window")
			}
		})
	}
	SetAppIcon(nil)
	if initialWindowIcon(NewForm(), nil) != nil {
		t.Fatal("unconfigured standalone form synthesized a missing-image icon")
	}
}

func BenchmarkDialogPolishInitialIconSelection(b *testing.B) {
	old := appIcon
	b.Cleanup(func() { appIcon = old })
	icon := paint.LoadIcon("document")
	SetAppIcon(icon)
	w := NewInputBox()
	parent := &Window{icon: icon}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if initialWindowIcon(w, parent) != icon {
			b.Fatal("identity lost")
		}
	}
}
