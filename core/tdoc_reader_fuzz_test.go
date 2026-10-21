package core

import (
	"bytes"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/uk0/silk/diagnostic"
)

func FuzzTDocPlainReader(f *testing.F) {
	for _, seed := range []string{"", "=", "=\r\nkey = 1\r\n", "root\\", "=\n\t\tjump = 1\n", "=\na\\sb = 2\n", "=\n\x00", "=\n\xff"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > 8192 || strings.HasPrefix(input, "\x1f\x8b") {
			t.Skip() // gzip integrity/truncation is covered independently.
		}
		doc, err := LoadTDoc(strings.NewReader(input))
		if err != nil {
			if doc != nil || diagnostic.CodeOf(err) == diagnostic.Internal {
				t.Fatalf("uncoded error or partial result: %v %v", doc, err)
			}
			return
		}
		var canonical, reencoded bytes.Buffer
		if err := doc.Save(&canonical); err != nil {
			t.Fatal(err)
		}
		roundTrip, err := LoadTDoc(bytes.NewReader(canonical.Bytes()))
		if err != nil {
			t.Fatalf("cannot reload canonical document %q: %v", canonical.String(), err)
		}
		if err := roundTrip.Save(&reencoded); err != nil || !bytes.Equal(canonical.Bytes(), reencoded.Bytes()) {
			t.Fatalf("unstable canonical document: %q -> %q (%v)", canonical.String(), reencoded.String(), err)
		}
	})
}

func FuzzTDocKeyValueRoundTrip(f *testing.F) {
	f.Add("中文 😀 /\\=\t\n\r", "value\n\x00\xff")
	f.Add("", "")
	f.Add(" space ", "quote\"backslash\\")
	f.Fuzz(func(t *testing.T, key, value string) {
		if len(key)+len(value) > 8192 || !utf8.ValidString(key) {
			t.Skip()
		}
		for _, ch := range key {
			if ch < 32 && ch != '\t' && ch != '\n' && ch != '\r' {
				t.Skip() // Other control characters are not representable as TDoc keys.
			}
		}
		doc, child := NewTDoc(), NewTDoc()
		child.SetKey(key)
		if err := child.SetValue(value); err != nil {
			t.Fatal(err)
		}
		doc.AddChild(child)
		for _, compressed := range []bool{false, true} {
			var b bytes.Buffer
			if err := doc.Save1(&b, compressed); err != nil {
				t.Fatal(err)
			}
			loaded, err := LoadTDoc(&b)
			if err != nil || loaded.Len() != 1 || loaded.Child(0).Key() != key {
				t.Fatalf("key lost: %q compressed=%v doc=%v err=%v", key, compressed, loaded, err)
			}
			var got string
			if err := loaded.Child(0).Value(&got); err != nil || got != value {
				t.Fatalf("value lost: got=%q want=%q err=%v", got, value, err)
			}
		}
	})
}
