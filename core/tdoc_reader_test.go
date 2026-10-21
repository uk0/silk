package core

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uk0/silk/diagnostic"
)

func tdocReadValue(t *testing.T, d *TDoc, key, want string) {
	t.Helper()
	if d == nil {
		t.Fatal("nil document")
	}
	var value string
	if err := d.ReadAttr(key, &value); err != nil || value != want {
		t.Fatalf("key=%q got=%q want=%q err=%v", key, value, want, err)
	}
}
func tdocGzip(t *testing.T, plain string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	if _, err := io.WriteString(w, plain); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

type tdocOnceFaultReader struct {
	data []byte
	err  error
}

func (r *tdocOnceFaultReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		if r.err != nil {
			err := r.err
			r.err = nil
			return 0, err
		}
		return 0, io.EOF
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	if len(r.data) == 0 {
		err := r.err
		r.err = nil
		return n, err
	}
	return n, nil
}

type tdocByteReader struct{ r io.Reader }

func (r tdocByteReader) Read(p []byte) (int, error) {
	if len(p) > 1 {
		p = p[:1]
	}
	return r.r.Read(p)
}

func TestTDocReaderEscapedKeysRoundTrip(t *testing.T) {
	for _, compressed := range []bool{false, true} {
		for _, key := range []string{"with space", "with\ttab", "with\nnewline", "with\rcarriage", "with\\slash", "with=equals", "中文😀 space"} {
			d := NewTDoc()
			if err := d.WriteAttr(key, "value"); err != nil {
				t.Fatal(err)
			}
			var b bytes.Buffer
			if err := d.Save1(&b, compressed); err != nil {
				t.Fatal(err)
			}
			got, err := LoadTDoc(&b)
			if err != nil {
				t.Fatal(err)
			}
			tdocReadValue(t, got, key, "value")
		}
	}
}
func TestTDocReaderPropagatesUnderlyingReadErrors(t *testing.T) {
	for _, input := range []string{"", "=", "=\n", "=\nkey = \"partial", "=\nkey = \"value\"\n"} {
		t.Run(strings.ReplaceAll(input, "\n", "/"), func(t *testing.T) {
			d, err := LoadTDoc(&tdocOnceFaultReader{data: []byte(input), err: io.ErrClosedPipe})
			if d != nil || !errors.Is(err, io.ErrClosedPipe) || diagnostic.CodeOf(err) != diagnostic.Code("SILK-DOC002") {
				t.Fatalf("doc=%v err=%v", d, err)
			}
		})
	}
}
func TestTDocReaderRejectsCorruptCompressedStreams(t *testing.T) {
	for _, plain := range []string{"=\nkey = \"value\"\n", "=\nkey = \"value\"\n\nignored tail\n"} {
		good := tdocGzip(t, plain)
		for _, kind := range []string{"header", "trailer", "checksum"} {
			data := append([]byte(nil), good...)
			var want error
			switch kind {
			case "header":
				data = data[:3]
				want = io.ErrUnexpectedEOF
			case "trailer":
				data = data[:len(data)-4]
				want = io.ErrUnexpectedEOF
			case "checksum":
				data[len(data)-8] ^= 0xff
				want = gzip.ErrChecksum
			}
			d, err := LoadTDoc(bytes.NewReader(data))
			if d != nil || !errors.Is(err, want) || diagnostic.CodeOf(err) != diagnostic.Code("SILK-DOC003") {
				t.Errorf("%s: doc=%v err=%v want=%v", kind, d, err, want)
			}
		}
	}
}
func TestTDocReaderGzipMembersKeepTheirBoundary(t *testing.T) {
	first := tdocGzip(t, "=\nkey = \"first\"\n")
	second := tdocGzip(t, "=\nkey = \"second\"\n")
	br := bufio.NewReader(bytes.NewReader(append(first, second...)))
	for _, want := range []string{"first", "second"} {
		d, err := LoadTDoc(br)
		if err != nil {
			t.Fatal(err)
		}
		tdocReadValue(t, d, "key", want)
	}
	if _, err := br.ReadByte(); !errors.Is(err, io.EOF) {
		t.Fatal("trailing member bytes", err)
	}
}
func TestTDocReaderPlainFramingAndLineLimits(t *testing.T) {
	for _, newline := range []string{"\n", "\r\n", "\r", "\n\r"} {
		input := "first = \"one\"" + newline + newline + "second = \"two\"" + newline
		for _, chunked := range []bool{false, true} {
			var r io.Reader = strings.NewReader(input)
			if chunked {
				r = tdocByteReader{r}
			}
			br := bufio.NewReader(r)
			for _, want := range []string{"first", "second"} {
				d, err := LoadTDoc(br)
				if err != nil || d.Key() != want {
					t.Fatalf("newline=%q chunked=%v key=%v err=%v", newline, chunked, d, err)
				}
			}
		}
		br := bufio.NewReader(tdocByteReader{strings.NewReader("root = \"one\"" + newline + "child = \"two\"" + newline)})
		d, err := LoadTDoc1(br, 1)
		if err != nil || d.Key() != "root" || d.Len() != 0 {
			t.Fatalf("limited: %v %v", d, err)
		}
		d, err = LoadTDoc(br)
		if err != nil || d.Key() != "child" {
			t.Fatalf("remaining: %v %v", d, err)
		}
	}
	// Read-ahead must not reject a valid prefix based on a later transport error.
	br := bufio.NewReader(&tdocOnceFaultReader{data: []byte("root = \"one\"\nchild = \"two\""), err: io.ErrClosedPipe})
	if d, err := LoadTDoc1(br, 1); err != nil || d.Key() != "root" {
		t.Fatalf("prefix rejected: %v %v", d, err)
	}
	if d, err := LoadTDoc(br); d != nil || !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("suffix error lost: %v %v", d, err)
	}
}
func TestTDocReaderLimitedCompressedPrefixDoesNotClaimIntegrity(t *testing.T) {
	data := tdocGzip(t, "root = \"one\"\nchild = \"two\"\n")
	data[len(data)-8] ^= 0xff
	if d, err := LoadTDoc1(bytes.NewReader(data), 1); err != nil || d.Key() != "root" || d.Len() != 0 {
		t.Fatalf("limited prefix: %v %v", d, err)
	}
	if _, err := LoadTDoc(bytes.NewReader(data)); !errors.Is(err, gzip.ErrChecksum) {
		t.Fatalf("full load must validate: %v", err)
	}
}

func TestTDocReaderDoesNotDropBoundaryReadErrors(t *testing.T) {
	for _, newline := range []string{"\n", "\r", "\r\n", "\n\r"} {
		for _, limit := range []uint64{0, 1} {
			input := "root" + newline
			if limit == 0 {
				input += newline // Complete document delimiter, not an incomplete node.
			}
			br := bufio.NewReader(&tdocOnceFaultReader{data: []byte(input), err: io.ErrClosedPipe})
			_, err := LoadTDoc1(br, limit)
			if err == nil {
				// Paired-newline lookahead may not have reached the underlying error.
				// It must then remain observable by the caller, not disappear as EOF.
				_, err = LoadTDoc(br)
			}
			if !errors.Is(err, io.ErrClosedPipe) || diagnostic.CodeOf(err) != diagnostic.DocumentRead {
				t.Errorf("newline=%q limit=%d: boundary error lost: %v", newline, limit, err)
			}
		}
	}
}

func TestTDocReaderIndentDiagnosticLocations(t *testing.T) {
	for _, newline := range []string{"\n", "\r", "\r\n", "\n\r"} {
		input := "=" + newline + "valid = 1" + newline + "\t\tinvalid = 2" + newline
		for _, compressed := range []bool{false, true} {
			var r io.Reader = strings.NewReader(input)
			if compressed {
				r = bytes.NewReader(tdocGzip(t, input))
			}
			d, err := LoadTDoc(r)
			var detail *diagnostic.Error
			if d != nil || !errors.As(err, &detail) || detail.Code != diagnostic.DocumentSyntax ||
				detail.Location.Line != 3 || detail.Location.Column != 1 || detail.Location.Offset != len("="+newline+"valid = 1"+newline) {
				t.Errorf("newline=%q compressed=%v: location=%+v err=%v", newline, compressed, detail, err)
			}
		}
	}
}
func TestTDocReaderSyntaxDiagnosticsAndFileLocation(t *testing.T) {
	for _, input := range []string{" =\n", "=\n child = x\n", "=\n\t\tchild = x\n", "root\\", "=\nkey = \"a\x00b\"\n", "=\nkey = \"\xff\"\n"} {
		d, err := LoadTDoc(strings.NewReader(input))
		var detail *diagnostic.Error
		if d != nil || !errors.As(err, &detail) || detail.Code != diagnostic.Code("SILK-DOC001") || detail.Hint == "" || detail.Location.Line < 1 {
			t.Errorf("input=%q doc=%v err=%v", input, d, err)
		}
	}
	path := filepath.Join(t.TempDir(), "broken.tdoc")
	if err := os.WriteFile(path, []byte("=\n bad = x\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadTDocFile(path)
	var detail *diagnostic.Error
	if !errors.As(err, &detail) || detail.Location.File != path || detail.Location.Line != 2 || detail.Location.Column != 1 {
		t.Fatalf("file diagnostics: %v", err)
	}
	if _, err = LoadTDocFile(path + ".missing"); !errors.Is(err, os.ErrNotExist) || diagnostic.CodeOf(err) != diagnostic.Code("SILK-DOC002") {
		t.Fatalf("open error: %v", err)
	}
}
