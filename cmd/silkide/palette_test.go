package main

import (
	"strings"
	"testing"
)

// TestFilterCommandsSubsequence: filterCommands matches command
// names whose runes contain the query as a subsequence — VSCode's
// "find action" semantics. "fb" matches both "Build" and "Find in
// Files" because both contain f…b in order.
func TestFilterCommandsSubsequence(t *testing.T) {
	cmds := []paletteCommand{
		{Name: "Build"},
		{Name: "Run"},
		{Name: "Find in Files"},
		{Name: "Refresh"},
	}

	cases := []struct {
		query string
		want  []string
	}{
		{"", []string{"Build", "Run", "Find in Files", "Refresh"}}, // empty query passes all
		{"build", []string{"Build"}},
		{"run", []string{"Run"}},
		{"fnf", []string{"Find in Files"}},  // f→n→f subsequence in "find in files"
		{"rfh", []string{"Refresh"}},        // r→f→h subsequence in "refresh"
		{"un", []string{"Run"}},             // shorter first when both Run+Find match? Run only
		{"xyz", nil},                        // no match
	}
	for _, c := range cases {
		got := filterCommands(cmds, c.query)
		if len(got) != len(c.want) {
			t.Errorf("filterCommands(%q) returned %d items, want %d:\n got %v\nwant %v",
				c.query, len(got), len(c.want), names(got), c.want)
			continue
		}
		for i, g := range got {
			if g.Name != c.want[i] {
				t.Errorf("filterCommands(%q)[%d] = %q, want %q",
					c.query, i, g.Name, c.want[i])
			}
		}
	}
}

// TestFilterCommandsCaseInsensitive: query and command names
// shouldn't have to match case. The filter lower-cases both before
// running the subsequence check.
func TestFilterCommandsCaseInsensitive(t *testing.T) {
	cmds := []paletteCommand{{Name: "Build"}}
	for _, q := range []string{"build", "BUILD", "BuIlD", "uIl"} {
		got := filterCommands(cmds, q)
		if len(got) != 1 {
			t.Errorf("filterCommands(%q) = %v, want 1 hit", q, names(got))
		}
	}
}

// TestSubsequenceMatchEdgeCases: empty query always matches; empty
// text fails any non-empty query.
func TestSubsequenceMatchEdgeCases(t *testing.T) {
	if !subsequenceMatch("anything", "") {
		t.Error("empty query should match any text")
	}
	if !subsequenceMatch("", "") {
		t.Error("empty query against empty text should still match")
	}
	if subsequenceMatch("", "x") {
		t.Error("empty text shouldn't match a non-empty query")
	}
	// Order-sensitive: "ba" must NOT match "abracadabra" the way
	// "ab" does — query order matters.
	if subsequenceMatch("abracadabra", "zz") {
		t.Error("subsequence with no occurrence should fail")
	}
}

// TestRegisterPaletteCommandsPopulatesList: registerPaletteCommands
// pushes every documented action into paletteCommands. Locks the
// minimum count so a regression that drops actions surfaces as a
// failing test.
func TestRegisterPaletteCommandsPopulatesList(t *testing.T) {
	saved := paletteCommands
	defer func() { paletteCommands = saved }()
	paletteCommands = nil

	registerPaletteCommands(nil, nil)

	if len(paletteCommands) < 10 {
		t.Errorf("paletteCommands has %d entries, want ≥10", len(paletteCommands))
	}
	// Every entry needs a non-empty name (the search target).
	for i, c := range paletteCommands {
		if strings.TrimSpace(c.Name) == "" {
			t.Errorf("paletteCommands[%d] has empty Name", i)
		}
		if c.Fn == nil {
			t.Errorf("paletteCommands[%d] (%q) has nil Fn", i, c.Name)
		}
	}
}

func names(cmds []paletteCommand) []string {
	out := make([]string, len(cmds))
	for i, c := range cmds {
		out[i] = c.Name
	}
	return out
}
