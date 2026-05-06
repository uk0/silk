package decl_test

// i18n_test.go pins the decl ↔ i18n bridge: a TrKey value must
// resolve through i18n.T() at Build time so widgets render in the
// active locale. These tests live in *_test (external) because they
// need silk/gui's real factories to produce a *gui.Button whose
// Text() can be inspected.

import (
	"testing"

	"silk/decl"
	"silk/gui"
	"silk/i18n"
)

// withLocale runs fn with the i18n.Default translator swapped to a
// fresh, isolated instance carrying the supplied translation table.
// Restores the prior translator on return so tests don't leak state.
func withLocale(t *testing.T, locale string, table map[string]string, fn func()) {
	t.Helper()
	saved := *i18n.Default
	defer func() { *i18n.Default = saved }()

	*i18n.Default = *i18n.NewTranslator()
	if len(table) > 0 {
		i18n.AddMany(locale, table)
	}
	if locale != "" {
		i18n.SetLocale(locale)
	}
	fn()
}

// TestTrKeyResolvesAtBuildTime: a Button declared with TrKey("OK")
// must end up with the locale-translated text after Build().
func TestTrKeyResolvesAtBuildTime(t *testing.T) {
	withLocale(t, "zh-CN", map[string]string{"OK": "确定"}, func() {
		n := decl.Button(decl.ID("ok"), decl.P("text", decl.TrKey{Source: "OK"}))
		obj, err := n.Build()
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		btn, ok := obj.(*gui.Button)
		if !ok {
			t.Fatalf("Build returned %T, want *gui.Button", obj)
		}
		if btn.Text() != "确定" {
			t.Errorf("btn.Text() = %q, want 确定 (zh-CN translation)", btn.Text())
		}
	})
}

// TestTrKeyFallsBackToSourceWhenNoLocale: with no locale set, a
// TrKey value resolves to the source string. Apps that don't ship
// translations still render text — the fundamental "translation off"
// guarantee inherited from i18n.T.
func TestTrKeyFallsBackToSourceWhenNoLocale(t *testing.T) {
	withLocale(t, "", nil, func() {
		n := decl.Button(decl.P("text", decl.TrKey{Source: "Save"}))
		obj, err := n.Build()
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		btn := obj.(*gui.Button)
		if btn.Text() != "Save" {
			t.Errorf("btn.Text() = %q, want Save", btn.Text())
		}
	})
}

// TestTrKeyMissFallsBackToSource: locale active but the specific
// key isn't translated — fall back to the source rather than blank.
func TestTrKeyMissFallsBackToSource(t *testing.T) {
	withLocale(t, "zh-CN", map[string]string{"Edit": "编辑"}, func() {
		// "Save" is not in the table; should pass through.
		n := decl.Button(decl.P("text", decl.TrKey{Source: "Save"}))
		obj, _ := n.Build()
		if got := obj.(*gui.Button).Text(); got != "Save" {
			t.Errorf("missing translation = %q, want Save", got)
		}
	})
}

// TestTrKeyTDocRoundTrip: serialise a tree containing a TrKey,
// parse it back, and verify the Value is still TrKey (not a Lit).
// Without this, a designer save+reload would silently downgrade
// translation keys into hardcoded strings.
func TestTrKeyTDocRoundTrip(t *testing.T) {
	orig := decl.Button(decl.ID("ok"),
		decl.P("text", decl.TrKey{Source: "OK"}))
	doc := decl.ToTDoc(orig)
	got, err := decl.FromTDoc(doc)
	if err != nil {
		t.Fatalf("FromTDoc: %v", err)
	}
	v, _ := got.PropValue("text")
	tk, ok := v.(decl.TrKey)
	if !ok {
		t.Fatalf("after round-trip, Value = %#v, want TrKey", v)
	}
	if tk.Source != "OK" {
		t.Errorf("TrKey.Source = %q, want OK", tk.Source)
	}
}

// TestTrKeyLocaleSwitchBeforeBuildAffectsResult: changing the
// locale between AST construction and Build() must update the text.
// Without this guarantee, an app that delayed widget construction
// past startup might render the wrong language.
func TestTrKeyLocaleSwitchBeforeBuildAffectsResult(t *testing.T) {
	saved := *i18n.Default
	defer func() { *i18n.Default = saved }()
	*i18n.Default = *i18n.NewTranslator()
	i18n.AddMany("zh-CN", map[string]string{"OK": "确定"})
	i18n.AddMany("ja", map[string]string{"OK": "OK (ja)"})

	n := decl.Button(decl.P("text", decl.TrKey{Source: "OK"}))

	// Build under zh-CN.
	i18n.SetLocale("zh-CN")
	objZh, _ := n.Build()
	if got := objZh.(*gui.Button).Text(); got != "确定" {
		t.Errorf("zh-CN build = %q, want 确定", got)
	}

	// Build same AST under ja — should produce the Japanese form.
	i18n.SetLocale("ja")
	objJa, _ := n.Build()
	if got := objJa.(*gui.Button).Text(); got != "OK (ja)" {
		t.Errorf("ja build = %q, want OK (ja)", got)
	}
}
