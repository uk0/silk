package gui

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestMenuTextInputFindsPrefixWithoutExecuting(t *testing.T) {
	oldHover, oldFocus := mouseHoverWidget, focusWidget
	t.Cleanup(func() { mouseHoverWidget, focusWidget = oldHover, oldFocus })
	m := visibleActivationMenu()
	m.AddButton1("Objects", nil)
	output := m.AddButton1("Output", nil)
	m.AddButton1("Outline", nil)
	calls := 0
	output.Action().BindFunc0(func() { calls++ })
	input, ok := any(m).(IEventTextInput)
	if !ok {
		t.Fatal("popup menus cannot navigate by typing a label")
	}
	input.OnTextInput("outp")
	if m.highlight != 1 || calls != 0 || !m.IsVisible() {
		t.Fatalf("highlight=%d calls=%d visible=%v", m.highlight, calls, m.IsVisible())
	}
	m.OnKeyDown(KeyEnter, false)
	if calls != 1 || m.IsVisible() {
		t.Fatal("Enter must execute only the selected item and close the popup")
	}
}

func typeaheadMenu(t *testing.T, labels ...string) *Menu {
	t.Helper()
	oldHover, oldFocus := mouseHoverWidget, focusWidget
	t.Cleanup(func() { mouseHoverWidget, focusWidget = oldHover, oldFocus })
	m := visibleActivationMenu()
	for _, label := range labels {
		m.AddButton1(label, nil)
	}
	return m
}

func TestMenuTypeaheadCyclesSkipsAndReadsLiveState(t *testing.T) {
	m := typeaheadMenu(t, "Search", "Save", "Settings", "Stop")
	m.items[1].SetEnabled(false)
	m.items[2].SetVisible(false)
	m.AddSeparator()
	m.OnTextInput("s")
	if m.highlight != 0 {
		t.Fatal(m.highlight)
	}
	m.OnTextInput("S")
	if m.highlight != 3 {
		t.Fatal(m.highlight)
	}
	m.OnTextInput("s")
	if m.highlight != 0 {
		t.Fatal(m.highlight)
	}
	m.items[1].SetEnabled(true)
	m.OnTextInput("s")
	if m.highlight != 1 {
		t.Fatal("enabled state stale", m.highlight)
	}
	m.items[1].(*Button).SetText("Rename")
	m.OnTextInput("r")
	if m.highlight != 1 {
		t.Fatal("label stale", m.highlight)
	}
	m.OnTextInput("z")
	if m.highlight != 1 {
		t.Fatal("unmatched query moved selection")
	}
}

func TestMenuTypeaheadUnicodeAndInvalidLabels(t *testing.T) {
	m := typeaheadMenu(t, "\xff", "  属性", "属性过滤", "Öffnen", "🙂Tools")
	m.OnTextInput("属性过")
	if m.highlight != 2 {
		t.Fatal("IME commit not matched", m.highlight)
	}
	m.OnKeyDown(KeyHome, false)
	m.OnTextInput("öF")
	if m.highlight != 3 {
		t.Fatal("Unicode case fold", m.highlight)
	}
	m.OnKeyDown(KeyHome, false)
	m.OnTextInput("🙂")
	if m.highlight != 4 {
		t.Fatal("emoji label", m.highlight)
	}
	for _, text := range []string{"\x00\n\t", "\xff", " "} {
		m.OnTextInput(text)
	}
	if m.highlight != 4 {
		t.Fatal("invalid text moved selection")
	}
}

func TestMenuTypeaheadTimeoutBackspaceAndNavigationReset(t *testing.T) {
	m := typeaheadMenu(t, "Objects", "Output", "Outline", "Properties")
	now := time.Now()
	m.typeaheadInput("outp", now)
	m.backspaceTypeahead(now.Add(time.Millisecond))
	if m.typePrefix != "out" || m.highlight != 1 {
		t.Fatal("backspace", m.typePrefix, m.highlight)
	}
	m.typeaheadInput("l", now.Add(2*time.Millisecond))
	if m.highlight != 2 {
		t.Fatal("edited prefix did not find Outline")
	}
	m.typeaheadInput("p", now.Add(menuTypeaheadTimeout+3*time.Millisecond))
	if m.highlight != 3 || m.typePrefix != "p" {
		t.Fatal("timeout did not start new query")
	}
	m.OnKeyDown(KeyHome, false)
	if m.typePrefix != "" {
		t.Fatal("navigation retained prefix")
	}
	m.typeaheadInput("outp", now)
	m.backspaceTypeahead(now.Add(2 * menuTypeaheadTimeout))
	if m.typePrefix != "" {
		t.Fatal("expired query backspace")
	}
	m.typeaheadInput("out", now)
	m.OnHide()
	if m.typePrefix != "" {
		t.Fatal("hide retained prefix")
	}
	m.typeaheadInput("out", now)
	m.RemoveWidget(m.items[0])
	if m.typePrefix != "" {
		t.Fatal("remove retained prefix")
	}
	m.typeaheadInput("out", now)
	m.Clear()
	if m.typePrefix != "" {
		t.Fatal("clear retained prefix")
	}
}

func TestMenuTypeaheadRequiresVisibleEnabledPopup(t *testing.T) {
	m := typeaheadMenu(t, "Search")
	m.SetEnabled(false)
	m.OnTextInput("s")
	if m.highlight != -1 {
		t.Fatal("disabled popup")
	}
	m.SetEnabled(true)
	m.setVisible(false)
	m.OnTextInput("s")
	if m.highlight != -1 {
		t.Fatal("hidden popup")
	}
	bar := NewMenuBar()
	bar.AddButton1("Search", nil)
	bar.OnTextInput("s")
	if bar.highlight != -1 {
		t.Fatal("menu bar consumed input")
	}
	parent := NewLabel("owner")
	m.SetParent(parent)
	m.setVisible(true)
	parent.Hide()
	m.OnTextInput("s")
	if m.highlight != -1 {
		t.Fatal("hidden ancestor")
	}
}

func TestMenuTypeaheadBoundedInputAndPrefix(t *testing.T) {
	m := typeaheadMenu(t, strings.Repeat("ab", 100), "Properties")
	m.OnTextInput(strings.Repeat("ab", 100000))
	if utf8.RuneCountInString(m.typePrefix) > menuTypeaheadLimit {
		t.Fatal("unbounded prefix")
	}
	m.resetTypeahead()
	m.highlight = -1
	m.OnTextInput(strings.Repeat("\n", menuTypeaheadLimit) + "p")
	if m.highlight != -1 {
		t.Fatal("input event scanning exceeded bound")
	}
}

func BenchmarkMenuPrefixLookup(b *testing.B) {
	m := visibleActivationMenu()
	for i := 0; i < 1000; i++ {
		m.AddButton1(fmt.Sprintf("Command %04d", i), nil)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if got := m.prefixIndex("Command 0999", false); got != 999 {
			b.Fatal(got)
		}
	}
}
