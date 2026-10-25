package locator

import (
	"errors"
	"strings"
	"testing"
)

func TestParseFileQuery(t *testing.T) {
	for _, tc := range []struct{ input, path, location string }{
		{"", "", ""}, {"  main.go  ", "main.go", ""}, {"gui/main.go:12", "gui/main.go", "12"},
		{" main.go:12:3 ", "main.go", "12:3"}, {"main.go:0001:02", "main.go", "0001:02"},
		{`C:\src\main.go:12:3`, `C:\src\main.go`, "12:3"}, {`C:\src\main.go`, `C:\src\main.go`, ""},
		{`C:123`, `C:123`, ""}, {`C:`, `C:`, ""}, {`\\server\share\x.go:2`, `\\server\share\x.go`, "2"},
		{`"report:2026"`, `report:2026`, ""}, {`"report:2026":3:2`, `report:2026`, "3:2"},
		{`"dir with spaces/main.go":4`, `dir with spaces/main.go`, "4"},
		{"目录/😀.go:2:3", "目录/😀.go", "2:3"}, {"dir:123/main.go", "dir:123/main.go", ""},
		{"notes:abc", "notes:abc", ""}, {"report:2026", "report", "2026"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			q, err := ParseFileQuery(tc.input)
			if err != nil || q.Path != tc.path || q.Location != tc.location {
				t.Fatalf("%q -> %+v %v", tc.input, q, err)
			}
		})
	}
	for _, input := range []string{`"missing`, `""`, `"x"oops`, `"x":`, "x.go:", "x.go:0", "x.go:-1", "x.go:+2", "x.go:1:0", "x.go:1:", "x.go:1:2x", ":23", "x.go:" + strings.Repeat("9", 70), strings.Repeat("x", MaxFileQueryLength+1)} {
		t.Run("invalid-"+input[:min(len(input), 30)], func(t *testing.T) {
			q, err := ParseFileQuery(input)
			if !errors.Is(err, ErrFileQuerySyntax) || q != (FileQuery{}) {
				t.Fatalf("invalid query actionable: %+v %v", q, err)
			}
		})
	}
}
func FuzzParseFileQuery(f *testing.F) {
	for _, s := range []string{"main.go:12:4", `C:\src\x.go:9`, `"x:1":3`, ":1", `a:`, `目录/🙂.go:2:3`} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		q, err := ParseFileQuery(s)
		if err != nil {
			if q != (FileQuery{}) {
				t.Fatal("actionable error")
			}
			return
		}
		if len(s) > MaxFileQueryLength {
			t.Fatal("unbounded query")
		}
		if q.Location != "" {
			line, col, hasCol := strings.Cut(q.Location, ":")
			if q.Path == "" || !positiveField(line) || hasCol && !positiveField(col) {
				t.Fatal("invalid location", q)
			}
		}
	})
}
func BenchmarkParseFileQuery(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = ParseFileQuery(`C:\source\project\gui\editor.go:120:24`)
	}
}
