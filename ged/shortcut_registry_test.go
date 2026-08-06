package ged

import (
	"testing"

	"github.com/uk0/silk/gui"
)

// panelRows returns every (chord, description) row the 快捷键 panel draws,
// keyed by category, so the tests below can compare the panel against the
// registry in both directions.
func panelRows(p *ShortcutsPanel) map[string][]shortcutEntry {
	out := map[string][]shortcutEntry{}
	for _, cat := range p.categories {
		out[cat.name] = append(out[cat.name], cat.shortcuts...)
	}
	return out
}

// TestShortcutsPanelListsEveryDesignerBinding: the 快捷键 panel is the only
// place a user can learn what the canvas responds to, so a binding missing from
// it is a binding nobody finds. The hand-written list this replaced had drifted
// from the canvas — Esc, Ctrl+A, Ctrl+D, Ctrl+P and F2 were all implemented and
// none of them were documented.
func TestShortcutsPanelListsEveryDesignerBinding(t *testing.T) {
	rows := panelRows(NewShortcutsPanel())
	for _, sc := range designerShortcuts {
		found := false
		for _, row := range rows[sc.Category] {
			if row.keys == sc.chord() && row.description == sc.Label {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("快捷键 panel has no %q row for %s (%s) under %q",
				sc.chord(), sc.Command, sc.Label, sc.Category)
		}
	}
}

// TestShortcutsPanelShowsNothingElse is the same check in the other direction:
// a row the panel draws that no binding backs is a promise the canvas breaks.
func TestShortcutsPanelShowsNothingElse(t *testing.T) {
	declared := map[string]bool{}
	for _, sc := range designerShortcuts {
		declared[sc.Category+"\x00"+sc.chord()+"\x00"+sc.Label] = true
	}
	for cat, rows := range panelRows(NewShortcutsPanel()) {
		for _, row := range rows {
			if !declared[cat+"\x00"+row.keys+"\x00"+row.description] {
				t.Errorf("快捷键 panel draws %q / %q under %q but designerShortcuts "+
					"declares no such binding", row.keys, row.description, cat)
			}
		}
	}
}

// TestChordString pins the rendering the panel and the drift test both read.
func TestChordString(t *testing.T) {
	cases := []struct {
		mods uint8
		key  int
		want string
	}{
		{0, gui.KeyTab, "Tab"},
		{gui.ModShift, gui.KeyTab, "Shift+Tab"},
		{gui.ModAction, 'A', "Ctrl+A"},
		{gui.ModAlt, 'L', "Alt+L"},
		{gui.ModAction | gui.ModShift, 'S', "Ctrl+Shift+S"},
		{0, gui.KeyEsc, "Esc"},
		{0, gui.KeyDelete, "Delete"},
		{0, gui.KeyF2, "F2"},
		{0, gui.KeyF5, "F5"},
		{0, '1', "1"},
		{0, 0, ""},
	}
	for _, tc := range cases {
		if got := chordString(tc.mods, tc.key); got != tc.want {
			t.Errorf("chordString(%#x, %#x) = %q, want %q", tc.mods, tc.key, got, tc.want)
		}
	}
}

// TestDesignerShortcutsAgreeWithKeymap: ged.KeyMap is the surface a user
// rebinds on and designerShortcuts is what the 快捷键 panel prints. Both are
// written by hand, so a command they share must carry the same chord and the
// same context — otherwise the panel advertises one key while 快捷键设置 offers
// another.
func TestDesignerShortcutsAgreeWithKeymap(t *testing.T) {
	documented := map[string]ShortcutDef{}
	for _, sc := range designerShortcuts {
		documented[sc.Command] = sc
	}
	for _, b := range defaultKeymap {
		sc, ok := documented[b.Command]
		if !ok {
			t.Errorf("ged/keymap.go binds %s to %q but designerShortcuts does not "+
				"document it, so the 快捷键 panel never shows it", b.Command, b.Key)
			continue
		}
		if got := sc.chord(); got != b.Key {
			t.Errorf("%s: designerShortcuts says %q, ged/keymap.go defaultKeymap says %q",
				b.Command, got, b.Key)
		}
		if sc.Context != b.Context {
			t.Errorf("%s: designerShortcuts context %q, defaultKeymap context %q",
				b.Command, sc.Context, b.Context)
		}
	}
}

// TestDesignerShortcutsHaveNoDuplicates: two commands on one chord in one
// context means the second is dead — OnKeyDown's switch takes the first
// matching case — and two entries for one command means the panel prints it
// twice. The same chord in two different contexts is fine and intended: F2
// renames on the canvas and steps bookmarks in the editor.
func TestDesignerShortcutsHaveNoDuplicates(t *testing.T) {
	byChord := map[string]string{}
	byCommand := map[string]bool{}
	for _, sc := range designerShortcuts {
		k := sc.Context + "\x00" + sc.chord()
		if prev, dup := byChord[k]; dup {
			t.Errorf("chord %q in context %q is bound to both %s and %s",
				sc.chord(), sc.Context, prev, sc.Command)
		}
		byChord[k] = sc.Command
		if byCommand[sc.Command] {
			t.Errorf("command %s appears twice in designerShortcuts", sc.Command)
		}
		byCommand[sc.Command] = true
	}
}

// TestDesignerShortcutLabelsAreFilledIn: an entry with no label draws a blank
// row, and one with no category lands in an unnamed group.
func TestDesignerShortcutLabelsAreFilledIn(t *testing.T) {
	for _, sc := range designerShortcuts {
		if sc.Label == "" || sc.Category == "" || sc.Command == "" {
			t.Errorf("incomplete entry: %+v", sc)
		}
		if sc.Context != "global" && sc.Context != "editor" {
			t.Errorf("%s has context %q, want global or editor", sc.Command, sc.Context)
		}
		if sc.chord() == "" {
			t.Errorf("%s renders an empty chord: set Key, or Chord for a key family", sc.Command)
		}
	}
}
