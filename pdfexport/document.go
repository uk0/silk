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

// pageData carries one page's media box + finalised content stream.
// Multi-page documents accumulate one entry per ShowPage / NewPage
// transition; single-page documents have len(pages) == 1.
type pageData struct {
	width   float64
	height  float64
	content string
}

// document.go assembles the PDF 1.4 file structure. Object layout:
//
//   Object 1:    Catalog
//   Object 2:    Pages tree (/Kids = [3, 5, 7, …], /Count = N)
//   Object 3:    Page 1   (/Parent = 2, /Contents = 4)
//   Object 4:    Contents 1
//   Object 5:    Page 2   (/Parent = 2, /Contents = 6)
//   Object 6:    Contents 2
//   …
//   Object 2N+1: Page N
//   Object 2N+2: Contents N
//   Object 2N+3: Font (Helvetica)
//   Object 2N+4..: Image XObjects (one per unique pixmap)
//
// All offsets in the xref are absolute from the start of the file and
// must be exact — readers will reject the document if they don't match
// the actual byte position of each "N 0 obj" header. We track offsets
// as we write and emit the xref last.
//
// The font object lives AFTER the page/contents pairs so single-page
// docs (the default and dominant case) keep object IDs that match the
// old layout: id 5 was the Font, id 5 is still the Font when N=1
// because 2N+3 = 5. Tests written before multi-page support arrived
// keep working without modification.

func buildDocument(pages []pageData, images []imageData) string {
	var b strings.Builder

	n := len(pages)
	if n == 0 {
		// Defensive: a painter that hasn't drawn anything can still
		// produce a valid empty single-page PDF. Emit a 1×1 page.
		pages = []pageData{{width: 1, height: 1, content: ""}}
		n = 1
	}

	totalObjs := 2 + 2*n + 1 + len(images)
	offsets := make([]int, totalObjs)

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
	b.WriteString("<< /Type /Pages /Kids [")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "%d 0 R ", 3+2*i)
	}
	fmt.Fprintf(&b, "] /Count %d >>\n", n)
	b.WriteString("endobj\n")

	// Object IDs for fixed-position references shared across pages.
	fontID := 2 + 2*n + 1 // 2N+3
	imageBaseID := fontID + 1

	// Pages + Contents alternating. Page 1 is at object id 3, Contents
	// 1 at id 4, Page 2 at id 5, Contents 2 at id 6, etc.
	for i, pg := range pages {
		pageObjID := 3 + 2*i
		contentObjID := pageObjID + 1

		// Page object.
		offsets[pageObjID-1] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n", pageObjID)
		fmt.Fprintf(&b, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %g %g]\n", pg.width, pg.height)
		fmt.Fprintf(&b, "   /Resources << /Font << /F1 %d 0 R >>", fontID)
		if len(images) > 0 {
			b.WriteString("\n             /XObject << ")
			for j, img := range images {
				fmt.Fprintf(&b, "/%s %d 0 R ", img.name, imageBaseID+j)
			}
			b.WriteString(">>")
		}
		b.WriteString(" >>\n")
		fmt.Fprintf(&b, "   /Contents %d 0 R\n", contentObjID)
		b.WriteString(">>\n")
		b.WriteString("endobj\n")

		// Contents stream.
		offsets[contentObjID-1] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n", contentObjID)
		fmt.Fprintf(&b, "<< /Length %d >>\n", len(pg.content))
		b.WriteString("stream\n")
		b.WriteString(pg.content)
		b.WriteString("endstream\n")
		b.WriteString("endobj\n")
	}

	// Font object (Helvetica, standard 14, no embedding needed).
	offsets[fontID-1] = b.Len()
	fmt.Fprintf(&b, "%d 0 obj\n", fontID)
	b.WriteString("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>\n")
	b.WriteString("endobj\n")

	// Image XObjects.
	for j, img := range images {
		objID := imageBaseID + j
		offsets[objID-1] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n", objID)
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
	// "<offset> 00000 n" where offset is fixed-width 10 digits. Each
	// xref line MUST be exactly 20 bytes including the trailing
	// newline — readers rely on this for direct seeking.
	xrefOffset := b.Len()
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
