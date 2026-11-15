package gui

import (
	"reflect"
	"slices"
	"testing"
)

// Repeated edits exercise both private-worker rebasing and independent,
// immutable UI metadata after comments are created, removed or joined.
func FuzzGoBodyProjectionEditSequence(f *testing.F) {
	for _, s := range [][]byte{{0, 0, 1, 1, 1, 2, 1, 2, 2, 3, 3, 0}, {1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, {2, 0, 0, 2, 0, 0, 3, 0, 0, 4}, {3, 4, 5, 0, 1, 2, 6, 7, 8}} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, script []byte) {
		if len(script) == 0 || len(script) > 128 {
			return
		}
		sources := []string{
			"package p\n// before\nfunc F(){_ = \"a\"}\n// after\nfunc G(){}",
			"func F(){}\n// comment\nfunc G(){if true { _ = `}` }}",
			"package p\r\nfunc F(){\r\n// inner\r\n_ = \"😀\"\r\n}\r\nfunc G(){}",
			"package p\ntype T[P any] struct{}\nfunc(t *T[P]) F(x P) struct{V P}{ return struct{V P}{x} }\nfunc G(){}",
		}
		replacements := []string{"", "x", "/", "*", "\"", "}", "{", ";", "\n", "\r", "😀", "//", "\x00", "\ufeff"}
		text := sources[int(script[0])%len(sources)]
		var p goSymbolIndex
		before := p.parse(text)
		view := p.projection()
		for n := 1; n+3 < len(script); n += 4 {
			start, end := int(script[n])%(len(text)+1), int(script[n+1])%(len(text)+1)
			if len(p.bodies) > 0 && script[n+2]&1 == 0 {
				b := p.bodies[int(script[n])%len(p.bodies)]
				start = b.start + int(script[n+1])%(b.end-b.start+1)
				end = start + int(script[n+2])%min(4, b.end-start+1)
			}
			if start > end {
				start, end = end, start
			}
			next := text[:start] + replacements[int(script[n+3])%len(replacements)] + text[end:]
			want := parseGoCodeSymbolsText(next)
			var advanced *goSymbolProjection
			if view != nil {
				oldText := view.text
				comments, bodies := slices.Clone(view.comments), slices.Clone(view.bodies)
				advanced = view.advance(next)
				if view.text != oldText || !slices.Equal(view.comments, comments) || !slices.Equal(view.bodies, bodies) {
					t.Fatal("delivered metadata mutated")
				}
				if advanced != nil && !reflect.DeepEqual(before, want) {
					t.Fatalf("UI sequence changed exact fields: %q -> %q", text, next)
				}
			}
			got := p.parse(next)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("private sequence differs: %q -> %q got=%v want=%v", text, next, got, want)
			}
			if advanced == nil {
				view = p.projection()
			} else {
				view = advanced
			}
			text, before = next, got
		}
	})
}
