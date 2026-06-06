package main

import (
	"reflect"
	"testing"
)

// TestSplitRunArgs pins the shell-style tokenisation
// runProjectInTerminal relies on. Quoted strings have to survive intact
// — a typical Go program reads `-msg "hello world"` as two args, not
// three — and an unquoted backslash escapes the next byte so a Windows
// user can paste `-path C:\foo\bar` without it falling apart on the
// backslashes.
func TestSplitRunArgs(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"empty returns nil", "", nil},
		{"whitespace returns nil", "   \t  ", nil},
		{"single arg", "-v", []string{"-v"}},
		{"multiple unquoted args", "-port 8080 -v", []string{"-port", "8080", "-v"}},
		{"collapsed whitespace", "  -port    8080   -v  ", []string{"-port", "8080", "-v"}},
		{
			"double-quoted argument keeps spaces",
			`-msg "hello world"`,
			[]string{"-msg", "hello world"},
		},
		{
			"single-quoted argument keeps spaces",
			`-msg 'hello world'`,
			[]string{"-msg", "hello world"},
		},
		{
			"backslash escapes space",
			`-path /tmp/with\ space/file`,
			[]string{"-path", "/tmp/with space/file"},
		},
		{
			"mixed quoted + unquoted",
			`-x 1 -y "two words" -z three`,
			[]string{"-x", "1", "-y", "two words", "-z", "three"},
		},
		{
			"unterminated quote flushes what was collected",
			`-msg "open`,
			[]string{"-msg", "open"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := splitRunArgs(c.in)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("splitRunArgs(%q) = %#v, want %#v", c.in, got, c.want)
			}
		})
	}
}
