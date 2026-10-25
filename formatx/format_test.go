package formatx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/uk0/silk/async"
)

func TestJSONFormatPreservesLexicalValuesAndIsIdempotent(t *testing.T) {
	for _, raw := range []string{`{"n":9007199254740993,"n":1e+009,"s":"中🙂\\\"{}[]","escape":"\u0041","a":[{},[],true,null]}`, `[]`, `{}`, `"text"`, `1.00`, `false`, " { \"x\" : [ 1, 2 ] } \r\n"} {
		var canonical bytes.Buffer
		if err := json.Compact(&canonical, []byte(raw)); err != nil {
			t.Fatal(err)
		}
		for _, compact := range []bool{false, true} {
			got, err := Format(context.Background(), "json", raw, Options{Compact: compact})
			if err != nil {
				t.Fatal(err)
			}
			var want bytes.Buffer
			if compact {
				want.Write(canonical.Bytes())
			} else {
				if err := json.Indent(&want, canonical.Bytes(), "", "  "); err != nil {
					t.Fatal(err)
				}
				want.WriteByte('\n')
			}
			if got != want.String() {
				t.Fatalf("got %q want %q", got, want.String())
			}
			again, err := Format(context.Background(), "json", got, Options{Compact: compact})
			if err != nil || again != got {
				t.Fatal("not idempotent", err)
			}
			size, err := jsonSize(context.Background(), canonical.String(), true)
			if !compact && (err != nil || size != len(got)) {
				t.Fatal("expansion estimate", size, len(got), err)
			}
		}
	}
}

func TestFormatErrorsNeverReturnPartialText(t *testing.T) {
	for _, tc := range []struct {
		language, text string
		options        Options
		target         error
	}{
		{"json", `{"broken":]}`, Options{}, nil}, {"json", `{} {}`, Options{}, nil},
		{"json", `// comment`, Options{}, nil}, {"json", "", Options{}, nil},
		{"go", "package", Options{}, nil}, {"go", "package p", Options{Compact: true}, ErrLanguage},
		{"plaintext", "hello", Options{}, ErrLanguage}, {"silkgo", "@macro", Options{}, ErrLanguage},
		{"json", "\"\xff\"", Options{}, ErrUTF8},
		{"json", strings.Repeat("[", MaxJSONDepth+1) + "0" + strings.Repeat("]", MaxJSONDepth+1), Options{}, ErrTooDeep},
		{"json", strings.Repeat(" ", MaxInputBytes+1), Options{}, ErrTooLarge},
	} {
		got, err := Format(context.Background(), tc.language, tc.text, tc.options)
		if got != "" || err == nil || tc.target != nil && !errors.Is(err, tc.target) {
			t.Fatal(tc.language, len(tc.text), got, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if s, err := Format(ctx, "go", "package p", Options{}); s != "" || !errors.Is(err, context.Canceled) {
		t.Fatal(s, err)
	}
	if _, err := Format(nil, "go", "package p", Options{}); !errors.Is(err, async.ErrInvalid) {
		t.Fatal(err)
	}
}

func TestJSONExpansionIsRejectedBeforeAllocation(t *testing.T) {
	raw := strings.Repeat("[", MaxJSONDepth) + strings.Repeat("0,", 70000) + "0" + strings.Repeat("]", MaxJSONDepth)
	if _, err := Format(context.Background(), "json", raw, Options{}); !errors.Is(err, ErrTooLarge) {
		t.Fatal(err)
	}
	if got, err := Format(context.Background(), "json", raw, Options{Compact: true}); err != nil || got != raw {
		t.Fatal("compact needlessly rejected", err)
	}
}

func TestFormatAsyncAndClosedExecutor(t *testing.T) {
	e := async.NewExecutor(1)
	defer e.Close()
	f, err := FormatAsync(context.Background(), e, "go", "package p\nfunc f( ){ }", Options{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.Await(context.Background())
	if err != nil || got != "package p\n\nfunc f() {}\n" {
		t.Fatal(got, err)
	}
	e.Close()
	if _, err := FormatAsync(context.Background(), e, "json", "{}", Options{}); !errors.Is(err, async.ErrClosed) {
		t.Fatal(err)
	}
}

func BenchmarkFormatJSON(b *testing.B) {
	text := `[` + strings.Repeat(`{"text":"中🙂","n":9007199254740993},`, 999) + `null]`
	b.ReportAllocs()
	b.SetBytes(int64(len(text)))
	for b.Loop() {
		if _, err := Format(context.Background(), "json", text, Options{}); err != nil {
			b.Fatal(err)
		}
	}
}
