package core

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"strings"
	"testing"
)

type tdocFaultWriter struct {
	calls  int
	failAt int
	short  bool
	data   bytes.Buffer
}

func (w *tdocFaultWriter) Write(p []byte) (int, error) {
	w.calls++
	if w.calls >= w.failAt {
		if w.short {
			return len(p) / 2, nil
		}
		return 0, io.ErrClosedPipe
	}
	return w.data.Write(p)
}
func TestTDocSaveReportsBufferedAndCompressedFailures(t *testing.T) {
	for _, compressed := range []bool{false, true} {
		for _, buffered := range []bool{false, true} {
			for _, large := range []bool{false, true} {
				name := "plain"
				if compressed {
					name = "gzip"
				}
				if buffered {
					name += "/caller-buffer"
				}
				if large {
					name += "/large"
				}
				t.Run(name, func(t *testing.T) {
					d := NewTDoc()
					value := "small"
					if large {
						value = strings.Repeat("large-data", 10000)
					}
					if err := d.WriteAttr("key", value); err != nil {
						t.Fatal(err)
					}
					for _, short := range []bool{false, true} {
						sink := &tdocFaultWriter{failAt: 1, short: short}
						var w io.Writer = sink
						if buffered {
							w = bufio.NewWriterSize(w, 32)
						}
						err := d.Save1(w, compressed)
						want := io.ErrClosedPipe
						if short {
							want = io.ErrShortWrite
						}
						if !errors.Is(err, want) {
							t.Errorf("short=%v: Save1=%v, want %v; actual writes=%d", short, err, want, sink.calls)
						}
					}
				})
			}
		}
	}
}
func TestTDocSaveReportsGzipFinalizationFailure(t *testing.T) {
	// gzip's first Write emits its header; the compressed body/trailer is only
	// emitted on Close for this small document. Save must report that late error.
	d := NewTDoc()
	_ = d.WriteAttr("key", "small")
	sink := &tdocFaultWriter{failAt: 2}
	if err := d.Save1(sink, true); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("Save1=%v, writes=%d", err, sink.calls)
	}
}
func TestTDocSaveFlushesCallerBufferAndRoundTrips(t *testing.T) {
	for _, compressed := range []bool{false, true} {
		d := NewTDoc()
		_ = d.WriteAttr("editor/key", "hello 世界")
		var data bytes.Buffer
		w := bufio.NewWriterSize(&data, 4096)
		if err := d.Save1(w, compressed); err != nil {
			t.Fatal(err)
		}
		if w.Buffered() != 0 {
			t.Fatalf("compressed=%v: caller buffer not flushed: %d", compressed, w.Buffered())
		}
		var raw io.Reader = &data
		if compressed {
			r, err := gzip.NewReader(raw)
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			raw = r
		}
		got, err := LoadTDoc(raw)
		if err != nil {
			t.Fatal(err)
		}
		var value string
		if err = got.ReadAttr("editor/key", &value); err != nil || value != "hello 世界" {
			t.Fatalf("roundtrip: value=%q err=%v", value, err)
		}
	}
}

type tdocFaultCloser struct {
	tdocFaultWriter
	closeErr error
	closed   int
}

func (w *tdocFaultCloser) Close() error { w.closed++; return w.closeErr }
func TestTDocSaveFileReportsWriteAndCloseErrors(t *testing.T) {
	closeErr := errors.New("close failed")
	for _, failWrite := range []bool{false, true} {
		w := &tdocFaultCloser{tdocFaultWriter: tdocFaultWriter{failAt: 100}, closeErr: closeErr}
		if failWrite {
			w.failAt = 1
		}
		err := saveTDocAndClose(NewTDoc(), w, false)
		if !errors.Is(err, closeErr) || w.closed != 1 {
			t.Fatalf("close error=%v count=%d", err, w.closed)
		}
		if failWrite && !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("write failure lost: %v", err)
		}
	}
}
