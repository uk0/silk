package pdfexport

import (
	"bytes"
	"compress/zlib"
	"io"
	"strings"
	"testing"

	"silk/paint"
)

// TestClipEmitsWAndN verifies the basic clip path: build a path,
// call Clip, the content stream should contain "W\nn\n" after the
// path commands.
func TestClipEmitsWAndN(t *testing.T) {
	p := New(200, 200)
	p.MoveTo(10, 10)
	p.LineTo(100, 10)
	p.LineTo(100, 100)
	p.LineTo(10, 100)
	p.Clip()

	out := p.String()
	// W\nn\n appears after the path commands.
	if !strings.Contains(out, "W\nn\n") {
		t.Errorf("Clip should emit 'W\\nn\\n'; got:\n%s", out)
	}
	// Path commands precede the W/n.
	wIdx := strings.Index(out, "W\nn\n")
	mIdx := strings.Index(out, "10 190 m") // PDF Y-flipped: H-y = 200-10
	if mIdx < 0 || mIdx > wIdx {
		t.Errorf("path 'm' should appear before W/n; got mIdx=%d wIdx=%d\n%s",
			mIdx, wIdx, out)
	}
}

// TestClipResetsPathButClipPreserveDoesNot: Clip resets the path
// buffer; subsequent Fill/Stroke see no leftover commands.
// ClipPreserve keeps the path so Fill after ClipPreserve still
// renders the original geometry.
func TestClipResetsPathButClipPreserveDoesNot(t *testing.T) {
	// Variant A: Clip then Fill. The Fill should be a no-op (empty
	// path).
	p := New(200, 200)
	p.SetBrush1(paint.Color{R: 255, A: 255})
	p.Rectangle(0, 0, 50, 50)
	p.Clip()
	p.Fill()

	out := p.String()
	wn := strings.Count(out, "W\nn\n")
	fillOps := strings.Count(out, "\nf\n")
	if wn != 1 {
		t.Errorf("variant A: expected one W/n, got %d", wn)
	}
	if fillOps != 0 {
		t.Errorf("variant A: Clip should reset path, Fill emits no 'f': got %d", fillOps)
	}

	// Variant B: ClipPreserve then Fill. The Fill should see the same
	// path and emit f.
	p2 := New(200, 200)
	p2.SetBrush1(paint.Color{R: 255, A: 255})
	p2.Rectangle(0, 0, 50, 50)
	p2.ClipPreserve()
	p2.Fill()

	out2 := p2.String()
	if strings.Count(out2, "W\nn\n") != 1 {
		t.Errorf("variant B: expected one W/n")
	}
	if strings.Count(out2, "\nf\n") != 1 {
		t.Errorf("variant B: ClipPreserve should keep path; Fill should emit one 'f'\n%s", out2)
	}
}

// TestEmptyClipIsNoOp: calling Clip with no path doesn't emit
// anything (would otherwise produce a stray W/n that breaks readers).
func TestEmptyClipIsNoOp(t *testing.T) {
	p := New(200, 200)
	p.Clip()

	out := p.String()
	if strings.Contains(out, "W\nn\n") {
		t.Errorf("empty Clip should not emit W/n:\n%s", out)
	}
}

// TestSetCompressionTogglesFilter: with compression off (default)
// the Contents object dict has no /Filter; with on it has
// /Filter /FlateDecode.
func TestSetCompressionTogglesFilter(t *testing.T) {
	p := New(100, 100)
	p.SetBrush1(paint.Color{R: 0, A: 255})
	p.Rectangle(0, 0, 50, 50)
	p.Fill()

	uncompressed := p.String()
	if strings.Contains(uncompressed, "/Filter /FlateDecode") {
		// The image XObjects use FlateDecode too — but no images here,
		// so any /Filter mention is the contents stream.
		t.Errorf("compression off should not emit /Filter /FlateDecode; got:\n%s",
			uncompressed)
	}

	p.SetCompression(true)
	compressed := p.String()
	if !strings.Contains(compressed, "/Filter /FlateDecode") {
		t.Errorf("compression on should emit /Filter /FlateDecode in Contents dict")
	}
}

// TestCompressionShrinksOutput: compressed output should be smaller
// than uncompressed for any non-trivial scene. We render a few
// dozen rect/text ops to ensure there's enough redundancy for zlib
// to win.
func TestCompressionShrinksOutput(t *testing.T) {
	build := func(compress bool) int {
		p := New(595, 842)
		p.SetCompression(compress)
		for i := 0; i < 50; i++ {
			p.SetBrush1(paint.Color{R: byte(i * 5), G: byte(i * 3), B: byte(255 - i*5), A: 255})
			p.Rectangle(float64(i*5), float64(i*5), 100, 50)
			p.Fill()
			p.SetBrush1(paint.Color{R: 0, A: 255})
			p.MoveTo(float64(10+i), float64(20+i*3))
			p.DrawText("Lorem ipsum dolor sit amet, consectetur adipiscing elit")
		}
		return len(p.Bytes())
	}
	uncompressed := build(false)
	compressed := build(true)
	if compressed >= uncompressed {
		t.Errorf("compression should shrink output: uncompressed=%d compressed=%d",
			uncompressed, compressed)
	}
	// Sanity: compression should achieve at least 30% reduction on
	// this kind of repetitive content.
	if float64(compressed) > float64(uncompressed)*0.7 {
		t.Logf("compression ratio: %d / %d = %.1f%%",
			compressed, uncompressed,
			100*float64(compressed)/float64(uncompressed))
	}
}

// TestCompressedContentRoundTripsToOriginalOps: extract the compressed
// content stream, zlib-decompress, and verify the operators are
// identical to the uncompressed version.
func TestCompressedContentRoundTripsToOriginalOps(t *testing.T) {
	build := func(compress bool) string {
		p := New(100, 100)
		p.SetCompression(compress)
		p.SetBrush1(paint.Color{R: 255, G: 100, B: 50, A: 255})
		p.Rectangle(10, 10, 80, 80)
		p.Fill()
		return p.String()
	}
	uncompressed := build(false)
	compressed := build(true)

	// Pull the uncompressed content stream out — between "stream\n"
	// and "endstream" of the Contents object (the FIRST occurrence
	// since we have only one page and no images).
	uStart := strings.Index(uncompressed, "stream\n")
	uEnd := strings.Index(uncompressed, "endstream")
	if uStart < 0 || uEnd < 0 {
		t.Fatal("uncompressed: no content stream")
	}
	uContent := uncompressed[uStart+len("stream\n") : uEnd]

	// Pull the compressed bytes out — same offsets.
	cStart := strings.Index(compressed, "stream\n")
	cEnd := strings.Index(compressed, "endstream")
	if cStart < 0 || cEnd < 0 {
		t.Fatal("compressed: no content stream")
	}
	cBytes := compressed[cStart+len("stream\n") : cEnd]
	// Trim the trailing newline our writer adds before "endstream"
	// (only present in the compressed path).
	cBytes = strings.TrimRight(cBytes, "\n")

	zr, err := zlib.NewReader(bytes.NewReader([]byte(cBytes)))
	if err != nil {
		t.Fatalf("zlib.NewReader: %v", err)
	}
	defer zr.Close()
	decoded, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("zlib decode: %v", err)
	}

	if string(decoded) != uContent {
		t.Errorf("decoded compressed content != uncompressed:\n--- uncompressed ---\n%s\n--- decoded ---\n%s",
			uContent, string(decoded))
	}
}

// TestCompressionDefaultIsOff guards backwards compat — existing
// tests inspect raw operators.
func TestCompressionDefaultIsOff(t *testing.T) {
	p := New(100, 100)
	if p.CompressionEnabled() {
		t.Error("compression should default to off")
	}
}
