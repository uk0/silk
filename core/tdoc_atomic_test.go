package core

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestTDocAtomicSaveRoundTrip(t *testing.T) {
	for _, gz := range []bool{false, true} {
		name := "plain"
		if gz {
			name = "gzip"
		}
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "文档.tdoc")
			doc := NewTDoc()
			doc.SetValue("root")
			doc.WriteAttr("message", "保留😀 and exact source")
			var expected bytes.Buffer
			if err := doc.Save1(&expected, gz); err != nil {
				t.Fatal(err)
			}
			if err := doc.SaveFileAtomic1(path, gz); err != nil {
				t.Fatal(err)
			}
			actual, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(actual, expected.Bytes()) {
				t.Fatalf("encoding changed: %v", err)
			}
			read, err := LoadTDocFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var message string
			read.ReadAttr("message", &message)
			if message != "保留😀 and exact source" {
				t.Fatalf("roundtrip=%q", message)
			}
			doc.WriteAttr("message", "next")
			if err := doc.SaveFileAtomic(path); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestTDocAtomicSaveFailurePreservesReadOnlyOriginal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "protected.tdoc")
	if err := os.WriteFile(path, []byte("original"), 0400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0600) })
	doc := NewTDoc()
	doc.SetValue("replacement")
	if err := doc.SaveFileAtomic(path); err == nil {
		t.Fatal("read-only file replaced")
	}
	actual, err := os.ReadFile(path)
	if err != nil || string(actual) != "original" {
		t.Fatalf("original=%q %v", actual, err)
	}
}
