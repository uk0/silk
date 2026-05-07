package pdfexport

import (
	"fmt"
	"strings"
)

// imageData carries one embedded image's metadata + zlib-compressed
// RGB bytes ready for the PDF /XObject stream. Width / Height are in
// pixels. Bytes are 3 bytes per pixel (RGB) compressed with FlateDecode
// — alpha (if any) was composited onto white during encoding by the
// PDFPainter, so we don't carry an SMask. That trade-off keeps the
// document object count linear in the unique-image count rather than
// 2× (image + soft mask).
type imageData struct {
	width, height int
	compressed    []byte
	name          string // "Im1", "Im2", ...
}

// document.go assembles the PDF 1.4 file structure around a content
// stream. The output is the minimum every PDF reader (Acrobat, macOS
// Preview, Chrome / Firefox / Safari built-in viewers) accepts:
//
//   %PDF-1.4
//   %<binary marker>           ← signals "this is a binary file"
//
//   1 0 obj                    ← Catalog
//   << /Type /Catalog /Pages 2 0 R >>
//   endobj
//
//   2 0 obj                    ← Pages tree (single page)
//   << /Type /Pages /Kids [3 0 R] /Count 1 >>
//   endobj
//
//   3 0 obj                    ← Page
//   << /Type /Page /Parent 2 0 R
//      /MediaBox [0 0 W H]
//      /Resources << /Font << /F1 5 0 R >> >>
//      /Contents 4 0 R
//   >>
//   endobj
//
//   4 0 obj                    ← Content stream
//   << /Length n >>
//   stream
//   <drawing operators>
//   endstream
//   endobj
//
//   5 0 obj                    ← Font (Helvetica, built-in)
//   << /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>
//   endobj
//
//   xref
//   0 6
//   <byte offsets per object, fixed width>
//
//   trailer
//   << /Size 6 /Root 1 0 R >>
//   startxref
//   <byte offset of xref>
//   %%EOF
//
// All offsets in the xref are absolute from the start of the file and
// must be exact — readers will reject the document if they don't match
// the actual byte position of each "N 0 obj" header. We track offsets
// as we write and emit the xref last.

func buildDocument(width, height float64, content string, images []imageData) string {
	var b strings.Builder
	// Object IDs: 1=Catalog, 2=Pages, 3=Page, 4=Contents, 5=Font,
	// 6..(5+N)=Images. offsets[i] is the byte position of object (i+1)
	// header — we size for 5 + len(images) objects.
	offsets := make([]int, 5+len(images))

	// Header. The four high-bit bytes after "%" are the canonical
	// "this is a binary file" marker every PDF should carry — many
	// transfer agents otherwise misclassify and corrupt PDFs.
	b.WriteString("%PDF-1.4\n")
	b.WriteString("%\xE2\xE3\xCF\xD3\n")

	// Object 1: Catalog
	offsets[0] = b.Len()
	b.WriteString("1 0 obj\n")
	b.WriteString("<< /Type /Catalog /Pages 2 0 R >>\n")
	b.WriteString("endobj\n")

	// Object 2: Pages tree
	offsets[1] = b.Len()
	b.WriteString("2 0 obj\n")
	b.WriteString("<< /Type /Pages /Kids [3 0 R] /Count 1 >>\n")
	b.WriteString("endobj\n")

	// Object 3: Page. Resources include the font + every image XObject.
	offsets[2] = b.Len()
	b.WriteString("3 0 obj\n")
	fmt.Fprintf(&b, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %g %g]\n", width, height)
	b.WriteString("   /Resources << /Font << /F1 5 0 R >>")
	if len(images) > 0 {
		b.WriteString("\n             /XObject << ")
		for i, img := range images {
			fmt.Fprintf(&b, "/%s %d 0 R ", img.name, 6+i)
		}
		b.WriteString(">>")
	}
	b.WriteString(" >>\n")
	b.WriteString("   /Contents 4 0 R\n")
	b.WriteString(">>\n")
	b.WriteString("endobj\n")

	// Object 4: Content stream
	offsets[3] = b.Len()
	b.WriteString("4 0 obj\n")
	fmt.Fprintf(&b, "<< /Length %d >>\n", len(content))
	b.WriteString("stream\n")
	b.WriteString(content)
	b.WriteString("endstream\n")
	b.WriteString("endobj\n")

	// Object 5: Font (Helvetica, built-in standard 14)
	offsets[4] = b.Len()
	b.WriteString("5 0 obj\n")
	b.WriteString("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>\n")
	b.WriteString("endobj\n")

	// Objects 6..N: image XObjects. Each carries its own zlib-compressed
	// RGB stream. PDF /Filter /FlateDecode + /ColorSpace /DeviceRGB +
	// /BitsPerComponent 8 is the standard "lossless RGB" embedding.
	for i, img := range images {
		offsets[5+i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n", 6+i)
		b.WriteString("<< /Type /XObject /Subtype /Image\n")
		fmt.Fprintf(&b, "   /Width %d /Height %d\n", img.width, img.height)
		b.WriteString("   /ColorSpace /DeviceRGB /BitsPerComponent 8\n")
		fmt.Fprintf(&b, "   /Filter /FlateDecode /Length %d\n", len(img.compressed))
		b.WriteString(">>\n")
		b.WriteString("stream\n")
		b.Write(img.compressed)
		b.WriteString("\nendstream\n")
		b.WriteString("endobj\n")
	}

	// Cross-reference table. Slot 0 is the free-list head; we use the
	// canonical "0000000000 65535 f" entry. Subsequent slots are
	// "<offset> 00000 n" where offset is fixed-width 10 digits.
	//
	// IMPORTANT: each xref line MUST be exactly 20 bytes including the
	// trailing newline — readers rely on this for direct seeking.
	xrefOffset := b.Len()
	totalObjs := 5 + len(images)
	b.WriteString("xref\n")
	fmt.Fprintf(&b, "0 %d\n", totalObjs+1)
	b.WriteString("0000000000 65535 f \n")
	for i := 0; i < totalObjs; i++ {
		fmt.Fprintf(&b, "%010d 00000 n \n", offsets[i])
	}

	// Trailer + EOF marker
	b.WriteString("trailer\n")
	fmt.Fprintf(&b, "<< /Size %d /Root 1 0 R >>\n", totalObjs+1)
	b.WriteString("startxref\n")
	fmt.Fprintf(&b, "%d\n", xrefOffset)
	b.WriteString("%%EOF\n")

	return b.String()
}
