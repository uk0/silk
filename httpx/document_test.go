package httpx

import (
	"errors"
	"testing"

	"github.com/uk0/silk/jsonx"
)

func TestResponseDocumentImmutableAndLimits(t *testing.T) {
	r := &Response{body: []byte(`{"n":9223372036854775807,"values":[true,null]}`)}
	doc, err := r.Document()
	if err != nil {
		t.Fatal(err)
	}
	if n, err := doc.Int("/n"); err != nil || n != 9223372036854775807 {
		t.Fatal(n, err)
	}
	clone := doc.Root().Value().(map[string]any)
	clone["n"] = 0
	if n, err := r.Int("/n"); err != nil || n != 9223372036854775807 {
		t.Fatal("existing extraction changed", n, err)
	}
	if _, err := r.DocumentWithOptions(jsonx.Options{MaxBytes: 5}); !errors.Is(err, jsonx.ErrLimit) {
		t.Fatal(err)
	}
	var empty *Response
	if _, err := empty.Document(); !errors.Is(err, jsonx.ErrNotFound) {
		t.Fatal(err)
	}
}

func BenchmarkRepeatedJSONField(b *testing.B) {
	r := &Response{body: []byte(`{"data":{"count":9007199254740993,"name":"中文"}}`)}
	b.Run("ResponseIntReparse", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _ = r.Int("/data/count")
		}
	})
	doc, _ := r.Document()
	b.Run("DocumentIntReuse", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _ = doc.Int("/data/count")
		}
	})
}
