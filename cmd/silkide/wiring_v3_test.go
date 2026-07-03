package main

import (
	"testing"

	"github.com/uk0/silk/core"
)

// TestFormatProjectStatusVariants pins the status-bar project-cell label
// shape across the four combinations the IDE will see at startup: no
// metadata, go.mod only, go.work only (with and without Uses), and both
// sources present. The status-bar cell is the first place a user spots a
// workspace project; the variants below double as documentation for what
// the label means.
func TestFormatProjectStatusVariants(t *testing.T) {
	cases := []struct {
		name    string
		project string
		module  string
		work    *core.GoWork
		want    string
	}{
		{"plain project", "silk", "", nil, "silk"},
		{"module only", "silk", "silk/example", nil, "silk · silk/example"},
		{"workspace empty Uses", "silk", "", &core.GoWork{}, "silk · workspace"},
		{
			"workspace with Uses",
			"silk", "",
			&core.GoWork{Uses: []string{"./a", "./b", "./c"}},
			"silk · workspace(3)",
		},
		{
			"module + workspace with Uses",
			"silk", "silk/example",
			&core.GoWork{Uses: []string{"./a", "./b", "./c"}},
			"silk · silk/example · workspace(3)",
		},
		{
			"module + workspace empty Uses",
			"silk", "silk/example",
			&core.GoWork{},
			"silk · silk/example · workspace",
		},
		// Empty project basename (filepath.Base("") == ".") gets the same
		// "silkide" fallback the unmodified status-bar code used. Without
		// the fallback the label would lead with "· module" — a visual
		// glitch the empty-cwd test path could surface.
		{"empty project falls back", "", "silk/example", nil, "silkide · silk/example"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := formatProjectStatus(c.project, c.module, c.work)
			if got != c.want {
				t.Errorf("formatProjectStatus(%q, %q, %+v) = %q, want %q",
					c.project, c.module, c.work, got, c.want)
			}
		})
	}
}

// TestEscapeTestRunRegexBasics pins the regex-escape policy the
// runSingleTest spawn uses: every metacharacter Go's regexp engine
// recognises in a `-run` pattern gets a leading backslash, so a subtest
// name like "Foo/Bar" or "Test.Suite/Case_1" doesn't accidentally widen
// the match into other tests' rows.
func TestEscapeTestRunRegexBasics(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"TestFoo", "TestFoo"},
		// '/' is a subtest separator for `go test -run`, not a regex
		// metacharacter — leave it unescaped so "Suite/Case" still selects
		// the subtest the user clicked on.
		{"TestFoo/Bar", "TestFoo/Bar"},
		{"TestFoo.Bar", `TestFoo\.Bar`},
		{"TestFoo|Bar", `TestFoo\|Bar`},
		{"TestFoo(Bar)", `TestFoo\(Bar\)`},
		{"TestFoo[Bar]", `TestFoo\[Bar\]`},
		{"TestFoo{Bar}", `TestFoo\{Bar\}`},
		{"TestFoo^Bar$", `TestFoo\^Bar\$`},
		{"TestFoo+Bar*", `TestFoo\+Bar\*`},
		{"TestFoo?Bar", `TestFoo\?Bar`},
		{`Test\Foo`, `Test\\Foo`},
		{"", ""},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got := escapeTestRunRegex(c.in)
			if got != c.want {
				t.Errorf("escapeTestRunRegex(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
