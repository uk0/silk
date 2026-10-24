package jsonx

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/uk0/silk/async"
)

func TestPointersTypesAndIndependentValues(t *testing.T) {
	d, err := Parse([]byte(`{"":null,"a/b":{"~":[9007199254740993,"中文😀",true,1.25]},"a":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := d.Int("/a~1b/~0/0"); err != nil || got != 9007199254740993 {
		t.Fatal(got, err)
	}
	if got, err := d.Str("/a~1b/~0/1"); err != nil || got != "中文😀" {
		t.Fatal(got, err)
	}
	if got, err := d.Bool("/a~1b/~0/2"); err != nil || !got {
		t.Fatal(got, err)
	}
	if got, err := d.Float("/a~1b/~0/3"); err != nil || got != 1.25 {
		t.Fatal(got, err)
	}
	n, err := d.Lookup("/")
	if err != nil || n.Kind() != Null {
		t.Fatal(n, err)
	}
	v := d.Root().Value().(map[string]any)
	v["a"] = 5
	delete(v, "a/b")
	if got, _ := d.Int("/a"); got != 1 {
		t.Fatal("caller mutated document")
	}
	children, err := d.Root().Children(0, 2)
	if err != nil || len(children) != 2 || children[0].Key() != "" || children[1].Key() != "a" {
		t.Fatal(children, err)
	}
	if children[1].Value().(json.Number) != "1" {
		t.Fatal("precision not retained")
	}
	if JoinPointer("/a~1b", "~") != "/a~1b/~0" {
		t.Fatal("double escaping")
	}
	for _, p := range []string{"a", "#/a", "/~", "/~2", "/a~1b/~0/01", "/a~1b/~0/-", "/a~1b/~0/+1"} {
		if _, err := d.Lookup(p); !errors.Is(err, ErrPointer) {
			t.Errorf("%q: %v", p, err)
		}
	}
	for _, p := range []string{"/missing", "/a~1b/~0/99999999999999999999999", "/a/x"} {
		if _, err := d.Lookup(p); !errors.Is(err, ErrNotFound) {
			t.Errorf("%q: %v", p, err)
		}
	}
	if _, err := d.Int("/a~1b/~0/1"); !errors.Is(err, ErrType) {
		t.Fatal(err)
	}
	if _, err := d.Int("/a~1b/~0/3"); err == nil {
		t.Fatal("fraction truncated")
	}
	for _, s := range []string{"9223372036854775808", "1e3"} {
		doc, _ := Parse([]byte(s))
		if _, err := doc.Int(""); err == nil {
			t.Fatal("non-int accepted", s)
		}
	}
	huge, _ := Parse([]byte("1e9999"))
	if _, err := huge.Float(""); err == nil {
		t.Fatal("infinity accepted")
	}
}
func TestLimitsInvalidInputAndZeroValues(t *testing.T) {
	for _, s := range []string{"", "{} {}", "{", "null trailing", string([]byte{'"', 255, '"'})} {
		if _, err := Parse([]byte(s)); err == nil {
			t.Fatalf("invalid accepted: %q", s)
		}
	}
	for _, o := range []Options{{MaxBytes: 2}, {MaxDepth: 1}, {MaxNodes: 2}} {
		if _, err := ParseWithOptions([]byte(`{"x":[1]}`), o); !errors.Is(err, ErrLimit) {
			t.Fatal(o, err)
		}
	}
	if _, err := ParseWithOptions([]byte(`null`), Options{MaxNodes: -1}); err == nil {
		t.Fatal("negative limit accepted")
	}
	var d *Document
	if _, err := d.Lookup(""); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	var n Node
	if n.Valid() || n.Kind() != Invalid || n.Len() != 0 || n.Preview(50) != "" {
		t.Fatal("bad zero node")
	}
	for _, a := range [][2]int{{-1, 2}, {0, -1}, {1, 1}} {
		if _, err := n.Children(a[0], a[1]); !errors.Is(err, ErrRange) {
			t.Fatal(a, err)
		}
	}
}
func TestPreviewBoundedAndConcurrentRead(t *testing.T) {
	d, _ := Parse([]byte(`{"text":"中文😀\nabcdef","array":[1,2]}`))
	n, _ := d.Lookup("/text")
	for _, limit := range []int{0, 1, 2, 4, 20} {
		p := n.Preview(limit)
		if utf8.RuneCountInString(p) > limit || !utf8.ValidString(p) {
			t.Fatal(limit, p)
		}
	}
	a, _ := d.Lookup("/array")
	if a.Preview(50) != "[2 items]" {
		t.Fatal(a.Preview(50))
	}
	if previewPrefix("1234", 4) != "1234" || previewPrefix("12345", 4) != "123…" {
		t.Fatal("bad preview boundary")
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Go(func() {
			for j := 0; j < 50; j++ {
				if got, e := d.Int("/array/1"); e != nil || got != 2 {
					t.Error(got, e)
				}
				b, _ := d.Root().JSON()
				if !json.Valid(b) {
					t.Error("invalid copy")
				}
			}
		})
	}
	wg.Wait()
}
func TestAsyncSnapshotAndCancellation(t *testing.T) {
	pool := async.NewExecutor(1)
	defer pool.Close()
	b := []byte(`{"value":42}`)
	future, err := ParseAsync(context.Background(), pool, b, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for i := range b {
		b[i] = 'x'
	}
	d, err := future.Await(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := d.Int("/value"); n != 42 {
		t.Fatal(n)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f, e := ParseAsync(ctx, pool, []byte(`null`), Options{})
	if e == nil {
		_, e = f.Await(context.Background())
	}
	if !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if _, err := ParseAsync(context.Background(), pool, []byte("123"), Options{MaxBytes: 2}); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
}
func FuzzPointerRoundTrip(f *testing.F) {
	for _, s := range []string{"", "a/b", "~1", "中文😀", "\x00"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if !utf8.ValidString(s) {
			return
		}
		tokens, e := SplitPointer(JoinPointer("", s))
		if e != nil || len(tokens) != 1 || tokens[0] != s {
			t.Fatal(tokens, e)
		}
	})
}
func BenchmarkLookupParsed(b *testing.B) {
	d, _ := Parse([]byte(`{"data":{"count":9007199254740993,"name":"中文"}}`))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = d.Int("/data/count")
	}
}
func BenchmarkPreviewLargeString(b *testing.B) {
	data, _ := json.Marshal(strings.Repeat("中", 100000))
	d, _ := Parse(data)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = d.Root().Preview(80)
	}
}
